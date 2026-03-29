# BDS Mail

A multi-domain mail server written in Go. Single binary, zero required external dependencies. Supports SMTP, POP3, IMAP, and a web interface with pluggable cloud-native storage backends. Includes comprehensive email security: DKIM signing, SPF/DKIM/DMARC verification, MTA-STS, DANE/TLSA, TLSRPT, REQUIRETLS, ClamAV antivirus, Rspamd spam filtering, Google Safe Browsing, rate limiting, and automated TLS certificate management.

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
- **Attachments**: Send and receive file attachments (MIME multipart), stored in GCS or S3
- **Virus scanning**: Optional ClamAV integration blocks infected emails and attachments (inbound and outbound)
- **Spam filtering**: Rspamd integration via HTTP API with configurable score thresholds for reject/junk
- **Dangerous link detection**: Google Safe Browsing API flags or blocks emails with malicious URLs
- **Sender verification**: Inbound SPF/DKIM/DMARC checks with policy-based reject or quarantine
- **MTA-STS (RFC 8461)**: Enforces TLS for outbound delivery to domains with MTA-STS policies
- **DANE/TLSA (RFC 6698/7672)**: Verifies recipient server certificates against DNSSEC TLSA records
- **TLSRPT (RFC 8460)**: Sends daily TLS connection failure reports to receiving domains
- **REQUIRETLS (RFC 8689)**: Supports the REQUIRETLS SMTP extension for end-to-end TLS enforcement
- **Rate limiting**: Per-IP connection rate limiting and brute-force login protection with automatic lockout
- **Email aliases**: Forward mail to one or more targets, with domain-level catch-all support (`@domain.com`)
- **Mailing lists**: Group distribution lists with List-Id/List-Unsubscribe headers and web admin management
- **Server-side filtering**: Sieve-style rules with conditions (from, to, subject, headers) and actions (move, mark read, delete, flag)
- **Default filter presets**: Auto-created filters for newsletters, social media, noreply senders, and large attachments
- **Auto-reply / Vacation**: Configurable out-of-office responses with date ranges and 24-hour per-sender cooldown
- **Full-text search**: Search across subject, body, from, and to fields (SQLite FTS5, PostgreSQL ILIKE, client-side for NoSQL)
- **Contacts / CardDAV**: Contact management via web UI and CardDAV protocol (compatible with macOS Contacts, DAVx5, etc.)
- **Web admin panel**: Manage domains, users, aliases, and mailing lists via the admin web UI
- **Dynamic folders**: Custom mail folders created by filters, visible in IMAP and web UI
- **Multiple database backends**: PostgreSQL, SQLite, DynamoDB, or Firestore
- **Multiple object stores**: GCS or S3 for attachments (configurable)

## Architecture

```mermaid
graph TD
    Internet((Internet))

    Internet -->|inbound| SMTP["SMTP :25<br/>(receive & relay)"]
    Internet --> POP3["POP3 :110<br/>(mail clients)"]
    Internet --> IMAP["IMAP :143<br/>(mail clients)"]
    Internet --> HTTPS["HTTPS :443<br/>(web UI)"]

    SMTP --> RateLimit["Rate Limiter<br/>(per-IP conn limit,<br/>brute-force protection)"]
    HTTPS --> RateLimit

    RateLimit --> Security["Security Checks<br/>(ClamAV, Rspamd,<br/>Safe Browsing,<br/>SPF/DKIM/DMARC)"]

    Security --> Filters["Mail Processing<br/>(Aliases, Mailing Lists,<br/>Sieve Filters, Auto-Reply)"]

    Filters --> Store["Store<br/>(facade)"]
    POP3 --> Store
    IMAP --> Store

    Internet --> CardDAV["CardDAV :443<br/>(contacts)"]
    CardDAV --> Store

    Store --> PostgreSQL[(PostgreSQL<br/>metadata)]
    Store --> GCS[(GCS Bucket<br/>mail bodies)]

    SMTP -->|outbound| TLS["Transport Security<br/>(MTA-STS, DANE/TLSA,<br/>REQUIRETLS, TLSRPT)"]
    HTTPS -->|outbound| TLS
    TLS --> SES["SMTP Relay :587<br/>(SES/SendGrid/Mailgun<br/>or direct MX)"]
    SES --> Internet
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
export BDS_GCS_BUCKET="${GCP_PROJECT_ID}-mail"
export BDS_REGION=us-west1
export BDS_ZONE=us-west1-b
export BDS_VM_NAME=your-vm-name
export BDS_STATIC_IP=your-vm-static-ip

gcloud storage buckets create "gs://${BDS_GCS_BUCKET}" \
    --project="${GCP_PROJECT_ID}" \
    --location="${BDS_REGION}" \
    --uniform-bucket-level-access
```

