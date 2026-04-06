package admin

import (
	"context"
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
	"github.com/mustafakarli/bdsmail/internal/awsutil"
	"github.com/mustafakarli/bdsmail/internal/cryptoutil"
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
	Domain           string      `json:"domain"`
	DNSRecords       []DNSRecord `json:"dns_records"`
	Message          string      `json:"message"`
	SESStatus        string      `json:"ses_status,omitempty"`        // SES verification status
	DomainAPIKey     string      `json:"domain_api_key,omitempty"`    // Per-domain API key (shown once)
	WebmailCNAME     string      `json:"webmail_cname,omitempty"`     // Amplify URL for webmail CNAME
}

// RegisterDomain adds a new domain to the running server:
// 1. Validates the domain
// 2. Generates DKIM key
// 3. Verifies domain in SES (if configured)
// 4. Generates per-domain API key
// 5. Adds domain to config
// 6. Expands TLS certificate (if certbot is available)
// 7. Persists config to .env
// 8. Returns DNS records, SES status, API key
func RegisterDomain(cfg *config.Config, relay *smtpserver.Relay, certStore *tlsutil.CertStore, domain string) (*DomainResult, error) {
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

	// Build DNS records
	records := buildDNSRecords(domain, cfg.DKIMSelector, dkimPubKey)

	// SES domain verification (if relay is SES)
	sesStatus := ""
	if cfg.RelayHost != "" && strings.Contains(cfg.RelayHost, "amazonaws.com") {
		region := extractSESRegion(cfg.RelayHost)
		sesClient, err := awsutil.NewSESClient(region)
		if err == nil {
			ctx := context.Background()
			verification, err := sesClient.VerifyDomain(ctx, domain)
			if err != nil {
				log.Printf("warning: SES domain verification failed: %v", err)
				sesStatus = "Failed"
			} else {
				sesStatus = "Pending"
				// Add SES DKIM CNAME records
				for i, token := range verification.DKIMTokens {
					records = append(records, DNSRecord{
						Type:  "CNAME",
						Name:  fmt.Sprintf("%s._domainkey", token),
						Value: fmt.Sprintf("%s.dkim.amazonses.com", token),
					})
					_ = i
				}
			}
		} else {
			log.Printf("warning: could not create SES client: %v", err)
		}
	}

	// Generate per-domain API key
	apiKey := generateDomainAPIKey()

	// Add webmail CNAME record if Amplify URL is configured
	webmailCNAME := ""
	if cfg.AmplifyURL != "" {
		webmailCNAME = cfg.AmplifyURL
		records = append(records, DNSRecord{
			Type:  "CNAME",
			Name:  "webmail",
			Value: cfg.AmplifyURL,
		})
	}

	// Add domain to config
	cfg.AddDomain(domain)

	// Issue per-domain TLS certificate
	issueDomainCert(cfg, certStore, domain)

	// Domain is persisted in the database (domain table), not .env

	return &DomainResult{
		Domain:       domain,
		DNSRecords:   records,
		Message:      fmt.Sprintf("Domain %s registered successfully. Add the DNS records below to your DNS provider.", domain),
		SESStatus:    sesStatus,
		DomainAPIKey: apiKey,
		WebmailCNAME: webmailCNAME,
	}, nil
}

// CheckDomainStatus checks SES verification and DKIM status for a domain.
func CheckDomainStatus(cfg *config.Config, domain string) (verifyStatus, dkimStatus string, err error) {
	if cfg.RelayHost == "" || !strings.Contains(cfg.RelayHost, "amazonaws.com") {
		return "N/A", "N/A", nil
	}
	region := extractSESRegion(cfg.RelayHost)
	sesClient, err := awsutil.NewSESClient(region)
	if err != nil {
		return "", "", err
	}
	ctx := context.Background()
	verifyStatus, _ = sesClient.CheckVerificationStatus(ctx, domain)
	dkimStatus, _ = sesClient.CheckDKIMStatus(ctx, domain)
	return verifyStatus, dkimStatus, nil
}

func generateDomainAPIKey() string {
	return cryptoutil.MustRandomHex(32)
}

func extractSESRegion(relayHost string) string {
	// email-smtp.us-east-1.amazonaws.com → us-east-1
	parts := strings.Split(relayHost, ".")
	if len(parts) >= 3 {
		return parts[1]
	}
	return "us-east-1"
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

// issueDomainCert issues a per-domain TLS certificate via certbot and loads it into the CertStore.
// Each domain gets its own cert at <SSLDir>/<domain>/fullchain.pem + privkey.pem.
// If certbot fails for this domain, other domains are unaffected.
func issueDomainCert(cfg *config.Config, certStore *tlsutil.CertStore, domain string) {
	if cfg.SSLDir == "" || certStore == nil {
		return
	}

	if _, err := exec.LookPath("certbot"); err != nil {
		log.Println("certbot not found, skipping TLS certificate issuance")
		return
	}

	certName := domain
	mailDomain := "mail." + domain

	log.Printf("Issuing TLS certificate for %s", mailDomain)
	cmd := exec.Command("certbot", "certonly",
		"--webroot", "-w", cfg.AcmeWebroot,
		"-d", mailDomain,
		"--cert-name", certName,
		"--non-interactive", "--agree-tos",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("warning: certbot failed for %s: %v", domain, err)
		return
	}

	// Copy cert files to SSL directory
	sslDir := filepath.Join(cfg.SSLDir, domain)
	if err := os.MkdirAll(sslDir, 0700); err != nil {
		log.Printf("warning: cannot create SSL dir %s: %v", sslDir, err)
		return
	}

	letsencryptDir := filepath.Join("/etc/letsencrypt/live", certName)
	for _, name := range []string{"fullchain.pem", "privkey.pem"} {
		src := filepath.Join(letsencryptDir, name)
		dst := filepath.Join(sslDir, name)
		data, err := os.ReadFile(src)
		if err != nil {
			log.Printf("warning: cannot read %s: %v", src, err)
			return
		}
		if err := os.WriteFile(dst, data, 0600); err != nil {
			log.Printf("warning: cannot write %s: %v", dst, err)
			return
		}
	}

	// Hot-load into CertStore — no restart needed, no other domains affected
	if err := certStore.LoadDomain(domain); err != nil {
		log.Printf("warning: failed to load cert for %s: %v", domain, err)
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
