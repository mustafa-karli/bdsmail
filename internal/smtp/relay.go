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
)

type Relay struct {
	mu       sync.RWMutex
	dkimKeys map[string]crypto.Signer // domain -> private key
	selector string
}

func NewRelay(dkimKeys map[string]crypto.Signer, selector string) *Relay {
	if dkimKeys == nil {
		dkimKeys = make(map[string]crypto.Signer)
	}
	return &Relay{dkimKeys: dkimKeys, selector: selector}
}

// AddDKIMKey adds a DKIM signing key for a domain at runtime.
func (r *Relay) AddDKIMKey(domain string, key crypto.Signer) {
	r.mu.Lock()
	r.dkimKeys[domain] = key
	r.mu.Unlock()
}

func (r *Relay) Send(from string, to []string, subject, contentType, body, messageID string) error {
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
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("From: %s\r\n", from))
	sb.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(to, ", ")))
	sb.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	if messageID != "" {
		sb.WriteString(fmt.Sprintf("Message-ID: %s\r\n", messageID))
	}
	sb.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 -0700")))
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString(fmt.Sprintf("Content-Type: %s; charset=UTF-8\r\n", contentType))
	sb.WriteString("\r\n")
	sb.WriteString(body)
	return sb.String()
}