### 1.2 Authorize the VM Service Account

```bash
# Grant the VM's existing service account access to the bucket
export VM_SERVICE_ACCOUNT=$(gcloud compute instances describe ${BDS_VM_NAME} \
    --zone="${BDS_ZONE}" \
    --format="get(serviceAccounts[0].email)")

gcloud storage buckets add-iam-policy-binding "gs://${BDS_GCS_BUCKET}" \
    --member="serviceAccount:${VM_SERVICE_ACCOUNT}" \
    --role=roles/storage.objectAdmin
```

> **Note**: No dedicated service account or key file needed — the VM's existing service account gets bucket access, and the app uses Application Default Credentials (ADC) automatically.

### 1.3 Prepare the PostgreSQL Database

Connect to your existing PostgreSQL instance and create a user and database:

```sql
CREATE USER bdsmail WITH PASSWORD 'your-secure-password';
CREATE DATABASE bdsmail OWNER bdsmail;
```

The application creates all required tables automatically on first startup.

### 1.4 Set Up Cloud SQL Auth Proxy

The Cloud SQL Auth Proxy provides a secure connection to your database without exposing it to public IPs or managing authorized networks. It authenticates via ADC automatically.

#### Install the proxy on the VM

```bash
curl -o /usr/local/bin/cloud-sql-proxy \
    https://storage.googleapis.com/cloud-sql-connectors/cloud-sql-proxy/v2.15.2/cloud-sql-proxy.linux.amd64
chmod +x /usr/local/bin/cloud-sql-proxy
```

#### Find your Cloud SQL connection name

```bash
gcloud sql instances describe YOUR_INSTANCE_NAME \
    --format="get(connectionName)"
```

This returns a value like `your-project:us-west1:your-instance`.

#### Create a systemd service for the proxy

```bash
cat > /etc/systemd/system/cloud-sql-proxy.service << 'EOF'
[Unit]
Description=Cloud SQL Auth Proxy
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/cloud-sql-proxy YOUR_CONNECTION_NAME --address=0.0.0.0 --port=5432
Restart=on-failure
RestartSec=5

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable cloud-sql-proxy
systemctl start cloud-sql-proxy
```

Replace `YOUR_CONNECTION_NAME` with the value from the previous step (e.g. `rankaipilot-demo:us-west1:rankaipilot-db`).

#### Verify the connection

```bash
psql "postgres://bdsmail:your-secure-password@localhost:5432/bdsmail?sslmode=disable"
```

Note your connection details for later:
- **Host**: `localhost` (via the proxy)
- **Port**: `5432`
- **User**: `bdsmail`
- **Password**: the password you set above
- **Database**: `bdsmail`
- **sslmode**: `disable` (the proxy handles encryption)

### 1.4 Configure the GCP VM

#### Open firewall ports

```bash
gcloud compute firewall-rules create allow-mail \
    --allow=tcp:25,tcp:80,tcp:110,tcp:143,tcp:443 \
    --target-tags=mail-server \
    --description="Allow SMTP, HTTP (cert challenges), POP3, IMAP, HTTPS"

# Tag your VM
gcloud compute instances add-tags ${BDS_VM_NAME} \
    --tags=mail-server \
    --zone="${BDS_ZONE}"
```

> **Port 80** is required for Let's Encrypt certificate issuance (HTTP-01 challenge). It's only used briefly during cert renewal.

---

## Step 2: GoDaddy DNS Configuration

For **each domain** (e.g. `domain1.com`, `domain2.com`), add the following DNS records in GoDaddy.

### 2.1 A Record — Points mail subdomain to your VM

| Type | Name | Value | TTL |
|------|------|-------|-----|
| A    | mail | `${BDS_STATIC_IP}` | 1 Hour |

