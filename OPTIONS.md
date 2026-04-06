# Options & Alternatives

Design decisions, alternatives considered, and current implementation choices for bdsmail.

## Deployment Options

See [DEPLOYMENT.md](DEPLOYMENT.md) for full step-by-step instructions.

| Option | Stack | Monthly Cost | Best For |
|--------|-------|-------------|----------|
| **AWS Self-Hosted PG + SES + S3** | Lightsail $5, self-hosted PostgreSQL, S3, SES | **~$5-6** | Recommended startup |
| **AWS Lightsail + DynamoDB + SES** | Lightsail $5, DynamoDB free, S3, SES | **~$6** | Managed DB, no search |
| **GCP + Firestore + SES** | e2-micro, Firestore free, GCS, SES | **~$10.50** | GCP ecosystem |
| **GCP + Cloud SQL** | e2-micro, managed PostgreSQL, GCS, SES | **~$20.50** | Managed PG on GCP |
| **AWS EC2 + RDS** | t4g.micro, managed PostgreSQL, S3, SES | **~$22.30** | Managed PG on AWS |

### Recommended: Self-Hosted PostgreSQL on Lightsail

Single Lightsail instance runs both the app and PostgreSQL. Full SQL, full-text search, zero managed DB cost. Daily pg_dump backups to S3 with 7-day local / 30-day remote rotation.

**Migration path**: When demand grows, `pg_dump` → `pg_restore` to RDS in 15 minutes. Same connection string change, zero code changes.

### Lightsail vs EC2

| Aspect | Lightsail $5/mo | EC2 t4g.micro |
|--------|----------------|---------------|
| CPU/RAM | 2 vCPU, 1GB | 2 vCPU, 1GB (burstable) |
| Storage | 40GB SSD included | EBS separate (~$0.80/10GB) |
| Transfer | 2TB included | 100GB free, then $0.09/GB |
| Static IP | Free | $3.65/mo (Elastic IP) |
| Long-term | $5/mo flat forever | $0 for 12 months, then ~$10+/mo |

Lightsail is simpler and cheaper long-term for a mail server.

---

## Database Backends

| Backend | Cost | Search | Managed | Migration to RDS |
|---------|------|--------|---------|-----------------|
| **Self-hosted PostgreSQL** | $0 | Full (ILIKE, tsvector) | No | pg_dump → pg_restore |
| **RDS PostgreSQL** | $12-15/mo | Full | Yes | Already there |
| **DynamoDB** | $0 (free tier) | Scan only | Yes | Schema rewrite needed |
| **Firestore** | $0 (free tier) | Client-side | Yes | Schema rewrite needed |

### DynamoDB Performance by Operation

| Operation | Approach | Performance | Concern |
|-----------|----------|-------------|---------|
| Get message by ID | GSI lookup | Fast | None |
| List messages by folder | Query partition + filter | Fast | None |
| Save/delete message | PutItem/DeleteItem | Fast | None |
| Count unread | Query + client count | Moderate | Reads all messages |
| **Search messages** | **Full scan + client filter** | **Slow** | **Degrades with mailbox size** |

### Recommendation

Start with **self-hosted PostgreSQL** (`--db_type=postgres`). Full SQL, full search, zero cost, proven migration path to RDS. DynamoDB is viable if you don't need search.

**Avoid cross-provider databases** (Neon, Supabase, PlanetScale) — egress cost (~$0.09/GB) and latency (10-50ms) when hosted outside AWS.

---

## TLS / SSL Certificate Management

Per-domain certificates with SNI (Server Name Indication). Each domain gets its own cert — independent issuance, renewal, and failure isolation.

### How It Works

```
/opt/bdsmail/ssl/
├── domain1.com/
│   ├── fullchain.pem
│   └── privkey.pem
├── domain2.com/
│   ├── fullchain.pem
│   └── privkey.pem
```

- **Startup**: `CertStore.LoadAll()` scans `ssl/` directory, loads all domain certs
- **TLS handshake**: `GetCertificate(SNI)` maps `mail.domain.com` → `domain.com` cert
- **New domain**: `certbot certonly -d mail.<domain>` → copy to `ssl/<domain>/` → `LoadDomain()` hot-loads
- **Renewal**: Daily cron runs `scripts/renew_certs.sh` → certbot renews → copies to ssl dir → reload
- **Failure**: One domain's cert failure doesn't affect others
- **Minimum TLS version**: 1.2

### Why Not Single Cert with --expand?

| | Single cert (old) | Per-domain SNI (current) |
|---|---|---|
| Adding a domain | Re-issues entire cert | Only new domain |
| One domain fails DNS | Blocks all domains | Only that domain |
| Renewal | All or nothing | Independent |
| Cert limit | 100 SANs max | Unlimited |
| Downtime risk | Brief risk during reissue | Zero |

---

## Object Storage

Attachments stored in cloud object storage via `github.com/mustafa-karli/basis/service/storage`.

| Backend | Cost | Config |
|---------|------|--------|
| S3 | ~$0.02/mo | `--bucket_type=s3 --s3_bucket=<name>` |
| GCS | ~$0.02/mo | `--bucket_type=gcs --gcs_bucket=<name>` |
| None | $0 | Attachments dropped |

Attachments are stored in a normalized `mail_attachment` table (3NF) with bucket keys referencing S3/GCS objects. No JSON blobs in the message table.

---

## Outbound Relay

| Service | Cost (10K emails/mo) | Notes |
|---------|---------------------|-------|
| Amazon SES | ~$1.00 | Cheapest. Auto-verified during domain onboarding |
| Mailgun | $15.00 | Basic plan |
| SendGrid | $19.95 | Essentials plan |

