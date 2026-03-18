# Deployment Options & Cost Analysis

Cost estimates for running bdsmail with ~10,000 emails/month, light usage.

## Compute Options

| Option | Specs | Monthly Cost | Notes |
|--------|-------|-------------|-------|
| GCP e2-micro | Shared, 1GB RAM | ~$6.50 | Current setup |
| AWS EC2 t4g.micro | 2 vCPU, 1GB ARM | ~$6.13 | Similar to GCP |
| AWS Lightsail | 1 vCPU, 1GB, static IP included | $5.00 | Simplest AWS option |

## Database Options

| Option | Monthly Cost | Pros | Cons |
|--------|-------------|------|------|
| GCP Cloud SQL (PostgreSQL) | ~$10.00 | Full SQL, managed backups | Most expensive for small workloads |
| AWS RDS (PostgreSQL) | ~$11.50 | Full SQL, managed backups | Similar cost to Cloud SQL |
| SQLite (on VM disk) | $0 | Zero config, no network dependency | Single-server only, no managed backups |
| AWS DynamoDB | $0 (always-free tier) | Managed, scalable, 25GB free | AWS only, NoSQL (different query model) |
| GCP Firestore | $0 (free tier) | Managed, real-time sync, 1GB free | GCP only, NoSQL, free tier has daily limits |

## Object Storage Options (for Attachments)

Mail bodies are stored in the database. Object storage is used for file attachments only.

| Option | Monthly Cost (< 1GB) | Config | Notes |
|--------|---------------------|--------|-------|
| GCP Cloud Storage | ~$0.02 | `BDS_BUCKET_TYPE=gcs` | Native to GCP |
| AWS S3 | ~$0.02 | `BDS_BUCKET_TYPE=s3` | Native to AWS |
| None (disabled) | $0 | `BDS_BUCKET_TYPE=` | No attachment support; inbound attachments dropped |

## Email Relay Options (Outbound)

GCP blocks outbound port 25. A relay service is required to send email to external recipients.

| Service | Cost for 10K emails/mo | Free Tier | Notes |
|---------|----------------------|-----------|-------|
| Amazon SES | $1.00 | 3K/mo for 12 months | Cheapest. $0.10 per 1K emails. |
| Mailgun | $15.00 | None at this volume | Basic plan, 10K included |
| SendGrid | $19.95 | 100/day forever | Essentials plan, 50K included |

## Full Stack Comparisons

### Option 1: Current GCP Setup
| Component | Service | Cost |
|-----------|---------|------|
| Compute | GCP e2-micro | $6.50 |
| Database | Cloud SQL PostgreSQL | $10.00 |
| Storage | Cloud Storage | $0.02 |
| Static IP | External IP | $3.00 |
| Relay | Amazon SES | $1.00 |
| **Total** | | **~$20.50/mo** |

### Option 2: GCP Minimal (SQLite + body in DB)
| Component | Service | Cost |
|-----------|---------|------|
| Compute | GCP e2-micro | $6.50 |
| Database | SQLite (on disk) | $0 |
| Storage | Body in DB | $0 |
| Static IP | External IP | $3.00 |
| Relay | Amazon SES | $1.00 |
| **Total** | | **~$10.50/mo** |

### Option 3: GCP with Firestore
| Component | Service | Cost |
|-----------|---------|------|
| Compute | GCP e2-micro | $6.50 |
| Database | Firestore (free tier) | $0 |
| Storage | Body in DB | $0 |
| Static IP | External IP | $3.00 |
| Relay | Amazon SES | $1.00 |
| **Total** | | **~$10.50/mo** |

### Option 4: AWS Lightsail (cheapest overall)
| Component | Service | Cost |
|-----------|---------|------|
| Compute + IP | Lightsail $5 plan | $5.00 |
| Database | SQLite or DynamoDB | $0 |
| Storage | Body in DB | $0 |
| Relay | Amazon SES | $1.00 |
| **Total** | | **~$6.00/mo** |

### Option 5: AWS EC2 + DynamoDB
| Component | Service | Cost |
|-----------|---------|------|
| Compute | EC2 t4g.micro | $6.13 |
| Database | DynamoDB (free tier) | $0 |
| Storage | Body in DB | $0 |
| Static IP | Elastic IP | $3.65 |
| Relay | Amazon SES | $1.00 |
| **Total** | | **~$10.80/mo** |

### Option 6: GCP Cloud Run (not recommended)
| Component | Service | Cost |
|-----------|---------|------|
| Compute | Cloud Run (always-on) | $45.00 |
| Database | Cloud SQL or Firestore | $0-10 |
| Storage | Body in DB | $0 |
| Relay | Amazon SES | $1.00 |
| **Total** | | **~$46-56/mo** |

> Cloud Run is not a good fit for a mail server. It's designed for HTTP workloads, not persistent TCP servers (SMTP/IMAP/POP3). Always-on instances are required, making it 5-8x more expensive than a VM.

## Supported Database Backends

Set `BDS_DB_TYPE` in `.env`:

| Value | Backend | Config Required |
|-------|---------|----------------|
| `postgres` | PostgreSQL (default) | `DATABASE_URL` |
| `sqlite` | SQLite file | `BDS_SQLITE_PATH` |
| `dynamodb` | AWS DynamoDB | `BDS_DYNAMODB_REGION` |
| `firestore` | GCP Firestore | `BDS_FIRESTORE_PROJECT` |
