package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/mustafakarli/bdsmail/internal/model"
	_ "github.com/lib/pq"
)

type DB struct {
	conn *sql.DB
}

func NewDB(connStr string) (*DB, error) {
	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(); err != nil {
		return nil, err
	}
	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username TEXT NOT NULL,
			domain TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(username, domain)
		)`,
		// Add display_name column if it doesn't exist (for existing databases)
		`DO $$ BEGIN
			ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name TEXT NOT NULL DEFAULT '';
		EXCEPTION WHEN others THEN NULL;
		END $$`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			message_id TEXT,
			from_addr TEXT NOT NULL,
			to_addrs TEXT NOT NULL,
			cc_addrs TEXT NOT NULL,
			bcc_addrs TEXT NOT NULL,
			subject TEXT,
			content_type TEXT,
			gcs_key TEXT NOT NULL,
			owner_user TEXT NOT NULL,
			folder TEXT NOT NULL DEFAULT 'INBOX',
			seen BOOLEAN NOT NULL DEFAULT FALSE,
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			received_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_owner_folder ON messages(owner_user, folder)`,
	}
	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// User operations

func (db *DB) CreateUser(username, domain, displayName, passwordHash string) error {
	_, err := db.conn.Exec(
		"INSERT INTO users (username, domain, display_name, password_hash) VALUES ($1, $2, $3, $4)",
		username, domain, displayName, passwordHash,
	)
	return err
}

func (db *DB) GetUser(username, domain string) (*model.User, error) {
	u := &model.User{}
	err := db.conn.QueryRow(
		"SELECT id, username, domain, display_name, password_hash, created_at FROM users WHERE username = $1 AND domain = $2",
		username, domain,
	).Scan(&u.ID, &u.Username, &u.Domain, &u.DisplayName, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (db *DB) GetUserByEmail(email string) (*model.User, error) {
	username, domain := SplitEmail(email)
	return db.GetUser(username, domain)
}

func (db *DB) UserExistsByEmail(email string) bool {
	var count int
	username, domain := SplitEmail(email)
	db.conn.QueryRow(
		"SELECT COUNT(*) FROM users WHERE username = $1 AND domain = $2",
		username, domain,
	).Scan(&count)
	return count > 0
}

func SplitEmail(email string) (string, string) {
	for i, c := range email {
		if c == '@' {
			return email[:i], email[i+1:]
		}
	}
	return email, ""
}

// Message operations

func marshalAddrs(addrs []string) string {
	if addrs == nil {
		addrs = []string{}
	}
	b, _ := json.Marshal(addrs)
	return string(b)
}

func unmarshalAddrs(s string) []string {
	var addrs []string
	json.Unmarshal([]byte(s), &addrs)
	if addrs == nil {
		return []string{}
	}
	return addrs
}

func (db *DB) SaveMessage(msg *model.Message) error {
	_, err := db.conn.Exec(
		`INSERT INTO messages (id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs,
			subject, content_type, gcs_key, owner_user, folder, seen, received_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		msg.ID, msg.MessageID, msg.From,
		marshalAddrs(msg.To), marshalAddrs(msg.CC), marshalAddrs(msg.BCC),
		msg.Subject, msg.ContentType, msg.GCSKey, msg.OwnerUser, msg.Folder,
		msg.Seen, msg.ReceivedAt,
	)
	return err
}

func (db *DB) ListMessages(ownerEmail, folder string) ([]*model.Message, error) {
	rows, err := db.conn.Query(
		`SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs,
			subject, content_type, gcs_key, owner_user, folder, seen, deleted, received_at
		FROM messages WHERE owner_user = $1 AND folder = $2 AND deleted = FALSE
		ORDER BY received_at DESC`,
		ownerEmail, folder,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*model.Message
	for rows.Next() {
		m := &model.Message{}
		var toJSON, ccJSON, bccJSON string
		var receivedAt time.Time
		err := rows.Scan(
			&m.ID, &m.MessageID, &m.From, &toJSON, &ccJSON, &bccJSON,
			&m.Subject, &m.ContentType, &m.GCSKey, &m.OwnerUser,
			&m.Folder, &m.Seen, &m.Deleted, &receivedAt,
		)
		if err != nil {
			return nil, err
		}
		m.To = unmarshalAddrs(toJSON)
		m.CC = unmarshalAddrs(ccJSON)
		m.BCC = unmarshalAddrs(bccJSON)
		m.ReceivedAt = receivedAt
		messages = append(messages, m)
	}
	return messages, nil
}

func (db *DB) GetMessage(id string) (*model.Message, error) {
	m := &model.Message{}
	var toJSON, ccJSON, bccJSON string
	err := db.conn.QueryRow(
		`SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs,
			subject, content_type, gcs_key, owner_user, folder, seen, deleted, received_at
		FROM messages WHERE id = $1`, id,
	).Scan(
		&m.ID, &m.MessageID, &m.From, &toJSON, &ccJSON, &bccJSON,
		&m.Subject, &m.ContentType, &m.GCSKey, &m.OwnerUser,
		&m.Folder, &m.Seen, &m.Deleted, &m.ReceivedAt,
	)
	if err != nil {
		return nil, err
	}
	m.To = unmarshalAddrs(toJSON)
	m.CC = unmarshalAddrs(ccJSON)
	m.BCC = unmarshalAddrs(bccJSON)
	return m, nil
}

func (db *DB) MarkSeen(id string) error {
	_, err := db.conn.Exec("UPDATE messages SET seen = TRUE WHERE id = $1", id)
	return err
}

func (db *DB) MarkDeleted(id string) error {
	_, err := db.conn.Exec("UPDATE messages SET deleted = TRUE WHERE id = $1", id)
	return err
}

func (db *DB) DeleteMessage(id string) error {
	_, err := db.conn.Exec("DELETE FROM messages WHERE id = $1", id)
	return err
}

func (db *DB) ListAllMessages(ownerEmail string) ([]*model.Message, error) {
	rows, err := db.conn.Query(
		`SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs,
			subject, content_type, gcs_key, owner_user, folder, seen, deleted, received_at
		FROM messages WHERE owner_user = $1 AND deleted = FALSE
		ORDER BY received_at DESC`,
		ownerEmail,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*model.Message
	for rows.Next() {
		m := &model.Message{}
		var toJSON, ccJSON, bccJSON string
		err := rows.Scan(
			&m.ID, &m.MessageID, &m.From, &toJSON, &ccJSON, &bccJSON,
			&m.Subject, &m.ContentType, &m.GCSKey, &m.OwnerUser,
			&m.Folder, &m.Seen, &m.Deleted, &m.ReceivedAt,
		)
		if err != nil {
			return nil, err
		}
		m.To = unmarshalAddrs(toJSON)
		m.CC = unmarshalAddrs(ccJSON)
		m.BCC = unmarshalAddrs(bccJSON)
		messages = append(messages, m)
	}
	return messages, nil
}
