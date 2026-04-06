package model

import "time"

// TrustedDevice represents a device that can bypass 2FA for a user.
type TrustedDevice struct {
	ID          string    `json:"id"`
	UserEmail   string    `json:"userEmail"`
	Fingerprint string    `json:"fingerprint"`
	Name        string    `json:"name"`
	TrustedAt   time.Time `json:"trustedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	LastSeenAt  time.Time `json:"lastSeenAt,omitempty"`
}

func (d *TrustedDevice) IsExpired() bool {
	return time.Now().After(d.ExpiresAt)
}

// OTP represents a one-time password for email/SMS verification.
type OTP struct {
	ID        string
	UserEmail string
	Code      string
	Purpose   string // "login", "verify"
	ExpiresAt time.Time
	Attempts  int
}

func (o *OTP) IsExpired() bool {
	return time.Now().After(o.ExpiresAt)
}

func (o *OTP) MaxAttemptsReached() bool {
	return o.Attempts >= 5
}

// LoginToken is a temporary token for the 2FA pending state.
type LoginToken struct {
	Token     string
	UserEmail string
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (t *LoginToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}
