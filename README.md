# BDS Mail

A multi-domain mail server written in Go. Supports SMTP, POP3, IMAP, and a web interface. Stores mail bodies in Google Cloud Storage and metadata in PostgreSQL. Includes DKIM signing for email deliverability and automated TLS certificate management.

## Features

- **Multi-domain**: Serve multiple domains from a single instance (e.g. `domain1.com`, `domain2.com`)
- **Web UI**: Login, inbox, sent folder, compose, and read messages at `https://mail.yourdomain.com`
- **SMTP**: Receive inbound mail and relay outbound mail via MX lookup
- **POP3 & IMAP**: Mail client access (Thunderbird, Outlook, etc.)
- **Text & HTML**: Supports plain text and HTML email bodies with links
- **CC & BCC**: Full support for CC and BCC recipients
- **DKIM signing**: Outbound emails are cryptographically signed to avoid spam folders
- **Automated TLS**: Certificates are obtained and renewed automatically via certbot
- **Dynamic domain registration**: Add new domains on the fly via web admin or CLI without server restart
- **User display names**: Users have display names shown in the UI and outbound email headers
- **Admin interface**: Web UI at `/admin/domains` for managing domains
- **GCS storage**: Mail bodies stored in a GCP Cloud Storage bucket
- **PostgreSQL**: User accounts and message metadata

## Architecture

```mermaid
graph TD
    Internet((Internet))

    Internet --> SMTP["SMTP :25<br/>(receive & relay)"]
    Internet --> POP3["POP3 :110<br/>(mail clients)"]
    Internet --> IMAP["IMAP :143<br/>(mail clients)"]
    Internet --> HTTPS["HTTPS :443<br/>(web UI)"]

    SMTP --> Store["Store<br/>(facade)"]
    POP3 --> Store
    IMAP --> Store
    HTTPS --> Store

    Store --> PostgreSQL[(PostgreSQL<br/>metadata)]
    Store --> GCS[(GCS Bucket<br/>mail bodies)]
```

## Prerequisites

- A **GCP project** with:
  - A Compute Engine VM with a static external IP
  - A Cloud SQL PostgreSQL instance (or any PostgreSQL database)
  - A Cloud Storage bucket
  - A service account with `storage.objectAdmin` role on the bucket
- **Domain names** managed in GoDaddy (or any DNS provider)
- **Go 1.21+** (for building on your dev machine)

---

## Step 1: GCP Setup

### 1.1 Create a GCS Bucket

```bash
export GCP_PROJECT_ID=your-project-id
export BDS_GCS_BUCKET=bdsmail-bodies

gcloud storage buckets create "gs://${BDS_GCS_BUCKET}" \
    --project="${GCP_PROJECT_ID}" \
    --location=us-central1 \
    --uniform-bucket-level-access
```

### 1.2 Create a Service Account

```bash
# Create the service account
gcloud iam service-accounts create bdsmail \
    --display-name="BDS Mail Service Account" \
    --project="${GCP_PROJECT_ID}"

# Grant bucket access
gcloud storage buckets add-iam-policy-binding "gs://${BDS_GCS_BUCKET}" \
    --member="serviceAccount:bdsmail@${GCP_PROJECT_ID}.iam.gserviceaccount.com" \
    --role=roles/storage.objectAdmin

# Create a key file (you'll upload this to the VM)
gcloud iam service-accounts keys create sa-key.json \
    --iam-account="bdsmail@${GCP_PROJECT_ID}.iam.gserviceaccount.com"
```

> **Tip**: If your VM uses a default service account with Cloud Storage access, you can skip the key file and rely on Application Default Credentials.

### 1.3 Prepare the PostgreSQL Database

Connect to your existing PostgreSQL instance and create the database:

```sql
CREATE DATABASE bdsmail;
```

The application creates all required tables automatically on first startup.

Note your connection details for later:
- **Host**: Private IP of your Cloud SQL instance (e.g. `10.x.x.x`)
- **Port**: `5432`
- **User/Password**: Your database credentials
- **Database**: `bdsmail`

