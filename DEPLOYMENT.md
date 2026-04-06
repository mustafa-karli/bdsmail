# Deployment Guide

Step-by-step deployment instructions for bdsmail on different cloud platforms.

All options require:
- A **Linux server** with a static public IP
- **Domain names** with DNS access (GoDaddy, Route 53, Cloudflare, etc.)
- **Go 1.21+** on your dev machine (for building)
- **Open ports**: 25 (SMTP), 80 (ACME), 110 (POP3), 143 (IMAP), 443 (HTTPS)

---

## DNS Configuration (All Platforms)

For **each domain** (e.g. `yourdomain.com`), add the following DNS records:

| Type | Name | Value |
|------|------|-------|
| A | mail | `<your-server-ip>` |
| MX | @ | `mail.yourdomain.com` (priority 10) |
| TXT | @ | `v=spf1 ip4:<your-server-ip> ~all` |
| TXT | _dmarc | `v=DMARC1; p=none; rua=mailto:postmaster@yourdomain.com` |
| TXT | default._domainkey | *(generated during deployment — see Step 3)* |

### Verify DNS propagation

```bash
dig A mail.yourdomain.com
dig MX yourdomain.com
dig TXT yourdomain.com           # SPF
dig TXT _dmarc.yourdomain.com   # DMARC
```

---

## Build the Binary

On your development machine:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/bdsmail ./cmd/bdsmail/
```

### Optional: Build Vue SPA

```bash
cd web/vue && npm install && npm run build && cd ../..
```

---

## Option 1: AWS Lightsail + DynamoDB + SES

**Cost: ~$6/month** — Cheapest full-featured deployment.

| Component | Service | Cost |
|-----------|---------|------|
| Compute + IP | Lightsail nano ($5 plan) | $5.00 |
| Database | DynamoDB (always-free tier) | $0.00 |
| Attachments | S3 (optional, 5GB free) | ~$0.50 |
| Outbound relay | Amazon SES | ~$0.10/1K emails |
| **Total** | | **~$6/month** |

### 1. Create Lightsail Instance

```bash
aws lightsail create-instances \
    --instance-names bdsmail \
    --availability-zone us-east-1a \
    --blueprint-id debian_12 \
    --bundle-id nano_3_2

# Allocate and attach static IP
aws lightsail allocate-static-ip --static-ip-name bdsmail-ip
aws lightsail attach-static-ip --static-ip-name bdsmail-ip --instance-name bdsmail
```

### 2. Open Firewall Ports

```bash
for PORT in 25 80 110 143 443; do
    aws lightsail open-instance-public-ports \
        --instance-name bdsmail \
        --port-info fromPort=$PORT,toPort=$PORT,protocol=tcp
done
```

### 3. Set Up DynamoDB IAM

```bash
cat > /tmp/bdsmail-policy.json << 'EOF'
{
    "Version": "2012-10-17",
    "Statement": [{
        "Effect": "Allow",
        "Action": [
            "dynamodb:CreateTable", "dynamodb:DescribeTable",
            "dynamodb:PutItem", "dynamodb:GetItem", "dynamodb:Query",
            "dynamodb:Scan", "dynamodb:UpdateItem", "dynamodb:DeleteItem"
        ],
        "Resource": "arn:aws:dynamodb:*:*:table/bdsmail-*"
    }]
}
EOF

aws iam create-policy --policy-name bdsmail-dynamodb \
    --policy-document file:///tmp/bdsmail-policy.json
