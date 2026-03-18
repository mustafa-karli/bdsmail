package smtp

import (
	"bytes"
	"crypto"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-msgauth/dkim"
	"github.com/mustafakarli/bdsmail/internal/mimeutil"
)

type Relay struct {
	mu            sync.RWMutex
	dkimKeys      map[string]crypto.Signer // domain -> private key
	selector      string
	relayHost     string // external SMTP relay host (e.g. smtp.sendgrid.net)
	relayPort     string // external relay port (default 587)
	relayUser     string
	relayPassword string
}

func NewRelay(dkimKeys map[string]crypto.Signer, selector string) *Relay {
	if dkimKeys == nil {
		dkimKeys = make(map[string]crypto.Signer)
	}
	return &Relay{dkimKeys: dkimKeys, selector: selector}
}

// SetExternalRelay configures an external SMTP relay for outbound delivery.
func (r *Relay) SetExternalRelay(host, port, user, password string) {
	r.relayHost = host
	r.relayPort = port
	r.relayUser = user
	r.relayPassword = password
	if r.relayHost != "" {
		log.Printf("External SMTP relay configured: %s:%s", host, port)
	}
}

// AddDKIMKey adds a DKIM signing key for a domain at runtime.
func (r *Relay) AddDKIMKey(domain string, key crypto.Signer) {
	r.mu.Lock()
	r.dkimKeys[domain] = key
	r.mu.Unlock()
}

func (r *Relay) Send(from string, to []string, subject, contentType, body, messageID string) error {
	// Use external relay if configured
	if r.relayHost != "" {
		return r.sendViaExternalRelay(from, to, subject, contentType, body, messageID)
	}

	// Direct delivery via MX lookup
	byDomain := make(map[string][]string)
	for _, addr := range to {
		parts := strings.SplitN(addr, "@", 2)
		if len(parts) != 2 {
			continue
		}
		domain := parts[1]
		byDomain[domain] = append(byDomain[domain], addr)
	}

	for domain, recipients := range byDomain {
		if err := r.sendToDomain(from, recipients, domain, subject, contentType, body, messageID); err != nil {
			log.Printf("relay to %s failed: %v", domain, err)
			return err
		}
	}
	return nil
}

func (r *Relay) sendViaExternalRelay(from string, to []string, subject, contentType, body, messageID string) error {
	addr := r.relayHost + ":" + r.relayPort
	msg := buildMessage(from, to, subject, contentType, body, messageID)
	signed := r.signDKIM(from, msg)

	auth := smtp.PlainAuth("", r.relayUser, r.relayPassword, r.relayHost)

	for attempt := 0; attempt < 3; attempt++ {
		err := smtp.SendMail(addr, auth, from, to, signed)
		if err == nil {
			return nil
		}
		log.Printf("external relay attempt %d to %s failed: %v", attempt+1, addr, err)
		time.Sleep(time.Duration(attempt+1) * 5 * time.Second)
	}
	return fmt.Errorf("external relay failed after 3 attempts to %s", addr)
}

func (r *Relay) sendToDomain(from string, to []string, domain, subject, contentType, body, messageID string) error {
	mxRecords, err := net.LookupMX(domain)
	if err != nil || len(mxRecords) == 0 {
		mxRecords = []*net.MX{{Host: domain, Pref: 0}}
	}

	var lastErr error
	for _, mx := range mxRecords {
		host := strings.TrimSuffix(mx.Host, ".")

		for attempt := 0; attempt < 3; attempt++ {
			err := r.tryRelay(from, to, host, subject, contentType, body, messageID)
			if err == nil {
				return nil
			}
			lastErr = err
			log.Printf("relay attempt %d to %s failed: %v", attempt+1, host, err)
			time.Sleep(time.Duration(attempt+1) * 5 * time.Second)
		}
	}
	return fmt.Errorf("all relay attempts failed: %w", lastErr)
}

func (r *Relay) tryRelay(from string, to []string, host, subject, contentType, body, messageID string) error {
	addr := host + ":25"
	msg := buildMessage(from, to, subject, contentType, body, messageID)
	signed := r.signDKIM(from, msg)
	return smtp.SendMail(addr, nil, from, to, signed)
}

func (r *Relay) signDKIM(from, msg string) []byte {
	parts := strings.SplitN(from, "@", 2)
	if len(parts) != 2 {
		return []byte(msg)
	}
	senderDomain := parts[1]

	r.mu.RLock()
	key, ok := r.dkimKeys[senderDomain]
	r.mu.RUnlock()

	if !ok {
		return []byte(msg)
	}

	opts := &dkim.SignOptions{
		Domain:   senderDomain,
		Selector: r.selector,
		Signer:   key,
		HeaderKeys: []string{
			"From", "To", "Subject", "Date", "Message-ID", "MIME-Version", "Content-Type",
		},
	}

	var signed bytes.Buffer
	err := dkim.Sign(&signed, strings.NewReader(msg), opts)
	if err != nil {
		log.Printf("DKIM signing failed for %s: %v, sending unsigned", senderDomain, err)
		return []byte(msg)
	}

	return signed.Bytes()
}

func buildMessage(from string, to []string, subject, contentType, body, messageID string) string {
	return mimeutil.BuildRFC822(from, to, nil, subject, contentType, body, messageID, nil)
}
