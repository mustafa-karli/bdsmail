package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/mustafakarli/bdsmail/internal/model"
)

// Query name constants for the named query map.
const (
	QCreateUser      = "create_user"
	QGetUser         = "get_user"
	QUserExists      = "user_exists"
	QSaveMessage     = "save_message"
	QListMessages    = "list_messages"
	QListAllMessages = "list_all_messages"
	QGetMessage      = "get_message"
	QMarkSeen        = "mark_seen"
	QMarkDeleted     = "mark_deleted"
	QDeleteMessage   = "delete_message"
)

// Database is the interface all database backends implement.
type Database interface {
	Close() error
	GetQueries() map[string]string
	CreateUser(username, domain, displayName, passwordHash string) error
	GetUser(username, domain string) (*model.User, error)
	GetUserByEmail(email string) (*model.User, error)
	UserExistsByEmail(email string) bool
	SaveMessage(msg *model.Message) error
	ListMessages(ownerEmail, folder string) ([]*model.Message, error)
	ListAllMessages(ownerEmail string) ([]*model.Message, error)
	GetMessage(id string) (*model.Message, error)
	MarkSeen(id string) error
	MarkDeleted(id string) error
	DeleteMessage(id string) error
}

// NoSQL document structs — shared by DynamoDB and Firestore via dual tags.
type docUser struct {
	Email        string `dynamodbav:"email" firestore:"email"`
	Username     string `dynamodbav:"username" firestore:"username"`
	Domain       string `dynamodbav:"domain" firestore:"domain"`
	DisplayName  string `dynamodbav:"display_name" firestore:"display_name"`
	PasswordHash string `dynamodbav:"password_hash" firestore:"password_hash"`
	CreatedAt    string `dynamodbav:"created_at" firestore:"created_at"`
}

type docMessage struct {
	ID          string `dynamodbav:"id" firestore:"id"`
	OwnerUser   string `dynamodbav:"owner_user" firestore:"owner_user"`
	SortKey     string `dynamodbav:"sort_key" firestore:"sort_key"` // DynamoDB range key: received_at#id
	MessageID   string `dynamodbav:"message_id" firestore:"message_id"`
	From        string `dynamodbav:"from_addr" firestore:"from_addr"`
	ToAddrs     string `dynamodbav:"to_addrs" firestore:"to_addrs"`
	CCAddrs     string `dynamodbav:"cc_addrs" firestore:"cc_addrs"`
	BCCAddrs    string `dynamodbav:"bcc_addrs" firestore:"bcc_addrs"`
	Subject     string `dynamodbav:"subject" firestore:"subject"`
	ContentType string `dynamodbav:"content_type" firestore:"content_type"`
	Body        string `dynamodbav:"body" firestore:"body"`
	Attachments string `dynamodbav:"attachments" firestore:"attachments"` // JSON array of Attachment
	GCSKey      string `dynamodbav:"gcs_key" firestore:"gcs_key"`
	Folder      string `dynamodbav:"folder" firestore:"folder"`
	Seen        bool   `dynamodbav:"seen" firestore:"seen"`
	Deleted     bool   `dynamodbav:"deleted" firestore:"deleted"`
	ReceivedAt  string `dynamodbav:"received_at" firestore:"received_at"`
}

func (b *DbBase) docUserToModel(du *docUser) *model.User {
	createdAt, _ := time.Parse(time.RFC3339, du.CreatedAt)
	return &model.User{
		Username:     du.Username,
		Domain:       du.Domain,
		DisplayName:  du.DisplayName,
		PasswordHash: du.PasswordHash,
		CreatedAt:    createdAt,
	}
}

