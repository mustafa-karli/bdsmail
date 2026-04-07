package store

import (
	"database/sql"
	"encoding/json"
	"log"
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

	// Unread count
	QCountUnread = "count_unread"

	// Search
	QSearchMessages = "search_messages"

	// Contacts
	QCreateContact = "create_contact"
	QGetContact    = "get_contact"
	QListContacts  = "list_contacts"
	QUpdateContact = "update_contact"
	QDeleteContact = "delete_contact"

	// Attachments
	QSaveAttachment       = "save_attachment"
	QListAttachments      = "list_attachments"
	QGetAttachment        = "get_attachment"
	QDeleteAttachmentsByMsg = "delete_attachments_by_msg"

	// Signup
	QCreateSignup = "create_signup"
	QGetSignup    = "get_signup"
	QDeleteSignup = "delete_signup"

	// Auth / 2FA
	QEnable2FA          = "enable_2fa"
	QDisable2FA         = "disable_2fa"
	QGet2FAStatus       = "get_2fa_status"
	QCreateTrustedDevice = "create_trusted_device"
	QIsTrustedDevice    = "is_trusted_device"
	QListTrustedDevices = "list_trusted_devices"
	QRevokeTrustedDevice = "revoke_trusted_device"
	QUpdateDeviceLastSeen = "update_device_last_seen"
	QCreateOTP          = "create_otp"
	QGetOTP             = "get_otp"
	QIncrementOTPAttempts = "increment_otp_attempts"
	QClearOTP           = "clear_otp"
	QCreateLoginToken   = "create_login_token"
	QGetLoginToken      = "get_login_token"
	QDeleteLoginToken   = "delete_login_token"

	// Domain
	QCreateDomain       = "create_domain"
	QGetDomain          = "get_domain"
	QListDomains        = "list_domains"
	QUpdateDomainStatus = "update_domain_status"
	QDeleteDomain       = "delete_domain"

	// OAuth
	QCreateOAuthClient     = "create_oauth_client"
	QGetOAuthClient        = "get_oauth_client"
	QListOAuthClients      = "list_oauth_clients"
	QDeleteOAuthClient     = "delete_oauth_client"
	QCreateOAuthCode       = "create_oauth_code"
	QGetOAuthCode          = "get_oauth_code"
	QMarkOAuthCodeUsed     = "mark_oauth_code_used"
	QCreateOAuthToken      = "create_oauth_token"
	QGetOAuthToken         = "get_oauth_token"
	QDeleteOAuthToken      = "delete_oauth_token"
	QDeleteExpiredOAuthCodes  = "delete_expired_oauth_codes"
	QDeleteExpiredOAuthTokens = "delete_expired_oauth_tokens"
)

// Focused sub-interfaces following Interface Segregation Principle.

type UserStore interface {
	CreateUser(username, domain, displayName, passwordHash string) error
	GetUser(username, domain string) (*model.User, error)
	GetUserByEmail(email string) (*model.User, error)
	UserExistsByEmail(email string) bool
	ListUsers() ([]*model.User, error)
	ListUsersByDomain(domain string) ([]*model.User, error)
	UpdateUser(email, displayName, passwordHash string) error
	DeleteUser(email string) error
}

type MessageStore interface {
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
	CountUnread(ownerEmail, folder string) int
}

type AliasStore interface {
	CreateAlias(aliasEmail string, targetEmails []string) error
	GetAlias(aliasEmail string) ([]string, error)
	ListAliases() ([]*model.Alias, error)
	UpdateAlias(aliasEmail string, targetEmails []string) error
	DeleteAlias(aliasEmail string) error
	GetCatchAll(domain string) ([]string, error)
}

