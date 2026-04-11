# Deployment Guide

bdsmail on Hetzner VPS with self-hosted PostgreSQL, SendGrid relay, and Cloudflare R2 storage.

| Component | Service | Cost |
|-----------|---------|------|
| Compute | Hetzner CX22 (2 vCPU, 4GB RAM, 40GB NVMe) | €4.50/mo |
| Database | Self-hosted PostgreSQL 18 | $0 |
| Outbound relay | SendGrid (100/day free) | $0 |
| Attachments + Backups | Cloudflare R2 (10GB free) | $0 |
| DNS + CDN | Cloudflare (free plan) | $0 |
| TLS | Let's Encrypt (per-domain SNI) | $0 |
| **Total** | | **~$5/mo** |

---

## Prerequisites

### Dev Machine Setup (macOS)

```bash
# Go
brew install go

# Node.js (for Vue SPA)
brew install node

# SSH key (if you don't have one)
ssh-keygen -t ed25519

# rclone (for R2 backup testing)
brew install rclone
```

---

## Step 1: Create Hetzner VPS

1. Go to [Hetzner Cloud Console](https://console.hetzner.cloud/)
2. Create new project: `bdsmail`
3. Add Server:
   - **Location**: Hillsboro, OR (closest to SoCal) or Ashburn, VA
   - **Image**: Debian 12
   - **Type**: CX22 (2 vCPU, 4GB RAM, 40GB NVMe) — €4.50/mo
   - **Networking**: Enable Public IPv4
   - **SSH Keys**: Add your public key (`cat ~/.ssh/id_ed25519.pub`)
   - **Name**: `bdsmail`
4. Note the **public IP address**

### SSH Config

```bash
cat >> ~/.ssh/config << 'EOF'

Host bdsmail
    HostName <hetzner-ip>
    User mustafa
    IdentityFile ~/.ssh/id_ed25519
EOF
```

### Initial Server Setup

```bash
ssh root@<hetzner-ip>

# Set timezone and hostname
timedatectl set-timezone America/Los_Angeles
hostnamectl set-hostname bdsmail

# Create admin user
adduser mustafa
usermod -aG sudo mustafa
mkdir -p /home/mustafa/.ssh
cp ~/.ssh/authorized_keys /home/mustafa/.ssh/
chown -R mustafa:mustafa /home/mustafa/.ssh
chmod 700 /home/mustafa/.ssh
chmod 600 /home/mustafa/.ssh/authorized_keys

# Disable exim4 (default mail server, conflicts with port 25)
systemctl stop exim4
systemctl disable exim4

# Create service user
groupadd bdsmail
useradd -r -g bdsmail -d /opt/bdsmail -s /usr/sbin/nologin bdsmail

# Allow admin to deploy files
usermod -aG bdsmail mustafa

# Create application directory structure
mkdir -p /opt/bdsmail/{bin,sql,log,ssl,dkim,acme,sec,scripts,backups}
mkdir -p /opt/bdsmail/web/{templates,static,vue/dist}
chown -R bdsmail:bdsmail /opt/bdsmail
chmod -R g+w /opt/bdsmail

# Install essentials
apt-get update && apt-get install -y curl ca-certificates gnupg cron rclone
systemctl enable cron && systemctl start cron

# Configure firewall
apt-get install -y ufw
ufw allow OpenSSH
ufw allow 25/tcp comment "SMTP"
ufw allow 80/tcp comment "ACME/HTTP"
ufw allow 110/tcp comment "POP3"
ufw allow 143/tcp comment "IMAP"
ufw allow 443/tcp comment "HTTPS"
ufw --force enable
```

---

## Step 2: Install PostgreSQL 18

```bash
# Add PostgreSQL official repo
curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc \
    | gpg --dearmor -o /usr/share/keyrings/postgresql.gpg
echo "deb [signed-by=/usr/share/keyrings/postgresql.gpg] \
    http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" \
    > /etc/apt/sources.list.d/pgdg.list

apt-get update && apt-get install -y postgresql-18 postgresql-client-18
systemctl enable postgresql && systemctl start postgresql
psql --version   # verify: PostgreSQL 18.x

# Tune for 4GB RAM
cat >> /etc/postgresql/18/main/postgresql.conf << 'CONF'
shared_buffers = 1GB
effective_cache_size = 3GB
work_mem = 10MB
maintenance_work_mem = 256MB
random_page_cost = 1.1
CONF
systemctl restart postgresql

# Create database and user
sudo -u postgres psql << 'SQL'
CREATE USER bdsmail WITH PASSWORD 'your-secure-password';
CREATE DATABASE bdsmail OWNER bdsmail;
SQL

echo "local bdsmail bdsmail md5" >> /etc/postgresql/18/main/pg_hba.conf
systemctl reload postgresql
```

---

## Step 3: Set Up Cloudflare R2

Single bucket for mail attachments and database backups.

### Create R2 Bucket

1. Cloudflare Dashboard → R2 → **Create Bucket**
2. Name: `bdsmail`
3. Location: Auto

### Create R2 API Token

1. Cloudflare Dashboard → R2 → **Manage R2 API Tokens**
2. Create token with **Object Read & Write** permissions for `bdsmail` bucket
3. Note: **Access Key ID** and **Secret Access Key**

### Configure rclone on Server

```bash
ssh bdsmail
sudo rclone config

# Name: r2
# Type: s3
# Provider: Cloudflare
# Access Key ID: <from R2 dashboard>
# Secret Access Key: <from R2 dashboard>
# Endpoint: https://<ACCOUNT_ID>.r2.cloudflarestorage.com
# ACL: private

# Test
rclone ls r2:bdsmail
```

---

## Step 4: Set Up SendGrid (Outbound Relay)

1. Sign up at [sendgrid.com](https://sendgrid.com) (free, 100 emails/day)
2. **Settings → API Keys → Create API Key** (Full Access)
3. Copy the key (starts with `SG.`)
4. **Settings → Sender Authentication → Authenticate Your Domain**
   - Enter your domain (e.g. daxoom.com)
   - Add the CNAME records SendGrid provides to Cloudflare
   - Verify

---

## Step 5: Configure DNS

### Platform Domain (bdscont.com)

| Type | Name | Value | Proxy |
|------|------|-------|-------|
| A | mailsrv | `<hetzner-ip>` | DNS only (grey) |

### Each Customer Domain (e.g. daxoom.com)

| Type | Name | Value | Proxy |
|------|------|-------|-------|
| CNAME | mail | mailsrv.bdscont.com | DNS only (grey) |
| MX | @ | mailsrv.bdscont.com (priority 10) | — |
| TXT | @ | `v=spf1 include:sendgrid.net ~all` | — |
| TXT | _dmarc | `v=DMARC1; p=none; rua=mailto:postmaster@domain.com` | — |
| TXT | bounce | `v=spf1 include:sendgrid.net ~all` | — |
| TXT | default._domainkey | `v=DKIM1; k=rsa; p=<key>` | — |

**Important**: `mail.*` subdomains must be **DNS only** (grey cloud) in Cloudflare — SMTP/IMAP/POP3 don't work through Cloudflare proxy.

---

## Step 6: Initialize Database

```bash
# Upload SQL file from dev machine
scp sql/bdsmail_pgsql.sql bdsmail:/opt/bdsmail/sql/

# Run on server (prompts for password)
ssh bdsmail "psql -h localhost -U bdsmail -d bdsmail -f /opt/bdsmail/sql/bdsmail_pgsql.sql"
```

---

## Step 7: Build and Upload

From dev machine:

```bash
cd /path/to/bdsmail

# Build Go binary for Linux
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/bdsmail ./cmd/bdsmail/

# Build Vue SPA
cd web/vue && npm install && npm run build && cd ../..
```

Use the deploy script:

```bash
scripts/deploy.sh all
```

---

## Step 8: Create Secrets

On the server (never upload secrets via scp):

```bash
ssh bdsmail
sudo vi /opt/bdsmail/sec/secrets.json
```

```json
{
  "database_url": "postgres://bdsmail:your-db-password@localhost:5432/bdsmail?sslmode=disable",
  "admin_secret": "your-admin-secret-from-openssl-rand-hex-32",
  "relay_host": "smtp.sendgrid.net",
  "relay_user": "apikey",
  "relay_password": "SG.your-sendgrid-api-key"
}
```

```bash
sudo chown bdsmail:bdsmail /opt/bdsmail/sec/secrets.json
sudo chmod 600 /opt/bdsmail/sec/secrets.json
```

### Generate Admin Secret

```bash
openssl rand -hex 32
```

---

## Step 9: Install Systemd Service

```bash
# Link service file (uploaded in Step 7)
sudo ln -s /opt/bdsmail/scripts/bdsmail.service /etc/systemd/system/bdsmail.service

# Edit to set your R2 bucket and mail hostname
sudo vi /opt/bdsmail/scripts/bdsmail.service

# Start
sudo systemctl daemon-reload
sudo systemctl enable bdsmail
sudo systemctl start bdsmail

# Check logs
sudo journalctl -u bdsmail -f
```

---

## Step 10: Bootstrap First Domain

```bash
ssh bdsmail
sudo -i

# Insert domain
psql -h localhost -U bdsmail -d bdsmail << 'SQL'
INSERT INTO domain (name, status, created_by) VALUES ('yourdomain.com', 'active', 'admin');
UPDATE domain SET ses_status = 'Success', dkim_status = 'Success' WHERE name = 'yourdomain.com';
SQL

# Install certbot and issue TLS cert
apt-get install -y certbot
systemctl stop bdsmail

certbot certonly --standalone \
    -d mail.yourdomain.com \
    --non-interactive --agree-tos \
    --email admin@yourdomain.com

# Also issue cert for platform domain
certbot certonly --standalone \
    -d mailsrv.bdscont.com \
    --non-interactive --agree-tos \
    --email admin@bdscont.com

# Copy certs to bdsmail SSL directory
for DOMAIN in yourdomain.com bdscont.com; do
    CERTDIR=$(echo $DOMAIN | sed 's/^/mail./; s/^mail.mailsrv./mailsrv./')
    mkdir -p /opt/bdsmail/ssl/$DOMAIN
    cp /etc/letsencrypt/live/$CERTDIR/fullchain.pem /opt/bdsmail/ssl/$DOMAIN/
    cp /etc/letsencrypt/live/$CERTDIR/privkey.pem /opt/bdsmail/ssl/$DOMAIN/
done
chown -R bdsmail:bdsmail /opt/bdsmail/ssl

# Generate DKIM key
openssl genrsa -out /opt/bdsmail/dkim/yourdomain.com.pem 2048
chmod 600 /opt/bdsmail/dkim/yourdomain.com.pem
chown bdsmail:bdsmail /opt/bdsmail/dkim/yourdomain.com.pem

# Print DKIM public key — add as TXT record: default._domainkey
openssl rsa -in /opt/bdsmail/dkim/yourdomain.com.pem -pubout -outform DER | openssl base64 -A
echo ""

# Start bdsmail
systemctl start bdsmail
journalctl -u bdsmail -n 10
```

### Create First User

```bash
apt-get install -y python3-bcrypt
python3 -c "import bcrypt; print(bcrypt.hashpw(b'YOUR_PASSWORD', bcrypt.gensalt()).decode())"

psql -h localhost -U bdsmail -d bdsmail << SQL
INSERT INTO user_account (id, username, domain, display_name, password_hash, status)
VALUES ('admin@yourdomain.com', 'admin', 'yourdomain.com', 'Admin', 'PASTE_HASH', 'A');

INSERT INTO user_permission (id, user_email, role, domain, end_date, created_by)
VALUES ('perm_owner_1', 'admin@yourdomain.com', 'owner', 'yourdomain.com', '2099-12-31 23:59:59+00', 'admin@yourdomain.com');

INSERT INTO user_permission (id, user_email, role, domain, end_date, created_by)
VALUES ('perm_superadmin_1', 'admin@yourdomain.com', 'superadmin', 'bdscont.com', '2099-12-31 23:59:59+00', 'admin@yourdomain.com');
SQL
```

Verify: open `https://mail.yourdomain.com` — login with `admin` and your password.

---

## Step 11: Set Up Cron Jobs

```bash
ssh bdsmail
sudo -i

# Certificate renewal — daily at 2 AM
echo "0 2 * * * /opt/bdsmail/scripts/renew_certs.sh /opt/bdsmail/ssl >> /var/log/bdsmail-certs.log 2>&1" >> /var/spool/cron/crontabs/root

# Database backup to R2 — daily at 3 AM
echo "0 3 * * * /opt/bdsmail/scripts/backup.sh bdsmail >> /var/log/bdsmail-backup.log 2>&1" >> /var/spool/cron/crontabs/root

# Verify
crontab -l
```

---

## Step 12: Test

1. **Web UI**: `https://mail.yourdomain.com`
2. **Send email**: Compose → send to your Gmail → check inbox
3. **Receive email**: Send from Gmail to `admin@yourdomain.com`
4. **DKIM check**: In Gmail, click ⋮ → Show original → verify `DKIM: PASS`
5. **Signup**: `https://mailsrv.bdscont.com/signup` (platform domain only)
6. **Super admin**: `https://mailsrv.bdscont.com/superadmin`
7. **Deliverability**: [mail-tester.com](https://www.mail-tester.com)

---

## Updating bdsmail.service

The service file needs to be updated for R2 storage. Edit `/opt/bdsmail/scripts/bdsmail.service`:

```ini
[Unit]
Description=BDS Mail Server
After=network.target postgresql.service

[Service]
Type=simple
User=bdsmail
Group=bdsmail
WorkingDirectory=/opt/bdsmail
AmbientCapabilities=CAP_NET_BIND_SERVICE
ExecStart=/opt/bdsmail/bin/bdsmail \
  --db_type=postgres \
  --ssl_dir=/opt/bdsmail/ssl \
  --inbound_smtp_port=25 \
  --pop3_port=110 \
  --imap_port=143 \
  --https_port=443 \
  --http_port=80 \
  --dkim_key_dir=/opt/bdsmail/dkim \
  --acme_webroot=/opt/bdsmail/acme \
  --bucket_type=s3 \
  --s3_bucket=bdsmail \
  --s3_region=auto \
  --secret_mode=local \
  --keystore=/opt/bdsmail/sec/secrets.json \
  --mail_hostname=mailsrv.bdscont.com
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Note: Cloudflare R2 is S3-compatible. Use `--bucket_type=s3` with `--s3_region=auto`.

---

## Updating backup.sh for R2

The backup script needs to use `rclone` instead of `aws s3`:

```bash
# In scripts/backup.sh, replace:
#   aws s3 cp → rclone copy
#   aws s3 ls → rclone ls
#   aws s3 rm → rclone delete
```

---

## Password Reset

```bash
python3 -c "import bcrypt; print(bcrypt.hashpw(b'NEW_PASSWORD', bcrypt.gensalt()).decode())"
psql -h localhost -U bdsmail -d bdsmail -c "UPDATE user_account SET password_hash = 'NEW_HASH' WHERE id = 'user@domain.com';"
```

---

## Redeploy

```bash
# From dev machine
scripts/deploy.sh bin    # code change
scripts/deploy.sh web    # template change
scripts/deploy.sh vue    # Vue SPA change
scripts/deploy.sh all    # everything
```

---

## Domain Onboarding

New domains are registered via self-service signup at `https://mailsrv.bdscont.com/signup`.

1. User enters domain, username, password
2. bdsmail shows DNS records to add (CNAME, MX, SPF, DMARC, bounce SPF)
3. User adds records → clicks Verify
4. bdsmail checks MX → provisions domain, creates user, generates DKIM
5. Shows remaining DNS records (DKIM TXT)
6. All DNS records saved to `domain_dns` table — viewable anytime at `/settings/users`

---

## Migration from AWS Lightsail

If migrating from an existing Lightsail deployment:

```bash
# On Lightsail — dump database
ssh old-bdsmail "sudo -u postgres pg_dump -Fc bdsmail > /tmp/bdsmail.dump"
scp old-bdsmail:/tmp/bdsmail.dump /tmp/

# On Hetzner — restore
scp /tmp/bdsmail.dump bdsmail:/tmp/
ssh bdsmail "pg_restore -h localhost -U bdsmail -d bdsmail /tmp/bdsmail.dump"

# Copy SSL certs and DKIM keys
scp -r old-bdsmail:/opt/bdsmail/ssl/ bdsmail:/opt/bdsmail/ssl/
scp -r old-bdsmail:/opt/bdsmail/dkim/ bdsmail:/opt/bdsmail/dkim/

# Update DNS — point mailsrv.bdscont.com to new Hetzner IP
# Update mail.yourdomain.com to new IP (or CNAME to mailsrv.bdscont.com)

# Delete Lightsail instance
aws lightsail delete-instance --instance-name bdsmail
aws lightsail release-static-ip --static-ip-name bdsmail-ip
aws s3 rb s3://bdsmail-<account-id> --force
```