```

Configure AWS credentials on the Lightsail instance:

```bash
aws configure   # Enter Access Key, Secret, region (e.g. us-east-1)
```

### 4. Set Up Amazon SES (Outbound Relay)

1. Open [Amazon SES Console](https://console.aws.amazon.com/ses/)
2. Go to **Verified Identities → Create Identity → Domain**
3. Enter your domain, add the DNS records SES provides (CNAME + TXT)
4. Go to **SMTP Settings → Create SMTP Credentials** — save the credentials
5. Request **Production Access** (sandbox only allows verified recipients)

### 5. Deploy

Upload from dev machine:

```bash
scp bin/bdsmail admin@<lightsail-ip>:/tmp/bdsmail
scp -r web admin@<lightsail-ip>:/tmp/web
scp -r scripts admin@<lightsail-ip>:/tmp/scripts
```

On the Lightsail instance:

```bash
sudo -i
cd /tmp
export BDS_DOMAINS="yourdomain.com"
export BDS_DB_TYPE=dynamodb
export BDS_DYNAMODB_REGION=us-east-1
bash /tmp/scripts/deploy.sh
```

### 6. Configure Systemd Service

The deploy script creates `/etc/systemd/system/bdsmail.service`. Update the `ExecStart` line with your settings:

```bash
ExecStart=/opt/bdsmail/bdsmail \
  --db_type=dynamodb \
  --dynamodb_region=us-east-1 \
  --bucket_type=s3 \
  --s3_bucket=your-bucket-name \
  --s3_region=us-east-1 \
  --secret_mode=aws \
  --smtp_port=25 \
  --pop3_port=110 \
  --imap_port=143 \
  --https_port=443 \
  --http_port=80 \
  --amplify_url=main.d1abc2def3.amplifyapp.com
```

Secrets (`admin_secret`, `relay_host`, `relay_user`, `relay_password`, `database_url`) are loaded from AWS Secrets Manager automatically when `--secret_mode=aws`.

Store them in Secrets Manager:
```bash
aws secretsmanager create-secret --name admin_secret --secret-string "your-admin-secret"
aws secretsmanager create-secret --name relay_host --secret-string "email-smtp.us-east-1.amazonaws.com"
aws secretsmanager create-secret --name relay_user --secret-string "your-ses-smtp-username"
aws secretsmanager create-secret --name relay_password --secret-string "your-ses-smtp-password"
```

```bash
sudo systemctl daemon-reload
sudo systemctl restart bdsmail
```

### 7. Create Users

```bash
cd /opt/bdsmail
./bdsmail -adduser you@yourdomain.com -password 'yourpassword' -displayname 'Your Name'
```

### Optional: Host Vue SPA on AWS Amplify

Instead of serving the Vue SPA from the Go binary, you can host it on Amplify for CDN-level performance:

```bash
npm install -g @aws-amplify/cli
cd web/vue && npm run build
amplify init && amplify add hosting && amplify publish
```

Configure Amplify rewrites: all paths → `/index.html`. Set API base URL to `https://mail.yourdomain.com/api`.

### Optional: S3 + CloudFront for SPA

```bash
aws s3 mb s3://bdsmail-spa-yourdomain
cd web/vue && npm run build
aws s3 sync dist/ s3://bdsmail-spa-yourdomain --delete
aws s3 website s3://bdsmail-spa-yourdomain \
    --index-document index.html --error-document index.html
```

Create a CloudFront distribution pointing to the S3 bucket. Set error page to return `index.html` with 200 status for SPA routing.

---

## Option 2: GCP + Firestore + SES

**Cost: ~$10.50/month** — GCP compute with serverless database.

| Component | Service | Cost |
|-----------|---------|------|
| Compute | GCP e2-micro | $6.50 |
| Database | Firestore (free tier) | $0.00 |
| Static IP | External IP | $3.00 |
| Outbound relay | Amazon SES | ~$1.00 |
| **Total** | | **~$10.50/month** |

### 1. Create GCP VM

```bash
export GCP_PROJECT_ID=your-project-id
export BDS_REGION=us-west1
export BDS_ZONE=us-west1-b
export BDS_VM_NAME=bdsmail

gcloud compute instances create $BDS_VM_NAME \
    --zone=$BDS_ZONE \
    --machine-type=e2-micro \
    --image-family=debian-12 \
    --image-project=debian-cloud \
    --tags=mail-server

# Reserve static IP
gcloud compute addresses create bdsmail-ip --region=$BDS_REGION
gcloud compute instances add-access-config $BDS_VM_NAME \
    --zone=$BDS_ZONE \
    --access-config-name="External NAT" \
    --address=$(gcloud compute addresses describe bdsmail-ip --region=$BDS_REGION --format='get(address)')
```

### 2. Open Firewall Ports

```bash
gcloud compute firewall-rules create allow-mail \
    --allow=tcp:25,tcp:80,tcp:110,tcp:143,tcp:443 \
    --target-tags=mail-server
```

