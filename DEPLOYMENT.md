# Deployment Guide

Step-by-step deployment instructions for bdsmail on AWS or GCP.

All deployments require:
- A **Linux server** with a static public IP
- **Domain names** with DNS access (Route 53, GoDaddy, Cloudflare, etc.)
- **Go 1.21+** on your dev machine (for building)
- **Open ports**: 25 (SMTP), 80 (ACME), 110 (POP3), 143 (IMAP), 443 (HTTPS)

### Dev Machine Setup (macOS)

```bash
# Go (if not installed)
brew install go

# Node.js (for Vue SPA build)
brew install node

# AWS CLI
brew install awscli
aws configure   # Enter Access Key ID, Secret Access Key, region

# GCP CLI (if deploying on GCP)
brew install --cask google-cloud-sdk
gcloud init
gcloud auth application-default login

# Verify
go version && node --version && aws --version
```

---

## DNS Configuration (All Platforms)

For **each domain** (e.g. `yourdomain.com`), add the following DNS records:

| Type | Name | Value |
|------|------|-------|
| A | mail | `<your-server-ip>` |
| MX | @ | `mail.yourdomain.com` (priority 10) |
| TXT | @ | `v=spf1 ip4:<your-server-ip> ~all` |
| TXT | _dmarc | `v=DMARC1; p=none; rua=mailto:postmaster@yourdomain.com` |
| TXT | default._domainkey | *(generated during domain onboarding)* |

```bash
# Verify propagation
dig A mail.yourdomain.com
dig MX yourdomain.com
dig TXT yourdomain.com
dig TXT _dmarc.yourdomain.com
```

---

## Build

```bash
# Build Go binary
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/bdsmail ./cmd/bdsmail/

# Optional: Build Vue SPA
cd web/vue && npm install && npm run build && cd ../..
```

---

## Option 1: Deploy on AWS

Lightsail instance + Amazon SES for outbound email. Single S3 bucket for attachments and backups.

### Cost Summary

| Stack | DB | Monthly Cost |
|-------|----|-------------|
| Lightsail + Self-hosted PG + SES + S3 | PostgreSQL (on instance) | **~$5-6** (recommended) |
| Lightsail + DynamoDB + SES + S3 | DynamoDB (free tier) | **~$6** |
| Lightsail + RDS + SES + S3 | PostgreSQL (managed) | **~$20** |

### Step 1: Create Lightsail Instance

```bash
aws lightsail create-instances \
    --instance-names bdsmail \
    --availability-zone us-east-1a \
    --blueprint-id debian_12 \
    --bundle-id micro_3_0

aws lightsail allocate-static-ip --static-ip-name bdsmail-ip
aws lightsail attach-static-ip --static-ip-name bdsmail-ip --instance-name bdsmail

for PORT in 25 80 110 143 443; do
    aws lightsail open-instance-public-ports \
        --instance-name bdsmail \
        --port-info fromPort=$PORT,toPort=$PORT,protocol=tcp
done

# Get your static IP (use this for DNS A records)
aws lightsail get-static-ip --static-ip-name bdsmail-ip \
    --query 'staticIp.ipAddress' --output text
```

#### SSH Access

```bash
# Download Lightsail default SSH key
aws lightsail download-default-key-pair \
    --query 'privateKeyBase64' --output text > ~/.ssh/lightsail-bdsmail.pem
chmod 600 ~/.ssh/lightsail-bdsmail.pem

# Add to SSH config for easy access
cat >> ~/.ssh/config << 'EOF'

Host bdsmail
    HostName <your-static-ip>
    User admin
    IdentityFile ~/.ssh/lightsail-bdsmail.pem
EOF

# Now connect with just:
ssh bdsmail
```

### Step 2: Create S3 Bucket

Single bucket for mail attachments and database backups:

```bash
aws sts get-caller-identity --query 'Account' --output text
aws s3 mb s3://bdsmail-<your-account-id> --region us-east-1
aws s3api put-object --bucket bdsmail-<your-account-id> --key attachments/
aws s3api put-object --bucket bdsmail-<your-account-id> --key db/
```

### Step 3: Set Up Amazon SES

```bash
# Verify domain in SES (auto-done during domain onboarding, but manual for first domain)
aws ses verify-domain-identity --domain yourdomain.com
aws ses verify-domain-dkim --domain yourdomain.com
```

