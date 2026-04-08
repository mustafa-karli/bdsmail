package cryptoutil

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// RandomHex generates a cryptographically secure random hex string of the given byte length.
func RandomHex(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto random failed: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// MustRandomHex generates a random hex string, panicking on failure (use only at init time).
func MustRandomHex(byteLen int) string {
	s, err := RandomHex(byteLen)
	if err != nil {
		panic(err)
	}
	return s
}

// HashSecret hashes a secret using bcrypt.
func HashSecret(secret string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hash failed: %w", err)
	}
	return string(hash), nil
}

// CheckSecret compares a bcrypt hash with a plaintext secret.
func CheckSecret(hash, secret string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil
}

// SHA256Hex returns the SHA-256 hash of a string as a hex string.
func SHA256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