### 3. Enable Firestore

```bash
gcloud firestore databases create --location=$BDS_REGION
```

The VM's default service account has Firestore access via ADC automatically.

### 4. Deploy

```bash
# Upload from dev machine
gcloud compute scp bin/bdsmail $BDS_VM_NAME:/tmp/bdsmail --zone=$BDS_ZONE
gcloud compute scp --recurse web $BDS_VM_NAME:/tmp/web --zone=$BDS_ZONE
gcloud compute scp --recurse scripts $BDS_VM_NAME:/tmp/scripts --zone=$BDS_ZONE

# On the VM
gcloud compute ssh $BDS_VM_NAME --zone=$BDS_ZONE
sudo -i && cd /tmp
export BDS_DOMAINS="yourdomain.com"
export BDS_DB_TYPE=firestore
export BDS_FIRESTORE_PROJECT=$GCP_PROJECT_ID
bash /tmp/scripts/deploy.sh
```

### 5. Configure `.env`

```bash
BDS_DOMAINS=yourdomain.com
BDS_DB_TYPE=firestore
BDS_FIRESTORE_PROJECT=your-project-id

BDS_RELAY_HOST=email-smtp.us-west-2.amazonaws.com
BDS_RELAY_PORT=587
BDS_RELAY_USER=your-ses-smtp-username
BDS_RELAY_PASSWORD=your-ses-smtp-password

# Optional: GCS for attachments
BDS_BUCKET_TYPE=gcs
BDS_GCS_BUCKET=your-project-id-mail
```

### Optional: Firebase Hosting for Vue SPA

```bash
npm install -g firebase-tools
cd web/vue && npm run build
firebase init hosting   # Select your GCP project, set public dir to "dist"
firebase deploy --only hosting
```

Free tier: 10GB/month transfer, custom domain support.

---

## Option 3: GCP or AWS with PostgreSQL

**Cost: ~$17-21/month** — Full SQL database for maximum query flexibility.

### GCP: Cloud SQL

| Component | Service | Cost |
|-----------|---------|------|
| Compute | GCP e2-micro | $6.50 |
| Database | Cloud SQL PostgreSQL (db-f1-micro) | $10.00 |
| Storage | Cloud Storage | ~$0.02 |
| Static IP | External IP | $3.00 |
| Outbound relay | Amazon SES | ~$1.00 |
| **Total** | | **~$20.50/month** |

#### Set up Cloud SQL

```bash
# Create PostgreSQL instance
gcloud sql instances create bdsmail-db \
    --database-version=POSTGRES_15 \
    --tier=db-f1-micro \
    --region=$BDS_REGION

# Create database and user
gcloud sql databases create bdsmail --instance=bdsmail-db
gcloud sql users create bdsmail --instance=bdsmail-db --password='your-secure-password'
```

#### Set up Cloud SQL Auth Proxy

```bash
# Install on VM
curl -o /usr/local/bin/cloud-sql-proxy \
    https://storage.googleapis.com/cloud-sql-connectors/cloud-sql-proxy/v2.15.2/cloud-sql-proxy.linux.amd64
chmod +x /usr/local/bin/cloud-sql-proxy

# Get connection name
gcloud sql instances describe bdsmail-db --format="get(connectionName)"

# Create systemd service
cat > /etc/systemd/system/cloud-sql-proxy.service << 'EOF'
[Unit]
Description=Cloud SQL Auth Proxy
After=network.target
[Service]
Type=simple
ExecStart=/usr/local/bin/cloud-sql-proxy YOUR_CONNECTION_NAME --address=0.0.0.0 --port=5432
Restart=on-failure
[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload && systemctl enable --now cloud-sql-proxy
```

#### `.env` configuration

```bash
BDS_DB_TYPE=postgres
DATABASE_URL=postgres://bdsmail:your-secure-password@localhost:5432/bdsmail?sslmode=disable
BDS_BUCKET_TYPE=gcs
BDS_GCS_BUCKET=your-project-id-mail
```

### AWS: RDS PostgreSQL

