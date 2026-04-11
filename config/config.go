package config

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sync"

	"github.com/mustafa-karli/basis/common"
	"github.com/mustafa-karli/basis/port"
	"github.com/mustafa-karli/basis/service/secret"
)

// CLI flags specific to bdsmail (basis common flags are used for shared settings)
var (
	FlagSMTPPort    = flag.Int("inbound_smtp_port", 25, "Inbound SMTP server port")
	FlagPOP3Port    = flag.Int("pop3_port", 110, "POP3 server port")
	FlagIMAPPort    = flag.Int("imap_port", 143, "IMAP server port")
	FlagHTTPPort    = flag.Int("http_port", 80, "HTTP port for ACME challenges")
	FlagDBType      = flag.String("db_type", "postgres", "Database backend: postgres or dynamodb")
	FlagDynamoDBRegion = flag.String("dynamodb_region", "us-east-1", "AWS region for DynamoDB")
	FlagS3Region    = flag.String("s3_region", "us-east-1", "S3 region (use 'auto' for R2)")
	FlagS3Bucket    = flag.String("s3_bucket", "", "S3/R2 bucket name")
	FlagS3Endpoint  = flag.String("s3_endpoint", "", "Custom S3 endpoint (for R2: https://<account-id>.r2.cloudflarestorage.com)")
	FlagDKIMKeyDir  = flag.String("dkim_key_dir", "/opt/bdsmail/dkim", "DKIM private keys directory")
	FlagDKIMSelector = flag.String("dkim_selector", "default", "DKIM selector name")
	FlagSSLDir      = flag.String("ssl_dir", "/opt/bdsmail/ssl", "Per-domain SSL certificate directory")
	FlagAcmeWebroot = flag.String("acme_webroot", "/opt/bdsmail/acme", "ACME challenge webroot")
	FlagAmplifyURL    = flag.String("amplify_url", "", "Amplify app URL for webmail CNAME")
	FlagMailHostname  = flag.String("mail_hostname", "", "Shared mail hostname for customer CNAME/MX (e.g. mailsrv.bdscont.com)")
	FlagMaxAttachmentBytes = flag.Int64("max_attachment_bytes", 10*1024*1024, "Maximum attachment size in bytes")
)

type Config struct {
	mu           sync.RWMutex
	Domains      []string // loaded from DB at startup
	SMTPPort     int
	POP3Port     int
	IMAPPort     int
	HTTPSPort    int
	HTTPPort     int
	SSLDir       string
	DatabaseURL  string // loaded from secrets
	DKIMKeyDir   string
	DKIMSelector string
	AdminSecret  string // loaded from secrets
	AcmeWebroot  string
	RelayHost    string // loaded from secrets
	RelayPort    int
	RelayUser    string // loaded from secrets
	RelayPassword string // loaded from secrets
	DBType       string
	DynamoDBRegion     string
	S3Region           string
	S3Bucket           string
	S3Endpoint         string
	MaxAttachmentBytes int64
	AmplifyURL         string
	MailHostname       string
	Secrets            port.SecretProvider
}

// Load creates Config from CLI flags and loads secrets from the configured provider.
func Load() *Config {
	if !flag.Parsed() {
		flag.Parse()
	}

	ctx := context.Background()

	// Initialize secret provider (local JSON, AWS Secrets Manager, or GCP Secret Manager)
	sp, err := secret.NewSecretProvider(ctx)
	if err != nil {
		log.Printf("warning: secret provider init failed: %v (secrets will be unavailable)", err)
	}

	cfg := &Config{
		SMTPPort:     *FlagSMTPPort,
		POP3Port:     *FlagPOP3Port,
		IMAPPort:     *FlagIMAPPort,
		HTTPSPort:    *common.HTTPSPort,
		HTTPPort:     *FlagHTTPPort,
		SSLDir:       *FlagSSLDir,
		DKIMKeyDir:   *FlagDKIMKeyDir,
		DKIMSelector: *FlagDKIMSelector,
		AcmeWebroot:  *FlagAcmeWebroot,
		DBType:       *FlagDBType,
		DynamoDBRegion:     *FlagDynamoDBRegion,
		S3Region:           *FlagS3Region,
		S3Bucket:           *FlagS3Bucket,
		S3Endpoint:         *FlagS3Endpoint,
		MaxAttachmentBytes: *FlagMaxAttachmentBytes,
		AmplifyURL:         *FlagAmplifyURL,
		MailHostname:       *FlagMailHostname,
		Secrets:            sp,
	}

	// Load secrets
	if sp != nil {
		cfg.DatabaseURL = getSecret(ctx, sp, "database_url")
		cfg.AdminSecret = getSecret(ctx, sp, "admin_secret")
		cfg.RelayHost = getSecret(ctx, sp, "relay_host")
		cfg.RelayUser = getSecret(ctx, sp, "relay_user")
		cfg.RelayPassword = getSecret(ctx, sp, "relay_password")
	}

	// Relay port from flag (not a secret)
	cfg.RelayPort = 587

	return cfg
}

func getSecret(ctx context.Context, sp port.SecretProvider, key string) string {
	val, err := sp.GetSecret(ctx, key)
	if err != nil {
		return ""
	}
	return val
}

// HostToDomain maps a Host header like "mail.domain1.com" to "domain1.com".
func (c *Config) HostToDomain(host string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Strip port
	for i, ch := range host {
		if ch == ':' {
			host = host[:i]
			break
		}
	}

	for _, d := range c.Domains {
		if host == "mail."+d || host == "webmail."+d || host == d {
			return d
		}
	}
	for _, prefix := range []string{"mail.", "webmail.", "mailsrv."} {
		if len(host) > len(prefix) && host[:len(prefix)] == prefix {
			candidate := host[len(prefix):]
			for _, d := range c.Domains {
				if candidate == d {
					return d
				}
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

// AddDomain adds a domain to the in-memory list.
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

// LoadDomainsFromDB populates the domain list from the database.
func (c *Config) LoadDomainsFromDB(domainNames []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Domains = domainNames
}

// GetHTTPSPortStr returns the HTTPS port as string.
func (c *Config) GetHTTPSPortStr() string {
	return itoa(c.HTTPSPort)
}

// GetHTTPPortStr returns the HTTP port as string.
func (c *Config) GetHTTPPortStr() string {
	return itoa(c.HTTPPort)
}

// GetSMTPPortStr returns the SMTP port as string.
func (c *Config) GetSMTPPortStr() string {
	return itoa(c.SMTPPort)
}

// GetPOP3PortStr returns the POP3 port as string.
func (c *Config) GetPOP3PortStr() string {
	return itoa(c.POP3Port)
}

// GetIMAPPortStr returns the IMAP port as string.
func (c *Config) GetIMAPPortStr() string {
	return itoa(c.IMAPPort)
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
