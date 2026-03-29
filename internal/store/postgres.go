package store

import (
	"database/sql"
	"time"

	_ "github.com/lib/pq"
)

func NewDbPgsql(connStr string) (*DbSQL, error) {
	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(); err != nil {
		return nil, err
	}
	db := &DbSQL{
		Conn:       conn,
		Queries:    pgsqlQueries(),
		FormatBool: func(b bool) interface{} { return b },
		FormatTime: func(t time.Time) interface{} { return t },
	}
	if err := db.Migrate(pgsqlMigrations()); err != nil {
		return nil, err
	}
	return db, nil
}

func pgsqlMigrations() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username TEXT NOT NULL,
			domain TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
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
			seen BOOLEAN NOT NULL DEFAULT FALSE,
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			received_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_owner_folder ON messages(owner_user, folder)`,
		`CREATE TABLE IF NOT EXISTS aliases (
			alias_email TEXT PRIMARY KEY,
			target_emails TEXT NOT NULL,
			is_catch_all BOOLEAN NOT NULL DEFAULT FALSE
		)`,
		`CREATE TABLE IF NOT EXISTS mailing_lists (
			list_address TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			owner_email TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
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
			enabled BOOLEAN NOT NULL DEFAULT TRUE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_filters_user ON filters(user_email)`,
		`CREATE TABLE IF NOT EXISTS auto_replies (
			user_email TEXT PRIMARY KEY,
			enabled BOOLEAN NOT NULL DEFAULT FALSE,
			subject TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL DEFAULT '',
			start_date TIMESTAMPTZ,
			end_date TIMESTAMPTZ
		)`,
		`CREATE TABLE IF NOT EXISTS auto_reply_log (
			user_email TEXT NOT NULL,
			sender_email TEXT NOT NULL,
			sent_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (user_email, sender_email)
		)`,
		`CREATE TABLE IF NOT EXISTS contacts (
			id TEXT PRIMARY KEY,
			owner_email TEXT NOT NULL,
			vcard_data TEXT NOT NULL,
			etag TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_owner ON contacts(owner_email)`,
	}
}

func pgsqlQueries() map[string]string {
	return map[string]string{
		QCreateUser:  "INSERT INTO users (username, domain, display_name, password_hash) VALUES ($1, $2, $3, $4)",
		QGetUser:     "SELECT id, username, domain, display_name, password_hash, created_at FROM users WHERE username = $1 AND domain = $2",
		QUserExists:  "SELECT COUNT(*) FROM users WHERE username = $1 AND domain = $2",
		QSaveMessage: `INSERT INTO messages (id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, attachments, gcs_key, owner_user, folder, seen, received_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		QListMessages: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, attachments, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM messages WHERE owner_user = $1 AND folder = $2 AND deleted = FALSE ORDER BY received_at DESC`,
		QListAllMessages: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, attachments, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM messages WHERE owner_user = $1 AND deleted = FALSE ORDER BY received_at DESC`,
		QGetMessage: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, attachments, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM messages WHERE id = $1`,
		QMarkSeen:      "UPDATE messages SET seen = TRUE WHERE id = $1",
		QMarkDeleted:   "UPDATE messages SET deleted = TRUE WHERE id = $1",
		QDeleteMessage: "DELETE FROM messages WHERE id = $1",

		// User provisioning
		QListUsers:         "SELECT id, username, domain, display_name, password_hash, created_at FROM users ORDER BY domain, username",
		QListUsersByDomain: "SELECT id, username, domain, display_name, password_hash, created_at FROM users WHERE domain = $1 ORDER BY username",
		QUpdateUser:        "UPDATE users SET display_name = $1, password_hash = $2 WHERE username = $3 AND domain = $4",
		QDeleteUser:        "DELETE FROM users WHERE username = $1 AND domain = $2",
		QDeleteUserMessages: "DELETE FROM messages WHERE owner_user = $1",

		// Aliases
		QCreateAlias: "INSERT INTO aliases (alias_email, target_emails, is_catch_all) VALUES ($1, $2, $3)",
		QGetAlias:    "SELECT target_emails FROM aliases WHERE alias_email = $1",
		QListAliases: "SELECT alias_email, target_emails, is_catch_all FROM aliases ORDER BY alias_email",
		QUpdateAlias: "UPDATE aliases SET target_emails = $1 WHERE alias_email = $2",
		QDeleteAlias: "DELETE FROM aliases WHERE alias_email = $1",
		QGetCatchAll: "SELECT target_emails FROM aliases WHERE alias_email = $1 AND is_catch_all = TRUE",

		// Mailing lists
		QCreateMailingList: "INSERT INTO mailing_lists (list_address, name, description, owner_email) VALUES ($1, $2, $3, $4)",
		QGetMailingList:    "SELECT list_address, name, description, owner_email, created_at FROM mailing_lists WHERE list_address = $1",
		QListMailingLists:  "SELECT list_address, name, description, owner_email, created_at FROM mailing_lists ORDER BY list_address",
		QDeleteMailingList: "DELETE FROM mailing_lists WHERE list_address = $1",
		QAddListMember:     "INSERT INTO list_members (list_address, member_email) VALUES ($1, $2) ON CONFLICT DO NOTHING",
		QRemoveListMember:  "DELETE FROM list_members WHERE list_address = $1 AND member_email = $2",
		QGetListMembers:    "SELECT member_email FROM list_members WHERE list_address = $1 ORDER BY member_email",
		QIsMailingList:     "SELECT COUNT(*) FROM mailing_lists WHERE list_address = $1",
		QDeleteListMembers: "DELETE FROM list_members WHERE list_address = $1",

		// Filters
		QCreateFilter: "INSERT INTO filters (id, user_email, name, priority, conditions, actions, enabled) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		QListFilters:  "SELECT id, user_email, name, priority, conditions, actions, enabled FROM filters WHERE user_email = $1 ORDER BY priority DESC",
		QUpdateFilter: "UPDATE filters SET name = $1, priority = $2, conditions = $3, actions = $4, enabled = $5 WHERE id = $6",
		QDeleteFilter: "DELETE FROM filters WHERE id = $1",
		QListUserFolders: "SELECT DISTINCT folder FROM messages WHERE owner_user = $1 AND deleted = FALSE ORDER BY folder",

		// Auto-reply
		QSetAutoReply: `INSERT INTO auto_replies (user_email, enabled, subject, body, start_date, end_date) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT(user_email) DO UPDATE SET enabled=EXCLUDED.enabled, subject=EXCLUDED.subject, body=EXCLUDED.body, start_date=EXCLUDED.start_date, end_date=EXCLUDED.end_date`,
		QGetAutoReply:          "SELECT user_email, enabled, subject, body, start_date, end_date FROM auto_replies WHERE user_email = $1",
		QDeleteAutoReply:       "DELETE FROM auto_replies WHERE user_email = $1",
		QRecordAutoReplySent:   `INSERT INTO auto_reply_log (user_email, sender_email) VALUES ($1, $2) ON CONFLICT(user_email, sender_email) DO UPDATE SET sent_at = NOW()`,
		QHasAutoRepliedRecently: "SELECT COUNT(*) FROM auto_reply_log WHERE user_email = $1 AND sender_email = $2 AND sent_at > $3",

		// Search
		QSearchMessages: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, attachments, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM messages WHERE owner_user = $1 AND deleted = FALSE AND (subject ILIKE $2 OR body ILIKE $3 OR from_addr ILIKE $4 OR to_addrs ILIKE $5) ORDER BY received_at DESC LIMIT 100`,

		// Contacts
		QCreateContact: "INSERT INTO contacts (id, owner_email, vcard_data, etag) VALUES ($1, $2, $3, $4)",
		QGetContact:    "SELECT id, owner_email, vcard_data, etag, created_at, updated_at FROM contacts WHERE id = $1",
		QListContacts:  "SELECT id, owner_email, vcard_data, etag, created_at, updated_at FROM contacts WHERE owner_email = $1 ORDER BY updated_at DESC",
		QUpdateContact: "UPDATE contacts SET vcard_data = $1, etag = $2, updated_at = NOW() WHERE id = $3",
		QDeleteContact: "DELETE FROM contacts WHERE id = $1",
	}
}