### 2.2 MX Record — Tells other servers where to deliver email

| Type | Name | Value | Priority | TTL |
|------|------|-------|----------|-----|
| MX   | @    | `mail.domain1.com` | 10 | 1 Hour |

### 2.3 SPF Record — Authorizes your VM to send email

| Type | Name | Value | TTL |
|------|------|-------|-----|
| TXT  | @    | `v=spf1 ip4:${BDS_STATIC_IP} ~all` | 1 Hour |

### 2.4 DKIM Record — Proves emails are authentically from your domain

**Skip this for now** — the DKIM keys are generated during deployment (Step 3.3). After the deploy script runs, it will print the exact DNS TXT record value. Come back here and add it:

| Type | Name | Value | TTL |
|------|------|-------|-----|
| TXT  | default._domainkey | `v=DKIM1; k=rsa; p=<value from deploy output>` | 1 Hour |

To regenerate or generate manually at any time:

```bash
bash /opt/bdsmail/scripts/generate_dkim.sh domain1.com /opt/bdsmail/dkim
```

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
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/bdsmail ./cmd/bdsmail/
```

### 3.2 Upload to the VM

```bash
gcloud compute scp bin/bdsmail ${BDS_VM_NAME}:/tmp/bdsmail --zone="${BDS_ZONE}"
gcloud compute scp --recurse web ${BDS_VM_NAME}:/tmp/web --zone="${BDS_ZONE}"
gcloud compute scp --recurse scripts ${BDS_VM_NAME}:/tmp/scripts --zone="${BDS_ZONE}"
```

### 3.3 Run the deploy script

SSH into the VM and run the automated deployment:

```bash
gcloud compute ssh ${BDS_VM_NAME} --zone="${BDS_ZONE}"

# On the VM:
sudo -i
cd /tmp

# Set required environment variables
export BDS_DOMAINS="domain1.com,domain2.com"
export DATABASE_URL="postgres://bdsmail:your-secure-password@localhost:5432/bdsmail?sslmode=disable"
export BDS_GCS_BUCKET="${GCP_PROJECT_ID}-mail"
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
./bdsmail -adduser matt@rankaipilot.net -password 's3cpas5w0rd123' -displayname 'Matthew Collins'
./bdsmail -adduser mahoni@rankaipilot.net -password 's3cpas5w0rd123' -displayname 'Mahoni Campell'
./bdsmail -adduser leo@rankaipilot.net -password 's3cpas5w0rd123' -displayname 'Leonardo Carlie'
./bdsmail -adduser noreply@rankaipilot.net -password 's3cpas5w0rd123' -displayname 'NOREPLY - Rank AI Pilot'
./bdsmail -adduser info@rankaipilot.net -password 's3cpas5w0rd123' -displayname 'Rank AI Pilot'