type MailingListStore interface {
	CreateMailingList(listAddr, name, description, ownerEmail string) error
	GetMailingList(listAddr string) (*model.MailingList, error)
	ListMailingLists() ([]*model.MailingList, error)
	DeleteMailingList(listAddr string) error
	AddListMember(listAddr, memberEmail string) error
	RemoveListMember(listAddr, memberEmail string) error
	GetListMembers(listAddr string) ([]string, error)
	IsMailingList(email string) bool
}

type FilterStore interface {
	CreateFilter(filter *model.Filter) error
	ListFilters(userEmail string) ([]*model.Filter, error)
	UpdateFilter(filter *model.Filter) error
	DeleteFilter(id string) error
}

type AutoReplyStore interface {
	SetAutoReply(reply *model.AutoReply) error
	GetAutoReply(userEmail string) (*model.AutoReply, error)
	DeleteAutoReply(userEmail string) error
	RecordAutoReplySent(userEmail, senderEmail string) error
	HasAutoRepliedRecently(userEmail, senderEmail string, cooldown time.Duration) bool
}

type ContactStore interface {
	CreateContact(contact *model.Contact) error
	GetContact(id string) (*model.Contact, error)
	ListContacts(ownerEmail string) ([]*model.Contact, error)
	UpdateContact(contact *model.Contact) error
	DeleteContact(id string) error
}

type SignupStore interface {
	CreateSignup(signup *model.DomainSignup) error
	GetSignup(id string) (*model.DomainSignup, error)
	DeleteSignup(id string) error
}

type AttachmentStore interface {
	SaveAttachment(att *model.Attachment) error
	ListAttachments(mailContentID string) ([]model.Attachment, error)
	GetAttachment(id string) (*model.Attachment, error)
	DeleteAttachmentsByMessage(mailContentID string) error
}

type AuthStore interface {
	Enable2FA(email, secret, backupCodes string) error
	Disable2FA(email string) error
	Get2FAStatus(email string) (enabled bool, secret string, backupCodes string, err error)
	CreateTrustedDevice(device *model.TrustedDevice) error
	IsTrustedDevice(email, fingerprint string) (bool, error)
	ListTrustedDevices(email string) ([]*model.TrustedDevice, error)
	RevokeTrustedDevice(id string) error
	UpdateDeviceLastSeen(id string) error
	CreateOTP(otp *model.OTP) error
	GetOTP(email string) (*model.OTP, error)
	IncrementOTPAttempts(email string) error
	ClearOTP(email string) error
	CreateLoginToken(token *model.LoginToken) error
	GetLoginToken(token string) (*model.LoginToken, error)
	DeleteLoginToken(token string) error
}

type DomainStore interface {
	CreateDomain(domain *model.Domain) error
	GetDomain(name string) (*model.Domain, error)
	ListDomains() ([]*model.Domain, error)
	UpdateDomainStatus(name, sesStatus, dkimStatus string) error
	DeleteDomain(name string) error
}

type OAuthStore interface {
	CreateOAuthClient(client *model.OAuthClient) error
	GetOAuthClient(clientID string) (*model.OAuthClient, error)
	ListOAuthClients(domain string) ([]*model.OAuthClient, error)
	DeleteOAuthClient(id string) error
	CreateOAuthCode(code *model.OAuthCode) error
	GetOAuthCode(code string) (*model.OAuthCode, error)
	MarkOAuthCodeUsed(code string) error
	CreateOAuthToken(token *model.OAuthToken) error
	GetOAuthToken(token string) (*model.OAuthToken, error)
	DeleteOAuthToken(token string) error
}

// Database composes all sub-interfaces. Backends implement the full interface.
type Database interface {
	Close() error
	GetQueries() map[string]string
	UserStore
	MessageStore
	AliasStore
	MailingListStore
	FilterStore
	AutoReplyStore
	ContactStore
	SignupStore
	AttachmentStore
	AuthStore
	DomainStore
	OAuthStore
}

