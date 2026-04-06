package model

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

// OAuthClient represents a registered application that can use "Sign in with bdsmail".
type OAuthClient struct {
	ID           string    // UUID
	Name         string    // App display name
	ClientID     string    // Public identifier (random hex)
	SecretHash   string    // Bcrypt hash of client_secret
	RedirectURI  string    // Callback URL
	OwnerEmail   string    // Developer who registered the app
	CreatedAt    time.Time
}

func (c *OAuthClient) CheckSecret(secret string) bool {
	return checkHash(c.SecretHash, secret)
}

// OAuthCode is a short-lived authorization code exchanged for tokens.
type OAuthCode struct {
	Code        string
	ClientID    string
	UserEmail   string
	RedirectURI string
	Scope       string
	Nonce       string    // OIDC nonce for replay protection
	ExpiresAt   time.Time
	Used        bool
}

func (c *OAuthCode) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// OAuthToken is an access token issued to an application.
type OAuthToken struct {
	Token     string
	ClientID  string
	UserEmail string
	Scope     string
	ExpiresAt time.Time
}

func (t *OAuthToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

func checkHash(hash, value string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(value)) == nil
}