# Create users for domain2.com
./bdsmail -adduser info@trvoo.net -password 's3cpas5w0rd123' -displayname 'TRVOO Experience Rides'
./bdsmail -adduser info@trvoo.net -password 's3cpas5w0rd123' -displayname 'TRVOO Experience Rides'
```

The display name is shown in the web UI navigation bar, compose screen, and in outbound email `From` headers (e.g. `Alice Smith <alice@domain1.com>`).

### 3.5 Update configuration

All configuration is stored in `/opt/bdsmail/.env`. The deploy script creates this file automatically, but you can edit it at any time:

```bash
sudo nano /opt/bdsmail/.env
```

After making changes, restart the service:

```bash
sudo systemctl restart bdsmail
```

> **Note**: The application reads all configuration from the `.env` file — environment variables are not used. Both the server and CLI commands (e.g. `-adduser`) read from the same file.

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

## External SMTP Relay

GCP blocks outbound port 25, so direct mail delivery to external recipients won't work. To send outbound email, configure an external SMTP relay service.

Add the following to `/opt/bdsmail/.env`:

```bash
BDS_RELAY_HOST=smtp.yourelayservice.com
BDS_RELAY_PORT=587
BDS_RELAY_USER=your-username
BDS_RELAY_PASSWORD=your-password
```

When configured, all outbound email is routed through the relay on port 587 with STARTTLS authentication. DKIM signing is still applied before relaying. When not configured, the server attempts direct delivery via MX lookup on port 25.

### Amazon SES Setup (Recommended — $0.10 per 1,000 emails)

#### 1. Create AWS Account & Open SES Console

- Sign up at [aws.amazon.com](https://aws.amazon.com) if you don't have an account
- Go to the [Amazon SES Console](https://console.aws.amazon.com/ses/)
- Select a region close to your VM (e.g. **us-west-2** for us-west1 GCP VMs)

#### 2. Verify Your Domain

- Go to **Configuration → Verified Identities → Create Identity**
- Select **Domain**, enter your domain (e.g. `domain1.com`)
- SES will provide DNS records to add in your DNS provider (e.g. GoDaddy):
  - **3 CNAME records** for DKIM verification (e.g. `xxxx._domainkey.domain1.com`)
  - A **TXT record** for domain ownership verification
- Add these records and wait for verification (usually 5-15 minutes)
- Repeat for each domain you serve

> **Note**: The SES DKIM records (CNAME) use different selectors than your existing DKIM key (`default._domainkey`). Both coexist in DNS without conflict — emails may carry both signatures, which improves deliverability.

#### 3. Create SMTP Credentials

- In SES Console, go to **SMTP Settings** (left sidebar)
- Note the **SMTP endpoint** (e.g. `email-smtp.us-west-2.amazonaws.com`)
- Click **Create SMTP Credentials**
- This creates an IAM user automatically
- **Download the credentials** — you only see the password once

#### 4. Request Production Access

New SES accounts are in **sandbox mode** — you can only send to verified email addresses. To send to anyone:

- Go to **Account Dashboard → Request Production Access**
- Fill in:
  - **Use case**: Transactional email for a mail server
  - **Expected volume**: Your estimated monthly emails
  - **How you handle bounces**: Bounces are logged by the application
- AWS usually approves within 1-2 business days

#### 5. Update `.env` on the VM

```bash
BDS_RELAY_HOST=email-smtp.us-west-2.amazonaws.com
BDS_RELAY_PORT=587
BDS_RELAY_USER=your-ses-smtp-username
BDS_RELAY_PASSWORD=your-ses-smtp-password
```

Restart the service:

```bash
sudo systemctl restart bdsmail
```

#### 6. Test (While in Sandbox)

While waiting for production access, you can still test:

- Verify a test email address in SES Console (e.g. your Gmail) under **Verified Identities → Create Identity → Email Address**
- Send a test email from the bdsmail web UI to that verified address
- Check logs: `journalctl -u bdsmail -f`

Once production access is approved, you can send to any address.

### Other Relay Services

**SendGrid** ($19.95/mo for 50K emails):
```bash
BDS_RELAY_HOST=smtp.sendgrid.net
BDS_RELAY_PORT=587
BDS_RELAY_USER=apikey
BDS_RELAY_PASSWORD=your-sendgrid-api-key
```

**Mailgun** ($15/mo for 10K emails):
```bash
BDS_RELAY_HOST=smtp.mailgun.org
BDS_RELAY_PORT=587
BDS_RELAY_USER=postmaster@your-mailgun-domain.com
BDS_RELAY_PASSWORD=your-mailgun-smtp-password
```

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
| `BDS_GCS_BUCKET` | GCS bucket name for mail bodies | `${GCP_PROJECT_ID}-mail` |
| `DATABASE_URL` | PostgreSQL connection string (via Cloud SQL Proxy) | `postgres://bdsmail:pass@localhost:5432/bdsmail?sslmode=disable` |
| `BDS_DKIM_KEY_DIR` | Directory containing DKIM private keys (`{domain}.pem`) | (none) |
| `BDS_DKIM_SELECTOR` | DKIM selector name | `default` |
| `BDS_HTTP_PORT` | Plain HTTP port for ACME certificate challenges | `8080` |
| `BDS_ADMIN_SECRET` | Secret for admin web UI and `-adddomain` CLI | (none) |
| `BDS_ACME_WEBROOT` | Directory for ACME challenge files | `/opt/bdsmail/acme` |
| `BDS_ENV_FILE` | Path to .env file (for persisting domain additions) | (none) |
| `GOOGLE_APPLICATION_CREDENTIALS` | Path to GCP service account JSON key | (uses ADC) |
| `BDS_CLAMAV_ENABLED` | Enable ClamAV virus scanning | `false` |
| `BDS_CLAMAV_ADDRESS` | clamd socket address | `unix:/var/run/clamav/clamd.ctl` |
| `BDS_CLAMAV_TIMEOUT` | ClamAV scan timeout (seconds) | `5` |
| `BDS_SAFEBROWSING_ENABLED` | Enable Google Safe Browsing link checks | `false` |
| `BDS_SAFEBROWSING_API_KEY` | Google Safe Browsing API key | (none) |
| `BDS_SAFEBROWSING_TIMEOUT` | Safe Browsing API timeout (seconds) | `5` |
| `BDS_AUTH_CHECK_ENABLED` | Enable inbound SPF/DKIM/DMARC verification | `true` |
| `BDS_AUTH_CHECK_TIMEOUT` | Auth verification timeout (seconds) | `5` |
| `BDS_RATELIMIT_ENABLED` | Enable per-IP rate limiting and brute-force protection | `true` |
| `BDS_RATELIMIT_CONN_PER_SEC` | Max connections per second per IP | `10` |
| `BDS_RATELIMIT_CONN_BURST` | Connection burst allowance per IP | `20` |
| `BDS_RATELIMIT_MAX_AUTH_FAIL` | Failed auth attempts before lockout | `5` |
| `BDS_RATELIMIT_LOCKOUT_SEC` | Lockout duration in seconds | `900` |
| `BDS_RSPAMD_ENABLED` | Enable Rspamd spam filtering | `true` |
| `BDS_RSPAMD_URL` | Rspamd HTTP API URL | `http://localhost:11333` |
| `BDS_RSPAMD_TIMEOUT` | Rspamd scan timeout (seconds) | `10` |
| `BDS_RSPAMD_REJECT_SCORE` | Spam score threshold for rejection | `15.0` |
| `BDS_RSPAMD_JUNK_SCORE` | Spam score threshold for Junk folder | `6.0` |
| `BDS_MTASTS_ENABLED` | Enable MTA-STS outbound TLS enforcement | `true` |
| `BDS_MTASTS_TIMEOUT` | MTA-STS policy fetch timeout (seconds) | `10` |
| `BDS_DANE_ENABLED` | Enable DANE/TLSA certificate verification | `true` |
| `BDS_DANE_TIMEOUT` | DANE TLSA lookup timeout (seconds) | `5` |
| `BDS_DANE_RESOLVER` | DNSSEC-validating DNS resolver | `1.1.1.1:53` |
| `BDS_TLSRPT_ENABLED` | Enable TLS reporting (RFC 8460) | `true` |
| `BDS_TLSRPT_INTERVAL` | TLSRPT report interval (seconds) | `86400` |
| `BDS_RELAY_HOST` | External SMTP relay host (e.g. `smtp.sendgrid.net`) | (none — direct delivery) |
| `BDS_RELAY_PORT` | External relay port | `587` |
| `BDS_RELAY_USER` | Relay authentication username | (none) |
| `BDS_RELAY_PASSWORD` | Relay authentication password | (none) |
| `BDS_DB_TYPE` | Database backend: `postgres`, `sqlite`, or `dynamodb` | `postgres` |
| `BDS_SQLITE_PATH` | Path to SQLite database file (when `BDS_DB_TYPE=sqlite`) | `/opt/bdsmail/bdsmail.db` |
| `BDS_DYNAMODB_REGION` | AWS region for DynamoDB (when `BDS_DB_TYPE=dynamodb`) | `us-west-2` |
| `BDS_FIRESTORE_PROJECT` | GCP project for Firestore (when `BDS_DB_TYPE=firestore`) | (none) |
| `BDS_BUCKET_TYPE` | Object storage backend: `gcs`, `s3`, or empty (disabled) | (none) |
| `BDS_S3_REGION` | AWS region for S3 bucket | `us-west-2` |
| `BDS_S3_BUCKET` | S3 bucket name (when `BDS_BUCKET_TYPE=s3`) | (none) |
| `BDS_MAX_ATTACHMENT_BYTES` | Maximum attachment size in bytes | `10485760` (10MB) |