// NoSQL document structs — shared by DynamoDB and Firestore via dual tags.
type docUser struct {
	Email         string `dynamodbav:"email" firestore:"email"`
	Username      string `dynamodbav:"username" firestore:"username"`
	Domain        string `dynamodbav:"domain" firestore:"domain"`
	DisplayName   string `dynamodbav:"display_name" firestore:"display_name"`
	PasswordHash  string `dynamodbav:"password_hash" firestore:"password_hash"`
	Phone         string `dynamodbav:"phone" firestore:"phone"`
	ExternalEmail string `dynamodbav:"external_email" firestore:"external_email"`
	Status        string `dynamodbav:"status" firestore:"status"`
	TwoFAEnabled  bool   `dynamodbav:"twofa_enabled" firestore:"twofa_enabled"`
	TwoFASecret   string `dynamodbav:"twofa_secret" firestore:"twofa_secret"`
	TwoFABackupCodes string `dynamodbav:"twofa_backup_codes" firestore:"twofa_backup_codes"`
	LoginAttempts int    `dynamodbav:"login_attempts" firestore:"login_attempts"`
	CreatedAt     string `dynamodbav:"created_at" firestore:"created_at"`
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
	GCSKey      string `dynamodbav:"gcs_key" firestore:"gcs_key"`
	Folder      string `dynamodbav:"folder" firestore:"folder"`
	Seen        bool   `dynamodbav:"seen" firestore:"seen"`
	Deleted     bool   `dynamodbav:"deleted" firestore:"deleted"`
	ReceivedAt  string `dynamodbav:"received_at" firestore:"received_at"`
}

func (b *DbBase) docUserToModel(du *docUser) *model.User {
	createdAt, _ := time.Parse(time.RFC3339, du.CreatedAt)
	return &model.User{
		ID:               du.Email,
		Username:         du.Username,
		Domain:           du.Domain,
		DisplayName:      du.DisplayName,
		PasswordHash:     du.PasswordHash,
		Phone:            du.Phone,
		ExternalEmail:    du.ExternalEmail,
		Status:           du.Status,
		TwoFAEnabled:     du.TwoFAEnabled,
		TwoFASecret:      du.TwoFASecret,
		TwoFABackupCodes: du.TwoFABackupCodes,
		LoginAttempts:    du.LoginAttempts,
		CreatedAt:        createdAt,
	}
}

func (b *DbBase) docMessageToModel(dm *docMessage) *model.Message {
	receivedAt, _ := time.Parse(time.RFC3339, dm.ReceivedAt)
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
		GCSKey:      dm.GCSKey,
		OwnerUser:   dm.OwnerUser,
		Folder:      dm.Folder,
		Seen:        dm.Seen,
		Deleted:     dm.Deleted,
		ReceivedAt:  receivedAt,
	}
}

// DbBase contains shared helper methods used by all database backends.
type DbBase struct{}

func (b *DbBase) MarshalJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("json marshal error: %v", err)
		return "[]"
	}
	return string(data)
}

func (b *DbBase) MarshalAddrs(addrs []string) string {
	if addrs == nil {
		addrs = []string{}
	}
	return b.MarshalJSON(addrs)
}

