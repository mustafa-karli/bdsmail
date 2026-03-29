package model

import "time"

// MailingList represents a group email distribution list.
type MailingList struct {
	ListAddress string
	Name        string
	Description string
	OwnerEmail  string
	CreatedAt   time.Time
}