| Component | Service | Cost |
|-----------|---------|------|
| Compute | EC2 t4g.micro | $6.13 |
| Database | RDS db.t4g.micro PostgreSQL | $11.50 |
| Static IP | Elastic IP | $3.65 |
| Outbound relay | Amazon SES | ~$1.00 |
| **Total** | | **~$22.30/month** |

#### Set up RDS

```bash
aws rds create-db-instance \
    --db-instance-identifier bdsmail-db \
    --db-instance-class db.t4g.micro \
    --engine postgres \
    --engine-version 15 \
    --master-username bdsmail \
    --master-user-password 'your-secure-password' \
    --allocated-storage 20

# Get endpoint
aws rds describe-db-instances --db-instance-identifier bdsmail-db \
    --query 'DBInstances[0].Endpoint.Address' --output text
```

#### `.env` configuration

```bash
BDS_DB_TYPE=postgres
DATABASE_URL=postgres://bdsmail:your-secure-password@your-rds-endpoint:5432/bdsmail?sslmode=require
BDS_BUCKET_TYPE=s3
BDS_S3_REGION=us-east-1
BDS_S3_BUCKET=your-bucket-name
```

---

## Option 4: AWS Self-Hosted PostgreSQL + SES + S3 (Recommended Startup)

**Cost: ~$5-6/month** — Full SQL, zero managed DB cost, proven migration path to RDS when demand grows.

Everything runs on a single Lightsail instance: app + PostgreSQL + backups to S3.

| Component | Service | Cost |
|-----------|---------|------|
| Compute + DB + IP | Lightsail $5 plan (app + PostgreSQL) | $5.00 |
| Attachments | S3 (5GB free tier) | ~$0.02 |
| Outbound relay | Amazon SES | ~$0.10/1K emails |
| Backups | S3 (pg_dump gzip) | ~$0.02 |
| **Total** | | **~$5-6/month** |

### 1. Create Lightsail Instance

```bash
aws lightsail create-instances \
    --instance-names bdsmail \
    --availability-zone us-east-1a \
    --blueprint-id debian_12 \
    --bundle-id nano_3_2

aws lightsail allocate-static-ip --static-ip-name bdsmail-ip
aws lightsail attach-static-ip --static-ip-name bdsmail-ip --instance-name bdsmail

for PORT in 25 80 110 143 443; do
    aws lightsail open-instance-public-ports \
        --instance-name bdsmail \
        --port-info fromPort=$PORT,toPort=$PORT,protocol=tcp
done
```

### 2. Install PostgreSQL

```bash
ssh admin@<lightsail-ip>
sudo -i

apt-get update && apt-get install -y postgresql postgresql-client
systemctl enable postgresql && systemctl start postgresql

sudo -u postgres psql << 'SQL'
CREATE USER bdsmail WITH PASSWORD 'your-secure-password';
CREATE DATABASE bdsmail OWNER bdsmail;
SQL

echo "local bdsmail bdsmail md5" >> /etc/postgresql/*/main/pg_hba.conf
systemctl reload postgresql
```

### 3. Initialize Database

```bash
# From dev machine — upload SQL and initialize
scp sql/bdsmail_pgsql.sql admin@<lightsail-ip>:/opt/bdsmail/sql/

# On the server
psql -U bdsmail -d bdsmail -f /opt/bdsmail/sql/bdsmail_pgsql.sql
```

### 4. Configure Backup to S3

The backup script (`scripts/backup.sh`) dumps PostgreSQL daily, uploads to S3, rotates local (7 days) and remote (30 days).

```bash
# From dev machine — upload scripts
scp -r scripts admin@<lightsail-ip>:/opt/bdsmail/scripts/

# On the server — set up daily cron at 3 AM
echo "0 3 * * * /opt/bdsmail/scripts/backup.sh your-bdsmail-backup-bucket >> /var/log/bdsmail-backup.log 2>&1" | crontab -

# Test backup manually
/opt/bdsmail/scripts/backup.sh your-bdsmail-backup-bucket
```

### 5. Build and Deploy bdsmail

From your dev machine:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/bdsmail ./cmd/bdsmail/

