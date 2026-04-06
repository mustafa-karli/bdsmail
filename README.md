# BDS Mail

A multi-domain mail server written in Go. Single binary, zero required external dependencies. Supports SMTP, POP3, IMAP, and a web interface with pluggable cloud-native storage backends. Includes comprehensive email security: DKIM signing, SPF/DKIM/DMARC verification, MTA-STS, DANE/TLSA, TLSRPT, REQUIRETLS, ClamAV antivirus, Rspamd spam filtering, Google Safe Browsing, rate limiting, and automated TLS certificate management.

## Features

- **Multi-domain**: Serve multiple domains from a single instance (e.g. `domain1.com`, `domain2.com`)
- **Dual Web UI**: Server-rendered Go templates + Vue 3 SPA with JSON REST API
- **SMTP**: Receive inbound mail and relay outbound mail via MX lookup or external relay (SES, SendGrid, Mailgun)
- **POP3 & IMAP**: Mail client access (Thunderbird, Outlook, etc.)
- **Attachments**: Send and receive file attachments (MIME multipart), stored in GCS or S3
- **Reply / Forward**: Built-in reply, reply-all, forward with quoted body
- **Full-text search**: Search across subject, body, from, and to fields
- **Pagination**: 50 messages per page with navigation
- **Keyboard shortcuts**: `c` to compose from any page
- **Mobile responsive**: Works on phones and tablets
- **DKIM signing**: Outbound emails are cryptographically signed
- **Automated TLS**: Let's Encrypt certificates via certbot
- **SPF/DKIM/DMARC**: Inbound sender verification with policy-based reject/quarantine
- **MTA-STS / DANE / REQUIRETLS / TLSRPT**: Full outbound transport security
- **Virus scanning**: ClamAV integration (inbound and outbound)
- **Spam filtering**: Rspamd with configurable thresholds
- **Dangerous link detection**: Google Safe Browsing API
- **Rate limiting**: Per-IP connection limiting and brute-force login protection
- **Email aliases**: Forward to one or more targets, domain-level catch-all
- **Mailing lists**: Group distribution lists with List-Id headers
- **Server-side filtering**: Sieve-style rules with default presets
- **Auto-reply / Vacation**: Configurable with date ranges and cooldown
- **Contacts / CardDAV**: Web UI and protocol-level contact sync
- **OAuth 2.0 / OpenID Connect**: Built-in identity provider — "Sign in with yourdomain.com"
- **Developer portal**: Self-service OAuth app registration for third-party integrations
- **Admin panel**: Web UI for domains, users, aliases, and mailing lists
- **Dynamic domains**: Add domains on the fly without restart
- **Multiple databases**: PostgreSQL, SQLite, DynamoDB, or Firestore
- **Multiple object stores**: GCS or S3 for attachments

## Architecture

```mermaid
graph TD
    Internet((Internet))

    Internet -->|inbound| SMTP["SMTP :25"]
    Internet --> POP3["POP3 :110"]
    Internet --> IMAP["IMAP :143"]
    Internet --> HTTPS["HTTPS :443<br/>(Web UI + API + OAuth)"]

    SMTP --> RateLimit["Rate Limiter"]
    HTTPS --> RateLimit

    RateLimit --> Security["Security Checks<br/>(ClamAV, Rspamd,<br/>Safe Browsing,<br/>SPF/DKIM/DMARC)"]

    Security --> Filters["Mail Processing<br/>(Aliases, Mailing Lists,<br/>Filters, Auto-Reply)"]

    Filters --> Store["Store"]
    POP3 --> Store
    IMAP --> Store

    Internet --> CardDAV["CardDAV :443"]
    Internet --> OAuth["OAuth/OIDC :443<br/>(authorize, token,<br/>userinfo, jwks)"]
    CardDAV --> Store
    OAuth --> Store

    Store --> DB[(PostgreSQL / SQLite<br/>DynamoDB / Firestore)]
    Store --> Bucket[(GCS / S3<br/>attachments)]

    SMTP -->|outbound| TLS["Transport Security<br/>(MTA-STS, DANE/TLSA,<br/>REQUIRETLS, TLSRPT)"]
    HTTPS -->|outbound| TLS
    TLS --> Relay["SMTP Relay :587<br/>(SES / SendGrid / Mailgun<br/>or direct MX)"]
    Relay --> Internet
```

## Deployment Options

See [DEPLOYMENT.md](DEPLOYMENT.md) for detailed step-by-step instructions.