1. Open [SES Console](https://console.aws.amazon.com/ses/) → **SMTP Settings → Create SMTP Credentials**
2. Save the credentials
3. Request **Production Access** (sandbox only allows verified recipients)

### Step 4: Set Up Database

Choose one:

#### Option A: Self-Hosted PostgreSQL (recommended, $0)

```bash
ssh bdsmail
sudo -i

# Create service user
groupadd bdsmail
useradd -r -g bdsmail -d /opt/bdsmail -s /usr/sbin/nologin bdsmail

# Create application directory structure
mkdir -p /opt/bdsmail/{bin,sql,log,ssl,dkim,acme,sec,scripts,backups}
mkdir -p /opt/bdsmail/web/{templates,static,vue/dist}
chown -R bdsmail:bdsmail /opt/bdsmail

# Allow admin user to upload files via scp
usermod -aG bdsmail admin
chmod -R g+w /opt/bdsmail

# Add PostgreSQL official repo (for latest version)
apt-get update && apt-get install -y curl ca-certificates gnupg
curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc \
    | gpg --dearmor -o /usr/share/keyrings/postgresql.gpg
echo "deb [signed-by=/usr/share/keyrings/postgresql.gpg] \
    http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" \
    > /etc/apt/sources.list.d/pgdg.list

# Install PostgreSQL 18
apt-get update && apt-get install -y postgresql-18 postgresql-client-18
systemctl enable postgresql && systemctl start postgresql
psql --version   # verify

# Create database and user
sudo -u postgres psql << 'SQL'
CREATE USER bdsmail WITH PASSWORD 'your-secure-password';
CREATE DATABASE bdsmail OWNER bdsmail;
SQL

echo "local bdsmail bdsmail md5" >> /etc/postgresql/18/main/pg_hba.conf
systemctl reload postgresql

# Initialize schema
psql -U bdsmail -d bdsmail -f /opt/bdsmail/sql/bdsmail_pgsql.sql
```

Set up daily backup to S3:

```bash
# Upload backup script
scp scripts/backup.sh bdsmail:/opt/bdsmail/scripts/

# On server — install cron and add backup job
apt-get install -y cron
systemctl enable cron && systemctl start cron
echo "0 3 * * * /opt/bdsmail/scripts/backup.sh bdsmail-<your-account-id> >> /var/log/bdsmail-backup.log 2>&1" | crontab -
```

**Migration to RDS later**: `pg_dump -Fc bdsmail > backup.dump` → create RDS instance → `pg_restore -h <rds-endpoint> -d bdsmail backup.dump` → update secrets.json.

#### Option B: DynamoDB ($0 free tier, no full-text search)

```bash
# On your dev machine — create tables
go run internal/store/init_dynamodb.go --region us-east-1

# Add IAM policy for Lightsail instance
aws iam create-policy --policy-name bdsmail-dynamodb \
    --policy-document file://iam-dynamodb-policy.json

# Configure AWS credentials on instance
ssh admin@<lightsail-ip>
aws configure
```

### Step 5: Build and Upload Everything

From your dev machine:

```bash
cd /path/to/bdsmail

# Build Go binary for Linux
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/bdsmail ./cmd/bdsmail/

# Build Vue SPA
cd web/vue && npm install && npm run build && cd ../..

# Upload all files
scp bin/bdsmail bdsmail:/opt/bdsmail/bin/
scp -r web/templates bdsmail:/opt/bdsmail/web/templates/
scp -r web/static bdsmail:/opt/bdsmail/web/static/
scp -r web/vue/dist bdsmail:/opt/bdsmail/web/vue/dist/
scp -r scripts bdsmail:/opt/bdsmail/scripts/
scp -r sql bdsmail:/opt/bdsmail/sql/

# Fix ownership and permissions once
ssh bdsmail "sudo chown -R bdsmail:bdsmail /opt/bdsmail \
    && sudo chmod +x /opt/bdsmail/scripts/*.sh \
    && sudo chmod 600 /opt/bdsmail/sec/secrets.json"
```

Vue SPA is served at `/app/` by the Go server automatically.

#### Optional: AWS Amplify (CDN frontend)

```bash
cd web/vue && npm run build
npm install -g @aws-amplify/cli
amplify init && amplify add hosting && amplify publish
```

Add `--amplify_url=<amplify-app-url>` to the systemd service. Each domain gets `webmail.<domain>` CNAME → Amplify URL.

### Step 7: Create Systemd Service

The service file is in `scripts/bdsmail.service` (uploaded in Step 5). Edit it on the server to set your S3 bucket name, then install:

```bash
ssh bdsmail
sudo -i

# Edit service file — change CHANGE_ME to your S3 bucket name
vi /opt/bdsmail/scripts/bdsmail.service

# For DynamoDB, also change --db_type=postgres to:
#   --db_type=dynamodb --dynamodb_region=us-west-2

# Link and enable
ln -s /opt/bdsmail/scripts/bdsmail.service /etc/systemd/system/bdsmail.service
```

Create secrets file on the server (never upload secrets via scp):

```bash
# Generate a strong admin secret
openssl rand -hex 32
# Copy the output — this is your admin panel password (/admin/domains)

# Get SES SMTP credentials:
# 1. Go to SES Console → SMTP Settings → Create SMTP Credentials
#    https://us-west-2.console.aws.amazon.com/ses/home?region=us-west-2#/smtp
# 2. Name the IAM user (e.g. bdsmail-ses) → Create User
# 3. Download credentials (shown only once):
#    - SMTP Username → relay_user
#    - SMTP Password → relay_password

cat > /opt/bdsmail/sec/secrets.json << 'EOF'
{
  "database_url": "postgres://bdsmail:YOUR_DB_PASSWORD@localhost:5432/bdsmail?sslmode=disable",
  "admin_secret": "YOUR_ADMIN_SECRET",
  "relay_host": "email-smtp.us-west-2.amazonaws.com",
  "relay_user": "YOUR_SES_SMTP_USERNAME",
  "relay_password": "YOUR_SES_SMTP_PASSWORD"
}
EOF
chmod 600 /opt/bdsmail/sec/secrets.json

# Start service
systemctl daemon-reload
systemctl enable bdsmail
systemctl start bdsmail

# Verify
systemctl status bdsmail
journalctl -u bdsmail -f
```

### Step 8: Set Up Certificate Renewal

```bash
# Upload renewal script
scp scripts/renew_certs.sh admin@<lightsail-ip>:/opt/bdsmail/scripts/

# On server — add daily cron at 2 AM
echo "0 2 * * * /opt/bdsmail/scripts/renew_certs.sh /opt/bdsmail/ssl >> /var/log/bdsmail-certs.log 2>&1" | crontab -
```

---

## Option 2: Deploy on GCP

GCP Compute Engine VM. Choose your database and outbound relay.

### Cost Summary

| Stack | DB | Monthly Cost |
|-------|----|-------------|
| e2-micro + Self-hosted PG + SES + GCS | PostgreSQL (on instance) | **~$10.50** |
| e2-micro + Firestore + SES + GCS | Firestore (free tier) | **~$10.50** |
| e2-micro + Cloud SQL + SES + GCS | PostgreSQL (managed) | **~$20.50** |

### Step 1: Create GCP VM

```bash
export GCP_PROJECT_ID=your-project-id
export BDS_REGION=us-west1
export BDS_ZONE=us-west1-b

gcloud compute instances create bdsmail \
    --zone=$BDS_ZONE \
    --machine-type=e2-micro \
    --image-family=debian-12 \
    --image-project=debian-cloud \
    --tags=mail-server

# Reserve static IP
gcloud compute addresses create bdsmail-ip --region=$BDS_REGION
gcloud compute instances add-access-config bdsmail \
    --zone=$BDS_ZONE \
    --access-config-name="External NAT" \
    --address=$(gcloud compute addresses describe bdsmail-ip --region=$BDS_REGION --format='get(address)')

# Open firewall ports
gcloud compute firewall-rules create allow-mail \
    --allow=tcp:25,tcp:80,tcp:110,tcp:143,tcp:443 \
    --target-tags=mail-server
```

### Step 2: Create GCS Bucket

Single bucket for mail attachments and database backups:

```bash
gcloud storage buckets create "gs://${GCP_PROJECT_ID}-bdsmail" \
    --location=$BDS_REGION \
    --uniform-bucket-level-access

# Grant VM service account access
export VM_SA=$(gcloud compute instances describe bdsmail \
    --zone=$BDS_ZONE --format="get(serviceAccounts[0].email)")

gcloud storage buckets add-iam-policy-binding "gs://${GCP_PROJECT_ID}-bdsmail" \
    --member="serviceAccount:${VM_SA}" \
    --role=roles/storage.objectAdmin
```

### Step 3: Set Up Outbound Relay

#### Option A: Amazon SES (cheapest, $0.10/1K emails)

Same SES setup as AWS option — create SMTP credentials in SES Console.

```bash
# secrets.json
"relay_host": "email-smtp.us-west-2.amazonaws.com",
"relay_user": "your-ses-smtp-username",
"relay_password": "your-ses-smtp-password"
```

#### Option B: SendGrid ($19.95/mo for 50K emails)

```bash
"relay_host": "smtp.sendgrid.net",
"relay_user": "apikey",
"relay_password": "your-sendgrid-api-key"
```

#### Option C: Mailgun ($15/mo for 10K emails)

```bash
"relay_host": "smtp.mailgun.org",
"relay_user": "postmaster@your-mailgun-domain.com",
"relay_password": "your-mailgun-smtp-password"
```

### Step 4: Set Up Database

Choose one:

#### Option A: Self-Hosted PostgreSQL (recommended, $0)

Same as AWS Step 4 Option A — install PostgreSQL on the VM, create user/database, initialize schema, set up backup (to GCS instead of S3):

```bash
# Backup to GCS (modify scripts/backup.sh to use gsutil)
gsutil cp "$BACKUP_DIR/$FILENAME" "gs://${GCP_PROJECT_ID}-bdsmail/db/$FILENAME"
```

#### Option B: Firestore ($0 free tier, no full-text search)

```bash
gcloud firestore databases create --location=$BDS_REGION

# Create composite indexes (see sql/bdsmail_firestore.md)
gcloud firestore indexes composite create \
  --collection-group=bdsmail-messages \
  --field-config field-path=owner_user,order=ASCENDING \
  --field-config field-path=folder,order=ASCENDING \
  --field-config field-path=deleted,order=ASCENDING \
  --field-config field-path=received_at,order=DESCENDING
```

#### Option C: Cloud SQL PostgreSQL ($10/mo managed)

```bash
gcloud sql instances create bdsmail-db \
    --database-version=POSTGRES_15 \
    --tier=db-f1-micro \
    --region=$BDS_REGION

gcloud sql databases create bdsmail --instance=bdsmail-db
gcloud sql users create bdsmail --instance=bdsmail-db --password='your-secure-password'
```

Set up [Cloud SQL Auth Proxy](https://cloud.google.com/sql/docs/postgres/connect-compute-engine) for secure local connection.

**Migration path**: Self-hosted PG → Cloud SQL via `pg_dump` / `pg_restore`.

### Step 5: Deploy Application

```bash
gcloud compute scp bin/bdsmail bdsmail:/opt/bdsmail/bin/ --zone=$BDS_ZONE
gcloud compute scp --recurse web bdsmail:/opt/bdsmail/web/ --zone=$BDS_ZONE
gcloud compute scp --recurse scripts bdsmail:/opt/bdsmail/scripts/ --zone=$BDS_ZONE
gcloud compute scp --recurse sql bdsmail:/opt/bdsmail/sql/ --zone=$BDS_ZONE
```

### Step 6: Deploy Vue Frontend

#### Option A: Same instance (embedded, $0)

Vue SPA served at `/app/` by Go binary.

#### Option B: Firebase Hosting ($0 free tier)

```bash
npm install -g firebase-tools
cd web/vue && npm run build
firebase init hosting   # Select project, set public dir to "dist"
firebase deploy --only hosting
```

### Step 7: Create Systemd Service

Same structure as AWS Step 7. Adjust flags:

```bash
ExecStart=/opt/bdsmail/bin/bdsmail \
  --db_type=postgres \
  --ssl_dir=/opt/bdsmail/ssl \
  --bucket_type=gcs \
  --gcs_bucket=${GCP_PROJECT_ID}-bdsmail \
  --secret_mode=local \
  --keystore=/opt/bdsmail/sec/secrets.json \
  ...
```

For Firestore: `--db_type=firestore --gcp_project_id=${GCP_PROJECT_ID}`

### Step 8: Set Up Certificate Renewal

Same as AWS Step 8 — cron runs `scripts/renew_certs.sh` daily.

---

## Database Initialization

DDL scripts are in the `sql/` folder. Run before starting the application.

| Backend | Command |
|---------|---------|
| PostgreSQL | `psql -U bdsmail -d bdsmail -f sql/bdsmail_pgsql.sql` |
| DynamoDB | `go run internal/store/init_dynamodb.go --region us-east-1` |
| Firestore | Collections auto-created. Indexes: see `sql/bdsmail_firestore.md` |

All scripts are idempotent (safe to re-run).

---

## Post-Deployment Checklist

1. Initialize database (see above)
2. Add DNS records (A, MX, SPF, DMARC) for each domain
3. Add DKIM record printed by domain onboarding
4. Create users: `./bdsmail -adduser user@domain.com -password 'pass' -displayname 'Name'`
5. Test web UI: `https://mail.yourdomain.com`
6. Test sending: compose to Gmail, check DKIM passes in "Show original"
7. Test receiving: send from Gmail to `user@yourdomain.com`
8. Check deliverability: [mail-tester.com](https://www.mail-tester.com)

---

## Configuration Reference

All configuration via CLI flags. Secrets loaded from SecretProvider (`--secret_mode=local|aws|gsm`):

| Flag | Description | Default |
|------|-------------|---------|
| `--smtp_port` | SMTP port | `25` |
| `--pop3_port` | POP3 port | `110` |
| `--imap_port` | IMAP port | `143` |
| `--https_port` | HTTPS port | `443` |
| `--http_port` | HTTP port (ACME) | `80` |
| `--ssl_dir` | Per-domain SSL certificate directory | `/opt/bdsmail/ssl` |
| `--db_type` | `postgres`, `dynamodb`, `firestore` | `postgres` |
| `--dynamodb_region` | AWS region for DynamoDB | `us-east-1` |
| `--gcp_project_id` | GCP project for Firestore | (none) |
| `--bucket_type` | `s3` or `gcs` | (none) |
| `--s3_bucket` / `--gcs_bucket` | Bucket name | (none) |
| `--s3_region` | S3 region | `us-east-1` |
| `--dkim_key_dir` | DKIM keys directory | `/opt/bdsmail/dkim` |
| `--acme_webroot` | ACME challenge directory | `/opt/bdsmail/acme` |
| `--amplify_url` | Amplify URL for webmail CNAME | (none) |
| `--secret_mode` | `local`, `aws`, or `gsm` | `local` |
| `--keystore` | Local secrets JSON path | `/opt/bdsmail/sec/secrets.json` |
| `--max_attachment_bytes` | Max attachment size | `10485760` (10MB) |

---

## Troubleshooting

```bash
# Service status
systemctl status bdsmail
journalctl -u bdsmail -f

# Certificate check
ls /opt/bdsmail/ssl/*/fullchain.pem

# SMTP connectivity
telnet mail.yourdomain.com 25

# TLS check
openssl s_client -connect mail.yourdomain.com:25 -starttls smtp

# DNS records
dig MX yourdomain.com
dig A mail.yourdomain.com
dig TXT default._domainkey.yourdomain.com

# Redeploy
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/bdsmail ./cmd/bdsmail/
scp bin/bdsmail admin@<ip>:/opt/bdsmail/bin/
ssh admin@<ip> "sudo systemctl restart bdsmail"
```

---

## Domain Onboarding

Adding a new domain is a single admin action. The server handles DKIM, SES, TLS, and DNS record generation automatically.

### Add Domain

**Web UI**: `https://mail.yourdomain.com/admin/domains` → enter domain → **Add Domain**

**CLI**: `./bdsmail -adddomain newdomain.com`

**API**:
```bash
curl -X POST https://mail.yourdomain.com/admin/api/domains \
  -H "Authorization: Bearer YOUR_ADMIN_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"domain": "newdomain.com"}'
```

### What Happens Automatically

| Step | Action |
|------|--------|
| 1 | Validate domain format, check not already registered |
| 2 | Generate DKIM 2048-bit RSA key |
| 3 | Verify domain in SES + request SES DKIM tokens |
| 4 | Generate per-domain API key |
| 5 | Issue TLS cert via `certbot certonly -d mail.<domain>` |
| 6 | Add domain to running server (no restart) |
| 7 | Return all DNS records to add |

### DNS Records per Domain

| Type | Name | Value |
|------|------|-------|
| A | `mail` | `<server-ip>` |
| CNAME | `webmail` | `<amplify-url>` (if Amplify) |
| MX | `@` | `mail.<domain>` (priority 10) |
| TXT | `@` | `v=spf1 a mx ~all` |
| TXT | `default._domainkey` | `v=DKIM1; k=rsa; p=<key>` |
| TXT | `_dmarc` | `v=DMARC1; p=none; ...` |
| CNAME | `<token>._domainkey` (x3) | `<token>.dkim.amazonses.com` (SES DKIM) |

### Bulk Onboarding

```bash
for DOMAIN in domain1.com domain2.com domain3.com; do
  curl -s -X POST "$SERVER/admin/api/domains" \
    -H "Authorization: Bearer $ADMIN_SECRET" \
    -H "Content-Type: application/json" \
    -d "{\"domain\": \"$DOMAIN\"}" | python3 -m json.tool
done
```

### Cost Per Additional Domain

| Resource | Cost |
|----------|------|
| Compute, DB, S3, Amplify | $0 (shared) |
| SES | $0.10/1K emails |
| DNS | $0.50/zone (Route 53) or free (GoDaddy, Cloudflare) |
| TLS | $0 (Let's Encrypt) |
