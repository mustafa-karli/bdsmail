# Options & Alternatives

Design decisions, alternatives considered, and current implementation choices for bdsmail.

## Deployment Options

See [DEPLOYMENT.md](DEPLOYMENT.md) for full step-by-step instructions.

| Option | Stack | Monthly Cost |
|--------|-------|-------------|
| **AWS Lightsail + DynamoDB + SES** | Lightsail $5, DynamoDB free, S3, SES | **~$6** |
| **GCP + Firestore + SES** | e2-micro, Firestore free, GCS, SES | **~$10.50** |
| **GCP + Cloud SQL** | e2-micro, PostgreSQL, GCS, SES | **~$20.50** |
| **AWS EC2 + RDS** | t4g.micro, PostgreSQL, S3, SES | **~$22.30** |
| **Any VPS + SQLite** | Any $5 VPS, SQLite on disk | **~$6** |

## Database Backends

| Backend | Cost | Pros | Cons |
|---------|------|------|------|
| PostgreSQL | $10-15/mo (managed) | Full SQL, robust, best query flexibility | Requires managed DB or self-hosted |
| SQLite | $0 | Zero config, no network dependency, full-text search | Single-server only |
| DynamoDB | $0 (always-free tier) | Managed, scalable, 25GB free | No full-text search (scan only) |
| Firestore | $0 (free tier) | Managed, 1GB free | GCP only, requires composite indexes |

### AWS Lightsail Database Comparison

When deploying on AWS Lightsail, keep all services within AWS to avoid cross-provider egress costs (~$0.09/GB) and latency (10-50ms per query).

| Option | Cost | Latency | Full-Text Search | Managed | Notes |
|--------|------|---------|-----------------|---------|-------|
| **SQLite on Lightsail** | $0 forever | 0ms (local disk) | Yes (LIKE, FTS5) | No | Best for single-server |
| **DynamoDB** | $0 forever | 1-5ms (same region) | No (client-side scan) | Yes | Best for managed + free |
| **RDS Free Tier** | $0 for 12 months | 1-5ms (same region) | Yes (ILIKE) | Yes | Expires → ~$12/mo |
| **RDS on Lightsail** | $15/mo | <1ms (internal) | Yes (ILIKE) | Yes | Fully managed, internal network |
| **Aurora Serverless v2** | ~$43/mo min | 1-5ms | Yes (ILIKE) | Yes | Overkill for mail server |

**Avoid**: Neon, Supabase, CockroachDB, PlanetScale — all hosted outside AWS, adding egress cost and latency.

### DynamoDB Performance by Operation

| Operation | DynamoDB Approach | Performance | Concern |
|-----------|------------------|-------------|---------|
| Get message by ID | GSI lookup | Fast (single-digit ms) | None |
| List messages by folder | Query partition key + filter | Fast | None |
| Save/delete message | PutItem/DeleteItem | Fast | None |
| Count unread | Query + client count | Moderate | Reads all messages |
| **Search messages** | **Full scan + client filter** | **Slow** | **Degrades with mailbox size** |
| OAuth/domain CRUD | Key lookups | Fast | None |

### Recommendation

**For Lightsail deployment**: Start with **SQLite** (`--db_type=sqlite`). Zero cost, zero latency, full search, single file backup to S3. If you outgrow single-server or need managed ops, migrate to **RDS PostgreSQL** on Lightsail ($15/mo).

**DynamoDB** is viable if you don't need search, or plan to add a dedicated search service (e.g. OpenSearch) later.

## Object Storage

| Backend | Cost | Config |
|---------|------|--------|
| GCS | ~$0.02/mo | `BDS_BUCKET_TYPE=gcs` |
| S3 | ~$0.02/mo | `BDS_BUCKET_TYPE=s3` |
| None | $0 | Attachments dropped |

## Outbound Relay

| Service | Cost (10K emails/mo) | Notes |
|---------|---------------------|-------|
| Amazon SES | ~$1.00 | Cheapest. $0.10/1K emails |
| Mailgun | $15.00 | Basic plan |
| SendGrid | $19.95 | Essentials plan |

---

## Frontend Options

### Framework Comparison

| | **Go Templates (current)** | **Angular** | **React** | **Vue (current)** |
|---|---|---|---|---|
| **Stack** | Server-rendered HTML, single CSS | TypeScript, RxJS | JSX, hooks | SFC (.vue), TypeScript |
| **Build tooling** | None | Angular CLI, Webpack | Vite, Babel | Vite |
| **Bundle size** | 0 KB | ~150-300 KB | ~40-80 KB | ~49 KB gzip |
| **Learning curve** | Low | Steep | Medium | Low-Medium |
| **Files to maintain** | 13 HTML + 1 CSS | 50-100+ | 30-60+ | 13 views + 5 components |
| **Dependencies** | 0 | ~800+ (node_modules) | ~300+ | ~90 (node_modules) |

### What's Implemented

**Both interfaces are available simultaneously:**