---

## Attachments

Attachments are supported for both inbound (SMTP) and outbound (web UI, SMTP relay) email. Mail bodies are stored in the database; attachments are stored in object storage (GCS or S3).

### Configuration

Enable object storage in `.env`:

**Google Cloud Storage:**
```bash
BDS_BUCKET_TYPE=gcs
BDS_GCS_BUCKET=your-bucket-name
```

**Amazon S3:**
```bash
BDS_BUCKET_TYPE=s3
BDS_S3_REGION=us-west-2
BDS_S3_BUCKET=your-bucket-name
```

### Size limits

Maximum attachment size defaults to 10MB. Change with:
```bash
BDS_MAX_ATTACHMENT_BYTES=20971520  # 20MB
```

### Security

- **Inbound**: ClamAV scans the full raw email (including MIME-encoded attachments)
- **Outbound**: Each uploaded attachment is scanned individually by ClamAV before sending
- Attachments exceeding the size limit are rejected with an SMTP 552 error

### How it works

- **Inbound SMTP**: MIME multipart messages are parsed; text body goes to DB, attachments go to bucket
- **Web UI compose**: File upload via `<input type="file" multiple>`; attachments saved to bucket
- **IMAP/POP3**: Messages are reconstructed as multipart MIME with base64-encoded attachments
- **Web UI read**: Attachments listed with download links
- Without object storage configured, attachments in inbound emails are silently dropped