func (b *DbBase) UnmarshalAddrs(s string) []string {
	var addrs []string
	if err := json.Unmarshal([]byte(s), &addrs); err != nil {
		log.Printf("unmarshal addrs error: %v", err)
	}
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

// User operations

func (db *DbSQL) CreateUser(username, domain, displayName, passwordHash string) error {
	id := username + "@" + domain
	_, err := db.Conn.Exec(db.Queries[QCreateUser], id, username, domain, displayName, passwordHash)
	return err
}

func (db *DbSQL) GetUser(username, domain string) (*model.User, error) {
	u := &model.User{}
	var createdAt, lastLoginAttempt, twofaEnabled interface{}
	err := db.Conn.QueryRow(db.Queries[QGetUser], username, domain).
		Scan(&u.ID, &u.Username, &u.Domain, &u.DisplayName, &u.PasswordHash,
			&u.Phone, &u.ExternalEmail, &u.Status, &twofaEnabled,
			&u.TwoFASecret, &u.TwoFABackupCodes,
			&u.LoginAttempts, &lastLoginAttempt, &createdAt)
	if err != nil {
		return nil, err
	}
	u.TwoFAEnabled = scanBool(twofaEnabled)
	u.LastLoginAttempt = scanTime(lastLoginAttempt)
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
		msg.Subject, msg.ContentType, msg.Body,
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
	var toJSON, ccJSON, bccJSON string
	var seen, deleted, receivedAt interface{}
	err := db.Conn.QueryRow(db.Queries[QGetMessage], id).Scan(
		&m.ID, &m.MessageID, &m.From, &toJSON, &ccJSON, &bccJSON,
		&m.Subject, &m.ContentType, &m.Body, &m.GCSKey, &m.OwnerUser,
		&m.Folder, &seen, &deleted, &receivedAt,
	)
	if err != nil {
		return nil, err
	}
	m.To = db.UnmarshalAddrs(toJSON)
	m.CC = db.UnmarshalAddrs(ccJSON)
	m.BCC = db.UnmarshalAddrs(bccJSON)
	m.Attachments, _ = db.ListAttachments(id)
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
		var toJSON, ccJSON, bccJSON string
		var seen, deleted, receivedAt interface{}
		err := rows.Scan(
			&m.ID, &m.MessageID, &m.From, &toJSON, &ccJSON, &bccJSON,
			&m.Subject, &m.ContentType, &m.Body, &m.GCSKey, &m.OwnerUser,
			&m.Folder, &seen, &deleted, &receivedAt,
		)
		if err != nil {
			return nil, err
		}
		m.To = db.UnmarshalAddrs(toJSON)
		m.CC = db.UnmarshalAddrs(ccJSON)
		m.BCC = db.UnmarshalAddrs(bccJSON)
		// Attachments loaded separately via ListAttachments() when needed (e.g. GetMessage)
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
	// QUpdateUser always sets both display_name and password_hash
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
		var createdAt, lastLoginAttempt, twofaEnabled interface{}
		err := rows.Scan(&u.ID, &u.Username, &u.Domain, &u.DisplayName, &u.PasswordHash,
			&u.Phone, &u.ExternalEmail, &u.Status, &twofaEnabled,
			&u.TwoFASecret, &u.TwoFABackupCodes,
			&u.LoginAttempts, &lastLoginAttempt, &createdAt)
		if err != nil {
			return nil, err
		}
		u.TwoFAEnabled = scanBool(twofaEnabled)
		u.LastLoginAttempt = scanTime(lastLoginAttempt)
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

// --- Count operations ---

func (db *DbSQL) CountUnread(ownerEmail, folder string) int {
	var count int
	db.Conn.QueryRow(db.Queries[QCountUnread], ownerEmail, folder).Scan(&count)
	return count
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

// --- Signup operations ---

func (db *DbSQL) CreateSignup(signup *model.DomainSignup) error {
	_, err := db.Conn.Exec(db.Queries[QCreateSignup],
		signup.ID, signup.Domain, signup.Username, signup.DisplayName,
		signup.PasswordHash, signup.Status, db.FormatTime(signup.ExpiresAt))
	return err
}

func (db *DbSQL) GetSignup(id string) (*model.DomainSignup, error) {
	s := &model.DomainSignup{}
	var createdAt, expiresAt interface{}
	err := db.Conn.QueryRow(db.Queries[QGetSignup], id).Scan(
		&s.ID, &s.Domain, &s.Username, &s.DisplayName,
		&s.PasswordHash, &s.Status, &createdAt, &expiresAt)
	if err != nil {
		return nil, err
	}
	s.CreatedAt = scanTime(createdAt)
	s.ExpiresAt = scanTime(expiresAt)
	return s, nil
}

func (db *DbSQL) DeleteSignup(id string) error {
	_, err := db.Conn.Exec(db.Queries[QDeleteSignup], id)
	return err
}

// --- Attachment operations ---

func (db *DbSQL) SaveAttachment(att *model.Attachment) error {
	_, err := db.Conn.Exec(db.Queries[QSaveAttachment],
		att.ID, att.MailContentID, att.Filename, att.ContentType, att.Size, att.BucketKey)
	return err
}

func (db *DbSQL) ListAttachments(mailContentID string) ([]model.Attachment, error) {
	rows, err := db.Conn.Query(db.Queries[QListAttachments], mailContentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var atts []model.Attachment
	for rows.Next() {
		var a model.Attachment
		if err := rows.Scan(&a.ID, &a.MailContentID, &a.Filename, &a.ContentType, &a.Size, &a.BucketKey); err != nil {
			return nil, err
		}
		atts = append(atts, a)
	}
	return atts, nil
}

func (db *DbSQL) GetAttachment(id string) (*model.Attachment, error) {
	a := &model.Attachment{}
	err := db.Conn.QueryRow(db.Queries[QGetAttachment], id).Scan(
		&a.ID, &a.MailContentID, &a.Filename, &a.ContentType, &a.Size, &a.BucketKey)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (db *DbSQL) DeleteAttachmentsByMessage(mailContentID string) error {
	_, err := db.Conn.Exec(db.Queries[QDeleteAttachmentsByMsg], mailContentID)
	return err
}

// --- Auth / 2FA operations ---

func (db *DbSQL) Enable2FA(email, secret, backupCodes string) error {
	_, err := db.Conn.Exec(db.Queries[QEnable2FA], secret, backupCodes, email)
	return err
}

func (db *DbSQL) Disable2FA(email string) error {
	_, err := db.Conn.Exec(db.Queries[QDisable2FA], email)
	return err
}

func (db *DbSQL) Get2FAStatus(email string) (bool, string, string, error) {
	var enabled interface{}
	var secret, backupCodes string
	err := db.Conn.QueryRow(db.Queries[QGet2FAStatus], email).Scan(&enabled, &secret, &backupCodes)
	if err != nil {
		return false, "", "", err
	}
	return scanBool(enabled), secret, backupCodes, nil
}

func (db *DbSQL) CreateTrustedDevice(device *model.TrustedDevice) error {
	_, err := db.Conn.Exec(db.Queries[QCreateTrustedDevice],
		device.ID, device.UserEmail, device.Fingerprint, device.Name, db.FormatTime(device.ExpiresAt))
	return err
}

func (db *DbSQL) IsTrustedDevice(email, fingerprint string) (bool, error) {
	var count int
	err := db.Conn.QueryRow(db.Queries[QIsTrustedDevice], email, fingerprint).Scan(&count)
	return count > 0, err
}

func (db *DbSQL) ListTrustedDevices(email string) ([]*model.TrustedDevice, error) {
	rows, err := db.Conn.Query(db.Queries[QListTrustedDevices], email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devices []*model.TrustedDevice
	for rows.Next() {
		d := &model.TrustedDevice{}
		var trustedAt, expiresAt, lastSeenAt interface{}
		if err := rows.Scan(&d.ID, &d.UserEmail, &d.Fingerprint, &d.Name, &trustedAt, &expiresAt, &lastSeenAt); err != nil {
			return nil, err
		}
		d.TrustedAt = scanTime(trustedAt)
		d.ExpiresAt = scanTime(expiresAt)
		d.LastSeenAt = scanTime(lastSeenAt)
		devices = append(devices, d)
	}
	return devices, nil
}

func (db *DbSQL) RevokeTrustedDevice(id string) error {
	_, err := db.Conn.Exec(db.Queries[QRevokeTrustedDevice], id)
	return err
}

func (db *DbSQL) UpdateDeviceLastSeen(id string) error {
	_, err := db.Conn.Exec(db.Queries[QUpdateDeviceLastSeen], id)
	return err
}

func (db *DbSQL) CreateOTP(otp *model.OTP) error {
	_, err := db.Conn.Exec(db.Queries[QCreateOTP],
		otp.ID, otp.UserEmail, otp.Code, otp.Purpose, db.FormatTime(otp.ExpiresAt))
	return err
}

func (db *DbSQL) GetOTP(email string) (*model.OTP, error) {
	o := &model.OTP{}
	var expiresAt interface{}
	err := db.Conn.QueryRow(db.Queries[QGetOTP], email).Scan(
		&o.ID, &o.UserEmail, &o.Code, &o.Purpose, &expiresAt, &o.Attempts)
	if err != nil {
		return nil, err
	}
	o.ExpiresAt = scanTime(expiresAt)
	return o, nil
}

func (db *DbSQL) IncrementOTPAttempts(email string) error {
	_, err := db.Conn.Exec(db.Queries[QIncrementOTPAttempts], email)
	return err
}

func (db *DbSQL) ClearOTP(email string) error {
	_, err := db.Conn.Exec(db.Queries[QClearOTP], email)
	return err
}

func (db *DbSQL) CreateLoginToken(token *model.LoginToken) error {
	_, err := db.Conn.Exec(db.Queries[QCreateLoginToken],
		token.Token, token.UserEmail, db.FormatTime(token.ExpiresAt))
	return err
}

func (db *DbSQL) GetLoginToken(token string) (*model.LoginToken, error) {
	t := &model.LoginToken{}
	var createdAt, expiresAt interface{}
	err := db.Conn.QueryRow(db.Queries[QGetLoginToken], token).Scan(
		&t.Token, &t.UserEmail, &createdAt, &expiresAt)
	if err != nil {
		return nil, err
	}
	t.CreatedAt = scanTime(createdAt)
	t.ExpiresAt = scanTime(expiresAt)
	return t, nil
}

func (db *DbSQL) DeleteLoginToken(token string) error {
	_, err := db.Conn.Exec(db.Queries[QDeleteLoginToken], token)
	return err
}

// --- Domain operations ---

func (db *DbSQL) CreateDomain(domain *model.Domain) error {
	_, err := db.Conn.Exec(db.Queries[QCreateDomain],
		domain.Name, domain.APIKeyHash, domain.SESStatus, domain.DKIMStatus,
		domain.Status, domain.CreatedBy)
	return err
}

func (db *DbSQL) GetDomain(name string) (*model.Domain, error) {
	d := &model.Domain{}
	var createdAt interface{}
	err := db.Conn.QueryRow(db.Queries[QGetDomain], name).Scan(
		&d.Name, &d.APIKeyHash, &d.SESStatus, &d.DKIMStatus, &d.Status, &d.CreatedBy, &createdAt)
	if err != nil {
		return nil, err
	}
	d.CreatedAt = scanTime(createdAt)
	return d, nil
}

func (db *DbSQL) ListDomains() ([]*model.Domain, error) {
	rows, err := db.Conn.Query(db.Queries[QListDomains])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var domains []*model.Domain
	for rows.Next() {
		d := &model.Domain{}
		var createdAt interface{}
		if err := rows.Scan(&d.Name, &d.APIKeyHash, &d.SESStatus, &d.DKIMStatus, &d.Status, &d.CreatedBy, &createdAt); err != nil {
			return nil, err
		}
		d.CreatedAt = scanTime(createdAt)
		domains = append(domains, d)
	}
	return domains, nil
}

func (db *DbSQL) UpdateDomainStatus(name, sesStatus, dkimStatus string) error {
	_, err := db.Conn.Exec(db.Queries[QUpdateDomainStatus], sesStatus, dkimStatus, name)
	return err
}

func (db *DbSQL) DeleteDomain(name string) error {
	_, err := db.Conn.Exec(db.Queries[QDeleteDomain], name)
	return err
}

// --- OAuth operations ---

func (db *DbSQL) CreateOAuthClient(client *model.OAuthClient) error {
	_, err := db.Conn.Exec(db.Queries[QCreateOAuthClient],
		client.ID, client.Name, client.ClientID, client.SecretHash,
		client.RedirectURI, client.Domain, client.CreatedBy)
	return err
}

func (db *DbSQL) GetOAuthClient(clientID string) (*model.OAuthClient, error) {
	c := &model.OAuthClient{}
	var createdAt interface{}
	err := db.Conn.QueryRow(db.Queries[QGetOAuthClient], clientID).Scan(
		&c.ID, &c.Name, &c.ClientID, &c.SecretHash, &c.RedirectURI, &c.Domain, &c.CreatedBy, &createdAt)
	if err != nil {
		return nil, err
	}
	c.CreatedAt = scanTime(createdAt)
	return c, nil
}

func (db *DbSQL) ListOAuthClients(domain string) ([]*model.OAuthClient, error) {
	rows, err := db.Conn.Query(db.Queries[QListOAuthClients], domain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var clients []*model.OAuthClient
	for rows.Next() {
		c := &model.OAuthClient{}
		var createdAt interface{}
		if err := rows.Scan(&c.ID, &c.Name, &c.ClientID, &c.SecretHash, &c.RedirectURI, &c.Domain, &c.CreatedBy, &createdAt); err != nil {
			return nil, err
		}
		c.CreatedAt = scanTime(createdAt)
		clients = append(clients, c)
	}
	return clients, nil
}

func (db *DbSQL) DeleteOAuthClient(id string) error {
	_, err := db.Conn.Exec(db.Queries[QDeleteOAuthClient], id)
	return err
}

func (db *DbSQL) CreateOAuthCode(code *model.OAuthCode) error {
	_, err := db.Conn.Exec(db.Queries[QCreateOAuthCode],
		code.Code, code.ClientID, code.UserEmail, code.RedirectURI,
		code.Scope, code.Nonce, db.FormatTime(code.ExpiresAt), db.FormatBool(false))
	return err
}

func (db *DbSQL) GetOAuthCode(code string) (*model.OAuthCode, error) {
	c := &model.OAuthCode{}
	var expiresAt, used interface{}
	err := db.Conn.QueryRow(db.Queries[QGetOAuthCode], code).Scan(
		&c.Code, &c.ClientID, &c.UserEmail, &c.RedirectURI, &c.Scope, &c.Nonce, &expiresAt, &used)
	if err != nil {
		return nil, err
	}
	c.ExpiresAt = scanTime(expiresAt)
	c.Used = scanBool(used)
	return c, nil
}

func (db *DbSQL) MarkOAuthCodeUsed(code string) error {
	_, err := db.Conn.Exec(db.Queries[QMarkOAuthCodeUsed], code)
	return err
}

func (db *DbSQL) CreateOAuthToken(token *model.OAuthToken) error {
	_, err := db.Conn.Exec(db.Queries[QCreateOAuthToken],
		token.Token, token.ClientID, token.UserEmail, token.Scope, db.FormatTime(token.ExpiresAt))
	return err
}

func (db *DbSQL) GetOAuthToken(token string) (*model.OAuthToken, error) {
	t := &model.OAuthToken{}
	var expiresAt interface{}
	err := db.Conn.QueryRow(db.Queries[QGetOAuthToken], token).Scan(
		&t.Token, &t.ClientID, &t.UserEmail, &t.Scope, &expiresAt)
	if err != nil {
		return nil, err
	}
	t.ExpiresAt = scanTime(expiresAt)
	return t, nil
}

func (db *DbSQL) DeleteOAuthToken(token string) error {
	_, err := db.Conn.Exec(db.Queries[QDeleteOAuthToken], token)
	return err
}
