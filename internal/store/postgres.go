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
	return db, nil
}

func pgsqlQueries() map[string]string {
	return map[string]string{
		QCreateUser:  `INSERT INTO user_account (id, username, domain, display_name, password_hash) VALUES ($1, $2, $3, $4, $5)`,
		QGetUser:     `SELECT id, username, domain, display_name, password_hash, phone, external_email, status, twofa_enabled, twofa_secret, twofa_backup_codes, login_attempts, last_login_attempt, created_at FROM user_account WHERE username = $1 AND domain = $2`,
		QUserExists:  `SELECT COUNT(*) FROM user_account WHERE username = $1 AND domain = $2`,
		QSaveMessage: `INSERT INTO mail_content (id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, gcs_key, owner_user, folder, seen, received_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		QListMessages: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM mail_content WHERE owner_user = $1 AND folder = $2 AND deleted = FALSE ORDER BY received_at DESC`,
		QListAllMessages: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM mail_content WHERE owner_user = $1 AND deleted = FALSE ORDER BY received_at DESC`,
		QGetMessage: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM mail_content WHERE id = $1`,
		QMarkSeen:      `UPDATE mail_content SET seen = TRUE WHERE id = $1`,
		QMarkDeleted:   `UPDATE mail_content SET deleted = TRUE WHERE id = $1`,
		QDeleteMessage: `DELETE FROM mail_content WHERE id = $1`,

		// User provisioning
		QListUsers:          `SELECT id, username, domain, display_name, password_hash, phone, external_email, status, twofa_enabled, twofa_secret, twofa_backup_codes, login_attempts, last_login_attempt, created_at FROM user_account ORDER BY domain, username`,
		QListUsersByDomain:  `SELECT id, username, domain, display_name, password_hash, phone, external_email, status, twofa_enabled, twofa_secret, twofa_backup_codes, login_attempts, last_login_attempt, created_at FROM user_account WHERE domain = $1 ORDER BY username`,
		QUpdateUser:         `UPDATE user_account SET display_name = $1, password_hash = $2 WHERE username = $3 AND domain = $4`,
		QDeleteUser:         `DELETE FROM user_account WHERE username = $1 AND domain = $2`,
		QDeleteUserMessages: `DELETE FROM mail_content WHERE owner_user = $1`,

		// Aliases
		QCreateAlias: `INSERT INTO mail_alias (alias_email, target_emails, is_catch_all) VALUES ($1, $2, $3)`,
		QGetAlias:    `SELECT target_emails FROM mail_alias WHERE alias_email = $1`,
		QListAliases: `SELECT alias_email, target_emails, is_catch_all FROM mail_alias ORDER BY alias_email`,
		QUpdateAlias: `UPDATE mail_alias SET target_emails = $1 WHERE alias_email = $2`,
		QDeleteAlias: `DELETE FROM mail_alias WHERE alias_email = $1`,
		QGetCatchAll: `SELECT target_emails FROM mail_alias WHERE alias_email = $1 AND is_catch_all = TRUE`,

		// Mailing lists
		QCreateMailingList: `INSERT INTO mailing_list (list_address, name, description, owner_email) VALUES ($1, $2, $3, $4)`,
		QGetMailingList:    `SELECT list_address, name, description, owner_email, created_at FROM mailing_list WHERE list_address = $1`,
		QListMailingLists:  `SELECT list_address, name, description, owner_email, created_at FROM mailing_list ORDER BY list_address`,
		QDeleteMailingList: `DELETE FROM mailing_list WHERE list_address = $1`,
		QAddListMember:     `INSERT INTO list_member (list_address, member_email) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		QRemoveListMember:  `DELETE FROM list_member WHERE list_address = $1 AND member_email = $2`,
		QGetListMembers:    `SELECT member_email FROM list_member WHERE list_address = $1 ORDER BY member_email`,
		QIsMailingList:     `SELECT COUNT(*) FROM mailing_list WHERE list_address = $1`,
		QDeleteListMembers: `DELETE FROM list_member WHERE list_address = $1`,

		// Filters
		QCreateFilter:    `INSERT INTO mail_filter (id, user_email, name, priority, conditions, actions, enabled) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		QListFilters:     `SELECT id, user_email, name, priority, conditions, actions, enabled FROM mail_filter WHERE user_email = $1 ORDER BY priority DESC`,
		QUpdateFilter:    `UPDATE mail_filter SET name = $1, priority = $2, conditions = $3, actions = $4, enabled = $5 WHERE id = $6`,
		QDeleteFilter:    `DELETE FROM mail_filter WHERE id = $1`,
		QListUserFolders: `SELECT DISTINCT folder FROM mail_content WHERE owner_user = $1 AND deleted = FALSE ORDER BY folder`,

		// Auto-reply
		QSetAutoReply: `INSERT INTO auto_reply (user_email, enabled, subject, body, start_date, end_date) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT(user_email) DO UPDATE SET enabled=EXCLUDED.enabled, subject=EXCLUDED.subject, body=EXCLUDED.body, start_date=EXCLUDED.start_date, end_date=EXCLUDED.end_date`,
		QGetAutoReply:           `SELECT user_email, enabled, subject, body, start_date, end_date FROM auto_reply WHERE user_email = $1`,
		QDeleteAutoReply:        `DELETE FROM auto_reply WHERE user_email = $1`,
		QRecordAutoReplySent:    `INSERT INTO auto_reply_log (user_email, sender_email) VALUES ($1, $2) ON CONFLICT(user_email, sender_email) DO UPDATE SET sent_at = NOW()`,
		QHasAutoRepliedRecently: `SELECT COUNT(*) FROM auto_reply_log WHERE user_email = $1 AND sender_email = $2 AND sent_at > $3`,

		// Unread count
		QCountUnread: `SELECT COUNT(*) FROM mail_content WHERE owner_user = $1 AND folder = $2 AND seen = FALSE AND deleted = FALSE`,

		// Search
		QSearchMessages: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM mail_content WHERE owner_user = $1 AND deleted = FALSE AND (subject ILIKE $2 OR body ILIKE $3 OR from_addr ILIKE $4 OR to_addrs ILIKE $5) ORDER BY received_at DESC LIMIT 100`,

		// Contacts
		QCreateContact: `INSERT INTO user_contact (id, owner_email, vcard_data, etag) VALUES ($1, $2, $3, $4)`,
		QGetContact:    `SELECT id, owner_email, vcard_data, etag, created_at, updated_at FROM user_contact WHERE id = $1`,
		QListContacts:  `SELECT id, owner_email, vcard_data, etag, created_at, updated_at FROM user_contact WHERE owner_email = $1 ORDER BY updated_at DESC`,
		QUpdateContact: `UPDATE user_contact SET vcard_data = $1, etag = $2, updated_at = NOW() WHERE id = $3`,
		QDeleteContact: `DELETE FROM user_contact WHERE id = $1`,

		// Domain DNS
		QSaveDNSRecord:    `INSERT INTO domain_dns (domain, record_type, name, value, priority) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (domain, record_type, name) DO UPDATE SET value = EXCLUDED.value, priority = EXCLUDED.priority`,
		QListDNSRecords:   `SELECT domain, record_type, name, value, priority, created_at FROM domain_dns WHERE domain = $1 ORDER BY record_type, name`,
		QDeleteDNSRecords: `DELETE FROM domain_dns WHERE domain = $1`,

		// Permissions
		QGrantPermission:  `INSERT INTO user_permission (id, user_email, role, domain, start_date, end_date, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		QRevokePermission: `DELETE FROM user_permission WHERE id = $1`,
		QGetPermissions:   `SELECT id, user_email, role, domain, start_date, end_date, created_by, created_at FROM user_permission WHERE user_email = $1 AND start_date <= NOW() AND end_date >= NOW() ORDER BY role`,
		QHasPermission:    `SELECT COUNT(*) FROM user_permission WHERE user_email = $1 AND role = $2 AND start_date <= NOW() AND end_date >= NOW()`,

		// Signup
		QCreateSignup: `INSERT INTO domain_signup (id, domain, username, display_name, password_hash, status, expires_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		QGetSignup:    `SELECT id, domain, username, display_name, password_hash, status, created_at, expires_at FROM domain_signup WHERE id = $1`,
		QDeleteSignup: `DELETE FROM domain_signup WHERE id = $1`,

		// Attachments
		QSaveAttachment:       `INSERT INTO mail_attachment (id, mail_content_id, filename, content_type, size, bucket_key) VALUES ($1, $2, $3, $4, $5, $6)`,
		QListAttachments:      `SELECT id, mail_content_id, filename, content_type, size, bucket_key FROM mail_attachment WHERE mail_content_id = $1`,
		QGetAttachment:        `SELECT id, mail_content_id, filename, content_type, size, bucket_key FROM mail_attachment WHERE id = $1`,
		QDeleteAttachmentsByMsg: `DELETE FROM mail_attachment WHERE mail_content_id = $1`,

		// Auth / 2FA
		QEnable2FA:    `UPDATE user_account SET twofa_enabled = TRUE, twofa_secret = $1, twofa_backup_codes = $2 WHERE username || '@' || domain = $3`,
		QDisable2FA:   `UPDATE user_account SET twofa_enabled = FALSE, twofa_secret = '', twofa_backup_codes = '' WHERE username || '@' || domain = $1`,
		QGet2FAStatus: `SELECT twofa_enabled, twofa_secret, twofa_backup_codes FROM user_account WHERE username || '@' || domain = $1`,

		QCreateTrustedDevice: `INSERT INTO user_trusted_device (id, user_email, device_fingerprint, device_name, expires_at) VALUES ($1, $2, $3, $4, $5)`,
		QIsTrustedDevice:     `SELECT COUNT(*) FROM user_trusted_device WHERE user_email = $1 AND device_fingerprint = $2 AND expires_at > NOW()`,
		QListTrustedDevices:  `SELECT id, user_email, device_fingerprint, device_name, trusted_at, expires_at, last_seen_at FROM user_trusted_device WHERE user_email = $1 AND expires_at > NOW() ORDER BY trusted_at DESC`,
		QRevokeTrustedDevice: `DELETE FROM user_trusted_device WHERE id = $1`,
		QUpdateDeviceLastSeen: `UPDATE user_trusted_device SET last_seen_at = NOW() WHERE id = $1`,

		QCreateOTP:          `INSERT INTO user_otp (id, user_email, code, purpose, expires_at) VALUES ($1, $2, $3, $4, $5)`,
		QGetOTP:             `SELECT id, user_email, code, purpose, expires_at, attempts FROM user_otp WHERE user_email = $1 ORDER BY expires_at DESC LIMIT 1`,
		QIncrementOTPAttempts: `UPDATE user_otp SET attempts = attempts + 1 WHERE user_email = $1`,
		QClearOTP:           `DELETE FROM user_otp WHERE user_email = $1`,

		QCreateLoginToken: `INSERT INTO login_token (token, user_email, expires_at) VALUES ($1, $2, $3)`,
		QGetLoginToken:    `SELECT token, user_email, created_at, expires_at FROM login_token WHERE token = $1`,
		QDeleteLoginToken: `DELETE FROM login_token WHERE token = $1`,

		// Domain
		QCreateDomain:       `INSERT INTO domain (name, api_key_hash, ses_status, dkim_status, status, created_by) VALUES ($1, $2, $3, $4, $5, $6)`,
		QGetDomain:          `SELECT name, api_key_hash, ses_status, dkim_status, status, created_by, created_at FROM domain WHERE name = $1`,
		QListDomains:        `SELECT name, api_key_hash, ses_status, dkim_status, status, created_by, created_at FROM domain ORDER BY name`,
		QUpdateDomainStatus: `UPDATE domain SET ses_status = $1, dkim_status = $2 WHERE name = $3`,
		QDeleteDomain:       `DELETE FROM domain WHERE name = $1`,

		// OAuth
		QCreateOAuthClient:        `INSERT INTO oauth_client (id, name, client_id, secret_hash, redirect_uri, domain, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		QGetOAuthClient:           `SELECT id, name, client_id, secret_hash, redirect_uri, domain, created_by, created_at FROM oauth_client WHERE client_id = $1`,
		QListOAuthClients:         `SELECT id, name, client_id, secret_hash, redirect_uri, domain, created_by, created_at FROM oauth_client WHERE domain = $1 ORDER BY created_at DESC`,
		QDeleteOAuthClient:        `DELETE FROM oauth_client WHERE id = $1`,
		QCreateOAuthCode:          `INSERT INTO oauth_code (code, client_id, user_email, redirect_uri, scope, nonce, expires_at, used) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		QGetOAuthCode:             `SELECT code, client_id, user_email, redirect_uri, scope, nonce, expires_at, used FROM oauth_code WHERE code = $1`,
		QMarkOAuthCodeUsed:        `UPDATE oauth_code SET used = TRUE WHERE code = $1`,
		QCreateOAuthToken:         `INSERT INTO oauth_token (token, client_id, user_email, scope, expires_at) VALUES ($1, $2, $3, $4, $5)`,
		QGetOAuthToken:            `SELECT token, client_id, user_email, scope, expires_at FROM oauth_token WHERE token = $1`,
		QDeleteOAuthToken:         `DELETE FROM oauth_token WHERE token = $1`,
		QDeleteExpiredOAuthCodes:  `DELETE FROM oauth_code WHERE expires_at < NOW()`,
		QDeleteExpiredOAuthTokens: `DELETE FROM oauth_token WHERE expires_at < NOW()`,
	}
}
