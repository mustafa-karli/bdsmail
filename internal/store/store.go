package store

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mustafakarli/bdsmail/internal/mimeutil"
	"github.com/mustafakarli/bdsmail/internal/model"
)

type Store struct {
	DB     Database
	Bucket ObjectStore // optional, for attachments
}

func NewStore(db Database, bucket ObjectStore) *Store {
	return &Store{DB: db, Bucket: bucket}
}

// newMessage creates a Message with common fields populated.
func newMessage(from string, to, cc, bcc []string, subject, contentType, body, ownerUser, folder string, seen bool) *model.Message {
	_, senderDomain := SplitEmail(from)
	id := uuid.New().String()
	return &model.Message{
		ID:          id,
		MessageID:   fmt.Sprintf("<%s@%s>", id, senderDomain),
		From:        from,
		To:          to,
		CC:          cc,
		BCC:         bcc,
		Subject:     subject,
		ContentType: contentType,
		Body:        body,
		OwnerUser:   ownerUser,
		Folder:      folder,
		Seen:        seen,
		ReceivedAt:  time.Now().UTC(),
	}
}

// saveAndDeliver creates a message, saves attachments to bucket + DB, and persists the message.
func (s *Store) saveAndDeliver(ctx context.Context, from string, to, cc, bcc []string, subject, contentType, body, ownerUser, folder string, seen bool, parsedAttachments []mimeutil.ParsedAttachment) (string, error) {
	msg := newMessage(from, to, cc, bcc, subject, contentType, body, ownerUser, folder, seen)

	// Save message first
	if err := s.DB.SaveMessage(msg); err != nil {
		return "", fmt.Errorf("save message for %s: %w", ownerUser, err)
	}

	// Save attachments to bucket + mail_attachment table
	attachments, err := s.saveAttachments(ctx, msg.ID, parsedAttachments)
	if err != nil {
		log.Printf("warning: failed to save attachments for %s: %v", msg.ID, err)
	}
	for _, att := range attachments {
		att.MailContentID = msg.ID
		if err := s.DB.SaveAttachment(&att); err != nil {
			log.Printf("warning: failed to save attachment metadata %s: %v", att.ID, err)
		}
	}

	return msg.MessageID, nil
}

// SaveIncomingMail stores a received message for all local recipients.
func (s *Store) SaveIncomingMail(ctx context.Context, from string, to, cc, bcc []string, subject, contentType, body, folder string, parsedAttachments []mimeutil.ParsedAttachment) error {
	if folder == "" {
		folder = "INBOX"
	}
	for _, email := range s.collectLocalRecipients(to, cc, bcc) {
		if _, err := s.saveAndDeliver(ctx, from, to, cc, []string{}, subject, contentType, body, email, folder, false, parsedAttachments); err != nil {
			return err
		}
	}
	return nil
}

// SaveOutgoingMail stores a sent message and delivers to local recipients.
func (s *Store) SaveOutgoingMail(ctx context.Context, senderEmail, from string, to, cc, bcc []string, subject, contentType, body string, parsedAttachments []mimeutil.ParsedAttachment) (string, error) {
	// Save sender's copy in Sent folder
	messageID, err := s.saveAndDeliver(ctx, from, to, cc, bcc, subject, contentType, body, senderEmail, "Sent", true, parsedAttachments)
	if err != nil {
		return "", err
	}

	// Deliver to local recipients
	for _, email := range s.collectLocalRecipients(to, cc, bcc) {
		if _, err := s.saveAndDeliver(ctx, from, to, cc, []string{}, subject, contentType, body, email, "INBOX", false, parsedAttachments); err != nil {
			return "", err
		}
	}
	return messageID, nil
}

// SaveIncomingMailForUser stores a message for a single resolved recipient with per-user folder/seen.
func (s *Store) SaveIncomingMailForUser(ctx context.Context, ownerEmail, from string, to, cc []string, subject, contentType, body, folder string, seen bool, parsedAttachments []mimeutil.ParsedAttachment) error {
	_, err := s.saveAndDeliver(ctx, from, to, cc, []string{}, subject, contentType, body, ownerEmail, folder, seen, parsedAttachments)
	return err
}

// ProcessAutoReply checks and sends an auto-reply if configured for the recipient.
func (s *Store) ProcessAutoReply(ctx context.Context, recipientEmail, senderEmail string, relay AutoReplyRelay) {
	reply, err := s.DB.GetAutoReply(recipientEmail)
	if err != nil || !reply.Enabled {
		return
	}

	now := time.Now()
	if !reply.StartDate.IsZero() && now.Before(reply.StartDate) {
		return
	}
	if !reply.EndDate.IsZero() && now.After(reply.EndDate) {
		return
	}

	// Don't reply to noreply/mailer-daemon/list addresses
	senderLower := strings.ToLower(senderEmail)
	for _, skip := range []string{"noreply", "no-reply", "donotreply", "do-not-reply", "mailer-daemon"} {
		if strings.Contains(senderLower, skip) {
			return
		}
	}

	// Check cooldown (24h default)
	if s.DB.HasAutoRepliedRecently(recipientEmail, senderEmail, 24*time.Hour) {
		return
	}

	// Send auto-reply
	if relay != nil {
		subject := reply.Subject
		if subject == "" {
			subject = "Auto-Reply"
		}
		err := relay.SendSimple(recipientEmail, []string{senderEmail}, subject, "text/plain", reply.Body)
		if err != nil {
			log.Printf("auto-reply send error from %s to %s: %v", recipientEmail, senderEmail, err)
			return
		}
	}

	s.DB.RecordAutoReplySent(recipientEmail, senderEmail)
}

