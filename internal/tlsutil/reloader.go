package tlsutil

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// CertStore manages per-domain TLS certificates with SNI-based selection.
// Each domain has its own cert at <dir>/<domain>/fullchain.pem + privkey.pem.
// Multiple servers (HTTPS, SMTP, POP3, IMAP) share one store.
type CertStore struct {
	mu       sync.RWMutex
	certs    map[string]*tls.Certificate // domain → cert
	dir      string                      // /opt/bdsmail/ssl
	fallback *tls.Certificate            // first loaded cert, used as default
}

// NewCertStore creates a CertStore and loads all certs from subdirectories of dir.
func NewCertStore(dir string) (*CertStore, error) {
	cs := &CertStore{
		certs: make(map[string]*tls.Certificate),
		dir:   dir,
	}
	if dir == "" {
		return cs, nil
	}
	if err := cs.LoadAll(); err != nil {
		return nil, err
	}
	return cs, nil
}

// LoadAll scans the SSL directory and loads every domain's cert.
func (cs *CertStore) LoadAll() error {
	entries, err := os.ReadDir(cs.dir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("SSL directory %s does not exist, no certs loaded", cs.dir)
			return nil
		}
		return fmt.Errorf("read SSL directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		domain := entry.Name()
		if err := cs.LoadDomain(domain); err != nil {
			log.Printf("warning: failed to load cert for %s: %v", domain, err)
		}
	}
	log.Printf("Loaded TLS certificates for %d domain(s)", len(cs.certs))
	return nil
}

// LoadDomain loads or reloads the certificate for a single domain.
func (cs *CertStore) LoadDomain(domain string) error {
	certPath := filepath.Join(cs.dir, domain, "fullchain.pem")
	keyPath := filepath.Join(cs.dir, domain, "privkey.pem")

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return fmt.Errorf("load cert for %s: %w", domain, err)
	}

	cs.mu.Lock()
	cs.certs[domain] = &cert
	if cs.fallback == nil {
		cs.fallback = &cert
	}
	cs.mu.Unlock()

	log.Printf("TLS certificate loaded for domain: %s", domain)
	return nil
}

// GetCertificate returns the certificate matching the SNI hostname.
// Strips "mail." and "webmail." prefixes to find the domain cert.
// Falls back to the first loaded cert if no match.
func (cs *CertStore) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	serverName := strings.ToLower(hello.ServerName)

	// Try exact match first
	if cert, ok := cs.certs[serverName]; ok {
		return cert, nil
	}

	// Strip known prefixes: mail.domain.com → domain.com
	for _, prefix := range []string{"mail.", "webmail.", "mailsrv."} {
		if strings.HasPrefix(serverName, prefix) {
			domain := serverName[len(prefix):]
			if cert, ok := cs.certs[domain]; ok {
				return cert, nil
			}
		}
	}

	// Fallback to default cert
	if cs.fallback != nil {
		return cs.fallback, nil
	}

	return nil, fmt.Errorf("no TLS certificate for %s", serverName)
}

// TLSConfig returns a tls.Config with SNI-based certificate selection and TLS 1.2 minimum.
func (cs *CertStore) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: cs.GetCertificate,
		MinVersion:     tls.VersionTLS12,
	}
}

// HasCerts returns true if any certificates are loaded.
func (cs *CertStore) HasCerts() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.certs) > 0
}
