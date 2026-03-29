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

	// User provisioning
	QListUsers         = "list_users"
	QListUsersByDomain = "list_users_by_domain"
	QUpdateUser        = "update_user"
	QDeleteUser        = "delete_user"
	QDeleteUserMessages = "delete_user_messages"

	// Aliases
	QCreateAlias = "create_alias"
	QGetAlias    = "get_alias"
	QListAliases = "list_aliases"
	QUpdateAlias = "update_alias"
	QDeleteAlias = "delete_alias"
	QGetCatchAll = "get_catch_all"

	// Mailing lists
	QCreateMailingList = "create_mailing_list"
	QGetMailingList    = "get_mailing_list"
	QListMailingLists  = "list_mailing_lists"
	QDeleteMailingList = "delete_mailing_list"
	QAddListMember     = "add_list_member"
	QRemoveListMember  = "remove_list_member"
	QGetListMembers    = "get_list_members"
	QIsMailingList     = "is_mailing_list"
	QDeleteListMembers = "delete_list_members"

	// Filters
	QCreateFilter = "create_filter"
	QListFilters  = "list_filters"
	QUpdateFilter = "update_filter"
	QDeleteFilter = "delete_filter"
	QListUserFolders = "list_user_folders"

	// Auto-reply
	QSetAutoReply          = "set_auto_reply"
	QGetAutoReply          = "get_auto_reply"
	QDeleteAutoReply       = "delete_auto_reply"
	QRecordAutoReplySent   = "record_auto_reply_sent"
	QHasAutoRepliedRecently = "has_auto_replied_recently"

	// Search
	QSearchMessages = "search_messages"

	// Contacts
	QCreateContact = "create_contact"
	QGetContact    = "get_contact"
	QListContacts  = "list_contacts"
	QUpdateContact = "update_contact"
	QDeleteContact = "delete_contact"
)

