# bdsmail vs Open Source Mail Servers

## Feature Comparison

| Feature | bdsmail | iRedMail | mailcow | Stalwart | Mail-in-a-Box | Mailu | Docker Mailserver | Zimbra OSE |
|---------|---------|----------|---------|----------|---------------|-------|-------------------|------------|
| **Language** | Go | Bash/Python (installer) | Python/PHP | Rust | Bash/Python | Python | Bash | Java |
| **SMTP** | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| **IMAP** | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| **POP3** | Yes | Yes | Yes | Yes | No | Yes | Yes | Yes |
| **JMAP** | No | No | No | Yes | No | No | No | No |
| **CardDAV** | Yes | Yes (SOGo) | Yes (SOGo) | Yes | Yes (Nextcloud) | No | No | Yes |
| **CalDAV** | No | Yes (SOGo) | Yes (SOGo) | Yes | Yes (Nextcloud) | No | No | Yes |
| | | | | | | | | |
| **Webmail** | Go templates + Vue 3 SPA | Roundcube | SOGo + Roundcube | Built-in | Roundcube | Roundcube | Optional | Built-in |
| **JSON REST API** | Yes (`/api/*`) | No | No | Yes (JMAP) | No | No | No | Yes (SOAP) |
| **Vue / React SPA** | Vue 3 + TypeScript | No | No | No | No | No | No | No |
| **Admin UI** | Built-in (web + API) | iRedAdmin | Built-in | Built-in | Built-in | Built-in | No (CLI) | Built-in |
| **Reply/Forward** | Built-in | Roundcube | SOGo/Roundcube | Built-in | Roundcube | Roundcube | N/A | Built-in |
| **Keyboard Shortcuts** | Yes (`c` to compose) | Roundcube | SOGo/Roundcube | Yes | Roundcube | Roundcube | N/A | Yes |
| **Pagination** | Built-in (50/page) | Roundcube | SOGo/Roundcube | Built-in | Roundcube | Roundcube | N/A | Built-in |
| **Unread Count** | Built-in (nav + folder) | Roundcube | SOGo/Roundcube | Built-in | Roundcube | Roundcube | N/A | Built-in |
| **Mobile Responsive** | Yes (CSS) | Roundcube Elastic | SOGo | Yes | Roundcube Elastic | Roundcube | N/A | Yes |
| | | | | | | | | |
| **SPF** | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| **DKIM** | Yes (auto-generated) | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| **DMARC** | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| **MTA-STS** | Yes | No | No | Yes | No | No | No | No |
| **DANE/TLSA** | Yes | No | Yes | Yes | Yes | No | Yes | No |
| **REQUIRETLS** | Yes | No | No | No | No | No | No | No |
| **TLSRPT** | Yes | No | No | Yes | No | No | No | No |
| **Antivirus** | ClamAV | ClamAV | ClamAV | ClamAV | No | ClamAV | ClamAV | ClamAV |
| **Antispam** | Rspamd | SpamAssassin | Rspamd | Built-in | SpamAssassin | Rspamd | Rspamd/Amavis | SpamAssassin |
| **Safe Browsing** | Yes (Google API) | No | No | No | No | No | No | No |
| **Rate Limiting** | Built-in | Fail2ban | Fail2ban | Built-in | Fail2ban | Fail2ban | Fail2ban | No |
| **Brute-force Protection** | Built-in | Fail2ban | Fail2ban | Built-in | Fail2ban | Fail2ban | Fail2ban | No |
| | | | | | | | | |
| **2FA (TOTP)** | Yes (+ backup codes, trusted devices) | No | No | No | No | No | No | No |
| **OAuth 2.0 / OIDC Provider** | Yes (built-in IdP) | No | No | No | No | No | No | No |
| **Developer Portal** | Yes (self-service) | No | No | No | No | No | No | No |
| **JWT ID Tokens** | Yes (RS256) | No | No | No | No | No | No | No |
| | | | | | | | | |
| **Mail Filtering** | Built-in (Sieve-style) | Sieve | Sieve | Sieve | Sieve | No | Sieve | Built-in |
| **Mailing Lists** | Built-in | No | No | No | No | No | No | Yes |
| **Aliases** | Yes (+ catch-all) | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| **Auto-Reply** | Built-in | Sieve | Sieve | Sieve | Sieve | No | Sieve | Built-in |
| **Full-Text Search** | Built-in | Dovecot FTS | Solr | Built-in | No | No | Optional | Built-in |
| | | | | | | | | |
| **Docker** | Optional | No | Required | Optional | No | Required | Required | No |
| **Single Binary** | Yes | No | No | Yes | No | No | No | No |
| **Multi-Domain** | Yes (dynamic) | Yes | Yes | Yes | Single domain | Yes | Yes | Yes |
| **Cloud DB Support** | PostgreSQL, SQLite, DynamoDB, Firestore | MySQL/PostgreSQL | MySQL | PostgreSQL, MySQL, SQLite, S3, Redis | PostgreSQL | PostgreSQL, MySQL, SQLite | None (file-based) | MySQL/MariaDB |
| **Object Storage** | GCS, S3 | Local | Local | S3, local | Local | Local | Local | Local |
| **External Relay** | SES, SendGrid, Mailgun, any SMTP | Yes | Yes | Yes | No | Yes | Yes | Yes |
| **Auto TLS** | Let's Encrypt (built-in) | Yes | Yes | Yes (ACME) | Yes | Yes | Yes | No |
| **Config Complexity** | Single `.env` file | Multi-file | Docker Compose | TOML/Web UI | Minimal | Docker Compose | Docker Compose + env | Multi-file XML |
| **Min Resources** | ~128MB RAM | ~2GB RAM | ~4GB RAM | ~256MB RAM | ~1GB RAM | ~1GB RAM | ~1GB RAM | ~8GB RAM |