// AutoReplyRelay is the interface needed to send auto-reply emails.
type AutoReplyRelay interface {
	SendSimple(from string, to []string, subject, contentType, body string) error
}

func (s *Store) GetMessageWithBody(ctx context.Context, id string) (*model.Message, error) {
	return s.DB.GetMessage(id)
}

// GetAttachmentData reads an attachment's binary data from the bucket.
func (s *Store) GetAttachmentData(ctx context.Context, bucketKey string) ([]byte, error) {
	if s.Bucket == nil {
		return nil, fmt.Errorf("object storage not configured")
	}
	return s.Bucket.Read(ctx, bucketKey)
}

// LoadAttachments reads all attachment data for a message from the bucket.
func (s *Store) LoadAttachments(ctx context.Context, msg *model.Message) ([]mimeutil.AttachmentData, error) {
	if s.Bucket == nil || len(msg.Attachments) == 0 {
		return nil, nil
	}
	var result []mimeutil.AttachmentData
	for _, att := range msg.Attachments {
		data, err := s.Bucket.Read(ctx, att.BucketKey)
		if err != nil {
			log.Printf("warning: failed to read attachment %s: %v", att.BucketKey, err)
			continue
		}
		result = append(result, mimeutil.AttachmentData{Meta: att, Data: data})
	}
	return result, nil
}

func (s *Store) DeleteMessageFull(ctx context.Context, id string) error {
	// Delete attachments from bucket
	attachments, _ := s.DB.ListAttachments(id)
	if s.Bucket != nil {
		for _, att := range attachments {
			if err := s.Bucket.Delete(ctx, att.BucketKey); err != nil {
				log.Printf("warning: failed to delete attachment %s: %v", att.BucketKey, err)
			}
		}
	}
	// Delete attachment metadata from DB
	s.DB.DeleteAttachmentsByMessage(id)
	// Delete the message
	return s.DB.DeleteMessage(id)
}

// saveAttachments writes parsed attachments to the bucket and returns metadata.
func (s *Store) saveAttachments(ctx context.Context, msgID string, parsed []mimeutil.ParsedAttachment) ([]model.Attachment, error) {
	if s.Bucket == nil || len(parsed) == 0 {
		return nil, nil
	}
	meta := mimeutil.ToAttachmentMeta(parsed, msgID)
	for i, pa := range parsed {
		if err := s.Bucket.Write(ctx, meta[i].BucketKey, pa.Data, pa.ContentType); err != nil {
			return nil, fmt.Errorf("failed to write attachment %s: %w", pa.Filename, err)
		}
	}
	return meta, nil
}

func (s *Store) collectLocalRecipients(to, cc, bcc []string) []string {
	seen := make(map[string]bool)
	var result []string
	allAddrs := make([]string, 0, len(to)+len(cc)+len(bcc))
	allAddrs = append(allAddrs, to...)
	allAddrs = append(allAddrs, cc...)
	allAddrs = append(allAddrs, bcc...)
	for _, addr := range allAddrs {
		resolved := s.ResolveRecipient(addr, 0)
		for _, r := range resolved {
			if !seen[r] {
				seen[r] = true
				result = append(result, r)
			}
		}
	}
	return result
}

// ResolveRecipient resolves an email address through aliases, returning the final target addresses.
// depth limits recursion to prevent alias loops.
func (s *Store) ResolveRecipient(email string, depth int) []string {
	if depth > 10 {
		return []string{email}
	}

	// Check alias first
	targets, err := s.DB.GetAlias(email)
	if err == nil && len(targets) > 0 {
		var resolved []string
		for _, t := range targets {
			resolved = append(resolved, s.ResolveRecipient(t, depth+1)...)
		}
		return resolved
	}

	// Check catch-all
	_, domain := SplitEmail(email)
	if domain != "" {
		targets, err = s.DB.GetCatchAll(domain)
		if err == nil && len(targets) > 0 {
			var resolved []string
			for _, t := range targets {
				resolved = append(resolved, s.ResolveRecipient(t, depth+1)...)
			}
			return resolved
		}
	}

	// No alias — return original if user exists
	if s.DB.UserExistsByEmail(email) {
		return []string{email}
	}
	return nil
}

// DistributeToList sends a message to all members of a mailing list.
func (s *Store) DistributeToList(ctx context.Context, listAddr, from string, to, cc, bcc []string, subject, contentType, body string, parsedAttachments []mimeutil.ParsedAttachment) error {
	members, err := s.DB.GetListMembers(listAddr)
	if err != nil {
		return fmt.Errorf("get list members: %w", err)
	}
	list, err := s.DB.GetMailingList(listAddr)
	if err != nil {
		return fmt.Errorf("get mailing list: %w", err)
	}

	listSubject := subject
	if list.Name != "" && !strings.Contains(subject, "["+list.Name+"]") {
		listSubject = "[" + list.Name + "] " + subject
	}

	for _, member := range members {
		if member == from || !s.DB.UserExistsByEmail(member) {
			continue
		}
		if _, err := s.saveAndDeliver(ctx, from, []string{listAddr}, []string{}, []string{}, listSubject, contentType, body, member, "INBOX", false, parsedAttachments); err != nil {
			log.Printf("failed to distribute to %s: %v", member, err)
		}
	}
	return nil
}