| Option | Stack | Monthly Cost |
|--------|-------|-------------|
| **AWS Lightsail + DynamoDB + SES** | Lightsail, DynamoDB (free tier), S3, SES | **~$6** |
| **GCP + Firestore + SES** | e2-micro, Firestore (free tier), GCS, SES | **~$10.50** |
| **GCP + Cloud SQL** | e2-micro, PostgreSQL, GCS, SES | **~$20.50** |
| **AWS EC2 + RDS** | t4g.micro, PostgreSQL, S3, SES | **~$22.30** |
| **Any VPS + SQLite** | Any $5 VPS, SQLite on disk | **~$6** |

SPA hosting (optional): Amplify (free tier), Firebase Hosting (free tier), or S3+CloudFront (~$0.50/month).

## Web Interface

Two interfaces served from the same server:

**Go Templates** (default at `/`) — Server-rendered, zero JS dependencies, works everywhere. Includes reply/forward, pagination, unread badges, keyboard shortcuts, mobile responsive.

**Vue 3 SPA** (at `/app/`) — Modern client-side app with Pinia state management, Vue Router, and Axios. Same features via JSON REST API at `/api/*`.

```bash
# Development
cd web/vue && npm install && npm run dev

# Production build
cd web/vue && npm run build   # Output served at /app/
```

## JSON REST API

All functionality is available via JSON endpoints at `/api/*`:

| Endpoint | Description |
|----------|-------------|
| `POST /api/auth/login` | Authenticate, returns user info |
| `GET /api/messages?folder=INBOX&page=1` | Paginated message list |
| `GET /api/messages/:id` | Message with body |
| `POST /api/compose` | Send message (multipart) |
| `GET /api/search?q=term` | Full-text search |
| `GET /api/folders` | User folder list |
| `GET /api/unread` | Unread count |
| `GET/POST /api/filters` | Manage mail filters |
| `GET/POST /api/autoreply` | Auto-reply settings |
| `GET/POST /api/contacts` | Contact management |
| `GET/POST /api/oauth/clients` | Developer portal (OAuth app registration) |
| `GET/POST /api/admin/*` | Admin operations (domains, users, aliases, lists) |

## OAuth 2.0 / OpenID Connect

bdsmail is an OIDC identity provider — enabling "Sign in with yourdomain.com" for any application.

### Flow

1. Developer registers app at `/developer` (gets `client_id` + `client_secret`)
2. App redirects user to `/oauth/authorize?client_id=...&redirect_uri=...&response_type=code&scope=openid email`
3. User authenticates and sees consent screen
4. bdsmail redirects back with authorization code
5. App exchanges code for `access_token` + `id_token` (JWT) at `/oauth/token`
6. App reads user identity from JWT or calls `/oauth/userinfo`

### Endpoints

| Endpoint | Description |
|----------|-------------|
| `/oauth/authorize` | Authorization (consent screen) |
| `/oauth/token` | Token exchange |
| `/oauth/userinfo` | User profile |
| `/oauth/jwks` | Public keys for JWT verification |
| `/.well-known/openid-configuration` | OIDC discovery |
| `/developer` | Self-service app registration |

### JWT Claims

`iss`, `sub`, `aud`, `email`, `name`, `domain`, `exp`, `iat`, `nonce`

## Security

All security features are **enabled by default** with a fail-open strategy.

| Layer | Features |
|-------|----------|
| **Connection** | Per-IP rate limiting, brute-force lockout |
| **Inbound** | ClamAV scan, SPF/DKIM/DMARC verify, Rspamd spam score, Safe Browsing URL check |
| **Outbound** | ClamAV scan, Safe Browsing check, DKIM signing |
| **Transport** | MTA-STS policy enforcement, DANE/TLSA verification, REQUIRETLS, TLSRPT reporting |
| **Auth** | Bcrypt passwords, session cookies, OAuth 2.0 with JWT |
| **Secrets** | CLI flags + SecretProvider (local JSON, AWS Secrets Manager, GCP Secret Manager) |

See [DEPLOYMENT.md](DEPLOYMENT.md) for security configuration details.

## Code Architecture

- **basis package** — Shared library (`github.com/mustafa-karli/basis`) for secrets, storage, logging, HTTP utilities
- **Interface Segregation** — Database split into `UserStore`, `MessageStore`, `AliasStore`, `FilterStore`, `ContactStore`, `DomainStore`, `OAuthStore` composed into `Database`
- **CLI flags** — All configuration via `flag` package, no `.env` file dependency. Secrets loaded via `SecretProvider` at startup
- **Domain table** — Domains stored in DB (not config file), with API key hash, SES/DKIM status, created_by
- **cryptoutil** — Shared crypto helpers for secure random generation and bcrypt hashing
- **Type-safe templates** — `pageData` uses concrete types (`[]*model.Message`, `[]*model.Filter`) instead of `interface{}`

