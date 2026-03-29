package model

import "time"

// Contact represents a CardDAV contact stored as vCard data.
type Contact struct {
	ID         string
	OwnerEmail string
	VCardData  string
	ETag       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
