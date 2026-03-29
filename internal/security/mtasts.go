package security

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// MTASTSPolicy represents a parsed MTA-STS policy.
type MTASTSPolicy struct {
	Mode   string   // "enforce", "testing", "none"
	MX     []string // allowed MX hostnames (may include wildcards like "*.example.com")
	MaxAge int      // seconds
}

type mtastsCacheEntry struct {
	policy    *MTASTSPolicy
	fetchedAt time.Time
}

// MTASTSChecker fetches and caches MTA-STS policies.
type MTASTSChecker struct {
	mu      sync.RWMutex
	cache   map[string]*mtastsCacheEntry
	client  *http.Client
	timeout time.Duration
}

func NewMTASTSChecker(timeout time.Duration) *MTASTSChecker {
	return &MTASTSChecker{
		cache:   make(map[string]*mtastsCacheEntry),
		client:  &http.Client{Timeout: timeout},
		timeout: timeout,
	}
}

// GetPolicy fetches the MTA-STS policy for a domain, using cache when available.
func (m *MTASTSChecker) GetPolicy(ctx context.Context, domain string) (*MTASTSPolicy, error) {
	// Check cache
	m.mu.RLock()
	entry, ok := m.cache[domain]
	m.mu.RUnlock()
	if ok && time.Since(entry.fetchedAt) < time.Duration(entry.policy.MaxAge)*time.Second {
		return entry.policy, nil
	}

	// Check DNS for _mta-sts TXT record
	txtRecords, err := net.DefaultResolver.LookupTXT(ctx, "_mta-sts."+domain)
	if err != nil {
		return nil, fmt.Errorf("mta-sts: DNS lookup for _mta-sts.%s failed: %w", domain, err)
	}

	var hasMTASTS bool
	for _, txt := range txtRecords {
		if strings.HasPrefix(txt, "v=STSv1;") || strings.HasPrefix(txt, "v=STSv1 ") {
			hasMTASTS = true
			break
		}
	}
	if !hasMTASTS {
		return nil, nil // no MTA-STS policy
	}

	// Fetch policy via HTTPS
	policyURL := "https://mta-sts." + domain + "/.well-known/mta-sts.txt"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, policyURL, nil)
	if err != nil {
		return nil, fmt.Errorf("mta-sts: create request: %w", err)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mta-sts: fetch policy from %s: %w", policyURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mta-sts: policy fetch returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("mta-sts: read policy body: %w", err)
	}

	policy, err := parseMTASTSPolicy(string(body))
	if err != nil {
		return nil, fmt.Errorf("mta-sts: parse policy: %w", err)
	}

	// Cache the policy
	m.mu.Lock()
	m.cache[domain] = &mtastsCacheEntry{policy: policy, fetchedAt: time.Now()}
	m.mu.Unlock()

	log.Printf("security: MTA-STS policy for %s: mode=%s mx=%v max_age=%d", domain, policy.Mode, policy.MX, policy.MaxAge)
	return policy, nil
}

// ValidateMX checks if a given MX hostname is allowed by the MTA-STS policy.
func (m *MTASTSChecker) ValidateMX(policy *MTASTSPolicy, mxHost string) bool {
	if policy == nil {
		return true
	}
	mxHost = strings.TrimSuffix(mxHost, ".")
	for _, pattern := range policy.MX {
		if matchMXPattern(pattern, mxHost) {
			return true
		}
	}
	return false
}

func matchMXPattern(pattern, host string) bool {
	pattern = strings.ToLower(pattern)
	host = strings.ToLower(host)
	if pattern == host {
		return true
	}
	// Wildcard: "*.example.com" matches "mx1.example.com" but not "sub.mx1.example.com"
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		if strings.HasSuffix(host, suffix) && !strings.Contains(host[:len(host)-len(suffix)], ".") {
			return true
		}
	}
	return false
}

func parseMTASTSPolicy(body string) (*MTASTSPolicy, error) {
	policy := &MTASTSPolicy{MaxAge: 86400} // default 24h
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "version":
			if value != "STSv1" {
				return nil, fmt.Errorf("unsupported MTA-STS version: %s", value)
			}
		case "mode":
			policy.Mode = value
		case "mx":
			policy.MX = append(policy.MX, value)
		case "max_age":
			fmt.Sscanf(value, "%d", &policy.MaxAge)
		}
	}

	if policy.Mode == "" {
		return nil, fmt.Errorf("MTA-STS policy missing mode")
	}
	return policy, nil
}