## Mail Client Access

| Setting | Value |
|---------|-------|
| **IMAP** | `mail.yourdomain.com:143` (SSL/TLS) |
| **POP3** | `mail.yourdomain.com:110` (SSL/TLS) |
| **SMTP** | `mail.yourdomain.com:25` (STARTTLS) |
| **Username** | Full email: `user@yourdomain.com` |

## Email Features

**Aliases** — Forward to one or more targets. Catch-all with `@domain.com`. Managed at `/admin/aliases`.

**Mailing Lists** — Group distribution with `[ListName]` subject prefix and List-Id headers. Managed at `/admin/lists`.

**Filters** — Sieve-style rules: conditions (from/to/subject) + actions (move/mark read/delete/flag). Default presets for newsletters, social, noreply, large attachments.

**Auto-Reply** — Out-of-office with date ranges. 24-hour cooldown per sender. Skips noreply/mailer-daemon addresses.

**Contacts / CardDAV** — Web UI at `/contacts`. CardDAV at `/carddav/user@domain/default/` (macOS Contacts, DAVx5, Thunderbird CardBook).

**Admin** — Web panel at `/admin/` for domains, users, aliases, mailing lists. Protected by `BDS_ADMIN_SECRET`.

**Adding Domains** — `./bdsmail -adddomain newdomain.com` or via `/admin/domains`. Auto-generates DKIM keys, expands TLS cert, persists to `.env`.

---

## Data Model

### Mail & User Data

```mermaid
erDiagram
    mail_content }o--|| user_account : "owner_user"
    mail_filter }o--|| user_account : "user_email"
    user_contact }o--|| user_account : "owner_email"
    auto_reply |o--|| user_account : "user_email"
    auto_reply_log }o--|| user_account : "user_email"

    mail_content {
        TEXT id PK
        TEXT message_id
        TEXT from_addr
        TEXT to_addrs
        TEXT subject
        TEXT body
        TEXT attachments
        TEXT owner_user FK
        TEXT folder
        BOOLEAN seen
        BOOLEAN deleted
        TIMESTAMPTZ received_at
    }

    mail_filter {
        TEXT id PK
        TEXT user_email FK
        TEXT name
        INTEGER priority
        TEXT conditions
        TEXT actions
        BOOLEAN enabled
    }

    user_contact {
        TEXT id PK
        TEXT owner_email FK
        TEXT vcard_data
        TEXT etag
        TIMESTAMPTZ updated_at
    }

    user_account {
        SERIAL id PK
        TEXT username
        TEXT domain
        TEXT display_name
        TEXT password_hash
        TIMESTAMPTZ created_at
    }

    auto_reply {
        TEXT user_email PK
        BOOLEAN enabled
        TEXT subject
        TEXT body
        TIMESTAMPTZ start_date
        TIMESTAMPTZ end_date
    }

    auto_reply_log {
        TEXT user_email PK
        TEXT sender_email PK
        TIMESTAMPTZ sent_at
    }
```

### Domain & OAuth

```mermaid
erDiagram
    user_account }o--|| domain : "domain"
    oauth_client }o--|| domain : "domain"
    oauth_client ||--o{ oauth_code : "client_id"
    oauth_client ||--o{ oauth_token : "client_id"

    user_account {
        SERIAL id PK
        TEXT username
        TEXT domain FK
    }

    domain {
        TEXT name PK
        TEXT api_key_hash
        TEXT ses_status
        TEXT dkim_status
        TEXT status
        TEXT created_by
        TIMESTAMPTZ created_at
    }

    oauth_client {
        TEXT id PK
        TEXT name
        TEXT client_id UK
        TEXT secret_hash
        TEXT redirect_uri
        TEXT domain FK
        TEXT created_by
        TIMESTAMPTZ created_at
    }

    oauth_code {
        TEXT code PK
        TEXT client_id FK
        TEXT user_email FK
        TEXT scope
        TEXT nonce
        TIMESTAMPTZ expires_at
        BOOLEAN used
    }

    oauth_token {
        TEXT token PK
        TEXT client_id FK
        TEXT user_email FK
        TEXT scope
        TIMESTAMPTZ expires_at
    }
```

### Mailing Lists & Aliases

```mermaid
erDiagram
    list_member }o--|| mailing_list : "list_address"

    mail_alias {
        TEXT alias_email PK
        TEXT target_emails
        BOOLEAN is_catch_all
    }

    mailing_list {
        TEXT list_address PK
        TEXT name
        TEXT description
        TEXT owner_email
        TIMESTAMPTZ created_at
    }

    list_member {
        TEXT list_address FK
        TEXT member_email PK
    }
```
