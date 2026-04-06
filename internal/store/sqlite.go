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
			if t.IsZero() {
				return ""
			}
			return t.UTC().Format(time.RFC3339)
		},
	}
	return db, nil
}

func sqliteQueries() map[string]string {
	return map[string]string{
		QCreateUser:  `INSERT INTO user_account (id, username, domain, display_name, password_hash) VALUES (?, ?, ?, ?, ?)`,
		QGetUser:     `SELECT id, username, domain, display_name, password_hash, phone, external_email, status, twofa_enabled, twofa_secret, twofa_backup_codes, login_attempts, last_login_attempt, created_at FROM user_account WHERE username = ? AND domain = ?`,
		QUserExists:  `SELECT COUNT(*) FROM user_account WHERE username = ? AND domain = ?`,
		QSaveMessage: `INSERT INTO mail_content (id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, gcs_key, owner_user, folder, seen, received_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		QListMessages: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM mail_content WHERE owner_user = ? AND folder = ? AND deleted = 0 ORDER BY received_at DESC`,
		QListAllMessages: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM mail_content WHERE owner_user = ? AND deleted = 0 ORDER BY received_at DESC`,
		QGetMessage: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM mail_content WHERE id = ?`,
		QMarkSeen:      `UPDATE mail_content SET seen = 1 WHERE id = ?`,
		QMarkDeleted:   `UPDATE mail_content SET deleted = 1 WHERE id = ?`,
		QDeleteMessage: `DELETE FROM mail_content WHERE id = ?`,

		QListUsers:          `SELECT id, username, domain, display_name, password_hash, phone, external_email, status, twofa_enabled, twofa_secret, twofa_backup_codes, login_attempts, last_login_attempt, created_at FROM user_account ORDER BY domain, username`,
		QListUsersByDomain:  `SELECT id, username, domain, display_name, password_hash, phone, external_email, status, twofa_enabled, twofa_secret, twofa_backup_codes, login_attempts, last_login_attempt, created_at FROM user_account WHERE domain = ? ORDER BY username`,
		QUpdateUser:         `UPDATE user_account SET display_name = ?, password_hash = ? WHERE username = ? AND domain = ?`,
		QDeleteUser:         `DELETE FROM user_account WHERE username = ? AND domain = ?`,
		QDeleteUserMessages: `DELETE FROM mail_content WHERE owner_user = ?`,

		QCreateAlias: `INSERT INTO mail_alias (alias_email, target_emails, is_catch_all) VALUES (?, ?, ?)`,
		QGetAlias:    `SELECT target_emails FROM mail_alias WHERE alias_email = ?`,
		QListAliases: `SELECT alias_email, target_emails, is_catch_all FROM mail_alias ORDER BY alias_email`,
		QUpdateAlias: `UPDATE mail_alias SET target_emails = ? WHERE alias_email = ?`,
		QDeleteAlias: `DELETE FROM mail_alias WHERE alias_email = ?`,
		QGetCatchAll: `SELECT target_emails FROM mail_alias WHERE alias_email = ? AND is_catch_all = 1`,

		QCreateMailingList: `INSERT INTO mailing_list (list_address, name, description, owner_email) VALUES (?, ?, ?, ?)`,
		QGetMailingList:    `SELECT list_address, name, description, owner_email, created_at FROM mailing_list WHERE list_address = ?`,
		QListMailingLists:  `SELECT list_address, name, description, owner_email, created_at FROM mailing_list ORDER BY list_address`,
		QDeleteMailingList: `DELETE FROM mailing_list WHERE list_address = ?`,
		QAddListMember:     `INSERT OR IGNORE INTO list_member (list_address, member_email) VALUES (?, ?)`,
		QRemoveListMember:  `DELETE FROM list_member WHERE list_address = ? AND member_email = ?`,
		QGetListMembers:    `SELECT member_email FROM list_member WHERE list_address = ? ORDER BY member_email`,
		QIsMailingList:     `SELECT COUNT(*) FROM mailing_list WHERE list_address = ?`,
		QDeleteListMembers: `DELETE FROM list_member WHERE list_address = ?`,

		QCreateFilter:    `INSERT INTO mail_filter (id, user_email, name, priority, conditions, actions, enabled) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		QListFilters:     `SELECT id, user_email, name, priority, conditions, actions, enabled FROM mail_filter WHERE user_email = ? ORDER BY priority DESC`,
		QUpdateFilter:    `UPDATE mail_filter SET name = ?, priority = ?, conditions = ?, actions = ?, enabled = ? WHERE id = ?`,
		QDeleteFilter:    `DELETE FROM mail_filter WHERE id = ?`,
		QListUserFolders: `SELECT DISTINCT folder FROM mail_content WHERE owner_user = ? AND deleted = 0 ORDER BY folder`,

		QSetAutoReply: `INSERT INTO auto_reply (user_email, enabled, subject, body, start_date, end_date) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(user_email) DO UPDATE SET enabled=excluded.enabled, subject=excluded.subject, body=excluded.body, start_date=excluded.start_date, end_date=excluded.end_date`,
		QGetAutoReply:           `SELECT user_email, enabled, subject, body, start_date, end_date FROM auto_reply WHERE user_email = ?`,
		QDeleteAutoReply:        `DELETE FROM auto_reply WHERE user_email = ?`,
		QRecordAutoReplySent:    `INSERT INTO auto_reply_log (user_email, sender_email) VALUES (?, ?) ON CONFLICT(user_email, sender_email) DO UPDATE SET sent_at = datetime('now')`,
		QHasAutoRepliedRecently: `SELECT COUNT(*) FROM auto_reply_log WHERE user_email = ? AND sender_email = ? AND sent_at > ?`,

		QCountUnread: `SELECT COUNT(*) FROM mail_content WHERE owner_user = ? AND folder = ? AND seen = 0 AND deleted = 0`,

		QSearchMessages: `SELECT id, message_id, from_addr, to_addrs, cc_addrs, bcc_addrs, subject, content_type, body, gcs_key, owner_user, folder, seen, deleted, received_at
			FROM mail_content WHERE owner_user = ? AND deleted = 0 AND (subject LIKE ? OR body LIKE ? OR from_addr LIKE ? OR to_addrs LIKE ?) ORDER BY received_at DESC LIMIT 100`,

		QCreateContact: `INSERT INTO user_contact (id, owner_email, vcard_data, etag) VALUES (?, ?, ?, ?)`,
		QGetContact:    `SELECT id, owner_email, vcard_data, etag, created_at, updated_at FROM user_contact WHERE id = ?`,
		QListContacts:  `SELECT id, owner_email, vcard_data, etag, created_at, updated_at FROM user_contact WHERE owner_email = ? ORDER BY updated_at DESC`,
		QUpdateContact: `UPDATE user_contact SET vcard_data = ?, etag = ?, updated_at = datetime('now') WHERE id = ?`,
		QDeleteContact: `DELETE FROM user_contact WHERE id = ?`,

		// Attachments
		QSaveAttachment:        `INSERT INTO mail_attachment (id, mail_content_id, filename, content_type, size, bucket_key) VALUES (?, ?, ?, ?, ?, ?)`,
		QListAttachments:       `SELECT id, mail_content_id, filename, content_type, size, bucket_key FROM mail_attachment WHERE mail_content_id = ?`,
		QGetAttachment:         `SELECT id, mail_content_id, filename, content_type, size, bucket_key FROM mail_attachment WHERE id = ?`,
		QDeleteAttachmentsByMsg: `DELETE FROM mail_attachment WHERE mail_content_id = ?`,

		// Auth / 2FA
		QEnable2FA:    `UPDATE user_account SET twofa_enabled = 1, twofa_secret = ?, twofa_backup_codes = ? WHERE username || '@' || domain = ?`,
		QDisable2FA:   `UPDATE user_account SET twofa_enabled = 0, twofa_secret = '', twofa_backup_codes = '' WHERE username || '@' || domain = ?`,
		QGet2FAStatus: `SELECT twofa_enabled, twofa_secret, twofa_backup_codes FROM user_account WHERE username || '@' || domain = ?`,

		QCreateTrustedDevice: `INSERT INTO user_trusted_device (id, user_email, device_fingerprint, device_name, expires_at) VALUES (?, ?, ?, ?, ?)`,
		QIsTrustedDevice:     `SELECT COUNT(*) FROM user_trusted_device WHERE user_email = ? AND device_fingerprint = ? AND expires_at > datetime('now')`,
		QListTrustedDevices:  `SELECT id, user_email, device_fingerprint, device_name, trusted_at, expires_at, last_seen_at FROM user_trusted_device WHERE user_email = ? AND expires_at > datetime('now') ORDER BY trusted_at DESC`,
		QRevokeTrustedDevice: `DELETE FROM user_trusted_device WHERE id = ?`,
		QUpdateDeviceLastSeen: `UPDATE user_trusted_device SET last_seen_at = datetime('now') WHERE id = ?`,

		QCreateOTP:            `INSERT INTO user_otp (id, user_email, code, purpose, expires_at) VALUES (?, ?, ?, ?, ?)`,
		QGetOTP:               `SELECT id, user_email, code, purpose, expires_at, attempts FROM user_otp WHERE user_email = ? ORDER BY expires_at DESC LIMIT 1`,
		QIncrementOTPAttempts: `UPDATE user_otp SET attempts = attempts + 1 WHERE user_email = ?`,
		QClearOTP:             `DELETE FROM user_otp WHERE user_email = ?`,

		QCreateLoginToken: `INSERT INTO login_token (token, user_email, expires_at) VALUES (?, ?, ?)`,
		QGetLoginToken:    `SELECT token, user_email, created_at, expires_at FROM login_token WHERE token = ?`,
		QDeleteLoginToken: `DELETE FROM login_token WHERE token = ?`,

		QCreateDomain:       `INSERT INTO domain (name, api_key_hash, ses_status, dkim_status, status, created_by) VALUES (?, ?, ?, ?, ?, ?)`,
		QGetDomain:          `SELECT name, api_key_hash, ses_status, dkim_status, status, created_by, created_at FROM domain WHERE name = ?`,
		QListDomains:        `SELECT name, api_key_hash, ses_status, dkim_status, status, created_by, created_at FROM domain ORDER BY name`,
		QUpdateDomainStatus: `UPDATE domain SET ses_status = ?, dkim_status = ? WHERE name = ?`,
		QDeleteDomain:       `DELETE FROM domain WHERE name = ?`,

		QCreateOAuthClient:        `INSERT INTO oauth_client (id, name, client_id, secret_hash, redirect_uri, domain, created_by) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		QGetOAuthClient:           `SELECT id, name, client_id, secret_hash, redirect_uri, domain, created_by, created_at FROM oauth_client WHERE client_id = ?`,
		QListOAuthClients:         `SELECT id, name, client_id, secret_hash, redirect_uri, domain, created_by, created_at FROM oauth_client WHERE domain = ? ORDER BY created_at DESC`,
		QDeleteOAuthClient:        `DELETE FROM oauth_client WHERE id = ?`,
		QCreateOAuthCode:          `INSERT INTO oauth_code (code, client_id, user_email, redirect_uri, scope, nonce, expires_at, used) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		QGetOAuthCode:             `SELECT code, client_id, user_email, redirect_uri, scope, nonce, expires_at, used FROM oauth_code WHERE code = ?`,
		QMarkOAuthCodeUsed:        `UPDATE oauth_code SET used = 1 WHERE code = ?`,
		QCreateOAuthToken:         `INSERT INTO oauth_token (token, client_id, user_email, scope, expires_at) VALUES (?, ?, ?, ?, ?)`,
		QGetOAuthToken:            `SELECT token, client_id, user_email, scope, expires_at FROM oauth_token WHERE token = ?`,
		QDeleteOAuthToken:         `DELETE FROM oauth_token WHERE token = ?`,
		QDeleteExpiredOAuthCodes:  `DELETE FROM oauth_code WHERE expires_at < datetime('now')`,
		QDeleteExpiredOAuthTokens: `DELETE FROM oauth_token WHERE expires_at < datetime('now')`,
	}
}
