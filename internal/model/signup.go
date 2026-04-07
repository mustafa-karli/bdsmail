package model

import "time"

// DomainSignup represents a pending domain registration awaiting DNS verification.
type DomainSignup struct {
	ID           string
	Domain       string
	Username     string
	DisplayName  string
	PasswordHash string
	Status       string // "pending", "verified", "expired"
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

func (s *DomainSignup) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}
