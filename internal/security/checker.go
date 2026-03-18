package security

import (
	"context"
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
	clamav     *ClamAVScanner
	safeBrowse *SafeBrowsing
	authVerify *AuthVerifier
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

	return c, nil
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

	// 3. Safe Browsing URL check
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
