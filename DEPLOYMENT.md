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

### 6. Configure `.env`

Edit `/opt/bdsmail/.env`:

```bash
BDS_DOMAINS=yourdomain.com
BDS_DB_TYPE=dynamodb
BDS_DYNAMODB_REGION=us-east-1

BDS_RELAY_HOST=email-smtp.us-east-1.amazonaws.com
BDS_RELAY_PORT=587
BDS_RELAY_USER=your-ses-smtp-username
BDS_RELAY_PASSWORD=your-ses-smtp-password

# Optional: S3 for attachments
BDS_BUCKET_TYPE=s3
BDS_S3_REGION=us-east-1
BDS_S3_BUCKET=your-bucket-name

BDS_SMTP_PORT=25
BDS_POP3_PORT=110
BDS_IMAP_PORT=143
BDS_HTTPS_PORT=443
BDS_HTTP_PORT=80
```

```bash
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

## Option 4: Single VM with SQLite (Simplest)

**Cost: ~$5-10/month** — Zero external dependencies, everything on one server.

Works on any Linux VPS (Lightsail, DigitalOcean, Linode, Hetzner, etc.).

```bash
BDS_DB_TYPE=sqlite
BDS_SQLITE_PATH=/opt/bdsmail/bdsmail.db
```

No database setup needed. The SQLite file is created automatically on first startup. Back up by copying the `.db` file.

---

## Post-Deployment Checklist

1. Add DNS records (A, MX, SPF, DMARC) for each domain
2. Add DKIM record printed by deploy script
3. Create users: `./bdsmail -adduser user@domain.com -password 'pass' -displayname 'Name'`
4. Test web UI: `https://mail.yourdomain.com`
5. Test sending: compose to Gmail, check DKIM passes in "Show original"
6. Test receiving: send from Gmail to `user@yourdomain.com`
7. Check deliverability: [mail-tester.com](https://www.mail-tester.com)

---

## Configuration Reference

All configuration via `.env` file at `/opt/bdsmail/.env`:

| Variable | Description | Default |
|----------|-------------|---------|
| `BDS_DOMAINS` | Comma-separated domains | `mydomain.com` |
| `BDS_SMTP_PORT` | SMTP port | `2525` |
| `BDS_POP3_PORT` | POP3 port | `1100` |
| `BDS_IMAP_PORT` | IMAP port | `1430` |
| `BDS_HTTPS_PORT` | HTTPS port | `8443` |
| `BDS_HTTP_PORT` | HTTP port (ACME) | `8080` |
| `BDS_TLS_CERT` | TLS certificate path | (none) |
| `BDS_TLS_KEY` | TLS key path | (none) |
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

```
                    ┌─────────────────────────────┐
                    │         DNS Provider         │
                    │  (Route 53, GoDaddy, etc.)   │
                    └─────────┬───────────────────┘
                              │
              ┌───────────────┼───────────────┐
              │               │               │
    mail.domain.com    webmail.domain.com    MX record
    (A → Lightsail IP)  (CNAME → Amplify)   (→ mail.domain.com)
              │               │
              ▼               ▼
    ┌─────────────────┐  ┌──────────────┐
    │   AWS Lightsail  │  │ AWS Amplify  │
    │                  │  │              │
    │  SMTP :25        │  │  Vue 3 SPA   │
    │  POP3 :110       │  │  (static)    │
    │  IMAP :143       │◄─┤              │
    │  HTTPS :443      │  │  Calls       │
    │   /api/*         │  │  mail.X/api  │
    │   /oauth/*       │  └──────────────┘
    │                  │
    └────┬────┬───┬────┘
         │    │   │
         ▼    ▼   ▼
    DynamoDB  S3  SES
    (data)  (att) (outbound)
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
| 6 | **Expand TLS cert** | Runs `certbot --expand` to add `mail.newdomain.com` |
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
