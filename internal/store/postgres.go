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
	}
}
