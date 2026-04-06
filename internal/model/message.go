package model

import "time"

type Attachment struct {
	ID            string `json:"id"`
	MailContentID string `json:"mailContentId"` // FK to mail_content.id
	Filename      string `json:"filename"`
	ContentType   string `json:"content_type"`
	Size          int64  `json:"size"`
	BucketKey     string `json:"bucket_key"`
}

type Message struct {
	ID          string
	MessageID   string
	From        string
	To          []string
	CC          []string
	BCC         []string
	Subject     string
	ContentType string
	GCSKey      string // deprecated, kept for backward compat
	OwnerUser   string
	Folder      string
	Seen        bool
	Deleted     bool
	ReceivedAt  time.Time
	Body        string
	Attachments []Attachment // JSON-serialized in DB
}

func (m *Message) HasAttachments() bool {
	return len(m.Attachments) > 0
}

func (m *Message) TotalAttachmentSize() int64 {
	var total int64
	for _, a := range m.Attachments {
		total += a.Size
	}
	return total
}
