-- BDS Mail - PostgreSQL DDL
-- Run this script to initialize the database schema.
-- All statements use IF NOT EXISTS for idempotency.

CREATE TABLE IF NOT EXISTS user_account (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    domain TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    phone TEXT NOT NULL DEFAULT '',
    external_email TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'A',
    twofa_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    twofa_secret TEXT NOT NULL DEFAULT '',
    twofa_backup_codes TEXT NOT NULL DEFAULT '',
    login_attempts INTEGER NOT NULL DEFAULT 0,
    last_login_attempt TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(username, domain)
);

CREATE TABLE IF NOT EXISTS domain (
    name TEXT PRIMARY KEY,
    api_key_hash TEXT NOT NULL DEFAULT '',
    ses_status TEXT NOT NULL DEFAULT '',
    dkim_status TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS mail_content (
    id TEXT PRIMARY KEY,
    message_id TEXT,
    from_addr TEXT NOT NULL,
    to_addrs TEXT NOT NULL,
    cc_addrs TEXT NOT NULL,
    bcc_addrs TEXT NOT NULL,
    subject TEXT,
    content_type TEXT,
    body TEXT NOT NULL DEFAULT '',
    gcs_key TEXT NOT NULL DEFAULT '',
    owner_user TEXT NOT NULL,
    folder TEXT NOT NULL DEFAULT 'INBOX',
    seen BOOLEAN NOT NULL DEFAULT FALSE,
    deleted BOOLEAN NOT NULL DEFAULT FALSE,
    received_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_mail_content_owner_folder ON mail_content(owner_user, folder);

CREATE TABLE IF NOT EXISTS mail_attachment (
    id TEXT PRIMARY KEY,
    mail_content_id TEXT NOT NULL,
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT '',
    size BIGINT NOT NULL DEFAULT 0,
    bucket_key TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_mail_attachment_msg ON mail_attachment(mail_content_id);

CREATE TABLE IF NOT EXISTS mail_alias (
    alias_email TEXT PRIMARY KEY,
    target_emails TEXT NOT NULL,
    is_catch_all BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS mailing_list (
    list_address TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    owner_email TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS list_member (
    list_address TEXT NOT NULL,
    member_email TEXT NOT NULL,
    PRIMARY KEY (list_address, member_email)
);

CREATE TABLE IF NOT EXISTS mail_filter (
    id TEXT PRIMARY KEY,
    user_email TEXT NOT NULL,
    name TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    conditions TEXT NOT NULL DEFAULT '[]',
    actions TEXT NOT NULL DEFAULT '[]',
    enabled BOOLEAN NOT NULL DEFAULT TRUE
);
CREATE INDEX IF NOT EXISTS idx_mail_filter_user ON mail_filter(user_email);

CREATE TABLE IF NOT EXISTS auto_reply (
    user_email TEXT PRIMARY KEY,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    subject TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    start_date TIMESTAMPTZ,
    end_date TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS auto_reply_log (
    user_email TEXT NOT NULL,
    sender_email TEXT NOT NULL,
    sent_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_email, sender_email)
);

CREATE TABLE IF NOT EXISTS user_contact (
    id TEXT PRIMARY KEY,
    owner_email TEXT NOT NULL,
    vcard_data TEXT NOT NULL,
    etag TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_contact_owner ON user_contact(owner_email);

CREATE TABLE IF NOT EXISTS oauth_client (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    client_id TEXT UNIQUE NOT NULL,
    secret_hash TEXT NOT NULL,
    redirect_uri TEXT NOT NULL,
    domain TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_oauth_client_domain ON oauth_client(domain);

CREATE TABLE IF NOT EXISTS oauth_code (
    code TEXT PRIMARY KEY,
    client_id TEXT NOT NULL,
    user_email TEXT NOT NULL,
    redirect_uri TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT '',
    nonce TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS oauth_token (
    token TEXT PRIMARY KEY,
    client_id TEXT NOT NULL,
    user_email TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS user_trusted_device (
    id TEXT PRIMARY KEY,
    user_email TEXT NOT NULL,
    device_fingerprint TEXT NOT NULL,
    device_name TEXT NOT NULL DEFAULT '',
    trusted_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_trusted_device_user ON user_trusted_device(user_email);

CREATE TABLE IF NOT EXISTS user_otp (
    id TEXT PRIMARY KEY,
    user_email TEXT NOT NULL,
    code TEXT NOT NULL,
    purpose TEXT NOT NULL DEFAULT 'login',
    expires_at TIMESTAMPTZ NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_otp_user ON user_otp(user_email);

CREATE TABLE IF NOT EXISTS login_token (
    token TEXT PRIMARY KEY,
    user_email TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS domain_signup (
    id TEXT PRIMARY KEY,
    domain TEXT UNIQUE NOT NULL,
    username TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS user_permission (
    id TEXT PRIMARY KEY,
    user_email TEXT NOT NULL,
    role TEXT NOT NULL,
    domain TEXT NOT NULL,
    start_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    end_date TIMESTAMPTZ NOT NULL DEFAULT '2099-12-31 23:59:59+00',
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_permission_user ON user_permission(user_email);
CREATE INDEX IF NOT EXISTS idx_permission_domain ON user_permission(domain);

CREATE TABLE IF NOT EXISTS domain_dns (
    domain TEXT NOT NULL,
    record_type TEXT NOT NULL,
    name TEXT NOT NULL,
    value TEXT NOT NULL,
    priority TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (domain, record_type, name)
);
