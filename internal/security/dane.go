package security

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"fmt"
	"log"
	"time"

	"github.com/miekg/dns"
)

// DANEChecker verifies TLSA records for outbound SMTP connections.
type DANEChecker struct {
	timeout  time.Duration
	resolver string // DNS resolver address (e.g. "1.1.1.1:53")
}

// TLSAResult contains the result of a TLSA lookup.
type TLSAResult struct {
	HasTLSA     bool
	Records     []TLSARecord
	DNSSECValid bool
}

// TLSARecord represents a parsed TLSA DNS record.
type TLSARecord struct {
	Usage        uint8 // 2=DANE-TA, 3=DANE-EE
	Selector     uint8 // 0=full cert, 1=SubjectPublicKeyInfo
	MatchingType uint8 // 0=exact, 1=SHA-256, 2=SHA-512
	CertData     []byte
}

func NewDANEChecker(timeout time.Duration, resolver string) *DANEChecker {
	if resolver == "" {
		resolver = "1.1.1.1:53"
	}
	return &DANEChecker{timeout: timeout, resolver: resolver}
}

// LookupTLSA queries TLSA records for _port._tcp.host.
func (d *DANEChecker) LookupTLSA(ctx context.Context, port int, host string) (*TLSAResult, error) {
	name := fmt.Sprintf("_%d._tcp.%s.", port, host)

	msg := new(dns.Msg)
	msg.SetQuestion(name, dns.TypeTLSA)
	msg.SetEdns0(4096, true) // enable DNSSEC OK flag
	msg.RecursionDesired = true

	client := new(dns.Client)
	client.Timeout = d.timeout

	resp, _, err := client.ExchangeContext(ctx, msg, d.resolver)
	if err != nil {
		return nil, fmt.Errorf("dane: DNS query for %s failed: %w", name, err)
	}

	result := &TLSAResult{
		DNSSECValid: resp.AuthenticatedData,
	}

	for _, rr := range resp.Answer {
		tlsa, ok := rr.(*dns.TLSA)
		if !ok {
			continue
		}
		result.HasTLSA = true
		result.Records = append(result.Records, TLSARecord{
			Usage:        tlsa.Usage,
			Selector:     tlsa.Selector,
			MatchingType: tlsa.MatchingType,
			CertData:     []byte(tlsa.Certificate),
		})
	}

	return result, nil
}

// VerifyCert checks the peer certificate chain against TLSA records.
// Returns true if any TLSA record matches, or if there are no TLSA records.
func (d *DANEChecker) VerifyCert(result *TLSAResult, peerCerts []*x509.Certificate) bool {
	if result == nil || !result.HasTLSA {
		return true // no TLSA records = no DANE constraint
	}
	if !result.DNSSECValid {
		log.Printf("security: DANE TLSA records found but DNSSEC not validated (AD=0), skipping DANE")
		return true // can't trust TLSA without DNSSEC
	}
	if len(peerCerts) == 0 {
		return false
	}

	for _, record := range result.Records {
		for _, cert := range peerCerts {
			if matchTLSA(record, cert) {
				return true
			}
		}
	}
	return false
}

func matchTLSA(record TLSARecord, cert *x509.Certificate) bool {
	// Only support DANE-EE (3) and DANE-TA (2) usages
	if record.Usage != 2 && record.Usage != 3 {
		return false
	}

	var data []byte
	switch record.Selector {
	case 0: // full certificate
		data = cert.Raw
	case 1: // SubjectPublicKeyInfo
		data = cert.RawSubjectPublicKeyInfo
	default:
		return false
	}

	var hash string
	switch record.MatchingType {
	case 0: // exact match
		hash = fmt.Sprintf("%x", data)
	case 1: // SHA-256
		h := sha256.Sum256(data)
		hash = fmt.Sprintf("%x", h[:])
	case 2: // SHA-512
		h := sha512.Sum512(data)
		hash = fmt.Sprintf("%x", h[:])
	default:
		return false
	}

	return hash == fmt.Sprintf("%x", record.CertData)
}