scp bin/bdsmail admin@<lightsail-ip>:/opt/bdsmail/bin/
scp -r web/templates admin@<lightsail-ip>:/opt/bdsmail/web/templates/
scp -r web/static admin@<lightsail-ip>:/opt/bdsmail/web/static/
```

### 6. Deploy Vue Frontend

**Option A: Same instance (embedded)**

```bash
cd web/vue && npm install && npm run build
scp -r web/vue/dist admin@<lightsail-ip>:/opt/bdsmail/web/vue/dist/
```

Go server serves the Vue SPA at `/app/` automatically.

**Option B: AWS Amplify (CDN)**

```bash
cd web/vue && npm install && npm run build
npm install -g @aws-amplify/cli
amplify init && amplify add hosting && amplify publish
```

Each domain gets `webmail.domain.com` CNAME → Amplify URL. Add `--amplify_url` flag to systemd service.

### 7. Create Systemd Service

```bash
cat > /etc/systemd/system/bdsmail.service << 'SERVICE'
[Unit]
Description=BDS Mail Server
After=network.target postgresql.service

[Service]
Type=simple
User=root
WorkingDirectory=/opt/bdsmail
AmbientCapabilities=CAP_NET_BIND_SERVICE
ExecStart=/opt/bdsmail/bin/bdsmail \
  --db_type=postgres \
  --smtp_port=25 \
  --pop3_port=110 \
  --imap_port=143 \
  --https_port=443 \
  --http_port=80 \
  --dkim_key_dir=/opt/bdsmail/dkim \
  --acme_webroot=/opt/bdsmail/acme \
  --bucket_type=s3 \
  --s3_bucket=your-bucket-name \
  --s3_region=us-east-1 \
  --secret_mode=local \
  --keystore=/opt/bdsmail/sec/secrets.json
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
SERVICE

# Create secrets file
mkdir -p /opt/bdsmail/sec
cat > /opt/bdsmail/sec/secrets.json << 'SECRETS'
{
  "database_url": "postgres://bdsmail:your-secure-password@localhost:5432/bdsmail?sslmode=disable",
  "admin_secret": "your-admin-secret",
  "relay_host": "email-smtp.us-east-1.amazonaws.com",
  "relay_user": "your-ses-smtp-username",
  "relay_password": "your-ses-smtp-password"
}
SECRETS
chmod 600 /opt/bdsmail/sec/secrets.json

systemctl daemon-reload
systemctl enable bdsmail
systemctl start bdsmail
```

### Migration to RDS (when demand grows)

When you need managed PostgreSQL, migrate in 15 minutes:

```bash
# Dump from self-hosted
sudo -u postgres pg_dump -Fc bdsmail > bdsmail.dump

# Create RDS instance
aws rds create-db-instance \
    --db-instance-identifier bdsmail-db \
    --db-instance-class db.t4g.micro \
    --engine postgres --allocated-storage 20 \
    --master-username bdsmail --master-user-password 'password'

# Restore to RDS
pg_restore -h <rds-endpoint> -U bdsmail -d bdsmail bdsmail.dump

# Update secrets.json with new database_url, restart service
```

---

## Database Initialization

Database tables must be created before starting the application. DDL scripts are in the `sql/` folder.

### PostgreSQL

```bash
psql "postgres://bdsmail:password@localhost:5432/bdsmail" -f sql/bdsmail_pgsql.sql
```

### SQLite

```bash
sqlite3 /opt/bdsmail/bdsmail.db < sql/bdsmail_sqlite.sql
```

### DynamoDB

```bash
go run internal/store/init_dynamodb.go --region us-east-1
```

### Firestore

Collections are created automatically on first write. Create the database and indexes:

```bash
# Create database (one-time)
gcloud firestore databases create --location=us-west1

# Create composite indexes (see sql/bdsmail_firestore.md for full list)
gcloud firestore indexes composite create \
  --collection-group=bdsmail-messages \
  --field-config field-path=owner_user,order=ASCENDING \
  --field-config field-path=folder,order=ASCENDING \
  --field-config field-path=deleted,order=ASCENDING \
  --field-config field-path=received_at,order=DESCENDING
