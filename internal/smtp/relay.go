package smtp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-msgauth/dkim"
	"github.com/mustafakarli/bdsmail/internal/mimeutil"
	"github.com/mustafakarli/bdsmail/internal/security"
)

type Relay struct {
	mu            sync.RWMutex
	dkimKeys      map[string]crypto.Signer // domain -> private key
	selector      string
	relayHost     string // external SMTP relay host (e.g. smtp.sendgrid.net)
	relayPort     string // external relay port (default 587)
	relayUser     string
	relayPassword string
	checker       *security.Checker
}

func NewRelay(dkimKeys map[string]crypto.Signer, selector string, checker *security.Checker) *Relay {
	if dkimKeys == nil {
		dkimKeys = make(map[string]crypto.Signer)
	}
	return &Relay{dkimKeys: dkimKeys, selector: selector, checker: checker}
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

func (r *Relay) Send(from string, to []string, subject, contentType, body, messageID string, opts ...SendOption) error {
	var sendOpts sendOptions
	for _, o := range opts {
		o(&sendOpts)
	}

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
		if err := r.sendToDomain(from, recipients, domain, subject, contentType, body, messageID, sendOpts.requireTLS); err != nil {
			log.Printf("relay to %s failed: %v", domain, err)
			return err
		}
	}
	return nil
}

type sendOptions struct {
	requireTLS bool
}

// SendOption configures relay send behavior.
type SendOption func(*sendOptions)

// WithRequireTLS enforces TLS for the entire delivery path.
func WithRequireTLS() SendOption {
	return func(o *sendOptions) { o.requireTLS = true }
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

func (r *Relay) sendToDomain(from string, to []string, domain, subject, contentType, body, messageID string, forceRequireTLS bool) error {
	mxRecords, err := net.LookupMX(domain)
	if err != nil || len(mxRecords) == 0 {
		mxRecords = []*net.MX{{Host: domain, Pref: 0}}
	}

	// Fetch MTA-STS policy for the recipient domain
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var mtastsPolicy *security.MTASTSPolicy
	if r.checker != nil {
		mtastsPolicy, err = r.checker.GetMTASTSPolicy(ctx, domain)
		if err != nil {
			log.Printf("MTA-STS policy fetch for %s failed (continuing): %v", domain, err)
		}
	}

	var lastErr error
	for _, mx := range mxRecords {
		host := strings.TrimSuffix(mx.Host, ".")

		// MTA-STS: validate MX host against policy
		if mtastsPolicy != nil && mtastsPolicy.Mode == "enforce" {
			if !r.checker.ValidateMTASTSMX(mtastsPolicy, host) {
				log.Printf("MTA-STS: MX host %s not in policy for %s, skipping", host, domain)
				continue
			}
		}

		requireTLS := forceRequireTLS || (mtastsPolicy != nil && mtastsPolicy.Mode == "enforce")

		for attempt := 0; attempt < 3; attempt++ {
			err := r.tryRelay(from, to, host, domain, subject, contentType, body, messageID, requireTLS)
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

// tryRelay delivers mail to a specific MX host using a custom SMTP client
// with TLS control for MTA-STS enforcement.
func (r *Relay) tryRelay(from string, to []string, host, domain, subject, contentType, body, messageID string, requireTLS bool) error {
	msg := buildMessage(from, to, subject, contentType, body, messageID)
	signed := r.signDKIM(from, msg)

	addr := host + ":25"
	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp client %s: %w", host, err)
	}
	defer client.Close()

	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("ehlo %s: %w", host, err)
	}

	// DANE: lookup TLSA records before TLS handshake
	var tlsaResult *security.TLSAResult
	if r.checker != nil {
		tlsaResult, err = r.checker.LookupTLSA(context.Background(), 25, host)
		if err != nil {
			log.Printf("DANE TLSA lookup for %s failed (continuing): %v", host, err)
		}
	}

	// Attempt STARTTLS
	tlsEstablished := false
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsCfg := &tls.Config{ServerName: host}
		if err := client.StartTLS(tlsCfg); err != nil {
			if requireTLS {
				r.recordTLSFailure(domain, host, "starttls-not-supported", err.Error())
				return fmt.Errorf("MTA-STS requires TLS but STARTTLS failed for %s: %w", host, err)
			}
			r.recordTLSFailure(domain, host, "validation-failure", err.Error())
			log.Printf("STARTTLS failed for %s (continuing without TLS): %v", host, err)
		} else {
			tlsEstablished = true
			// DANE: verify peer certificate against TLSA records
			if tlsaResult != nil && tlsaResult.HasTLSA && r.checker != nil {
				tlsState, ok := client.TLSConnectionState()
				if ok && len(tlsState.PeerCertificates) > 0 {
					if !r.checker.VerifyDANECert(tlsaResult, tlsState.PeerCertificates) {
						r.recordTLSFailure(domain, host, "dane-required", "DANE certificate verification failed")
						return fmt.Errorf("DANE: certificate verification failed for %s", host)
					}
					log.Printf("DANE: certificate verified for %s", host)
				}
			}
		}
	} else if requireTLS {
		r.recordTLSFailure(domain, host, "starttls-not-supported", "STARTTLS not offered")
		return fmt.Errorf("MTA-STS requires TLS but %s does not support STARTTLS", host)
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM %s: %w", host, err)
	}
	for _, addr := range to {
		if err := client.Rcpt(addr); err != nil {
			return fmt.Errorf("RCPT TO %s at %s: %w", addr, host, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA %s: %w", host, err)
	}
	if _, err := w.Write(signed); err != nil {
		w.Close()
		return fmt.Errorf("write data %s: %w", host, err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close data %s: %w", host, err)
	}

	// Record TLS result for TLSRPT
	if tlsEstablished {
		r.recordTLSSuccess(domain, host)
	}

	return client.Quit()
}

func (r *Relay) recordTLSSuccess(domain, mx string) {
	if r.checker != nil {
		r.checker.RecordTLSSuccess(domain, mx)
	}
}

func (r *Relay) recordTLSFailure(domain, mx, resultType, reason string) {
	if r.checker != nil {
		r.checker.RecordTLSFailure(domain, security.TLSFailureEvent{
			ResultType:    resultType,
			ReceivingMX:   mx,
			FailureReason: reason,
			Timestamp:     time.Now(),
		})
	}
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
