# Options & Alternatives

Design decisions, alternatives considered, and current implementation choices for bdsmail.

## Deployment Stack

See [DEPLOYMENT.md](DEPLOYMENT.md) for full step-by-step instructions.

| Component | Service | Cost | Why |
|-----------|---------|------|-----|
| **Compute** | Hetzner CX22 | €4.50/mo | 2 vCPU, 4GB RAM, 40GB NVMe. Port 25 open. |
| **Database** | Self-hosted PostgreSQL 18 | $0 | Full SQL, full-text search, on same instance |
| **Outbound relay** | SendGrid | $0 | 100/day free. Instant approval, no sandbox. |
| **Attachments + Backups** | Cloudflare R2 | $0 | S3-compatible, 10GB free tier |
| **DNS** | Cloudflare | $0 | Free plan, DNS-only mode for mail subdomains |
| **TLS** | Let's Encrypt | $0 | Per-domain SNI certificates |

### Why This Stack

| Decision | Alternatives Considered | Why Chosen |
|----------|------------------------|------------|
| **Hetzner over AWS** | Lightsail, EC2 | Better specs for the price (4GB vs 512MB). Port 25 open. No SES approval needed. |
| **SendGrid over SES** | SES, Mailgun, Postmark | SES rejected sandbox exit. SendGrid approves instantly with free 100/day. |
| **R2 over S3** | S3, GCS | S3-compatible API, 10GB free, no egress fees. Already used for daxoom. |
| **Self-hosted PG over managed** | RDS, Cloud SQL, DynamoDB | $0 cost, full SQL, same instance = zero latency. pg_dump for backups. |
| **Cloudflare over Route 53** | Route 53, GoDaddy | Free DNS, already managing domains there. |

### Database

Self-hosted PostgreSQL 18 on the same Hetzner VPS. Tuned for 4GB RAM.

- **Backups**: Daily pg_dump → gzip → Cloudflare R2 via rclone (7-day local, 30-day remote)
- **Migration path**: If demand grows, pg_dump → pg_restore to any managed PostgreSQL

### Outbound Relay

| Service | Cost | Free Tier | Approval |
|---------|------|-----------|----------|
| **SendGrid** (current) | $0 | 100/day forever | Instant |
| Mailgun | $15/mo | 1K/mo for 3 months | Quick |
| Postmark | $1.25/1K | None | Instant |
| Amazon SES | $0.10/1K | Sandbox only | **Rejected** (multi-tenant concern) |

Relay config is in `secrets.json` — switch anytime by changing `relay_host`, `relay_user`, `relay_password`.

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

## Object Storage

Cloudflare R2 (S3-compatible). Single bucket for attachments and backups.

| Config Flag | Value |
|-------------|-------|
| `--s3_bucket` | `bdsmail` |
| `--s3_region` | `auto` |
| `--s3_endpoint` | `https://<account-id>.r2.cloudflarestorage.com` |

R2 uses the same AWS S3 SDK — just set a custom endpoint. No code changes needed.

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
  --s3_bucket=bdsmail \
  --s3_region=auto \
  --s3_endpoint=https://<account-id>.r2.cloudflarestorage.com \
  --secret_mode=local \
  --keystore=/opt/bdsmail/sec/secrets.json \
  --inbound_smtp_port=25 \
  --https_port=443 \
  --mail_hostname=mailsrv.bdscont.com
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