### 1.4 Configure the GCP VM

#### Reserve a static IP

```bash
gcloud compute addresses create bdsmail-ip --region=YOUR_REGION
```

Note the IP address — you'll need it for DNS records.

#### Open firewall ports

```bash
gcloud compute firewall-rules create allow-mail \
    --allow=tcp:25,tcp:80,tcp:110,tcp:143,tcp:443 \
    --target-tags=mail-server \
    --description="Allow SMTP, HTTP (cert challenges), POP3, IMAP, HTTPS"

# Tag your VM
gcloud compute instances add-tags YOUR_VM_NAME \
    --tags=mail-server \
    --zone=YOUR_ZONE
```

> **Port 80** is required for Let's Encrypt certificate issuance (HTTP-01 challenge). It's only used briefly during cert renewal.

---

## Step 2: GoDaddy DNS Configuration

For **each domain** (e.g. `domain1.com`, `domain2.com`), add the following DNS records in GoDaddy.

### 2.1 A Record — Points mail subdomain to your VM

| Type | Name | Value | TTL |
|------|------|-------|-----|
| A    | mail | `YOUR_VM_STATIC_IP` | 1 Hour |

### 2.2 MX Record — Tells other servers where to deliver email

| Type | Name | Value | Priority | TTL |
|------|------|-------|----------|-----|
| MX   | @    | `mail.domain1.com` | 10 | 1 Hour |

### 2.3 SPF Record — Authorizes your VM to send email

| Type | Name | Value | TTL |
|------|------|-------|-----|
| TXT  | @    | `v=spf1 ip4:YOUR_VM_STATIC_IP ~all` | 1 Hour |

### 2.4 DKIM Record — Proves emails are authentically from your domain

The deploy script generates DKIM keys and prints the exact DNS record to add. It will look like:

| Type | Name | Value | TTL |
|------|------|-------|-----|
| TXT  | default._domainkey | `v=DKIM1; k=rsa; p=BASE64_PUBLIC_KEY...` | 1 Hour |

> **Note**: The DKIM value is a long string. GoDaddy may require you to paste it in a single line. If it's over 255 characters, GoDaddy usually handles the splitting automatically.

### 2.5 DMARC Record — Policy for handling unauthenticated emails

| Type | Name | Value | TTL |
|------|------|-------|-----|
| TXT  | _dmarc | `v=DMARC1; p=none; rua=mailto:postmaster@domain1.com` | 1 Hour |

### 2.6 Repeat for each domain

Add all five records (A, MX, SPF, DKIM, DMARC) for every domain you serve.

### 2.7 How to add records in GoDaddy

