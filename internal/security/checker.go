package security

import (
	"context"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"strings"
)

type CheckResult struct {
	Reject        bool
	SubjectPrefix string
	Folder        string
	Reason        string
}

type Checker struct {
	clamav      *ClamAVScanner
	safeBrowse  *SafeBrowsing
	authVerify  *AuthVerifier
	rateLimiter *RateLimiter
	rspamd      *RspamdScanner
	mtasts      *MTASTSChecker
	dane        *DANEChecker
	tlsReporter *TLSReporter
}

func NewChecker(cfg *Config) (*Checker, error) {
	c := &Checker{}

	if cfg.ClamAVEnabled {
		c.clamav = NewClamAVScanner(cfg.ClamAVAddress, cfg.ClamAVTimeout)
		log.Printf("security: ClamAV enabled (address: %s)", cfg.ClamAVAddress)
	}

	if cfg.SafeBrowsingEnabled {
		if cfg.SafeBrowsingAPIKey == "" {
			return nil, fmt.Errorf("BDS_SAFEBROWSING_API_KEY is required when Safe Browsing is enabled")
		}
		c.safeBrowse = NewSafeBrowsing(cfg.SafeBrowsingAPIKey, cfg.SafeBrowsingTimeout)
		log.Printf("security: Google Safe Browsing enabled")
	}

	if cfg.AuthCheckEnabled {
		c.authVerify = NewAuthVerifier(cfg.AuthCheckTimeout)
		log.Printf("security: SPF/DKIM/DMARC verification enabled")
	}

	if cfg.RspamdEnabled {
		c.rspamd = NewRspamdScanner(cfg.RspamdURL, cfg.RspamdTimeout, cfg.RspamdRejectScore, cfg.RspamdJunkScore)
		log.Printf("security: Rspamd enabled (url: %s, reject>=%.1f, junk>=%.1f)", cfg.RspamdURL, cfg.RspamdRejectScore, cfg.RspamdJunkScore)
	}

	if cfg.MTASTSEnabled {
		c.mtasts = NewMTASTSChecker(cfg.MTASTSTimeout)
		log.Printf("security: MTA-STS enabled")
	}

	if cfg.DANEEnabled {
		c.dane = NewDANEChecker(cfg.DANETimeout, cfg.DANEResolver)
		log.Printf("security: DANE/TLSA enabled (resolver: %s)", cfg.DANEResolver)
	}

	if cfg.TLSRPTEnabled {
		senderDomain := cfg.TLSRPTSenderDomain
		if idx := strings.Index(senderDomain, ","); idx != -1 {
			senderDomain = senderDomain[:idx]
		}
		c.tlsReporter = NewTLSReporter(senderDomain, cfg.TLSRPTInterval)
		log.Printf("security: TLSRPT enabled (interval: %v, sender: %s)", cfg.TLSRPTInterval, senderDomain)
	}

	if cfg.RateLimitEnabled {
		c.rateLimiter = NewRateLimiter(cfg)
		log.Printf("security: rate limiting enabled (conn: %.0f/s burst %d, lockout after %d failures for %v)",
			cfg.RateLimitConnPerSec, cfg.RateLimitConnBurst, cfg.RateLimitMaxAuthFail, cfg.RateLimitLockoutDur)
	}

	return c, nil
}

// AllowConnection checks if the IP is within its connection rate limit.
func (c *Checker) AllowConnection(ip net.IP) bool {
	if c.rateLimiter == nil {
		return true
	}
	return c.rateLimiter.AllowConnection(ip)
}

// IsLockedOut returns true if the IP has exceeded the max auth failure threshold.
func (c *Checker) IsLockedOut(ip net.IP) bool {
	if c.rateLimiter == nil {
		return false
	}
	return c.rateLimiter.IsLockedOut(ip)
}

// RecordAuthResult records the result of an authentication attempt.
func (c *Checker) RecordAuthResult(ip net.IP, success bool) {
	if c.rateLimiter == nil {
		return
	}
	if success {
		c.rateLimiter.RecordAuthSuccess(ip)
	} else {
		c.rateLimiter.RecordAuthFailure(ip)
	}
}

// GetMTASTSPolicy returns the MTA-STS policy for a domain, or nil if MTA-STS is disabled.
func (c *Checker) GetMTASTSPolicy(ctx context.Context, domain string) (*MTASTSPolicy, error) {
	if c.mtasts == nil {
		return nil, nil
	}
	return c.mtasts.GetPolicy(ctx, domain)
}

// ValidateMTASTSMX checks if an MX host is allowed by the MTA-STS policy.
func (c *Checker) ValidateMTASTSMX(policy *MTASTSPolicy, mxHost string) bool {
	if c.mtasts == nil {
		return true
	}
	return c.mtasts.ValidateMX(policy, mxHost)
}

// LookupTLSA queries TLSA records for a given host and port.
func (c *Checker) LookupTLSA(ctx context.Context, port int, host string) (*TLSAResult, error) {
	if c.dane == nil {
		return nil, nil
	}
	return c.dane.LookupTLSA(ctx, port, host)
}

