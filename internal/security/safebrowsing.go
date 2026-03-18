package security

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var urlRegex = regexp.MustCompile(`https?://[^\s<>"'\` + "`" + `\)\]]+`)
var hrefRegex = regexp.MustCompile(`(?i)href\s*=\s*["'](https?://[^"']+)["']`)

type SafeBrowsing struct {
	apiKey  string
	timeout time.Duration
	client  *http.Client
}

func NewSafeBrowsing(apiKey string, timeout time.Duration) *SafeBrowsing {
	return &SafeBrowsing{
		apiKey:  apiKey,
		timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

// ExtractURLs extracts all URLs from the email body.
func (s *SafeBrowsing) ExtractURLs(body string, contentType string) []string {
	seen := make(map[string]bool)
	var urls []string

	// Extract from plain text / HTML body using generic URL regex
	for _, u := range urlRegex.FindAllString(body, -1) {
		u = strings.TrimRight(u, ".,;:!?")
		if !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}

	// Also extract href attributes from HTML
	if strings.Contains(contentType, "html") {
		for _, match := range hrefRegex.FindAllStringSubmatch(body, -1) {
			if len(match) > 1 && !seen[match[1]] {
				seen[match[1]] = true
				urls = append(urls, match[1])
			}
		}
	}

	return urls
}

// CheckURLs checks a list of URLs against the Google Safe Browsing v4 API.
// Returns the list of URLs flagged as dangerous.
func (s *SafeBrowsing) CheckURLs(ctx context.Context, urls []string) ([]string, error) {
	if len(urls) == 0 {
		return nil, nil
	}

	// Build threat entries
	var threatEntries []map[string]string
	for _, u := range urls {
		threatEntries = append(threatEntries, map[string]string{"url": u})
	}

	reqBody := map[string]interface{}{
		"client": map[string]string{
			"clientId":      "bdsmail",
			"clientVersion": "1.0.0",
		},
		"threatInfo": map[string]interface{}{
			"threatTypes":      []string{"MALWARE", "SOCIAL_ENGINEERING", "UNWANTED_SOFTWARE", "POTENTIALLY_HARMFUL_APPLICATION"},
			"platformTypes":    []string{"ANY_PLATFORM"},
			"threatEntryTypes": []string{"URL"},
			"threatEntries":    threatEntries,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := "https://safebrowsing.googleapis.com/v4/threatMatches:find?key=" + s.apiKey
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("safe browsing API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("safe browsing API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Matches []struct {
			Threat struct {
				URL string `json:"url"`
			} `json:"threat"`
			ThreatType string `json:"threatType"`
		} `json:"matches"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var dangerous []string
	seen := make(map[string]bool)
	for _, match := range result.Matches {
		if !seen[match.Threat.URL] {
			seen[match.Threat.URL] = true
			dangerous = append(dangerous, match.Threat.URL)
			log.Printf("safebrowsing: dangerous URL detected: %s (%s)", match.Threat.URL, match.ThreatType)
		}
	}

	return dangerous, nil
}
