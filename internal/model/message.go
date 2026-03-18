package model

import "time"

type Message struct {
	ID          string
	MessageID   string
	From        string
	To          []string
	CC          []string
	BCC         []string
	Subject     string
	ContentType string
	GCSKey      string
	OwnerUser   string
	Folder      string
	Seen        bool
	Deleted     bool
	ReceivedAt  time.Time
	Body        string // not stored in DB, loaded from GCS on demand
}