1. Log in to [GoDaddy](https://www.godaddy.com)
2. Go to **My Products** → find your domain → **DNS** (or **Manage DNS**)
3. Click **Add Record**
4. Select the record type (A, MX, or TXT)
5. Fill in the Name, Value, and Priority fields as shown above
6. Click **Save**
7. Wait for DNS propagation (usually 15-60 minutes, up to 48 hours)

### 2.8 Verify DNS propagation

```bash
# Check A record
dig A mail.domain1.com

# Check MX record
dig MX domain1.com

# Check SPF
dig TXT domain1.com

# Check DKIM
dig TXT default._domainkey.domain1.com

# Check DMARC
dig TXT _dmarc.domain1.com
```

---

## Step 3: Build and Deploy

### 3.1 Build the binary

On your development machine:

```bash
GOOS=linux GOARCH=amd64 go build -o bdsmail ./cmd/bdsmail/
```

### 3.2 Upload to the VM

```bash
gcloud compute scp bdsmail YOUR_VM_NAME:/tmp/bdsmail --zone=YOUR_ZONE
gcloud compute scp --recurse web YOUR_VM_NAME:/tmp/web --zone=YOUR_ZONE
gcloud compute scp --recurse scripts YOUR_VM_NAME:/tmp/scripts --zone=YOUR_ZONE
gcloud compute scp sa-key.json YOUR_VM_NAME:/tmp/sa-key.json --zone=YOUR_ZONE
```

### 3.3 Run the deploy script

SSH into the VM and run the automated deployment:

```bash
gcloud compute ssh YOUR_VM_NAME --zone=YOUR_ZONE

# On the VM:
sudo -i
cd /tmp

# Set required environment variables
export BDS_DOMAINS="domain1.com,domain2.com"
export DATABASE_URL="postgres://user:password@10.x.x.x:5432/bdsmail?sslmode=require"
export BDS_GCS_BUCKET="bdsmail-bodies"
export GOOGLE_APPLICATION_CREDENTIALS="/opt/bdsmail/sa-key.json"

# Copy the service account key
cp /tmp/sa-key.json /opt/bdsmail/sa-key.json

# Run the deploy script — this does everything automatically:
#   - Copies binary and web assets
#   - Installs certbot and obtains TLS certificates
#   - Sets up automatic certificate renewal (systemd timer)
#   - Generates DKIM keys and prints DNS records to add
#   - Creates the systemd service
#   - Starts the mail server
bash /tmp/scripts/deploy.sh
```

The script will print the **DKIM DNS records** for each domain. Copy these and add them to GoDaddy as described in Step 2.4.

### 3.4 Create users

```bash
cd /opt/bdsmail

# Create users with display names
./bdsmail -adduser alice@domain1.com -password 'securepassword123' -displayname 'Alice Smith'
./bdsmail -adduser bob@domain1.com -password 'anotherpassword' -displayname 'Bob Jones'

# Create users for domain2.com
./bdsmail -adduser support@domain2.com -password 'supportpass' -displayname 'Support Team'
```

The display name is shown in the web UI navigation bar, compose screen, and in outbound email `From` headers (e.g. `Alice Smith <alice@domain1.com>`).

---

## Step 4: Verify Everything Works

### Check the service is running

```bash
systemctl status bdsmail
journalctl -u bdsmail -f
```

### Check certificate auto-renewal is active

```bash
systemctl list-timers | grep certbot
```

### Test the web UI

Open `https://mail.domain1.com` in your browser. Log in with the username (e.g. `alice`) and password you created.

### Test sending email

Compose a message to an external address (e.g. your Gmail). Check:
- Email arrives in inbox (not spam)
- Click "Show original" in Gmail to verify DKIM signature passes

### Test receiving email

Send an email from Gmail to `alice@domain1.com`. It should appear in the inbox.

### Verify DKIM and SPF

Use [mail-tester.com](https://www.mail-tester.com) — send an email to the address it gives you and check your score.

---

## Step 5: Sending Email from Backend Applications

Your other backend applications can send email through this server via SMTP.

### Connection settings for your backends

| Setting | Value |
|---------|-------|
| SMTP Host | `mail.domain1.com` (or the VM's internal IP if on same GCP network) |
| SMTP Port | `25` |
| Username | Full email: `app@domain1.com` |
| Password | The user's password |
| Auth | PLAIN |
| TLS | STARTTLS (if connecting from outside GCP network) |

### Example: Sending from a Go application

```go
import "net/smtp"

auth := smtp.PlainAuth("", "app@domain1.com", "password", "mail.domain1.com")
err := smtp.SendMail("mail.domain1.com:25", auth,
    "app@domain1.com",
    []string{"customer@gmail.com"},
    []byte("From: app@domain1.com\r\nTo: customer@gmail.com\r\nSubject: Hello\r\n\r\nMessage body"),
)
```

> **Tip**: Create a dedicated user (e.g. `noreply@domain1.com`) for automated emails.

---

## Accessing via Mail Clients

Configure Thunderbird, Outlook, or other mail clients:

| Setting | Value |
|---------|-------|
| **Incoming (IMAP)** | Server: `mail.domain1.com`, Port: `143`, Security: SSL/TLS |
| **Incoming (POP3)** | Server: `mail.domain1.com`, Port: `110`, Security: SSL/TLS |
| **Outgoing (SMTP)** | Server: `mail.domain1.com`, Port: `25`, Security: STARTTLS |
| **Username** | Full email: `alice@domain1.com` |
| **Password** | Your account password |

---

## Configuration Reference

All configuration is via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `BDS_DOMAINS` | Comma-separated list of served domains | `mydomain.com` |
| `BDS_SMTP_PORT` | SMTP server port | `2525` |
| `BDS_POP3_PORT` | POP3 server port | `1100` |
| `BDS_IMAP_PORT` | IMAP server port | `1430` |
| `BDS_HTTPS_PORT` | HTTPS web UI port | `8443` |
| `BDS_TLS_CERT` | Path to TLS certificate (auto-managed by certbot) | (none) |
| `BDS_TLS_KEY` | Path to TLS private key (auto-managed by certbot) | (none) |
| `BDS_GCS_BUCKET` | GCS bucket name for mail bodies | `bdsmail-bodies` |
| `DATABASE_URL` | PostgreSQL connection string | `postgres://localhost:5432/bdsmail?sslmode=disable` |
| `BDS_DKIM_KEY_DIR` | Directory containing DKIM private keys (`{domain}.pem`) | (none) |
| `BDS_DKIM_SELECTOR` | DKIM selector name | `default` |
| `BDS_HTTP_PORT` | Plain HTTP port for ACME certificate challenges | `8080` |
| `BDS_ADMIN_SECRET` | Secret for admin web UI and `-adddomain` CLI | (none) |
| `BDS_ACME_WEBROOT` | Directory for ACME challenge files | `/opt/bdsmail/acme` |
| `BDS_ENV_FILE` | Path to .env file (for persisting domain additions) | (none) |
| `GOOGLE_APPLICATION_CREDENTIALS` | Path to GCP service account JSON key | (uses ADC) |

---

## Adding a New Domain

You can add domains on the fly without restarting the server, using either the CLI or the web admin.

### Option A: CLI (recommended)

```bash
./bdsmail -adddomain newdomain.com
```

This connects to the running server and automatically:
- Generates a DKIM key pair
- Adds the domain to the running config
- Expands the TLS certificate via certbot
- Persists the domain to `.env`
- Prints the DNS records you need to add to GoDaddy

Requires `BDS_ADMIN_SECRET` to be set.

### Option B: Web Admin

1. Go to `https://mail.yourdomain.com/admin/domains`
2. Enter the admin secret
3. Type the new domain name and click **Add Domain**
4. The page shows the DNS records to add

### After adding a domain

1. Add the printed DNS records (A, MX, SPF, DKIM, DMARC) to GoDaddy
2. Wait for DNS propagation (15-60 minutes)
3. Create users: `./bdsmail -adduser user@newdomain.com -password 'password' -displayname 'Full Name'`

### Manual method (if server is not running)

1. Generate DKIM key: `/opt/bdsmail/generate_dkim.sh newdomain.com /opt/bdsmail/dkim`
2. Expand TLS cert: `certbot certonly --webroot -w /opt/bdsmail/acme --expand -d mail.newdomain.com`
3. Add domain to `BDS_DOMAINS` in `/opt/bdsmail/.env`
4. Restart: `systemctl restart bdsmail`

---

## Troubleshooting

```bash
# Check service status
systemctl status bdsmail

# View logs
journalctl -u bdsmail -f

# Check certificate renewal timer
systemctl list-timers | grep certbot

# Test SMTP connectivity
telnet mail.domain1.com 25

# Test TLS on HTTPS
curl -I https://mail.domain1.com

# Verify SMTP TLS
openssl s_client -connect mail.domain1.com:25 -starttls smtp

# Check DNS records
dig MX domain1.com
dig A mail.domain1.com
dig TXT domain1.com                        # SPF
dig TXT default._domainkey.domain1.com     # DKIM
dig TXT _dmarc.domain1.com                # DMARC

# Check firewall rules
gcloud compute firewall-rules list --filter="name=allow-mail"

# Test DKIM signing
# Send an email and check headers for "DKIM-Signature:"
# Or use https://www.mail-tester.com
```