SES domain verification + DKIM tokens are issued automatically when adding a domain (if relay is SES).

---

## Frontend

### Dual Interface

| | Go Templates (`/`) | Vue 3 SPA (`/app/`) |
|---|---|---|
| Rendering | Server-side | Client-side |
| Dependencies | Zero | Vue + Router + Pinia + Axios (~49KB gzip) |
| Build | None | `npm run build` |
| 2FA pages | verify_2fa.html, setup_2fa.html | Verify2FAView.vue, Setup2FAView.vue |
| Deploy | Embedded in binary | Embedded or Amplify/S3 |

### SPA Hosting

| Option | Cost | Notes |
|--------|------|-------|
| Same instance (embedded) | $0 | Served at `/app/` by Go binary |
| AWS Amplify | $0 | Free tier. `webmail.<domain>` CNAME → Amplify |
| S3 + CloudFront | ~$0.50 | CDN. SPA detects backend from hostname |

Vue SPA auto-detects backend: `webmail.domain.com` → calls `mail.domain.com/api`.

---

## Authentication & Security

### Password + 2FA

| Feature | Implementation |
|---------|---------------|
| Password hashing | bcrypt (DefaultCost) |
| 2FA | TOTP via `github.com/pquerna/otp` (same lib as basis) |
| Backup codes | 10 one-time codes, bcrypt-hashed, pipe-separated |
| Trusted devices | 30-day bypass via device fingerprint |
| Login flow | Password → if 2FA enabled + untrusted device → login token → /verify-2fa → session |
| Brute-force | Per-IP rate limiting + login attempt counter with lockout |

### OAuth 2.0 / OIDC Identity Provider

"Sign in with yourdomain.com" — built-in OIDC provider with self-service developer portal.

| Endpoint | Purpose |
|----------|---------|
| `/oauth/authorize` | Consent screen |
| `/oauth/token` | Code → access_token + JWT id_token |
| `/oauth/userinfo` | Bearer token → user profile |
| `/oauth/jwks` | RSA public keys for JWT verification |
| `/.well-known/openid-configuration` | OIDC discovery |
| `/developer` | Self-service app registration |

OAuth clients are linked to **domains** (not users). JWT claims: `iss`, `sub`, `aud`, `email`, `name`, `domain`, `exp`, `iat`, `nonce`.

---

## Configuration & Secrets

### CLI Flags (not .env)

No `.env` file — all config via CLI flags passed to the binary or systemd `ExecStart`:

```bash
bdsmail \
  --db_type=postgres \
  --ssl_dir=/opt/bdsmail/ssl \
  --bucket_type=s3 \
  --s3_bucket=bdsmail-attachments \
  --secret_mode=local \
  --keystore=/opt/bdsmail/sec/secrets.json \
  --smtp_port=25 \
  --https_port=443
```

### Secrets Management

Via `github.com/mustafa-karli/basis/service/secret`:

| Mode | Flag | Storage |
|------|------|---------|
| Local JSON | `--secret_mode=local --keystore=/path/secrets.json` | File on disk |
| AWS Secrets Manager | `--secret_mode=aws` | AWS cloud |
| GCP Secret Manager | `--secret_mode=gsm --gcp_project_id=<id>` | GCP cloud |

Secrets loaded at startup: `database_url`, `admin_secret`, `relay_host`, `relay_user`, `relay_password`.

---

## Shared Library (basis)

Reusable components from `github.com/mustafa-karli/basis`:

| Package | Used For |
|---------|----------|
| `service/secret` | SecretProvider (local, AWS, GCP) |
| `service/storage` | S3 + GCS object storage |
| `common` | `WriteError` (RFC 7807), CLI flags, type helpers |
| `port` | Interface patterns (SecretProvider, ObjectStorage) |

### What's Not Reusable (yet)

basis `LocalUserService` for 2FA uses `userID int` + `port.QueryService`. bdsmail uses `email string` + `store.Database`. The TOTP/backup code crypto logic (~80 lines) is reimplemented natively in `internal/auth/auth.go` using the same `pquerna/otp` library.

**Future basis contribution**: Abstract user identifier to support both int and string keys.

---

## Data Model

### Key Design Decisions

| Decision | Choice | Why |
|----------|--------|-----|
| User PK | `TEXT id` = `username@domain` | Natural key, no joins needed, same across all backends |
| Attachments | Separate `mail_attachment` table | 3NF, queryable, no JSON blobs |
| Domains | `domain` table in DB | Not in config file. Supports API keys, SES status, per-domain management |
| OAuth clients | FK to `domain`, not `user_account` | API serves under a domain, not a user |
| Table names | Descriptive singular (`user_account`, `mail_content`, `mail_filter`) | Avoids reserved words, clear purpose |
| DDL | External SQL files (`sql/`) | Not embedded in Go code. DB init is a deployment step |

### Schema Files

| File | Backend |
|------|---------|
| `sql/bdsmail_pgsql.sql` | PostgreSQL DDL |
| `sql/bdsmail_sqlite.sql` | SQLite DDL |
| `sql/bdsmail_dynamodb.md` | DynamoDB table reference |
| `sql/bdsmail_firestore.md` | Firestore collection + index reference |
| `internal/store/init_dynamodb.go` | DynamoDB table creation script |

### Backup Strategy

Daily pg_dump to S3 via `scripts/backup.sh`:
- Dump → gzip → upload to S3
- Local rotation: keep last 7
- S3 rotation: delete older than 30 days
- Cron: `0 3 * * *`
