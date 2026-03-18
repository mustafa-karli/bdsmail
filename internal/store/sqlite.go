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
	}
}