// VerifyDANECert checks peer certificates against TLSA records.
func (c *Checker) VerifyDANECert(result *TLSAResult, peerCerts []*x509.Certificate) bool {
	if c.dane == nil {
		return true
	}
	return c.dane.VerifyCert(result, peerCerts)
}

// RecordTLSSuccess records a successful TLS connection for TLSRPT.
func (c *Checker) RecordTLSSuccess(recipientDomain, mx string) {
	if c.tlsReporter == nil {
		return
	}
	c.tlsReporter.RecordSuccess(recipientDomain, mx)
}

// RecordTLSFailure records a TLS connection failure for TLSRPT.
func (c *Checker) RecordTLSFailure(recipientDomain string, event TLSFailureEvent) {
	if c.tlsReporter == nil {
		return
	}
	c.tlsReporter.RecordFailure(recipientDomain, event)
}

// SetTLSRPTSendFunc sets the function used to send TLSRPT report emails.
func (c *Checker) SetTLSRPTSendFunc(fn func(from string, to []string, subject, contentType, body, messageID string) error) {
	if c.tlsReporter == nil {
		return
	}
	c.tlsReporter.SetSendFunc(fn)
}

// CheckInbound runs all enabled checks on an inbound email.
// rawEmail is the full RFC 5322 message (needed for DKIM verification).
// bodyText is the parsed message body, contentType is "text/plain" or "text/html".
func (c *Checker) CheckInbound(ctx context.Context, rawEmail []byte, bodyText string, contentType string, remoteIP net.IP, from string, ehloHostname string) *CheckResult {
	result := &CheckResult{Folder: "INBOX"}

	// 1. ClamAV virus scan
	if c.clamav != nil {
		virusFound, virusName, err := c.clamav.Scan(ctx, rawEmail)
		if err != nil {
			log.Printf("security: ClamAV error (fail-open): %v", err)
		} else if virusFound {
			result.Reject = true
			result.Reason = "virus detected: " + virusName
			return result
		}
	}

	// 2. SPF/DKIM/DMARC verification
	if c.authVerify != nil {
		authResult, err := c.authVerify.Verify(ctx, remoteIP, from, ehloHostname, rawEmail)
		if err != nil {
			log.Printf("security: auth verification error (fail-open): %v", err)
		} else {
			if authResult.Reject {
				result.Reject = true
				result.Reason = "DMARC policy reject: sender authentication failed"
				return result
			}
			if authResult.Quarantine {
				result.Folder = "Junk"
			}
		}
	}

	// 3. Rspamd spam scoring
	if c.rspamd != nil {
		rspamdResult, err := c.rspamd.Scan(ctx, rawEmail, remoteIP, from)
		if err != nil {
			log.Printf("security: Rspamd error (fail-open): %v", err)
		} else {
			if rspamdResult.Reject {
				result.Reject = true
				result.Reason = fmt.Sprintf("spam score %.2f exceeds threshold", rspamdResult.Score)
				return result
			}
			if rspamdResult.Junk && result.Folder != "Junk" {
				result.Folder = "Junk"
			}
		}
	}

	// 4. Safe Browsing URL check
	if c.safeBrowse != nil {
		urls := c.safeBrowse.ExtractURLs(bodyText, contentType)
		if len(urls) > 0 {
			dangerous, err := c.safeBrowse.CheckURLs(ctx, urls)
			if err != nil {
				log.Printf("security: Safe Browsing error (fail-open): %v", err)
			} else if len(dangerous) > 0 {
				result.SubjectPrefix = "[WARNING: Suspicious Links]"
				log.Printf("security: %d dangerous URL(s) found in email from %s: %s", len(dangerous), from, strings.Join(dangerous, ", "))
			}
		}
	}

	return result
}

// CheckOutbound runs ClamAV and Safe Browsing checks on an outbound email.
// attachmentData is optional raw attachment bytes to scan individually.
func (c *Checker) CheckOutbound(ctx context.Context, bodyText string, contentType string, attachmentData ...[]byte) *CheckResult {
	result := &CheckResult{}

	// 1. ClamAV virus scan — body + each attachment
	if c.clamav != nil {
		virusFound, virusName, err := c.clamav.Scan(ctx, []byte(bodyText))
		if err != nil {
			log.Printf("security: ClamAV error (fail-open): %v", err)
		} else if virusFound {
			result.Reject = true
			result.Reason = "virus detected: " + virusName
			return result
		}
		// Scan each attachment separately
		for _, data := range attachmentData {
			virusFound, virusName, err = c.clamav.Scan(ctx, data)
			if err != nil {
				log.Printf("security: ClamAV attachment scan error (fail-open): %v", err)
			} else if virusFound {
				result.Reject = true
				result.Reason = "virus detected in attachment: " + virusName
				return result
			}
		}
	}

	// 2. Safe Browsing URL check
	if c.safeBrowse != nil {
		urls := c.safeBrowse.ExtractURLs(bodyText, contentType)
		if len(urls) > 0 {
			dangerous, err := c.safeBrowse.CheckURLs(ctx, urls)
			if err != nil {
				log.Printf("security: Safe Browsing error (fail-open): %v", err)
			} else if len(dangerous) > 0 {
				result.Reject = true
				result.Reason = "message contains dangerous links: " + strings.Join(dangerous, ", ")
				return result
			}
		}
	}

	return result
}