func (b *DbBase) docMessageToModel(dm *docMessage) *model.Message {
	receivedAt, _ := time.Parse(time.RFC3339, dm.ReceivedAt)
	var attachments []model.Attachment
	json.Unmarshal([]byte(dm.Attachments), &attachments)
	return &model.Message{
		ID:          dm.ID,
		MessageID:   dm.MessageID,
		From:        dm.From,
		To:          b.UnmarshalAddrs(dm.ToAddrs),
		CC:          b.UnmarshalAddrs(dm.CCAddrs),
		BCC:         b.UnmarshalAddrs(dm.BCCAddrs),
		Subject:     dm.Subject,
		ContentType: dm.ContentType,
		Body:        dm.Body,
		Attachments: attachments,
		GCSKey:      dm.GCSKey,
		OwnerUser:   dm.OwnerUser,
		Folder:      dm.Folder,
		Seen:        dm.Seen,
		Deleted:     dm.Deleted,
		ReceivedAt:  receivedAt,
	}
}

func (b *DbBase) MarshalAttachments(attachments []model.Attachment) string {
	if attachments == nil {
		attachments = []model.Attachment{}
	}
	data, _ := json.Marshal(attachments)
	return string(data)
}

func (b *DbBase) UnmarshalAttachments(s string) []model.Attachment {
	var attachments []model.Attachment
	json.Unmarshal([]byte(s), &attachments)
	return attachments
}

// DbBase contains shared helper methods used by all database backends.
type DbBase struct{}

func (b *DbBase) MarshalAddrs(addrs []string) string {
	if addrs == nil {
		addrs = []string{}
	}
	data, _ := json.Marshal(addrs)
	return string(data)
}

func (b *DbBase) UnmarshalAddrs(s string) []string {
	var addrs []string
	json.Unmarshal([]byte(s), &addrs)
	if addrs == nil {
		return []string{}
	}
	return addrs
}

// DbSQL is a shared base for SQL-based backends (PostgreSQL, SQLite).
// It implements all Database methods using named queries from GetQueries().
type DbSQL struct {
	DbBase
	Conn    *sql.DB
	Queries map[string]string
	// FormatBool converts a Go bool to the DB-specific value (bool for PG, int for SQLite).
	FormatBool func(bool) interface{}
	// FormatTime converts a Go time to the DB-specific value (time.Time for PG, string for SQLite).
	FormatTime func(time.Time) interface{}
}

func (db *DbSQL) GetQueries() map[string]string {
	return db.Queries
}

func (db *DbSQL) Close() error {
	return db.Conn.Close()
}

