package auth

import (
	"fmt"
	"strings"
	"time"

	"github.com/mustafakarli/bdsmail/internal/cryptoutil"
	"github.com/mustafakarli/bdsmail/internal/model"
	"github.com/mustafakarli/bdsmail/internal/store"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

const (
	backupCodeCount  = 10
	backupCodeLength = 8
	trustedDeviceTTL = 30 * 24 * time.Hour // 30 days
	loginTokenTTL    = 5 * time.Minute
	otpTTL           = 2 * time.Minute
	bcryptCost       = 10
)

// Service handles 2FA, OTP, and trusted device operations.
type Service struct {
	db     store.Database
	issuer string
}

func NewService(db store.Database, issuer string) *Service {
	return &Service{db: db, issuer: issuer}
}

// --- 2FA Setup ---

// Setup2FA generates a TOTP secret and backup codes. Returns the secret, QR URI, and plaintext backup codes.
// The user must confirm with Confirm2FA before 2FA is active.
func (s *Service) Setup2FA(email string) (secret, qrURI string, backupCodes []string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      s.issuer,
		AccountName: email,
	})
	if err != nil {
		return "", "", nil, fmt.Errorf("generate TOTP key: %w", err)
	}

	backupCodes = make([]string, backupCodeCount)
	hashedCodes := make([]string, backupCodeCount)
	for i := range backupCodes {
		code, _ := cryptoutil.RandomHex(backupCodeLength / 2) // 4 bytes = 8 hex chars
		backupCodes[i] = code
		hash, _ := bcrypt.GenerateFromPassword([]byte(code), bcryptCost)
		hashedCodes[i] = string(hash)
	}

	// Store as pending — Enable2FA activates it
	backupStr := strings.Join(hashedCodes, "|")
	if err := s.db.Enable2FA(email, key.Secret(), backupStr); err != nil {
		return "", "", nil, fmt.Errorf("store 2FA secret: %w", err)
	}

	return key.Secret(), key.URL(), backupCodes, nil
}

// Verify2FA validates a TOTP code against the user's stored secret.
func (s *Service) Verify2FA(email, code string) (bool, error) {
	enabled, secret, _, err := s.db.Get2FAStatus(email)
	if err != nil {
		return false, err
	}
	if !enabled || secret == "" {
		return false, fmt.Errorf("2FA not configured")
	}
	return totp.Validate(code, secret), nil
}

// Disable2FA removes 2FA for the user after verifying the current code.
func (s *Service) Disable2FA(email, code string) error {
	valid, err := s.Verify2FA(email, code)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("invalid 2FA code")
	}
	return s.db.Disable2FA(email)
}

// VerifyBackupCode checks a one-time backup code. On success, the code is consumed.
func (s *Service) VerifyBackupCode(email, code string) (bool, error) {
	enabled, secret, backupStr, err := s.db.Get2FAStatus(email)
	if err != nil || !enabled {
		return false, err
	}
	if backupStr == "" {
		return false, nil
	}

	codes := strings.Split(backupStr, "|")
	for i, hashedCode := range codes {
		if bcrypt.CompareHashAndPassword([]byte(hashedCode), []byte(code)) == nil {
			// Consume the code
			remaining := append(codes[:i], codes[i+1:]...)
			newBackups := strings.Join(remaining, "|")
			s.db.Enable2FA(email, secret, newBackups)
			return true, nil
		}
	}
	return false, nil
}

// --- Trusted Devices ---

// RegisterTrustedDevice marks a device as trusted for 30 days.
func (s *Service) RegisterTrustedDevice(email, fingerprint, name string) error {
	id, err := cryptoutil.RandomHex(16)
	if err != nil {
		return err
	}
	return s.db.CreateTrustedDevice(&model.TrustedDevice{
		ID:          id,
		UserEmail:   email,
		Fingerprint: fingerprint,
		Name:        name,
		ExpiresAt:   time.Now().Add(trustedDeviceTTL),
	})
}

// IsTrustedDevice checks if a device can bypass 2FA.
func (s *Service) IsTrustedDevice(email, fingerprint string) (bool, error) {
	return s.db.IsTrustedDevice(email, fingerprint)
}

// ListTrustedDevices returns all active trusted devices for a user.
func (s *Service) ListTrustedDevices(email string) ([]*model.TrustedDevice, error) {
	return s.db.ListTrustedDevices(email)
}

// RevokeTrustedDevice removes a trusted device.
func (s *Service) RevokeTrustedDevice(id string) error {
	return s.db.RevokeTrustedDevice(id)
}

// --- Login Tokens (2FA pending state) ---

// CreateLoginToken generates a temporary token for the 2FA verification step.
func (s *Service) CreateLoginToken(email string) (string, error) {
	token, err := cryptoutil.RandomHex(32)
	if err != nil {
		return "", err
	}
	lt := &model.LoginToken{
		Token:     token,
		UserEmail: email,
		ExpiresAt: time.Now().Add(loginTokenTTL),
	}
	if err := s.db.CreateLoginToken(lt); err != nil {
		return "", err
	}
	return token, nil
}

// ValidateLoginToken verifies a login token and returns the associated email.
func (s *Service) ValidateLoginToken(token string) (string, error) {
	lt, err := s.db.GetLoginToken(token)
	if err != nil {
		return "", fmt.Errorf("invalid login token")
	}
	if lt.IsExpired() {
		s.db.DeleteLoginToken(token)
		return "", fmt.Errorf("login token expired")
	}
	return lt.UserEmail, nil
}

// ConsumeLoginToken validates and deletes the token in one step.
func (s *Service) ConsumeLoginToken(token string) (string, error) {
	email, err := s.ValidateLoginToken(token)
	if err != nil {
		return "", err
	}
	s.db.DeleteLoginToken(token)
	return email, nil
}