## Per-Server Summary

### bdsmail
Single Go binary (~48MB) designed for cost-effective cloud deployment. Dual webmail interface: server-rendered Go templates for zero-dependency access, plus a Vue 3 SPA with full JSON REST API for modern client-side experience. Covers all major email security standards including MTA-STS, DANE, REQUIRETLS, and TLSRPT. Built-in OAuth 2.0 / OpenID Connect identity provider with self-service developer portal — the only mail server offering "Sign in with yourdomain.com." Pluggable database and storage backends across AWS and GCP. Runs on ~$6-20/month.

**Strengths:** Minimal footprint, cloud-native storage, comprehensive email security, single binary, dual webmail (Go templates + Vue SPA), JSON REST API, built-in OIDC identity provider, Google Safe Browsing.
**Gaps:** No CalDAV/calendar, no JMAP, no shared folders, no WYSIWYG editor.

### iRedMail
Installer-based solution that bundles Postfix, Dovecot, Roundcube, and SpamAssassin on a fresh Linux server. Well-documented and widely deployed. Free edition covers basics; paid iRedAdmin-Pro adds advanced management features.

**Strengths:** Mature ecosystem, good documentation, large community.
**Gaps:** Heavy resource usage (~2GB+), no Docker, no MTA-STS/DANE, paid admin features, no API.

### mailcow (dockerized)
Docker Compose-based stack with a polished web UI. Bundles Postfix, Dovecot, SOGo, Rspamd, ClamAV, and Solr. Popular in the self-hosting community for its ease of management.

**Strengths:** Excellent admin UI, SOGo groupware, active community, DANE support.
**Gaps:** Docker-only, resource-heavy (~4GB+), no MTA-STS, no REST API, complex multi-container setup.

### Stalwart Mail Server
Modern Rust-based server with JMAP support alongside IMAP and SMTP. Built-in spam filter, full-text search, and CalDAV/CardDAV. The closest competitor to bdsmail in terms of modern architecture and feature completeness.

**Strengths:** Rust performance, JMAP support, CalDAV, built-in everything, low resource usage.
**Gaps:** Newer project with smaller community, more complex configuration, no OAuth/OIDC provider.

### Mail-in-a-Box
One-command installer that turns a fresh Ubuntu server into a complete mail server. Includes Nextcloud for contacts/calendar. Designed for non-technical users.

**Strengths:** Simplest setup, includes Nextcloud, automatic DNS checks.
**Gaps:** Single domain only, Ubuntu-only, no Docker, no external relay, no API, limited customization.

### Mailu
Lightweight Docker-based mail server with a clean web UI. Focuses on simplicity with a straightforward Docker Compose deployment.

**Strengths:** Simple Docker setup, clean admin UI, low-ish resources for Docker.
**Gaps:** No CalDAV/CardDAV, no mail filtering, no DANE/MTA-STS, no API, Docker-required.

### Docker Mailserver
Production-grade containerized Postfix/Dovecot stack. CLI-driven with no web admin UI. Favored by users who prefer configuration-as-code.

**Strengths:** Well-maintained, production-proven, flexible Sieve filtering, DANE support.
**Gaps:** No admin GUI (CLI-only), Docker-required, no CalDAV/CardDAV, no API, steeper learning curve.

### Zimbra Open Source Edition
Enterprise-grade Java-based collaboration suite with email, calendar, contacts, and document sharing. Designed for large organizations.

**Strengths:** Full collaboration suite, enterprise features, mature and stable.
**Gaps:** Very heavy (~8GB+ RAM), complex administration, slow release cycle, Java monolith.

## Key Takeaways

### Where bdsmail excels
- **Email security** — Only server implementing SPF + DKIM + DMARC + MTA-STS + DANE + REQUIRETLS + TLSRPT together
- **Identity provider** — Only mail server with built-in OAuth 2.0 / OIDC ("Sign in with yourdomain.com") and developer portal
- **Modern API** — JSON REST API at `/api/*` with Vue 3 SPA frontend, while alternatives rely on server-rendered webmail
- **Resource efficiency** — Runs on ~128MB RAM vs 1-8GB for alternatives
- **Cloud-native** — Native GCS/S3 and serverless DB support (DynamoDB, Firestore)
- **Deployment simplicity** — Single binary, single `.env` file, from $6/month
- **Unique features** — Google Safe Browsing, built-in mailing lists, dynamic domain addition

### Where alternatives have an edge
- **CalDAV/Calendar** — Stalwart, mailcow (SOGo), iRedMail (SOGo), Mail-in-a-Box (Nextcloud), Zimbra
- **JMAP** — Only Stalwart supports this modern mail protocol
- **Community size** — iRedMail, mailcow, and Docker Mailserver have larger communities
- **Groupware** — Zimbra and mailcow (via SOGo) offer shared calendars, document collaboration
- **Ecosystem maturity** — Most alternatives have been in production longer