func (db *DbSQL) Migrate(migrationQueries []string) error {
	for _, q := range migrationQueries {
		if _, err := db.Conn.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// User operations

func (db *DbSQL) CreateUser(username, domain, displayName, passwordHash string) error {
	_, err := db.Conn.Exec(db.Queries[QCreateUser], username, domain, displayName, passwordHash)
	return err
}

func (db *DbSQL) GetUser(username, domain string) (*model.User, error) {
	u := &model.User{}
	var createdAt interface{}
	err := db.Conn.QueryRow(db.Queries[QGetUser], username, domain).
		Scan(&u.ID, &u.Username, &u.Domain, &u.DisplayName, &u.PasswordHash, &createdAt)
	if err != nil {
		return nil, err
	}
	u.CreatedAt = scanTime(createdAt)
	return u, nil
}

func (db *DbSQL) GetUserByEmail(email string) (*model.User, error) {
	username, domain := SplitEmail(email)
	return db.GetUser(username, domain)
}

func (db *DbSQL) UserExistsByEmail(email string) bool {
	var count int
	username, domain := SplitEmail(email)
	db.Conn.QueryRow(db.Queries[QUserExists], username, domain).Scan(&count)
	return count > 0
}

// Message operations

func (db *DbSQL) SaveMessage(msg *model.Message) error {
	_, err := db.Conn.Exec(db.Queries[QSaveMessage],
		msg.ID, msg.MessageID, msg.From,
		db.MarshalAddrs(msg.To), db.MarshalAddrs(msg.CC), db.MarshalAddrs(msg.BCC),
		msg.Subject, msg.ContentType, msg.Body, db.MarshalAttachments(msg.Attachments),
		msg.GCSKey, msg.OwnerUser, msg.Folder,
		db.FormatBool(msg.Seen), db.FormatTime(msg.ReceivedAt),
	)
	return err
}

func (db *DbSQL) ListMessages(ownerEmail, folder string) ([]*model.Message, error) {
	rows, err := db.Conn.Query(db.Queries[QListMessages], ownerEmail, folder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return db.scanMessages(rows)
}

func (db *DbSQL) ListAllMessages(ownerEmail string) ([]*model.Message, error) {
	rows, err := db.Conn.Query(db.Queries[QListAllMessages], ownerEmail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return db.scanMessages(rows)
}

func (db *DbSQL) GetMessage(id string) (*model.Message, error) {
	m := &model.Message{}
	var toJSON, ccJSON, bccJSON, attachJSON string
	var seen, deleted, receivedAt interface{}
	err := db.Conn.QueryRow(db.Queries[QGetMessage], id).Scan(
		&m.ID, &m.MessageID, &m.From, &toJSON, &ccJSON, &bccJSON,
		&m.Subject, &m.ContentType, &m.Body, &attachJSON, &m.GCSKey, &m.OwnerUser,
		&m.Folder, &seen, &deleted, &receivedAt,
	)
	if err != nil {
		return nil, err
	}
	m.To = db.UnmarshalAddrs(toJSON)
	m.CC = db.UnmarshalAddrs(ccJSON)
	m.BCC = db.UnmarshalAddrs(bccJSON)
	m.Attachments = db.UnmarshalAttachments(attachJSON)
	m.Seen = scanBool(seen)
	m.Deleted = scanBool(deleted)
	m.ReceivedAt = scanTime(receivedAt)
	return m, nil
}

func (db *DbSQL) MarkSeen(id string) error {
	_, err := db.Conn.Exec(db.Queries[QMarkSeen], id)
	return err
}

func (db *DbSQL) MarkDeleted(id string) error {
	_, err := db.Conn.Exec(db.Queries[QMarkDeleted], id)
	return err
}

func (db *DbSQL) DeleteMessage(id string) error {
	_, err := db.Conn.Exec(db.Queries[QDeleteMessage], id)
	return err
}

func (db *DbSQL) scanMessages(rows *sql.Rows) ([]*model.Message, error) {
	var messages []*model.Message
	for rows.Next() {
		m := &model.Message{}
		var toJSON, ccJSON, bccJSON, attachJSON string
		var seen, deleted, receivedAt interface{}
		err := rows.Scan(
			&m.ID, &m.MessageID, &m.From, &toJSON, &ccJSON, &bccJSON,
			&m.Subject, &m.ContentType, &m.Body, &attachJSON, &m.GCSKey, &m.OwnerUser,
			&m.Folder, &seen, &deleted, &receivedAt,
		)
		if err != nil {
			return nil, err
		}
		m.To = db.UnmarshalAddrs(toJSON)
		m.CC = db.UnmarshalAddrs(ccJSON)
		m.BCC = db.UnmarshalAddrs(bccJSON)
		m.Attachments = db.UnmarshalAttachments(attachJSON)
		m.Seen = scanBool(seen)
		m.Deleted = scanBool(deleted)
		m.ReceivedAt = scanTime(receivedAt)
		messages = append(messages, m)
	}
	return messages, nil
}

// scanBool converts a DB-scanned value to bool (handles bool from PG, int64 from SQLite).
func scanBool(v interface{}) bool {
	switch b := v.(type) {
	case bool:
		return b
	case int64:
		return b != 0
	}
	return false
}

// scanTime converts a DB-scanned value to time.Time (handles time.Time from PG, string from SQLite).
func scanTime(v interface{}) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t
	case string:
		parsed, _ := time.Parse(time.RFC3339, t)
		if parsed.IsZero() {
			parsed, _ = time.Parse("2006-01-02 15:04:05", t)
		}
		return parsed
	}
	return time.Time{}
}

// SplitEmail splits an email address into username and domain parts.
func SplitEmail(email string) (string, string) {
	for i, c := range email {
		if c == '@' {
			return email[:i], email[i+1:]
		}
	}
	return email, ""
}
