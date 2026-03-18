package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mustafakarli/bdsmail/config"
	"github.com/mustafakarli/bdsmail/internal/admin"
	"github.com/mustafakarli/bdsmail/internal/imap"
	"github.com/mustafakarli/bdsmail/internal/model"
	"github.com/mustafakarli/bdsmail/internal/pop3"
	smtpserver "github.com/mustafakarli/bdsmail/internal/smtp"
	"github.com/mustafakarli/bdsmail/internal/store"
	"github.com/mustafakarli/bdsmail/internal/tlsutil"
	"github.com/mustafakarli/bdsmail/internal/web"
)

func main() {
	addUser := flag.String("adduser", "", "Add a new user (user@domain, e.g. alice@domain1.com)")
	addPass := flag.String("password", "", "Password for the new user")
	displayName := flag.String("displayname", "", "Display name for the new user (e.g. 'Alice Smith')")
	addDomain := flag.String("adddomain", "", "Add a new domain to the running server (e.g. newdomain.com)")
	flag.Parse()

	cfg := config.Load()
	ctx := context.Background()

	// Handle adddomain command (talks to running server via admin API)
	if *addDomain != "" {
		handleAddDomain(cfg, *addDomain)
		return
	}

	// Initialize PostgreSQL
	db, err := store.NewDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Handle adduser command
	if *addUser != "" {
		if *addPass == "" {
			fmt.Fprintln(os.Stderr, "Usage: bdsmail -adduser user@domain -password <password> [-displayname 'Full Name']")
			os.Exit(1)
		}
		if !strings.Contains(*addUser, "@") {
			fmt.Fprintln(os.Stderr, "Error: username must be in user@domain format (e.g. alice@domain1.com)")
			os.Exit(1)
		}
		username, domain := store.SplitEmail(*addUser)
		hash, err := model.HashPassword(*addPass)
		if err != nil {
			log.Fatalf("Failed to hash password: %v", err)
		}
		if err := db.CreateUser(username, domain, *displayName, hash); err != nil {
			log.Fatalf("Failed to create user: %v", err)
		}
		name := *displayName
		if name == "" {
			name = username
		}
		fmt.Printf("User '%s' <%s@%s> created successfully\n", name, username, domain)
		return
	}

	// Initialize GCS bucket
	bucket, err := store.NewBucket(ctx, cfg.GCSBucket)
	if err != nil {
		log.Fatalf("Failed to initialize GCS bucket: %v", err)
	}
	defer bucket.Close()

	// Create store facade
	s := store.NewStore(db, bucket)

	// Load DKIM keys
	dkimKeys := loadDKIMKeys(cfg.DKIMKeyDir)

	// Create SMTP relay for outbound delivery
	relay := smtpserver.NewRelay(dkimKeys, cfg.DKIMSelector)

	// Initialize TLS cert reloader (if TLS is configured)
	var certReloader *tlsutil.CertReloader
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		certReloader, err = tlsutil.NewCertReloader(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			log.Printf("warning: TLS cert reloader failed, running without TLS: %v", err)
			certReloader = nil
		}
	}

	// Start HTTP server on port 80 for ACME challenges
	go startACMEServer(cfg)

	// Start SMTP server
	smtpSrv := smtpserver.NewServer(cfg, s, certReloader)
	go func() {
		if err := smtpSrv.Start(); err != nil {
			log.Printf("SMTP server error: %v", err)
		}
	}()

	// Start POP3 server
	pop3Srv := pop3.NewServer(cfg, s, certReloader)
	go func() {
		if err := pop3Srv.Start(); err != nil {
			log.Printf("POP3 server error: %v", err)
		}
	}()

	// Start IMAP server
	imapSrv := imap.NewServer(cfg, s, certReloader)
	go func() {
		if err := imapSrv.Start(); err != nil {
			log.Printf("IMAP server error: %v", err)
		}
	}()

	// Start web server
	webSrv, err := web.NewServer(cfg, s, relay, certReloader)
	if err != nil {
		log.Fatalf("Failed to initialize web server: %v", err)
	}

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		smtpSrv.Close()
		pop3Srv.Close()
		imapSrv.Close()
		os.Exit(0)
	}()

	// Start web server (blocking)
	log.Printf("BDS Mail server starting for domains: %s", strings.Join(cfg.GetDomains(), ", "))
	if err := webSrv.Start(); err != nil {
		log.Fatalf("Web server error: %v", err)
	}
}

