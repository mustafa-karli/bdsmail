package security

import (
	"bytes"
	"context"
	"log"
	"net"
	"strings"
	"time"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-msgauth/dkim"
)

type AuthResult struct {
	SPFPass     bool
	DKIMPass    bool
	DMARCPolicy string // "none", "quarantine", "reject", or "" if no record
	Reject      bool
	Quarantine  bool
}

type AuthVerifier struct {
	timeout time.Duration
}

func NewAuthVerifier(timeout time.Duration) *AuthVerifier {
	return &AuthVerifier{timeout: timeout}
}

// Verify checks SPF, DKIM, and DMARC for an inbound email.
func (v *AuthVerifier) Verify(ctx context.Context, remoteIP net.IP, from string, ehloHostname string, rawEmail []byte) (*AuthResult, error) {
	result := &AuthResult{}

	_, senderDomain := splitEmail(from)
	if senderDomain == "" {
		return result, nil
	}

	// SPF check
	spfResult, _ := spf.CheckHostWithSender(remoteIP, ehloHostname, from)
	result.SPFPass = (spfResult == spf.Pass)
	if !result.SPFPass {
		log.Printf("authverify: SPF failed for %s from IP %s (result: %s)", from, remoteIP, spfResult)
	}

	// DKIM check
	verifications, err := dkim.Verify(bytes.NewReader(rawEmail))
	if err == nil {
		for _, v := range verifications {
			if v.Err == nil {
				result.DKIMPass = true
				break
			}
		}
	}
	if !result.DKIMPass {
		log.Printf("authverify: DKIM failed for %s", from)
	}

	// DMARC check
	result.DMARCPolicy = lookupDMARCPolicy(senderDomain)

	// Apply DMARC policy: reject/quarantine only if BOTH SPF and DKIM fail
	if !result.SPFPass && !result.DKIMPass {
		switch result.DMARCPolicy {
		case "reject":
			result.Reject = true
			log.Printf("authverify: DMARC reject for %s (SPF and DKIM both failed)", from)
		case "quarantine":
			result.Quarantine = true
			log.Printf("authverify: DMARC quarantine for %s (SPF and DKIM both failed)", from)
		}
	}

	return result, nil
}

// lookupDMARCPolicy fetches the DMARC TXT record and extracts the policy.
func lookupDMARCPolicy(domain string) string {
	records, err := net.LookupTXT("_dmarc." + domain)
	if err != nil {
		return ""
	}

	for _, record := range records {
		if !strings.HasPrefix(record, "v=DMARC1") {
			continue
		}
		parts := strings.Split(record, ";")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "p=") {
				return strings.TrimPrefix(part, "p=")
			}
		}
	}
	return ""
}

func splitEmail(email string) (string, string) {
	for i, c := range email {
		if c == '@' {
			return email[:i], email[i+1:]
		}
	}
	return email, ""
}
