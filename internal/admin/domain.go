package admin

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mustafakarli/bdsmail/config"
	smtpserver "github.com/mustafakarli/bdsmail/internal/smtp"
	"github.com/mustafakarli/bdsmail/internal/tlsutil"
)

// DNSRecord represents a DNS record to be added by the user.
type DNSRecord struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	Priority string `json:"priority,omitempty"`
}

// DomainResult is returned after a successful domain registration.
type DomainResult struct {
	Domain     string      `json:"domain"`
	DNSRecords []DNSRecord `json:"dns_records"`
	Message    string      `json:"message"`
}

// RegisterDomain adds a new domain to the running server:
// 1. Validates the domain
// 2. Generates DKIM key
// 3. Adds domain to config
// 4. Expands TLS certificate (if certbot is available)
// 5. Persists config to .env
// 6. Returns DNS records to add
func RegisterDomain(cfg *config.Config, relay *smtpserver.Relay, certReloader *tlsutil.CertReloader, domain string) (*DomainResult, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return nil, fmt.Errorf("domain cannot be empty")
	}
	if !strings.Contains(domain, ".") {
		return nil, fmt.Errorf("invalid domain: %s", domain)
	}
	if cfg.IsDomainServed(domain) {
		return nil, fmt.Errorf("domain %s is already configured", domain)
	}

	// Generate DKIM key
	dkimPubKey, err := generateDKIMKey(cfg.DKIMKeyDir, domain)
	if err != nil {
		return nil, fmt.Errorf("DKIM key generation failed: %w", err)
	}

	// Load the generated key and add to relay
	keyPath := filepath.Join(cfg.DKIMKeyDir, domain+".pem")
	keyData, err := os.ReadFile(keyPath)
	if err == nil {
		block, _ := pem.Decode(keyData)
		if block != nil {
			if privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
				relay.AddDKIMKey(domain, privKey)
				log.Printf("DKIM key loaded for new domain: %s", domain)
			}
		}
	}

	// Add domain to config
	cfg.AddDomain(domain)

	// Expand TLS certificate
	expandTLSCert(cfg, certReloader)

	// Persist to .env
	if err := cfg.PersistDomains(); err != nil {
		log.Printf("warning: failed to persist domains to .env: %v", err)
	}

	// Build DNS records
	records := buildDNSRecords(domain, cfg.DKIMSelector, dkimPubKey)

	return &DomainResult{
		Domain:     domain,
		DNSRecords: records,
		Message:    fmt.Sprintf("Domain %s registered successfully. Add the DNS records below to your DNS provider.", domain),
	}, nil
}

func generateDKIMKey(keyDir, domain string) (string, error) {
	if keyDir == "" {
		return "", fmt.Errorf("DKIM key directory not configured (set BDS_DKIM_KEY_DIR)")
	}

	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return "", fmt.Errorf("cannot create DKIM key directory: %w", err)
	}

	keyPath := filepath.Join(keyDir, domain+".pem")

	// Don't overwrite existing keys
	if _, err := os.Stat(keyPath); err == nil {
		// Key exists, read public key from it
		return extractPublicKey(keyPath)
	}

	// Generate 2048-bit RSA key
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("key generation failed: %w", err)
	}

	// Write private key in PKCS1 PEM format
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	})
	if err := os.WriteFile(keyPath, privPEM, 0600); err != nil {
		return "", fmt.Errorf("cannot write private key: %w", err)
	}

	// Extract public key in DER format, base64-encoded
	pubDER, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		return "", fmt.Errorf("cannot marshal public key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(pubDER), nil
}

func extractPublicKey(keyPath string) (string, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(keyData)
	if block == nil {
		return "", fmt.Errorf("no PEM block in %s", keyPath)
	}
	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(pubDER), nil
}

func expandTLSCert(cfg *config.Config, certReloader *tlsutil.CertReloader) {
	if cfg.TLSCert == "" || certReloader == nil {
		return
	}

	// Check if certbot is available
	if _, err := exec.LookPath("certbot"); err != nil {
		log.Println("certbot not found, skipping TLS certificate expansion")
		return
	}

	// Build certbot command with all domains
	domains := cfg.GetDomains()
	args := []string{"certonly", "--expand", "--webroot", "-w", cfg.AcmeWebroot, "--non-interactive", "--agree-tos"}
	for _, d := range domains {
		args = append(args, "-d", "mail."+d)
	}

	log.Printf("Expanding TLS certificate for domains: %v", domains)
	cmd := exec.Command("certbot", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("warning: certbot expand failed: %v", err)
		return
	}

	// Reload the certificate
	if err := certReloader.Reload(); err != nil {
		log.Printf("warning: TLS cert reload failed: %v", err)
	}
}

func buildDNSRecords(domain, selector, dkimPubKey string) []DNSRecord {
	records := []DNSRecord{
		{
			Type:  "A",
			Name:  "mail",
			Value: "YOUR_VM_STATIC_IP",
		},
		{
			Type:     "MX",
			Name:     "@",
			Value:    "mail." + domain,
			Priority: "10",
		},
		{
			Type:  "TXT",
			Name:  "@",
			Value: "v=spf1 a mx ~all",
		},
		{
			Type:  "TXT",
			Name:  selector + "._domainkey",
			Value: fmt.Sprintf("v=DKIM1; k=rsa; p=%s", dkimPubKey),
		},
		{
			Type:  "TXT",
			Name:  "_dmarc",
			Value: fmt.Sprintf("v=DMARC1; p=none; rua=mailto:postmaster@%s", domain),
		},
	}
	return records
}