// startACMEServer runs a plain HTTP server for Let's Encrypt ACME challenges.
func startACMEServer(cfg *config.Config) {
	if cfg.AcmeWebroot == "" {
		return
	}

	os.MkdirAll(filepath.Join(cfg.AcmeWebroot, ".well-known", "acme-challenge"), 0755)

	mux := http.NewServeMux()
	mux.Handle("/.well-known/", http.FileServer(http.Dir(cfg.AcmeWebroot)))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Redirect everything else to HTTPS
		target := "https://" + r.Host + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})

	addr := ":" + cfg.HTTPPort
	log.Printf("ACME HTTP server starting on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("ACME HTTP server error: %v", err)
	}
}

// handleAddDomain sends a request to the running server's admin API.
func handleAddDomain(cfg *config.Config, domain string) {
	if cfg.AdminSecret == "" {
		fmt.Fprintln(os.Stderr, "Error: BDS_ADMIN_SECRET must be set to use -adddomain")
		os.Exit(1)
	}

	url := fmt.Sprintf("https://localhost:%s/admin/api/domains", cfg.HTTPSPort)

	body, _ := json.Marshal(map[string]string{"domain": domain})
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AdminSecret)
	req.Header.Set("Content-Type", "application/json")

	// Allow self-signed certs for localhost
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: nil, // uses default
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		// Try without TLS verification for self-signed certs
		fmt.Fprintf(os.Stderr, "Error connecting to server: %v\n", err)
		fmt.Fprintln(os.Stderr, "Make sure the BDS Mail server is running.")
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		json.NewDecoder(resp.Body).Decode(&errResp)
		fmt.Fprintf(os.Stderr, "Error: %s\n", errResp["error"])
		os.Exit(1)
	}

	var result admin.DomainResult
	json.NewDecoder(resp.Body).Decode(&result)

	fmt.Printf("\n%s\n\n", result.Message)
	fmt.Println("DNS Records to add:")
	fmt.Println(strings.Repeat("-", 70))
	for _, rec := range result.DNSRecords {
		priority := ""
		if rec.Priority != "" {
			priority = " (Priority: " + rec.Priority + ")"
		}
		fmt.Printf("  %-4s  %-25s  %s%s\n", rec.Type, rec.Name, truncate(rec.Value, 60), priority)
	}
	fmt.Println(strings.Repeat("-", 70))
	fmt.Println("\nAdd these records to your DNS provider (e.g. GoDaddy).")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// loadDKIMKeys reads PEM private keys from the given directory.
func loadDKIMKeys(keyDir string) map[string]crypto.Signer {
	keys := make(map[string]crypto.Signer)
	if keyDir == "" {
		log.Println("DKIM key directory not configured, outbound mail will not be DKIM-signed")
		return keys
	}

	entries, err := os.ReadDir(keyDir)
	if err != nil {
		log.Printf("warning: cannot read DKIM key directory %s: %v", keyDir, err)
		return keys
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pem") {
			continue
		}

		domain := strings.TrimSuffix(entry.Name(), ".pem")
		keyPath := filepath.Join(keyDir, entry.Name())

		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			log.Printf("warning: cannot read DKIM key %s: %v", keyPath, err)
			continue
		}

		block, _ := pem.Decode(keyData)
		if block == nil {
			log.Printf("warning: no PEM block found in %s", keyPath)
			continue
		}

		privKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			privKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				log.Printf("warning: cannot parse DKIM key %s: %v", keyPath, err)
				continue
			}
		}

		signer, ok := privKey.(crypto.Signer)
		if !ok {
			log.Printf("warning: DKIM key %s is not a signing key", keyPath)
			continue
		}

		keys[domain] = signer
		log.Printf("Loaded DKIM key for domain: %s", domain)
	}

	return keys
}