**Go Templates** at `/` — Server-rendered, zero JS dependencies. 13 HTML templates + 1 CSS file. Includes reply/forward, pagination, unread badges, keyboard shortcuts, mobile responsive, developer portal, consent screen.

**Vue 3 SPA** at `/app/` — Client-side app in `web/vue/`. 13 views, 5 reusable components, Pinia stores, Vue Router with auth guards. Calls JSON REST API at `/api/*`. ~49 KB gzip total.

| Component | Go Templates | Vue SPA |
|-----------|-------------|---------|
| Auth | Cookie/session | Same cookies via API |
| Data | Server injects into template | Axios → `/api/*` → JSON |
| Routing | Server-side | Vue Router (client-side) |
| State | Page reload = fresh | Pinia stores (reactive) |
| Build | None | `npm run build` |
| Deploy | Embedded | Embedded at `/app/` or Amplify/Firebase/S3 |

### SPA Hosting Options

The Vue SPA can be served from the Go binary (default) or hosted separately:

| Option | Cost | Notes |
|--------|------|-------|
| Embedded in Go binary | $0 | Default — `npm run build` → served at `/app/` |
| AWS Amplify | $0 | Free tier: 15GB/month |
| Firebase Hosting | $0 | Free tier: 10GB/month |
| S3 + CloudFront | ~$0.50 | CDN-level performance |

---

## Identity Provider (OAuth 2.0 / OIDC)

bdsmail includes a built-in OAuth 2.0 / OpenID Connect identity provider — "Sign in with yourdomain.com."

### What's Implemented

- **Developer Portal** (`/developer`) — Self-service OAuth app registration
- **Authorization Endpoint** (`/oauth/authorize`) — Consent screen with user authentication
- **Token Endpoint** (`/oauth/token`) — Authorization code → access_token + JWT id_token
- **UserInfo Endpoint** (`/oauth/userinfo`) — Bearer token → user profile
- **JWKS** (`/oauth/jwks`) — RSA public keys for JWT verification
- **OIDC Discovery** (`/.well-known/openid-configuration`) — Standard discovery document

### Database Tables

| Table | Purpose |
|-------|---------|
| `oauth_clients` | Registered apps (client_id, secret_hash, redirect_uri, owner) |
| `oauth_codes` | Authorization codes (10 min expiry, single-use) |
| `oauth_tokens` | Access tokens (1 hour expiry) |

### JWT ID Token Claims

```json
{
  "iss": "https://mail.yourdomain.com",
  "sub": "alice@yourdomain.com",
  "aud": "client_id",
  "email": "alice@yourdomain.com",
  "name": "alice",
  "domain": "yourdomain.com",
  "exp": 1234567890,
  "iat": 1234567890,
  "nonce": "..."
}
```

### Supported on

- PostgreSQL and SQLite backends (full support)
- DynamoDB and Firestore (full support)

---

## Shared Library (basis)

bdsmail reuses `github.com/mustafa-karli/basis` for cross-cutting concerns:

| basis Package | Used For |
|--------------|----------|
| `service/secret` | SecretProvider — local JSON, AWS Secrets Manager, GCP Secret Manager |
| `service/storage` | S3 + GCS object storage (replaces bdsmail's custom bucket code) |
| `common` | `WriteError` (RFC 7807 Problem Detail) for API error responses |
| `common` | CLI flag definitions shared across projects |

### Configuration

No `.env` file — all config via CLI flags + secrets:

```bash
bdsmail \
  --db_type=dynamodb \
  --dynamodb_region=us-east-1 \
  --bucket_type=s3 \
  --s3_bucket=bdsmail-attachments \
  --secret_mode=aws \
  --smtp_port=25 \
  --https_port=443
```

Secrets (`database_url`, `admin_secret`, `relay_host`, `relay_user`, `relay_password`) are loaded from the configured SecretProvider at startup.

---

## Two-Factor Authentication (2FA)

TOTP-based 2FA using the same `github.com/pquerna/otp` library as the basis project.

### Features
- **TOTP** — Google Authenticator, Authy, or any TOTP app
- **Backup codes** — 10 one-time codes (bcrypt-hashed, pipe-separated)
- **Trusted devices** — 30-day bypass via device fingerprint
- **Login tokens** — 5-minute pending state during 2FA verification

### Flow
1. User logs in with password
2. If 2FA enabled and device not trusted → issue login token, redirect to `/verify-2fa`
3. User enters TOTP code (or backup code)
4. Optionally trusts device for 30 days
5. Session created

### Database Tables
- `user_account` — columns: `twofa_enabled`, `twofa_secret`, `twofa_backup_codes`, `login_attempts`
- `user_trusted_device` — device fingerprint, 30-day expiry
- `user_otp` — 6-digit codes, 2-minute expiry, max 5 attempts
- `login_token` — 5-minute pending 2FA state

### Data Model
- `user_account.id` is `TEXT` (natural key: `username@domain`), not an auto-increment integer
- `phone` and `external_email` columns for OTP delivery
- All child tables reference `user_email` (same value as `user_account.id`)