---

## Database Backends

The application supports three database backends. Set `BDS_DB_TYPE` in `.env` to choose.

### PostgreSQL (default)

The default backend. Requires a PostgreSQL instance (Cloud SQL, RDS, or self-hosted).

```bash
BDS_DB_TYPE=postgres
DATABASE_URL=postgres://bdsmail:password@localhost:5432/bdsmail?sslmode=disable
```

### SQLite (simplest, $0)

File-based database stored on the VM's disk. No external service needed. Best for single-server deployments with moderate traffic.

```bash
BDS_DB_TYPE=sqlite
BDS_SQLITE_PATH=/opt/bdsmail/bdsmail.db
```

Tables are created automatically on first startup. The database file is created at the specified path.

### DynamoDB ($0 with AWS free tier)

AWS managed NoSQL database. The free tier (always-free, not time-limited) includes 25GB storage and 25 read/write capacity units — more than enough for a mail server.

```bash
BDS_DB_TYPE=dynamodb
BDS_DYNAMODB_REGION=us-west-2
```

The application automatically creates two DynamoDB tables (`bdsmail-users` and `bdsmail-messages`) with the required indexes. Your VM's IAM role or AWS credentials must have DynamoDB permissions.

### Cost comparison

| Backend | Monthly Cost | Pros | Cons |
|---------|-------------|------|------|
| PostgreSQL | $10-15 (managed) | Full SQL, robust | Requires managed DB service |
| SQLite | $0 | Zero config, no network dependency | Single-server only |
| DynamoDB | $0 (free tier) | Managed, scalable, always-free tier | AWS only, requires IAM setup |

---

## Security

BDS Mail provides comprehensive security at every layer. All security features are **enabled by default** and follow a fail-open strategy (if a check service is unavailable, mail is still accepted rather than dropped).

### Rate Limiting & Brute-Force Protection

Per-IP connection rate limiting and authentication failure tracking with automatic lockout.

- **Connection rate**: Limits connections per second per IP (default: 10/s, burst 20)
- **Auth lockout**: Locks out IPs after repeated failed login attempts (default: 5 failures, 15-minute lockout)
- **Applies to**: Both SMTP and web UI login

```bash
BDS_RATELIMIT_ENABLED=true
BDS_RATELIMIT_CONN_PER_SEC=10
BDS_RATELIMIT_CONN_BURST=20
BDS_RATELIMIT_MAX_AUTH_FAIL=5
BDS_RATELIMIT_LOCKOUT_SEC=900
```

### Rspamd Spam Filtering

Integrates with Rspamd via HTTP API for spam scoring on inbound mail.

- **Score >= reject threshold** (default 15.0): Email rejected (SMTP 550)
- **Score >= junk threshold** (default 6.0): Email delivered to "Junk" folder

