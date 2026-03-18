package tlsutil

import (
	"crypto/tls"
	"fmt"
	"log"
	"sync"
)

// CertReloader provides dynamic TLS certificate reloading from disk.
// Multiple servers (HTTPS, SMTP, POP3, IMAP) share one reloader.
type CertReloader struct {
	certPath string
	keyPath  string
	mu       sync.RWMutex
	cert     *tls.Certificate
}

func NewCertReloader(certPath, keyPath string) (*CertReloader, error) {
	cr := &CertReloader{certPath: certPath, keyPath: keyPath}
	if err := cr.Reload(); err != nil {
		return nil, err
	}
	return cr, nil
}

// Reload reads the certificate files from disk.
func (cr *CertReloader) Reload() error {
	cert, err := tls.LoadX509KeyPair(cr.certPath, cr.keyPath)
	if err != nil {
		return fmt.Errorf("failed to load TLS cert: %w", err)
	}
	cr.mu.Lock()
	cr.cert = &cert
	cr.mu.Unlock()
	log.Println("TLS certificate reloaded from disk")
	return nil
}

// GetCertificate returns the cached certificate. Use as tls.Config.GetCertificate.
func (cr *CertReloader) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.cert, nil
}

// TLSConfig returns a tls.Config that uses dynamic cert reloading.
func (cr *CertReloader) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: cr.GetCertificate,
	}
}
