package model

import (
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           int64
	Username     string // local part (e.g. "alice")
	Domain       string // domain (e.g. "domain1.com")
	DisplayName  string // display name (e.g. "Alice Smith")
	PasswordHash string
	CreatedAt    time.Time
}

// Email returns the full email address user@domain.
func (u *User) Email() string {
	return u.Username + "@" + u.Domain
}

// FormattedFrom returns "Display Name <user@domain>" or just "user@domain".
func (u *User) FormattedFrom() string {
	if u.DisplayName != "" {
		return u.DisplayName + " <" + u.Email() + ">"
	}
	return u.Email()
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	return err == nil
}
