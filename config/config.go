package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
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
	AdminSecret    string
	AcmeWebroot    string
	EnvFile        string
	RelayHost      string
	RelayPort      string
	RelayUser      string
	RelayPassword  string
	DBType         string
	SQLitePath     string
	DynamoDBRegion     string
	FirestoreProject   string
	BucketType         string // "gcs", "s3", or "" (disabled)
	S3Region           string
	S3Bucket           string
	MaxAttachmentBytes int64
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

func Load() (*Config, EnvMap) {
	envFile := "/opt/bdsmail/.env"
	if v := os.Getenv("BDS_ENV_FILE"); v != "" {
		envFile = v
	}
	env := loadEnvFile(envFile)

	domainsStr := env.Get("BDS_DOMAINS", "mydomain.com")
	var domains []string
	for _, d := range strings.Split(domainsStr, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			domains = append(domains, d)
		}
	}

	return &Config{
		Domains:      domains,
		SMTPPort:     env.Get("BDS_SMTP_PORT", "2525"),
		POP3Port:     env.Get("BDS_POP3_PORT", "1100"),
		IMAPPort:     env.Get("BDS_IMAP_PORT", "1430"),
		HTTPSPort:    env.Get("BDS_HTTPS_PORT", "8443"),
		HTTPPort:     env.Get("BDS_HTTP_PORT", "8080"),
		TLSCert:      env.Get("BDS_TLS_CERT", ""),
		TLSKey:       env.Get("BDS_TLS_KEY", ""),
		GCSBucket:    env.Get("BDS_GCS_BUCKET", "bdsmail-bodies"),
		DatabaseURL:  env.Get("DATABASE_URL", "postgres://localhost:5432/bdsmail?sslmode=disable"),
		DKIMKeyDir:   env.Get("BDS_DKIM_KEY_DIR", ""),
		DKIMSelector: env.Get("BDS_DKIM_SELECTOR", "default"),
		AdminSecret:  env.Get("BDS_ADMIN_SECRET", ""),
		AcmeWebroot:  env.Get("BDS_ACME_WEBROOT", "/opt/bdsmail/acme"),
		EnvFile:       envFile,
		DBType:         env.Get("BDS_DB_TYPE", "postgres"),
		SQLitePath:     env.Get("BDS_SQLITE_PATH", "/opt/bdsmail/bdsmail.db"),
		DynamoDBRegion:   env.Get("BDS_DYNAMODB_REGION", "us-west-2"),
		FirestoreProject:   env.Get("BDS_FIRESTORE_PROJECT", ""),
		BucketType:         env.Get("BDS_BUCKET_TYPE", ""),
		S3Region:           env.Get("BDS_S3_REGION", "us-west-2"),
		S3Bucket:           env.Get("BDS_S3_BUCKET", ""),
		MaxAttachmentBytes: env.GetInt64("BDS_MAX_ATTACHMENT_BYTES", 10*1024*1024),
		RelayHost:      env.Get("BDS_RELAY_HOST", ""),
		RelayPort:     env.Get("BDS_RELAY_PORT", "587"),
		RelayUser:     env.Get("BDS_RELAY_USER", ""),
		RelayPassword: env.Get("BDS_RELAY_PASSWORD", ""),
	}, env
}

type EnvMap map[string]string

func (e EnvMap) Get(key, fallback string) string {
	if v, ok := e[key]; ok && v != "" {
		return v
	}
	return fallback
}

func (e EnvMap) GetBool(key string, fallback bool) bool {
	v, ok := e[key]
	if !ok || v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func (e EnvMap) GetDuration(key string, fallback time.Duration) time.Duration {
	v, ok := e[key]
	if !ok || v == "" {
		return fallback
	}
	secs, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return time.Duration(secs) * time.Second
}

func (e EnvMap) GetInt64(key string, fallback int64) int64 {
	v, ok := e[key]
	if !ok || v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

// loadEnvFile reads a .env file and returns a map of key=value pairs.
func loadEnvFile(path string) EnvMap {
	env := make(EnvMap)
	data, err := os.ReadFile(path)
	if err != nil {
		return env
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		env[key] = value
	}
	return env
}
