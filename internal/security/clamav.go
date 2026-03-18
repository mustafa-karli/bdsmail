package security

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/dutchcoders/go-clamd"
)

type ClamAVScanner struct {
	address string
	timeout time.Duration
}

func NewClamAVScanner(address string, timeout time.Duration) *ClamAVScanner {
	return &ClamAVScanner{address: address, timeout: timeout}
}

// Scan checks the body for viruses using clamd.
// Returns virusFound=true and the virus name if detected.
// Returns an error if clamd is unreachable (caller should fail-open).
func (s *ClamAVScanner) Scan(ctx context.Context, body []byte) (virusFound bool, virusName string, err error) {
	clam := clamd.NewClamd(s.address)

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	responseCh, err := clam.ScanStream(strings.NewReader(string(body)), make(chan bool))
	if err != nil {
		return false, "", fmt.Errorf("clamd connection failed: %w", err)
	}

	for response := range responseCh {
		if response.Status == clamd.RES_FOUND {
			log.Printf("clamav: virus detected: %s", response.Description)
			return true, response.Description, nil
		}
		if response.Status == clamd.RES_ERROR {
			return false, "", fmt.Errorf("clamd scan error: %s", response.Description)
		}
	}

	return false, "", nil
}
