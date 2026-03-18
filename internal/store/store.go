package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mustafakarli/bdsmail/internal/model"
)

type Store struct {
	DB     *DB
	Bucket *Bucket
}

func NewStore(db *DB, bucket *Bucket) *Store {
	return &Store{DB: db, Bucket: bucket}
}

// SaveIncomingMail stores a received message for all local recipients.
// folder overrides the destination folder (defaults to "INBOX" if empty).
func (s *Store) SaveIncomingMail(ctx context.Context, from string, to, cc, bcc []string, subject, contentType, body, folder string) error {
	if folder == "" {
		folder = "INBOX"
	}
	allRecipients := s.collectLocalRecipients(to, cc, bcc)

	_, senderDomain := SplitEmail(from)

	for _, email := range allRecipients {
		msgID := uuid.New().String()
		gcsKey := msgID

		if err := s.Bucket.WriteBody(ctx, gcsKey, []byte(body)); err != nil {
			return fmt.Errorf("failed to store body: %w", err)
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
			GCSKey:      gcsKey,
			OwnerUser:   email, // full email as owner
			Folder:      folder,
			Seen:        false,
			ReceivedAt:  time.Now().UTC(),
		}
		if err := s.DB.SaveMessage(msg); err != nil {
			return fmt.Errorf("failed to save message metadata: %w", err)
		}
	}
	return nil
}

// SaveOutgoingMail stores a sent message and delivers to local recipients.
// senderEmail is the full email address (user@domain).
func (s *Store) SaveOutgoingMail(ctx context.Context, senderEmail, from string, to, cc, bcc []string, subject, contentType, body string) (string, error) {
	msgID := uuid.New().String()
	gcsKey := msgID
	_, senderDomain := SplitEmail(senderEmail)

	if err := s.Bucket.WriteBody(ctx, gcsKey, []byte(body)); err != nil {
		return "", fmt.Errorf("failed to store body: %w", err)
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
		GCSKey:      gcsKey,
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
		localGCSKey := localMsgID

		if err := s.Bucket.WriteBody(ctx, localGCSKey, []byte(body)); err != nil {
			return "", fmt.Errorf("failed to store body for local recipient: %w", err)
		}

		localMsg := &model.Message{
			ID:          localMsgID,
			MessageID:   msg.MessageID,
			From:        from,
			To:          to,
			CC:          cc,
			BCC:         []string{},
			Subject:     subject,
			ContentType: contentType,
			GCSKey:      localGCSKey,
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
	msg, err := s.DB.GetMessage(id)
	if err != nil {
		return nil, err
	}
	body, err := s.Bucket.ReadBody(ctx, msg.GCSKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}
	msg.Body = string(body)
	return msg, nil
}

func (s *Store) DeleteMessageFull(ctx context.Context, id string) error {
	msg, err := s.DB.GetMessage(id)
	if err != nil {
		return err
	}
	if err := s.Bucket.DeleteBody(ctx, msg.GCSKey); err != nil {
		fmt.Printf("warning: failed to delete GCS object %s: %v\n", msg.GCSKey, err)
	}
	return s.DB.DeleteMessage(id)
}

// collectLocalRecipients returns the full email addresses of recipients
// that exist as local users.
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