**Setup**:
```bash
# Install Rspamd
apt-get install -y rspamd
systemctl enable rspamd
systemctl start rspamd

# Configure in .env
BDS_RSPAMD_ENABLED=true
BDS_RSPAMD_URL=http://localhost:11333
BDS_RSPAMD_REJECT_SCORE=15.0
BDS_RSPAMD_JUNK_SCORE=6.0
```

### ClamAV Virus Scanning

Scans email bodies for viruses and malware using a local ClamAV daemon.

- **Inbound**: Infected emails are rejected (SMTP 550)
- **Outbound**: Infected emails are blocked with an error shown to the user

**Setup**:
```bash
# Install ClamAV on the VM
apt-get install -y clamav clamav-daemon
freshclam
systemctl enable clamav-daemon clamav-freshclam
systemctl start clamav-daemon clamav-freshclam

# Enable in .env
BDS_CLAMAV_ENABLED=true
BDS_CLAMAV_ADDRESS=unix:/var/run/clamav/clamd.ctl
```

### Google Safe Browsing (Dangerous Link Detection)

Checks URLs in email bodies against Google's phishing/malware database.

- **Inbound**: Subject is prefixed with `[WARNING: Suspicious Links]`
- **Outbound**: Emails with dangerous links are blocked

**Setup**:
1. Enable the [Safe Browsing API](https://console.cloud.google.com/apis/library/safebrowsing.googleapis.com) in your GCP project
2. Create an API key in the [Credentials page](https://console.cloud.google.com/apis/credentials)
3. Add to `.env`:
```bash
BDS_SAFEBROWSING_ENABLED=true
BDS_SAFEBROWSING_API_KEY=your-api-key
```

### SPF/DKIM/DMARC Verification (Inbound)

Verifies sender authenticity on inbound mail by checking SPF, DKIM signatures, and DMARC policy.

- **DMARC `p=reject`**: Email is rejected (SMTP 550)
- **DMARC `p=quarantine`**: Email is delivered to the "Junk" folder
- **DMARC `p=none` or no record**: Email is delivered normally

```bash
BDS_AUTH_CHECK_ENABLED=true
```

### MTA-STS (RFC 8461) — Outbound TLS Policy Enforcement

Fetches and enforces MTA-STS policies for recipient domains during outbound delivery.

- Looks up `_mta-sts.<domain>` DNS TXT record, then fetches policy via HTTPS
- **Mode `enforce`**: Requires STARTTLS with valid certificate; rejects delivery to unauthorized MX hosts
- **Mode `testing`**: Logs violations but still delivers
- Policies are cached in memory with `max_age` expiry

```bash
BDS_MTASTS_ENABLED=true
BDS_MTASTS_TIMEOUT=10
```

### DANE/TLSA (RFC 6698/7672) — Certificate Verification via DNSSEC

Verifies outbound SMTP server certificates against TLSA DNS records.

- Queries `_25._tcp.<mx-host>` for TLSA records using a DNSSEC-validating resolver
- Supports DANE-EE (usage 3) and DANE-TA (usage 2) with SHA-256/SHA-512 matching
- Only enforced when DNSSEC AD flag is set (authenticated data)

```bash
BDS_DANE_ENABLED=true
BDS_DANE_TIMEOUT=5
BDS_DANE_RESOLVER=1.1.1.1:53
```

### TLSRPT (RFC 8460) — TLS Reporting

Sends daily aggregate reports about TLS connection successes/failures to receiving domains.

- Looks up `_smtp._tls.<domain>` for the `rua=` reporting address
- Generates JSON reports per RFC 8460 with failure details (STARTTLS not supported, certificate mismatch, DANE failure, etc.)

```bash
BDS_TLSRPT_ENABLED=true
BDS_TLSRPT_INTERVAL=86400
```

### REQUIRETLS (RFC 8689)

Supports the REQUIRETLS SMTP extension. When a sending MTA includes the REQUIRETLS flag, bdsmail ensures the entire outbound delivery path uses verified TLS.

- Advertised in EHLO response when TLS is available
- Combines with MTA-STS and DANE enforcement

This feature is always enabled when TLS is configured — no separate configuration needed.

### Security Check Pipeline (Inbound)

```
Connection → Rate Limit Check
    ↓
Auth (SMTP/Web) → Brute-force Check
    ↓
Message → ClamAV Scan → SPF/DKIM/DMARC → Rspamd Spam Score → Safe Browsing URL Check
    ↓
Deliver to INBOX / Junk / Reject
```

### Security Check Pipeline (Outbound)

```
Message → ClamAV Scan → Safe Browsing URL Check
    ↓
DKIM Sign → MTA-STS Policy Check → DANE/TLSA Verification → STARTTLS (enforced if required)
    ↓
Deliver via relay or direct MX
```

---

## Email Aliases

Aliases forward mail from one address to one or more targets. Managed via the admin web UI at `/admin/aliases`.

- **Standard alias**: `sales@domain.com` → `alice@domain.com, bob@domain.com`
- **Catch-all**: `@domain.com` → `admin@domain.com` (catches all undelivered mail for the domain)

Aliases are resolved recursively (max depth 10) during inbound SMTP delivery.

---

## Mailing Lists

Group distribution lists managed via `/admin/lists`. When mail is sent to a list address:

- The message is delivered to all list members' INBOX
- Subject is prefixed with `[ListName]`
- `List-Id` and `List-Unsubscribe` headers are added
- The sender does not receive their own copy

---

## Mail Filters

Server-side filtering rules at `/filters`. Each filter has:
- **Conditions**: field (from/to/subject/header), operator (contains/equals/exists), value
- **Actions**: move to folder, mark as read, delete, flag

### Default Presets

Four filters are available by default:
1. **Newsletters**: Messages with `List-Unsubscribe` header → "Newsletters" folder
2. **Social**: Messages from facebook/twitter/linkedin/instagram → "Social" folder
3. **Auto-read noreply**: Messages from noreply/no-reply addresses → mark as read
4. **Large attachments**: Attachments > 5MB → flag

Custom folders created by filter rules appear automatically in the IMAP folder list and web UI.

---

## Auto-Reply / Vacation

Configure at `/settings/autoreply`:
- Enable/disable auto-reply
- Custom subject and body
- Optional start and end dates

**Loop prevention**: Auto-replies are NOT sent to:
- `noreply`, `no-reply`, `mailer-daemon` addresses
- Messages with `List-Unsubscribe` or `Auto-Submitted` headers
- Senders already replied to within the last 24 hours

---

## Full-Text Search

Search across subject, body, from, and to fields:
- **Web UI**: Search bar in the inbox toolbar
- **IMAP**: `SEARCH TEXT "query"` command
- **SQLite**: Uses LIKE matching
- **PostgreSQL**: Uses ILIKE matching
- **DynamoDB/Firestore**: Client-side filtering

---

## Contacts / CardDAV

### Web UI

Manage contacts at `/contacts` — add, view, and delete contacts with name, email, and phone.

### CardDAV Protocol

Compatible with standard CardDAV clients (macOS Contacts, DAVx5, Thunderbird CardBook, etc.):

| Setting | Value |
|---------|-------|
| Server URL | `https://mail.yourdomain.com/carddav/user@domain.com/default/` |
| Username | Full email: `user@domain.com` |
| Password | Account password |
| Auth | HTTP Basic Auth (over HTTPS) |

Auto-discovery: `/.well-known/carddav` redirects to `/carddav/`.

---

## Admin Panel

The admin panel provides web-based management for:

| Page | URL | Description |
|------|-----|-------------|
| Domains | `/admin/domains` | Add/manage domains, view DNS records |
| Users | `/admin/users` | Create, list, delete users |
| Aliases | `/admin/aliases` | Create/delete email aliases and catch-alls |
| Mailing Lists | `/admin/lists` | Create lists, manage members |

All admin pages require the `BDS_ADMIN_SECRET` for authentication.

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

# Full copy

# Build
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/bdsmail ./cmd/bdsmail/

# Upload binary and templates
gcloud compute scp bin/bdsmail ${BDS_VM_NAME}:/tmp/bdsmail --zone="${BDS_ZONE}"
gcloud compute scp --recurse web/templates ${BDS_VM_NAME}:/tmp/templates --zone="${BDS_ZONE}"

# Then on the VM:

sudo -i
systemctl stop bdsmail
cp /tmp/bdsmail /opt/bdsmail/bdsmail
chmod +x /opt/bdsmail/bdsmail
cp /tmp/templates/* /opt/bdsmail/web/templates/
systemctl start bdsmail
```