// Database is the interface all database backends implement.
type Database interface {
	Close() error
	GetQueries() map[string]string

	// User operations
	CreateUser(username, domain, displayName, passwordHash string) error
	GetUser(username, domain string) (*model.User, error)
	GetUserByEmail(email string) (*model.User, error)
	UserExistsByEmail(email string) bool
	ListUsers() ([]*model.User, error)
	ListUsersByDomain(domain string) ([]*model.User, error)
	UpdateUser(email, displayName, passwordHash string) error
	DeleteUser(email string) error

	// Message operations
	SaveMessage(msg *model.Message) error
	ListMessages(ownerEmail, folder string) ([]*model.Message, error)
	ListAllMessages(ownerEmail string) ([]*model.Message, error)
	GetMessage(id string) (*model.Message, error)
	MarkSeen(id string) error
	MarkDeleted(id string) error
	DeleteMessage(id string) error
	DeleteUserMessages(email string) error
	SearchMessages(ownerEmail, query string) ([]*model.Message, error)
	ListUserFolders(ownerEmail string) ([]string, error)

	// Alias operations
	CreateAlias(aliasEmail string, targetEmails []string) error
	GetAlias(aliasEmail string) ([]string, error)
	ListAliases() ([]*model.Alias, error)
	UpdateAlias(aliasEmail string, targetEmails []string) error
	DeleteAlias(aliasEmail string) error
	GetCatchAll(domain string) ([]string, error)

	// Mailing list operations
	CreateMailingList(listAddr, name, description, ownerEmail string) error
	GetMailingList(listAddr string) (*model.MailingList, error)
	ListMailingLists() ([]*model.MailingList, error)
	DeleteMailingList(listAddr string) error
	AddListMember(listAddr, memberEmail string) error
	RemoveListMember(listAddr, memberEmail string) error
	GetListMembers(listAddr string) ([]string, error)
	IsMailingList(email string) bool

	// Filter operations
	CreateFilter(filter *model.Filter) error
	ListFilters(userEmail string) ([]*model.Filter, error)
	UpdateFilter(filter *model.Filter) error
	DeleteFilter(id string) error

	// Auto-reply operations
	SetAutoReply(reply *model.AutoReply) error
	GetAutoReply(userEmail string) (*model.AutoReply, error)
	DeleteAutoReply(userEmail string) error
	RecordAutoReplySent(userEmail, senderEmail string) error
	HasAutoRepliedRecently(userEmail, senderEmail string, cooldown time.Duration) bool

	// Contact operations
	CreateContact(contact *model.Contact) error
	GetContact(id string) (*model.Contact, error)
	ListContacts(ownerEmail string) ([]*model.Contact, error)
	UpdateContact(contact *model.Contact) error
	DeleteContact(id string) error
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

// --- User provisioning ---

func (db *DbSQL) ListUsers() ([]*model.User, error) {
	rows, err := db.Conn.Query(db.Queries[QListUsers])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return db.scanUsers(rows)
}

func (db *DbSQL) ListUsersByDomain(domain string) ([]*model.User, error) {
	rows, err := db.Conn.Query(db.Queries[QListUsersByDomain], domain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return db.scanUsers(rows)
}

func (db *DbSQL) UpdateUser(email, displayName, passwordHash string) error {
	username, domain := SplitEmail(email)
	if passwordHash == "" {
		_, err := db.Conn.Exec("UPDATE users SET display_name = ? WHERE username = ? AND domain = ?", displayName, username, domain)
		return err
	}
	_, err := db.Conn.Exec(db.Queries[QUpdateUser], displayName, passwordHash, username, domain)
	return err
}

func (db *DbSQL) DeleteUser(email string) error {
	username, domain := SplitEmail(email)
	_, err := db.Conn.Exec(db.Queries[QDeleteUser], username, domain)
	return err
}

func (db *DbSQL) DeleteUserMessages(email string) error {
	_, err := db.Conn.Exec(db.Queries[QDeleteUserMessages], email)
	return err
}

func (db *DbSQL) scanUsers(rows *sql.Rows) ([]*model.User, error) {
	var users []*model.User
	for rows.Next() {
		u := &model.User{}
		var createdAt interface{}
		err := rows.Scan(&u.ID, &u.Username, &u.Domain, &u.DisplayName, &u.PasswordHash, &createdAt)
		if err != nil {
			return nil, err
		}
		u.CreatedAt = scanTime(createdAt)
		users = append(users, u)
	}
	return users, nil
}

// --- Alias operations ---

func (db *DbSQL) CreateAlias(aliasEmail string, targetEmails []string) error {
	isCatchAll := len(aliasEmail) > 0 && aliasEmail[0] == '@'
	targetsJSON, _ := json.Marshal(targetEmails)
	_, err := db.Conn.Exec(db.Queries[QCreateAlias], aliasEmail, string(targetsJSON), db.FormatBool(isCatchAll))
	return err
}

func (db *DbSQL) GetAlias(aliasEmail string) ([]string, error) {
	var targetsJSON string
	err := db.Conn.QueryRow(db.Queries[QGetAlias], aliasEmail).Scan(&targetsJSON)
	if err != nil {
		return nil, err
	}
	var targets []string
	json.Unmarshal([]byte(targetsJSON), &targets)
	return targets, nil
}

func (db *DbSQL) ListAliases() ([]*model.Alias, error) {
	rows, err := db.Conn.Query(db.Queries[QListAliases])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var aliases []*model.Alias
	for rows.Next() {
		a := &model.Alias{}
		var targetsJSON string
		var isCatchAll interface{}
		if err := rows.Scan(&a.AliasEmail, &targetsJSON, &isCatchAll); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(targetsJSON), &a.TargetEmails)
		a.IsCatchAll = scanBool(isCatchAll)
		aliases = append(aliases, a)
	}
	return aliases, nil
}

func (db *DbSQL) UpdateAlias(aliasEmail string, targetEmails []string) error {
	targetsJSON, _ := json.Marshal(targetEmails)
	_, err := db.Conn.Exec(db.Queries[QUpdateAlias], string(targetsJSON), aliasEmail)
	return err
}

func (db *DbSQL) DeleteAlias(aliasEmail string) error {
	_, err := db.Conn.Exec(db.Queries[QDeleteAlias], aliasEmail)
	return err
}

func (db *DbSQL) GetCatchAll(domain string) ([]string, error) {
	return db.GetAlias("@" + domain)
}

// --- Mailing list operations ---

func (db *DbSQL) CreateMailingList(listAddr, name, description, ownerEmail string) error {
	_, err := db.Conn.Exec(db.Queries[QCreateMailingList], listAddr, name, description, ownerEmail)
	return err
}

func (db *DbSQL) GetMailingList(listAddr string) (*model.MailingList, error) {
	ml := &model.MailingList{}
	var createdAt interface{}
	err := db.Conn.QueryRow(db.Queries[QGetMailingList], listAddr).Scan(
		&ml.ListAddress, &ml.Name, &ml.Description, &ml.OwnerEmail, &createdAt)
	if err != nil {
		return nil, err
	}
	ml.CreatedAt = scanTime(createdAt)
	return ml, nil
}

func (db *DbSQL) ListMailingLists() ([]*model.MailingList, error) {
	rows, err := db.Conn.Query(db.Queries[QListMailingLists])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lists []*model.MailingList
	for rows.Next() {
		ml := &model.MailingList{}
		var createdAt interface{}
		if err := rows.Scan(&ml.ListAddress, &ml.Name, &ml.Description, &ml.OwnerEmail, &createdAt); err != nil {
			return nil, err
		}
		ml.CreatedAt = scanTime(createdAt)
		lists = append(lists, ml)
	}
	return lists, nil
}

func (db *DbSQL) DeleteMailingList(listAddr string) error {
	db.Conn.Exec(db.Queries[QDeleteListMembers], listAddr)
	_, err := db.Conn.Exec(db.Queries[QDeleteMailingList], listAddr)
	return err
}

func (db *DbSQL) AddListMember(listAddr, memberEmail string) error {
	_, err := db.Conn.Exec(db.Queries[QAddListMember], listAddr, memberEmail)
	return err
}

func (db *DbSQL) RemoveListMember(listAddr, memberEmail string) error {
	_, err := db.Conn.Exec(db.Queries[QRemoveListMember], listAddr, memberEmail)
	return err
}

func (db *DbSQL) GetListMembers(listAddr string) ([]string, error) {
	rows, err := db.Conn.Query(db.Queries[QGetListMembers], listAddr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		members = append(members, email)
	}
	return members, nil
}

func (db *DbSQL) IsMailingList(email string) bool {
	var count int
	db.Conn.QueryRow(db.Queries[QIsMailingList], email).Scan(&count)
	return count > 0
}

// --- Filter operations ---

func (db *DbSQL) CreateFilter(filter *model.Filter) error {
	condJSON, _ := json.Marshal(filter.Conditions)
	actJSON, _ := json.Marshal(filter.Actions)
	_, err := db.Conn.Exec(db.Queries[QCreateFilter],
		filter.ID, filter.UserEmail, filter.Name, filter.Priority,
		string(condJSON), string(actJSON), db.FormatBool(filter.Enabled))
	return err
}

func (db *DbSQL) ListFilters(userEmail string) ([]*model.Filter, error) {
	rows, err := db.Conn.Query(db.Queries[QListFilters], userEmail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var filters []*model.Filter
	for rows.Next() {
		f := &model.Filter{}
		var condJSON, actJSON string
		var enabled interface{}
		if err := rows.Scan(&f.ID, &f.UserEmail, &f.Name, &f.Priority, &condJSON, &actJSON, &enabled); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(condJSON), &f.Conditions)
		json.Unmarshal([]byte(actJSON), &f.Actions)
		f.Enabled = scanBool(enabled)
		filters = append(filters, f)
	}
	return filters, nil
}

func (db *DbSQL) UpdateFilter(filter *model.Filter) error {
	condJSON, _ := json.Marshal(filter.Conditions)
	actJSON, _ := json.Marshal(filter.Actions)
	_, err := db.Conn.Exec(db.Queries[QUpdateFilter],
		filter.Name, filter.Priority, string(condJSON), string(actJSON), db.FormatBool(filter.Enabled), filter.ID)
	return err
}

func (db *DbSQL) DeleteFilter(id string) error {
	_, err := db.Conn.Exec(db.Queries[QDeleteFilter], id)
	return err
}

func (db *DbSQL) ListUserFolders(ownerEmail string) ([]string, error) {
	rows, err := db.Conn.Query(db.Queries[QListUserFolders], ownerEmail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var folders []string
	for rows.Next() {
		var folder string
		if err := rows.Scan(&folder); err != nil {
			return nil, err
		}
		folders = append(folders, folder)
	}
	return folders, nil
}

// --- Auto-reply operations ---

func (db *DbSQL) SetAutoReply(reply *model.AutoReply) error {
	_, err := db.Conn.Exec(db.Queries[QSetAutoReply],
		reply.UserEmail, db.FormatBool(reply.Enabled), reply.Subject, reply.Body,
		db.FormatTime(reply.StartDate), db.FormatTime(reply.EndDate))
	return err
}

func (db *DbSQL) GetAutoReply(userEmail string) (*model.AutoReply, error) {
	r := &model.AutoReply{}
	var enabled, startDate, endDate interface{}
	err := db.Conn.QueryRow(db.Queries[QGetAutoReply], userEmail).Scan(
		&r.UserEmail, &enabled, &r.Subject, &r.Body, &startDate, &endDate)
	if err != nil {
		return nil, err
	}
	r.Enabled = scanBool(enabled)
	r.StartDate = scanTime(startDate)
	r.EndDate = scanTime(endDate)
	return r, nil
}

func (db *DbSQL) DeleteAutoReply(userEmail string) error {
	_, err := db.Conn.Exec(db.Queries[QDeleteAutoReply], userEmail)
	return err
}

func (db *DbSQL) RecordAutoReplySent(userEmail, senderEmail string) error {
	_, err := db.Conn.Exec(db.Queries[QRecordAutoReplySent], userEmail, senderEmail)
	return err
}

func (db *DbSQL) HasAutoRepliedRecently(userEmail, senderEmail string, cooldown time.Duration) bool {
	var count int
	cutoff := time.Now().Add(-cooldown)
	db.Conn.QueryRow(db.Queries[QHasAutoRepliedRecently], userEmail, senderEmail, db.FormatTime(cutoff)).Scan(&count)
	return count > 0
}

// --- Search operations ---

func (db *DbSQL) SearchMessages(ownerEmail, query string) ([]*model.Message, error) {
	rows, err := db.Conn.Query(db.Queries[QSearchMessages], ownerEmail, "%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return db.scanMessages(rows)
}

// --- Contact operations ---

func (db *DbSQL) CreateContact(contact *model.Contact) error {
	_, err := db.Conn.Exec(db.Queries[QCreateContact],
		contact.ID, contact.OwnerEmail, contact.VCardData, contact.ETag)
	return err
}

func (db *DbSQL) GetContact(id string) (*model.Contact, error) {
	c := &model.Contact{}
	var createdAt, updatedAt interface{}
	err := db.Conn.QueryRow(db.Queries[QGetContact], id).Scan(
		&c.ID, &c.OwnerEmail, &c.VCardData, &c.ETag, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	c.CreatedAt = scanTime(createdAt)
	c.UpdatedAt = scanTime(updatedAt)
	return c, nil
}

func (db *DbSQL) ListContacts(ownerEmail string) ([]*model.Contact, error) {
	rows, err := db.Conn.Query(db.Queries[QListContacts], ownerEmail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var contacts []*model.Contact
	for rows.Next() {
		c := &model.Contact{}
		var createdAt, updatedAt interface{}
		if err := rows.Scan(&c.ID, &c.OwnerEmail, &c.VCardData, &c.ETag, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		c.CreatedAt = scanTime(createdAt)
		c.UpdatedAt = scanTime(updatedAt)
		contacts = append(contacts, c)
	}
	return contacts, nil
}

func (db *DbSQL) UpdateContact(contact *model.Contact) error {
	_, err := db.Conn.Exec(db.Queries[QUpdateContact],
		contact.VCardData, contact.ETag, contact.ID)
	return err
}

func (db *DbSQL) DeleteContact(id string) error {
	_, err := db.Conn.Exec(db.Queries[QDeleteContact], id)
	return err
}
