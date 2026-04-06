package store

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

func NewDbSqlite(path string) (*DbSQL, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, err
	}
	if err := conn.Ping(); err != nil {
		return nil, err
	}
	db := &DbSQL{
		Conn:    conn,
		Queries: sqliteQueries(),
		FormatBool: func(b bool) interface{} {
			if b {
				return 1
			}
			return 0
		},
		FormatTime: func(t time.Time) interface{} {
			return t.UTC().Format("2006-01-02 15:04:05")
		},
	}
	if err := db.Migrate(sqliteMigrations()); err != nil {
		return nil, err
	}
	return db, nil
}

func sqliteMigrations() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			domain TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL,
			created_at TEXT DEFAULT (datetime('now')),
			UNIQUE(username, domain)
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			message_id TEXT,
			from_addr TEXT NOT NULL,
			to_addrs TEXT NOT NULL,
			cc_addrs TEXT NOT NULL,
			bcc_addrs TEXT NOT NULL,
			subject TEXT,
			content_type TEXT,
			body TEXT NOT NULL DEFAULT '',
			attachments TEXT NOT NULL DEFAULT '[]',
			gcs_key TEXT NOT NULL DEFAULT '',
			owner_user TEXT NOT NULL,
			folder TEXT NOT NULL DEFAULT 'INBOX',
			seen INTEGER NOT NULL DEFAULT 0,
			deleted INTEGER NOT NULL DEFAULT 0,
			received_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_owner_folder ON messages(owner_user, folder)`,
		`CREATE TABLE IF NOT EXISTS aliases (
			alias_email TEXT PRIMARY KEY,
			target_emails TEXT NOT NULL,
			is_catch_all INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS mailing_lists (
			list_address TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			owner_email TEXT NOT NULL,
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS list_members (
			list_address TEXT NOT NULL,
			member_email TEXT NOT NULL,
			PRIMARY KEY (list_address, member_email)
		)`,
		`CREATE TABLE IF NOT EXISTS filters (
			id TEXT PRIMARY KEY,
			user_email TEXT NOT NULL,
			name TEXT NOT NULL,
			priority INTEGER NOT NULL DEFAULT 0,
			conditions TEXT NOT NULL DEFAULT '[]',
			actions TEXT NOT NULL DEFAULT '[]',
			enabled INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX IF NOT EXISTS idx_filters_user ON filters(user_email)`,
		`CREATE TABLE IF NOT EXISTS auto_replies (
			user_email TEXT PRIMARY KEY,
			enabled INTEGER NOT NULL DEFAULT 0,
			subject TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL DEFAULT '',
			start_date TEXT,
			end_date TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS auto_reply_log (
			user_email TEXT NOT NULL,
			sender_email TEXT NOT NULL,
			sent_at TEXT DEFAULT (datetime('now')),
			PRIMARY KEY (user_email, sender_email)
		)`,
		`CREATE TABLE IF NOT EXISTS contacts (
			id TEXT PRIMARY KEY,
			owner_email TEXT NOT NULL,
			vcard_data TEXT NOT NULL,
			etag TEXT NOT NULL,
			created_at TEXT DEFAULT (datetime('now')),
			updated_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_owner ON contacts(owner_email)`,
		`CREATE TABLE IF NOT EXISTS oauth_clients (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			client_id TEXT UNIQUE NOT NULL,
			secret_hash TEXT NOT NULL,
			redirect_uri TEXT NOT NULL,
			owner_email TEXT NOT NULL,
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_codes (
			code TEXT PRIMARY KEY,
			client_id TEXT NOT NULL,
			user_email TEXT NOT NULL,
			redirect_uri TEXT NOT NULL,
			scope TEXT NOT NULL DEFAULT '',
			nonce TEXT NOT NULL DEFAULT '',
			expires_at TEXT NOT NULL,
			used INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_tokens (
			token TEXT PRIMARY KEY,
			client_id TEXT NOT NULL,
			user_email TEXT NOT NULL,
			scope TEXT NOT NULL DEFAULT '',
			expires_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_clients_owner ON oauth_clients(owner_email)`,
	}
}

func sqliteQueries() map[string]string {
	return map[string]string{
		QCreateUser:  "INSERT INTO users (username, domain, display_name, password_hash) VALUES (?, ?, ?, ?)",
		QGetUser:     "SELECT id, username, domain, display_name, password_hash, created_at FROM users WHERE username = ? AND domain = ?",
		QUserExists:  "SELECT COUNT(*) FROM users WHERE username = ? AND domain = ?",
		QSaveMessage: `INSERT INTO messages (id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, attachments, gcs_key, owner_user, folder, seen, received_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		QListMessages: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, attachments, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM messages WHERE owner_user = ? AND folder = ? AND deleted = 0 ORDER BY received_at DESC`,
		QListAllMessages: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, attachments, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM messages WHERE owner_user = ? AND deleted = 0 ORDER BY received_at DESC`,
		QGetMessage: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, attachments, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM messages WHERE id = ?`,
		QMarkSeen:      "UPDATE messages SET seen = 1 WHERE id = ?",
		QMarkDeleted:   "UPDATE messages SET deleted = 1 WHERE id = ?",
		QDeleteMessage: "DELETE FROM messages WHERE id = ?",

		// User provisioning
		QListUsers:         "SELECT id, username, domain, display_name, password_hash, created_at FROM users ORDER BY domain, username",
		QListUsersByDomain: "SELECT id, username, domain, display_name, password_hash, created_at FROM users WHERE domain = ? ORDER BY username",
		QUpdateUser:        "UPDATE users SET display_name = ?, password_hash = ? WHERE username = ? AND domain = ?",
		QDeleteUser:        "DELETE FROM users WHERE username = ? AND domain = ?",
		QDeleteUserMessages: "DELETE FROM messages WHERE owner_user = ?",

		// Aliases
		QCreateAlias: "INSERT INTO aliases (alias_email, target_emails, is_catch_all) VALUES (?, ?, ?)",
		QGetAlias:    "SELECT target_emails FROM aliases WHERE alias_email = ?",
		QListAliases: "SELECT alias_email, target_emails, is_catch_all FROM aliases ORDER BY alias_email",
		QUpdateAlias: "UPDATE aliases SET target_emails = ? WHERE alias_email = ?",
		QDeleteAlias: "DELETE FROM aliases WHERE alias_email = ?",
		QGetCatchAll: "SELECT target_emails FROM aliases WHERE alias_email = ? AND is_catch_all = 1",

		// Mailing lists
		QCreateMailingList: "INSERT INTO mailing_lists (list_address, name, description, owner_email) VALUES (?, ?, ?, ?)",
		QGetMailingList:    "SELECT list_address, name, description, owner_email, created_at FROM mailing_lists WHERE list_address = ?",
		QListMailingLists:  "SELECT list_address, name, description, owner_email, created_at FROM mailing_lists ORDER BY list_address",
		QDeleteMailingList: "DELETE FROM mailing_lists WHERE list_address = ?",
		QAddListMember:     "INSERT OR IGNORE INTO list_members (list_address, member_email) VALUES (?, ?)",
		QRemoveListMember:  "DELETE FROM list_members WHERE list_address = ? AND member_email = ?",
		QGetListMembers:    "SELECT member_email FROM list_members WHERE list_address = ? ORDER BY member_email",
		QIsMailingList:     "SELECT COUNT(*) FROM mailing_lists WHERE list_address = ?",
		QDeleteListMembers: "DELETE FROM list_members WHERE list_address = ?",

		// Filters
		QCreateFilter: "INSERT INTO filters (id, user_email, name, priority, conditions, actions, enabled) VALUES (?, ?, ?, ?, ?, ?, ?)",
		QListFilters:  "SELECT id, user_email, name, priority, conditions, actions, enabled FROM filters WHERE user_email = ? ORDER BY priority DESC",
		QUpdateFilter: "UPDATE filters SET name = ?, priority = ?, conditions = ?, actions = ?, enabled = ? WHERE id = ?",
		QDeleteFilter: "DELETE FROM filters WHERE id = ?",
		QListUserFolders: "SELECT DISTINCT folder FROM messages WHERE owner_user = ? AND deleted = 0 ORDER BY folder",

		// Auto-reply
		QSetAutoReply: `INSERT INTO auto_replies (user_email, enabled, subject, body, start_date, end_date) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(user_email) DO UPDATE SET enabled=excluded.enabled, subject=excluded.subject, body=excluded.body, start_date=excluded.start_date, end_date=excluded.end_date`,
		QGetAutoReply:          "SELECT user_email, enabled, subject, body, start_date, end_date FROM auto_replies WHERE user_email = ?",
		QDeleteAutoReply:       "DELETE FROM auto_replies WHERE user_email = ?",
		QRecordAutoReplySent:   `INSERT INTO auto_reply_log (user_email, sender_email) VALUES (?, ?) ON CONFLICT(user_email, sender_email) DO UPDATE SET sent_at = datetime('now')`,
		QHasAutoRepliedRecently: "SELECT COUNT(*) FROM auto_reply_log WHERE user_email = ? AND sender_email = ? AND sent_at > ?",

		// Unread count
		QCountUnread: "SELECT COUNT(*) FROM messages WHERE owner_user = ? AND folder = ? AND seen = 0 AND deleted = 0",

		// Search
		QSearchMessages: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, attachments, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM messages WHERE owner_user = ? AND deleted = 0 AND (subject LIKE ? OR body LIKE ? OR from_addr LIKE ? OR to_addrs LIKE ?) ORDER BY received_at DESC LIMIT 100`,

		// Contacts
		QCreateContact: "INSERT INTO contacts (id, owner_email, vcard_data, etag) VALUES (?, ?, ?, ?)",
		QGetContact:    "SELECT id, owner_email, vcard_data, etag, created_at, updated_at FROM contacts WHERE id = ?",
		QListContacts:  "SELECT id, owner_email, vcard_data, etag, created_at, updated_at FROM contacts WHERE owner_email = ? ORDER BY updated_at DESC",
		QUpdateContact: "UPDATE contacts SET vcard_data = ?, etag = ?, updated_at = datetime('now') WHERE id = ?",
		QDeleteContact: "DELETE FROM contacts WHERE id = ?",

		// OAuth
		QCreateOAuthClient: "INSERT INTO oauth_clients (id, name, client_id, secret_hash, redirect_uri, owner_email) VALUES (?, ?, ?, ?, ?, ?)",
		QGetOAuthClient:    "SELECT id, name, client_id, secret_hash, redirect_uri, owner_email, created_at FROM oauth_clients WHERE client_id = ?",
		QListOAuthClients:  "SELECT id, name, client_id, secret_hash, redirect_uri, owner_email, created_at FROM oauth_clients WHERE owner_email = ? ORDER BY created_at DESC",
		QDeleteOAuthClient: "DELETE FROM oauth_clients WHERE id = ?",
		QCreateOAuthCode:   "INSERT INTO oauth_codes (code, client_id, user_email, redirect_uri, scope, nonce, expires_at, used) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		QGetOAuthCode:      "SELECT code, client_id, user_email, redirect_uri, scope, nonce, expires_at, used FROM oauth_codes WHERE code = ?",
		QMarkOAuthCodeUsed: "UPDATE oauth_codes SET used = 1 WHERE code = ?",
		QCreateOAuthToken:  "INSERT INTO oauth_tokens (token, client_id, user_email, scope, expires_at) VALUES (?, ?, ?, ?, ?)",
		QGetOAuthToken:     "SELECT token, client_id, user_email, scope, expires_at FROM oauth_tokens WHERE token = ?",
		QDeleteOAuthToken:  "DELETE FROM oauth_tokens WHERE token = ?",
		QDeleteExpiredOAuthCodes:  "DELETE FROM oauth_codes WHERE expires_at < datetime('now')",
		QDeleteExpiredOAuthTokens: "DELETE FROM oauth_tokens WHERE expires_at < datetime('now')",
	}
}
