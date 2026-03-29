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

// SaveIncomingMail stores a received message for all local recipients.
// attachments are saved to the bucket if available.
func (s *Store) SaveIncomingMail(ctx context.Context, from string, to, cc, bcc []string, subject, contentType, body, folder string, parsedAttachments []mimeutil.ParsedAttachment) error {
	if folder == "" {
		folder = "INBOX"
	}
	allRecipients := s.collectLocalRecipients(to, cc, bcc)
	_, senderDomain := SplitEmail(from)

	for _, email := range allRecipients {
		msgID := uuid.New().String()

		// Save attachments to bucket
		attachments, err := s.saveAttachments(ctx, msgID, parsedAttachments)
		if err != nil {
			log.Printf("warning: failed to save attachments for %s: %v", msgID, err)
		}

		msg := &model.Message{
			ID:          msgID,
			MessageID:   fmt.Sprintf("<%s@%s>", msgID, senderDomain),
			From:        from,
			To:          to,
			CC:          cc,
			BCC:         []string{},
			Subject:     subject,
			ContentType: contentType,
			Body:        body,
			Attachments: attachments,
			OwnerUser:   email,
			Folder:      folder,
			Seen:        false,
			ReceivedAt:  time.Now().UTC(),
		}
		if err := s.DB.SaveMessage(msg); err != nil {
			return fmt.Errorf("failed to save message: %w", err)
		}
	}
	return nil
}

// SaveOutgoingMail stores a sent message and delivers to local recipients.
func (s *Store) SaveOutgoingMail(ctx context.Context, senderEmail, from string, to, cc, bcc []string, subject, contentType, body string, parsedAttachments []mimeutil.ParsedAttachment) (string, error) {
	msgID := uuid.New().String()
	_, senderDomain := SplitEmail(senderEmail)

	// Save attachments to bucket
	attachments, err := s.saveAttachments(ctx, msgID, parsedAttachments)
	if err != nil {
		log.Printf("warning: failed to save attachments for %s: %v", msgID, err)
	}

	// Save sender's copy in Sent folder
	msg := &model.Message{
		ID:          msgID,
		MessageID:   fmt.Sprintf("<%s@%s>", msgID, senderDomain),
		From:        from,
		To:          to,
		CC:          cc,
		BCC:         bcc,
		Subject:     subject,
		ContentType: contentType,
		Body:        body,
		Attachments: attachments,
		OwnerUser:   senderEmail,
		Folder:      "Sent",
		Seen:        true,
		ReceivedAt:  time.Now().UTC(),
	}
	if err := s.DB.SaveMessage(msg); err != nil {
		return "", fmt.Errorf("failed to save sent message: %w", err)
	}

	// Deliver to local recipients
	localRecipients := s.collectLocalRecipients(to, cc, bcc)
	for _, email := range localRecipients {
		localMsgID := uuid.New().String()

		// Save separate attachment copies for local recipients
		localAttachments, _ := s.saveAttachments(ctx, localMsgID, parsedAttachments)

		localMsg := &model.Message{
			ID:          localMsgID,
			MessageID:   msg.MessageID,
			From:        from,
			To:          to,
			CC:          cc,
			BCC:         []string{},
			Subject:     subject,
			ContentType: contentType,
			Body:        body,
			Attachments: localAttachments,
			OwnerUser:   email,
			Folder:      "INBOX",
			Seen:        false,
			ReceivedAt:  time.Now().UTC(),
		}
		if err := s.DB.SaveMessage(localMsg); err != nil {
			return "", fmt.Errorf("failed to save local delivery: %w", err)
		}
	}

	return msg.MessageID, nil
}

// SaveIncomingMailForUser stores a message for a single resolved recipient with per-user folder/seen.
func (s *Store) SaveIncomingMailForUser(ctx context.Context, ownerEmail, from string, to, cc []string, subject, contentType, body, folder string, seen bool, parsedAttachments []mimeutil.ParsedAttachment) error {
	msgID := uuid.New().String()
	_, senderDomain := SplitEmail(from)

	attachments, err := s.saveAttachments(ctx, msgID, parsedAttachments)
	if err != nil {
		log.Printf("warning: failed to save attachments for %s: %v", msgID, err)
	}

	msg := &model.Message{
		ID:          msgID,
		MessageID:   fmt.Sprintf("<%s@%s>", msgID, senderDomain),
		From:        from,
		To:          to,
		CC:          cc,
		BCC:         []string{},
		Subject:     subject,
		ContentType: contentType,
		Body:        body,
		Attachments: attachments,
		OwnerUser:   ownerEmail,
		Folder:      folder,
		Seen:        seen,
		ReceivedAt:  time.Now().UTC(),
	}
	return s.DB.SaveMessage(msg)
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
	msg, err := s.DB.GetMessage(id)
	if err != nil {
		return err
	}
	// Delete attachments from bucket
	if s.Bucket != nil {
		for _, att := range msg.Attachments {
			if err := s.Bucket.Delete(ctx, att.BucketKey); err != nil {
				log.Printf("warning: failed to delete attachment %s: %v", att.BucketKey, err)
			}
		}
	}
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
		return fmt.Errorf("failed to get list members: %w", err)
	}

	list, err := s.DB.GetMailingList(listAddr)
	if err != nil {
		return fmt.Errorf("failed to get mailing list: %w", err)
	}

	// Add List-Id and List-Unsubscribe headers to subject metadata
	listSubject := subject
	if list.Name != "" && !strings.Contains(subject, "["+list.Name+"]") {
		listSubject = "[" + list.Name + "] " + subject
	}

	for _, member := range members {
		if member == from {
			continue // don't deliver back to sender
		}
		if !s.DB.UserExistsByEmail(member) {
			continue
		}
		msgID := uuid.New().String()
		_, senderDomain := SplitEmail(from)
		attachments, _ := s.saveAttachments(ctx, msgID, parsedAttachments)

		msg := &model.Message{
			ID:          msgID,
			MessageID:   fmt.Sprintf("<%s@%s>", msgID, senderDomain),
			From:        from,
			To:          []string{listAddr},
			CC:          []string{},
			BCC:         []string{},
			Subject:     listSubject,
			ContentType: contentType,
			Body:        body,
			Attachments: attachments,
			OwnerUser:   member,
			Folder:      "INBOX",
			Seen:        false,
			ReceivedAt:  time.Now().UTC(),
		}
		if err := s.DB.SaveMessage(msg); err != nil {
			log.Printf("failed to distribute to %s: %v", member, err)
		}
	}
	return nil
}