```

All scripts are idempotent and safe to re-run.

---

## Post-Deployment Checklist

1. Initialize database (see above)
2. Add DNS records (A, MX, SPF, DMARC) for each domain
2. Add DKIM record printed by deploy script
3. Create users: `./bdsmail -adduser user@domain.com -password 'pass' -displayname 'Name'`
4. Test web UI: `https://mail.yourdomain.com`
5. Test sending: compose to Gmail, check DKIM passes in "Show original"
6. Test receiving: send from Gmail to `user@yourdomain.com`
7. Check deliverability: [mail-tester.com](https://www.mail-tester.com)

---

## Configuration Reference

All configuration via CLI flags. Secrets loaded from SecretProvider (`--secret_mode=local|aws|gsm`):

| Variable | Description | Default |
|----------|-------------|---------|
| `BDS_DOMAINS` | Comma-separated domains | `mydomain.com` |
| `BDS_SMTP_PORT` | SMTP port | `2525` |
| `BDS_POP3_PORT` | POP3 port | `1100` |
| `BDS_IMAP_PORT` | IMAP port | `1430` |
| `BDS_HTTPS_PORT` | HTTPS port | `8443` |
| `BDS_HTTP_PORT` | HTTP port (ACME) | `8080` |
| `--ssl_dir` | Per-domain SSL certificate directory | `/opt/bdsmail/ssl` |
| `BDS_DB_TYPE` | `postgres`, `sqlite`, `dynamodb`, `firestore` | `postgres` |
| `DATABASE_URL` | PostgreSQL connection string | (none) |
| `BDS_SQLITE_PATH` | SQLite file path | `/opt/bdsmail/bdsmail.db` |
| `BDS_DYNAMODB_REGION` | AWS region for DynamoDB | `us-west-2` |
| `BDS_FIRESTORE_PROJECT` | GCP project for Firestore | (none) |
| `BDS_BUCKET_TYPE` | `gcs`, `s3`, or empty | (none) |
| `BDS_GCS_BUCKET` | GCS bucket name | (none) |
| `BDS_S3_REGION` | S3 region | `us-west-2` |
| `BDS_S3_BUCKET` | S3 bucket name | (none) |
| `BDS_RELAY_HOST` | SMTP relay host | (none — direct MX) |
| `BDS_RELAY_PORT` | Relay port | `587` |
| `BDS_RELAY_USER` | Relay username | (none) |
| `BDS_RELAY_PASSWORD` | Relay password | (none) |
| `BDS_DKIM_KEY_DIR` | DKIM keys directory | (none) |
| `BDS_ADMIN_SECRET` | Admin web UI secret | (none) |
| `BDS_CLAMAV_ENABLED` | ClamAV toggle | `false` |
| `BDS_RSPAMD_ENABLED` | Rspamd toggle | `true` |
| `BDS_SAFEBROWSING_ENABLED` | Safe Browsing toggle | `false` |
| `BDS_RATELIMIT_ENABLED` | Rate limiting toggle | `true` |
| `BDS_MAX_ATTACHMENT_BYTES` | Max attachment size | `10485760` (10MB) |

---

## Troubleshooting

```bash
# Service status
systemctl status bdsmail
journalctl -u bdsmail -f

# Certificate renewal
systemctl list-timers | grep certbot

# SMTP connectivity
telnet mail.yourdomain.com 25

# TLS check
openssl s_client -connect mail.yourdomain.com:25 -starttls smtp

# DNS records
dig MX yourdomain.com
dig A mail.yourdomain.com
dig TXT default._domainkey.yourdomain.com

# Redeploy (build + upload + restart)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/bdsmail ./cmd/bdsmail/
scp bin/bdsmail admin@<ip>:/tmp/bdsmail
# On server:
systemctl stop bdsmail
cp /tmp/bdsmail /opt/bdsmail/bdsmail && chmod +x /opt/bdsmail/bdsmail
systemctl start bdsmail
```

---

## Reference Architecture: Lightsail + Amplify + DynamoDB + S3 + SES

This is the recommended multi-domain deployment. One Lightsail instance serves all domains. One Amplify app serves the webmail UI for all domains. Each domain gets its own DNS records, SES verification, DKIM keys, and OAuth API keys.

### Architecture

```mermaid
graph TD
    DNS["DNS Provider<br/>(Route 53, GoDaddy, etc.)"]

    DNS -->|"A record<br/>mail.domain.com"| Lightsail
    DNS -->|"CNAME<br/>webmail.domain.com"| Amplify
    DNS -->|"MX record<br/>→ mail.domain.com"| Lightsail

    Amplify["AWS Amplify<br/>Vue 3 SPA<br/>(static)"]
    Amplify -->|"calls mail.domain.com/api"| Lightsail

    Lightsail["AWS Lightsail<br/>SMTP :25 · POP3 :110<br/>IMAP :143 · HTTPS :443<br/>/api/* · /oauth/*"]

    Lightsail --> DynamoDB[(DynamoDB<br/>data)]
    Lightsail --> S3[(S3<br/>attachments)]
    Lightsail --> SES["SES<br/>outbound email"]
```

### What Each Domain Gets

| Component | Per Domain | Shared |
|-----------|-----------|--------|
| **Backend** | `mail.domain.com` → A record to Lightsail IP | Single Lightsail instance |
| **Frontend** | `webmail.domain.com` → CNAME to Amplify URL | Single Amplify deployment |
| **Email** | MX record → `mail.domain.com` | Single SMTP server |
| **DKIM** | Unique RSA key pair in `dkim/{domain}.pem` | Same server signs |
| **SES** | Separate domain verification + DKIM tokens | Same SES account |
| **OAuth** | Per-domain API key + OAuth clients | Same OIDC provider |
| **Storage** | S3 prefix `{domain}/` in shared bucket | Single S3 bucket |
| **Database** | Data isolated by `owner_user` (email includes domain) | Single DynamoDB |

### Initial Platform Setup (One-Time)

#### 1. Deploy Lightsail Backend

Follow [Option 1: AWS Lightsail + DynamoDB + SES](#option-1-aws-lightsail--dynamodb--ses) above. This creates the shared backend.

Note the Lightsail **static IP** — all domains will point here.

#### 2. Deploy Amplify Frontend

```bash
# Build Vue SPA
cd web/vue && npm install && npm run build

# Install Amplify CLI and init
npm install -g @aws-amplify/cli
amplify init    # Choose: JavaScript, Vue, dist, npm run build
amplify add hosting    # Choose: Amplify Console, Manual deployment
amplify publish
```

Note the Amplify **app URL** (e.g. `main.d1abc2def3.amplifyapp.com`).

#### 3. Configure Backend for Amplify

Add to `/opt/bdsmail/.env` on Lightsail:

```bash
BDS_AMPLIFY_URL=main.d1abc2def3.amplifyapp.com
```

Restart: `sudo systemctl restart bdsmail`

This enables:
- CORS headers for `webmail.{domain}` origins on `/api/*`
- Webmail CNAME record in domain onboarding output
- Vue SPA auto-detects backend from `webmail.{domain}` → `mail.{domain}/api`

---

### Domain Onboarding Process

Adding a new domain is a single admin action. The server handles everything automatically.

#### Step 1: Add Domain via Admin Panel

**Option A: Web Admin UI**

Go to `https://mail.firstdomain.com/admin/domains`, enter admin secret, type the new domain, click **Add Domain**.

**Option B: CLI**

```bash
cd /opt/bdsmail
./bdsmail -adddomain newdomain.com
```

**Option C: JSON API**

```bash
curl -X POST https://mail.firstdomain.com/admin/api/domains \
  -H "Authorization: Bearer YOUR_ADMIN_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"domain": "newdomain.com"}'
```

#### What Happens Automatically

When you add a domain, the server performs these steps in sequence:

| Step | Action | Result |
|------|--------|--------|
| 1 | **Validate** | Checks domain format, not already registered |
| 2 | **Generate DKIM key** | Creates 2048-bit RSA key at `dkim/newdomain.com.pem` |
| 3 | **Verify in SES** | Calls AWS SES API: `VerifyDomainIdentity` + `VerifyDomainDkim` |
| 4 | **Generate API key** | Creates 64-char hex key for this domain |
| 5 | **Add to config** | Domain added to running server (no restart) |
| 6 | **Issue TLS cert** | Runs `certbot certonly -d mail.newdomain.com` per domain, copies to `/opt/bdsmail/ssl/<domain>/` |
| 7 | **Persist to .env** | Updates `BDS_DOMAINS=` in env file |
| 8 | **Return DNS records** | Full list of records the user needs to add |

#### Step 2: Add DNS Records

The server returns all required DNS records. Add them to your DNS provider:

| Type | Name | Value | Purpose |
|------|------|-------|---------|
| **A** | `mail` | `<lightsail-ip>` | Backend server |
| **CNAME** | `webmail` | `main.d1abc2def3.amplifyapp.com` | Frontend (Amplify) |
| **MX** | `@` | `mail.newdomain.com` (priority 10) | Inbound email routing |
| **TXT** | `@` | `v=spf1 a mx ~all` | SPF sender authorization |
| **TXT** | `default._domainkey` | `v=DKIM1; k=rsa; p=<key>` | bdsmail DKIM signing |
| **TXT** | `_dmarc` | `v=DMARC1; p=none; ...` | DMARC policy |
| **CNAME** | `<token1>._domainkey` | `<token1>.dkim.amazonses.com` | SES DKIM (1 of 3) |
| **CNAME** | `<token2>._domainkey` | `<token2>.dkim.amazonses.com` | SES DKIM (2 of 3) |
| **CNAME** | `<token3>._domainkey` | `<token3>.dkim.amazonses.com` | SES DKIM (3 of 3) |

The A record and CNAME for webmail are the same for every domain (shared infrastructure). The rest are domain-specific.

#### Step 3: Wait for Verification

- **DNS propagation**: 5-60 minutes
- **SES verification**: 5-15 minutes after DNS propagates
- **TLS certificate**: Immediate (certbot auto-obtains)

Check status:

```bash
# DNS
dig A mail.newdomain.com
dig MX newdomain.com

# SES (via admin panel or API — shows Pending/Success)
curl https://mail.firstdomain.com/admin/api/domains \
  -H "Authorization: Bearer YOUR_ADMIN_SECRET"
```

#### Step 4: Create Users

```bash
./bdsmail -adduser alice@newdomain.com -password 'password' -displayname 'Alice'
./bdsmail -adduser bob@newdomain.com -password 'password' -displayname 'Bob'
```

#### Step 5: Test

- **Web UI**: `https://mail.newdomain.com` (Go templates) or `https://webmail.newdomain.com` (Vue SPA)
- **Send test**: Compose from webmail to Gmail, verify DKIM passes
- **Receive test**: Send from Gmail to `alice@newdomain.com`
- **IMAP**: Configure Thunderbird with `mail.newdomain.com:143`

#### Step 6: Register OAuth Apps (Optional)

Users of `newdomain.com` can register their own OAuth apps at the Developer Portal:
- Go templates: `https://mail.newdomain.com/developer`
- Vue SPA: `https://webmail.newdomain.com/developer`

This enables "Sign in with newdomain.com" for their applications.

---

### Adding Many Domains

For bulk onboarding, use the JSON API in a script:

```bash
#!/bin/bash
ADMIN_SECRET="your-admin-secret"
SERVER="https://mail.firstdomain.com"

for DOMAIN in domain1.com domain2.com domain3.com; do
  echo "=== Onboarding $DOMAIN ==="
  curl -s -X POST "$SERVER/admin/api/domains" \
    -H "Authorization: Bearer $ADMIN_SECRET" \
    -H "Content-Type: application/json" \
    -d "{\"domain\": \"$DOMAIN\"}" | python3 -m json.tool
  echo ""
done
```

The output includes all DNS records for each domain. Add them to your DNS provider in bulk.

---

### Cost Per Additional Domain

| Resource | Additional Cost |
|----------|----------------|
| Lightsail | $0 (shared instance) |
| DynamoDB | $0 (shared tables, free tier) |
| S3 | $0 (shared bucket, pennies per GB) |
| Amplify | $0 (shared deployment, free tier) |
| SES | $0.10/1K emails (per domain usage) |
| DNS | $0.50/zone/month (Route 53) or free (GoDaddy, Cloudflare) |
| TLS | $0 (Let's Encrypt, auto-renewed) |
| **Total per domain** | **~$0-0.50/month + email volume** |
