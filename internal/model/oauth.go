package model

import (
	"time"

	"github.com/mustafakarli/bdsmail/internal/cryptoutil"
)

// OAuthClient represents a registered application that can use "Sign in with bdsmail".
type OAuthClient struct {
	ID          string
	Name        string
	ClientID    string
	SecretHash  string
	RedirectURI string
	Domain      string
	CreatedBy   string
	CreatedAt   time.Time
}

func (c *OAuthClient) CheckSecret(secret string) bool {
	return cryptoutil.CheckSecret(c.SecretHash, secret)
}

// OAuthCode is a short-lived authorization code exchanged for tokens.
type OAuthCode struct {
	Code        string
	ClientID    string
	UserEmail   string
	RedirectURI string
	Scope       string
	Nonce       string
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
