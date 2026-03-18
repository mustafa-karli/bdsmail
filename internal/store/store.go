package store

import (
	"context"
	"fmt"
	"log"
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
		if !seen[addr] && s.DB.UserExistsByEmail(addr) {
			seen[addr] = true
			result = append(result, addr)
		}
	}
	return result
}
