package model

import "time"

// Domain represents a registered mail domain served by bdsmail.
type Domain struct {
	Name       string    // Primary key (e.g. "example.com")
	APIKeyHash string    // Bcrypt hash of per-domain API key
	SESStatus  string    // SES verification status
	DKIMStatus string    // SES DKIM status
	Status     string    // "active", "suspended"
	CreatedBy  string    // Email of the admin who onboarded
	CreatedAt  time.Time
}
