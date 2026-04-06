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
| SQLite | $0 | Zero config, no network dependency | Single-server only |
| DynamoDB | $0 (always-free tier) | Managed, scalable, 25GB free | AWS only, OAuth not yet supported |
| Firestore | $0 (free tier) | Managed, 1GB free | GCP only, OAuth not yet supported |

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
- DynamoDB and Firestore (not yet implemented — returns error)
