package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
)

type Config struct {
	mu           sync.RWMutex
	Domains      []string
	SMTPPort     string
	POP3Port     string
	IMAPPort     string
	HTTPSPort    string
	HTTPPort     string // plain HTTP for ACME challenges, default 80
	TLSCert      string
	TLSKey       string
	GCSBucket    string
	DatabaseURL  string
	DKIMKeyDir   string
	DKIMSelector string
	AdminSecret  string
	AcmeWebroot  string
	EnvFile      string
}

// HostToDomain maps a Host header like "mail.domain1.com" to "domain1.com".
func (c *Config) HostToDomain(host string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	for _, d := range c.Domains {
		if host == "mail."+d || host == d {
			return d
		}
	}
	if strings.HasPrefix(host, "mail.") {
		candidate := host[5:]
		for _, d := range c.Domains {
			if candidate == d {
				return d
			}
		}
	}
	if len(c.Domains) > 0 {
		return c.Domains[0]
	}
	return host
}

// IsDomainServed checks if a domain is in the configured list.
func (c *Config) IsDomainServed(domain string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, d := range c.Domains {
		if d == domain {
			return true
		}
	}
	return false
}

// GetDomains returns a copy of the current domain list.
func (c *Config) GetDomains() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cp := make([]string, len(c.Domains))
	copy(cp, c.Domains)
	return cp
}

// AddDomain adds a domain to the list if not already present.
func (c *Config) AddDomain(domain string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, d := range c.Domains {
		if d == domain {
			return
		}
	}
	c.Domains = append(c.Domains, domain)
}

// PersistDomains updates the BDS_DOMAINS line in the env file.
func (c *Config) PersistDomains() error {
	if c.EnvFile == "" {
		return nil
	}
	c.mu.RLock()
	newValue := strings.Join(c.Domains, ",")
	c.mu.RUnlock()

	data, err := os.ReadFile(c.EnvFile)
	if err != nil {
		return fmt.Errorf("cannot read env file: %w", err)
	}

	var lines []string
	found := false
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "BDS_DOMAINS=") {
			lines = append(lines, "BDS_DOMAINS="+newValue)
			found = true
		} else {
			lines = append(lines, line)
		}
	}
	if !found {
		lines = append(lines, "BDS_DOMAINS="+newValue)
	}

	return os.WriteFile(c.EnvFile, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

func Load() *Config {
	domainsStr := getEnv("BDS_DOMAINS", "mydomain.com")
	var domains []string
	for _, d := range strings.Split(domainsStr, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			domains = append(domains, d)
		}
	}

	return &Config{
		Domains:      domains,
		SMTPPort:     getEnv("BDS_SMTP_PORT", "2525"),
		POP3Port:     getEnv("BDS_POP3_PORT", "1100"),
		IMAPPort:     getEnv("BDS_IMAP_PORT", "1430"),
		HTTPSPort:    getEnv("BDS_HTTPS_PORT", "8443"),
		HTTPPort:     getEnv("BDS_HTTP_PORT", "8080"),
		TLSCert:      getEnv("BDS_TLS_CERT", ""),
		TLSKey:       getEnv("BDS_TLS_KEY", ""),
		GCSBucket:    getEnv("BDS_GCS_BUCKET", "bdsmail-bodies"),
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://localhost:5432/bdsmail?sslmode=disable"),
		DKIMKeyDir:   getEnv("BDS_DKIM_KEY_DIR", ""),
		DKIMSelector: getEnv("BDS_DKIM_SELECTOR", "default"),
		AdminSecret:  getEnv("BDS_ADMIN_SECRET", ""),
		AcmeWebroot:  getEnv("BDS_ACME_WEBROOT", "/opt/bdsmail/acme"),
		EnvFile:      getEnv("BDS_ENV_FILE", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
