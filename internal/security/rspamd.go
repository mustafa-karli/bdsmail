package security

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

// RspamdScanner integrates with an Rspamd instance via its HTTP API.
type RspamdScanner struct {
	url             string
	timeout         time.Duration
	client          *http.Client
	rejectThreshold float64
	junkThreshold   float64
}

type rspamdResponse struct {
	Score         float64                    `json:"score"`
	RequiredScore float64                   `json:"required_score"`
	Action        string                    `json:"action"`
	Symbols       map[string]rspamdSymbol   `json:"symbols"`
}

type rspamdSymbol struct {
	Name  string  `json:"name"`
	Score float64 `json:"score"`
}

func NewRspamdScanner(url string, timeout time.Duration, rejectThreshold, junkThreshold float64) *RspamdScanner {
	return &RspamdScanner{
		url:             url,
		timeout:         timeout,
		client:          &http.Client{Timeout: timeout},
		rejectThreshold: rejectThreshold,
		junkThreshold:   junkThreshold,
	}
}

// Scan sends the raw email to Rspamd for spam scoring.
func (s *RspamdScanner) Scan(ctx context.Context, rawEmail []byte, remoteIP net.IP, from string) (*RspamdResult, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url+"/checkv2", io.NopCloser(
		io.NewSectionReader(readerAt(rawEmail), 0, int64(len(rawEmail))),
	))
	if err != nil {
		return nil, fmt.Errorf("rspamd: create request: %w", err)
	}

	if remoteIP != nil {
		req.Header.Set("IP", remoteIP.String())
	}
	if from != "" {
		req.Header.Set("From", from)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rspamd: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rspamd: unexpected status %d", resp.StatusCode)
	}

	var rspamdResp rspamdResponse
	if err := json.NewDecoder(resp.Body).Decode(&rspamdResp); err != nil {
		return nil, fmt.Errorf("rspamd: decode response: %w", err)
	}

	result := &RspamdResult{
		Score:  rspamdResp.Score,
		Action: rspamdResp.Action,
	}

	if rspamdResp.Score >= s.rejectThreshold {
		result.Reject = true
	} else if rspamdResp.Score >= s.junkThreshold {
		result.Junk = true
	}

	log.Printf("security: rspamd score=%.2f action=%s reject=%v junk=%v", result.Score, result.Action, result.Reject, result.Junk)
	return result, nil
}

type RspamdResult struct {
	Score  float64
	Action string
	Reject bool
	Junk   bool
}

// readerAt wraps a byte slice to implement io.ReaderAt.
type readerAt []byte

func (r readerAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r)) {
		return 0, io.EOF
	}
	n := copy(p, r[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
