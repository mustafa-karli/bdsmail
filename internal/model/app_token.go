package model

import "time"

// AppToken represents an API key for an application to send email via REST API.
type AppToken struct {
	ID          string
	Name        string // e.g. "Registration App", "Newsletter System"
	TokenHash   string // bcrypt hash of the token (token shown once at creation)
	Domain      string
	SenderEmail string // which email address this token can send from
	CreatedBy   string
	CreatedAt   time.Time
	LastUsedAt  time.Time
}
