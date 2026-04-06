package security

import (
	"flag"
	"time"
)

// CLI flags for security features
var (
	flagClamAVEnabled  = flag.Bool("clamav_enabled", false, "Enable ClamAV virus scanning")
	flagClamAVAddress  = flag.String("clamav_address", "unix:/var/run/clamav/clamd.ctl", "ClamAV socket address")
	flagClamAVTimeout  = flag.Int("clamav_timeout", 5, "ClamAV timeout in seconds")

	flagSafeBrowsingEnabled = flag.Bool("safebrowsing_enabled", false, "Enable Google Safe Browsing")
	flagSafeBrowsingAPIKey  = flag.String("safebrowsing_api_key", "", "Safe Browsing API key")
	flagSafeBrowsingTimeout = flag.Int("safebrowsing_timeout", 5, "Safe Browsing timeout in seconds")

	flagAuthCheckEnabled = flag.Bool("auth_check_enabled", false, "Enable inbound SPF/DKIM/DMARC checks")
	flagAuthCheckTimeout = flag.Int("auth_check_timeout", 5, "Auth check timeout in seconds")

	flagRateLimitEnabled     = flag.Bool("ratelimit_enabled", true, "Enable rate limiting")
	flagRateLimitConnPerSec  = flag.Float64("ratelimit_conn_per_sec", 10.0, "Max connections per second per IP")
	flagRateLimitConnBurst   = flag.Int("ratelimit_conn_burst", 20, "Connection burst allowance")
	flagRateLimitMaxAuthFail = flag.Int("ratelimit_max_auth_fail", 5, "Auth failures before lockout")
	flagRateLimitLockoutSec  = flag.Int("ratelimit_lockout_sec", 900, "Lockout duration in seconds")

	flagRspamdEnabled     = flag.Bool("rspamd_enabled", true, "Enable Rspamd spam filtering")
	flagRspamdURL         = flag.String("rspamd_url", "http://localhost:11333", "Rspamd HTTP URL")
	flagRspamdTimeout     = flag.Int("rspamd_timeout", 10, "Rspamd timeout in seconds")
	flagRspamdRejectScore = flag.Float64("rspamd_reject_score", 15.0, "Spam reject threshold")
	flagRspamdJunkScore   = flag.Float64("rspamd_junk_score", 6.0, "Spam junk threshold")

	flagMTASTSEnabled = flag.Bool("mtasts_enabled", true, "Enable MTA-STS enforcement")
	flagMTASTSTimeout = flag.Int("mtasts_timeout", 10, "MTA-STS timeout in seconds")

	flagDANEEnabled  = flag.Bool("dane_enabled", true, "Enable DANE/TLSA verification")
	flagDANETimeout  = flag.Int("dane_timeout", 5, "DANE timeout in seconds")
	flagDANEResolver = flag.String("dane_resolver", "1.1.1.1:53", "DNSSEC resolver")

	flagTLSRPTEnabled  = flag.Bool("tlsrpt_enabled", true, "Enable TLS reporting")
	flagTLSRPTInterval = flag.Int("tlsrpt_interval", 86400, "TLSRPT interval in seconds")
)

type Config struct {
	ClamAVEnabled  bool
	ClamAVAddress  string
	ClamAVTimeout  time.Duration

	SafeBrowsingEnabled bool
	SafeBrowsingAPIKey  string
	SafeBrowsingTimeout time.Duration

	AuthCheckEnabled bool
	AuthCheckTimeout time.Duration

	RateLimitEnabled      bool
	RateLimitConnPerSec   float64
	RateLimitConnBurst    int
	RateLimitMaxAuthFail  int
	RateLimitLockoutDur   time.Duration

	RspamdEnabled       bool
	RspamdURL           string
	RspamdTimeout       time.Duration
	RspamdRejectScore   float64
	RspamdJunkScore     float64

	MTASTSEnabled bool
	MTASTSTimeout time.Duration

	DANEEnabled  bool
	DANETimeout  time.Duration
	DANEResolver string

	TLSRPTEnabled      bool
	TLSRPTInterval     time.Duration
	TLSRPTSenderDomain string
}

func (c *Config) AnyEnabled() bool {
	return c.ClamAVEnabled || c.SafeBrowsingEnabled || c.AuthCheckEnabled || c.RateLimitEnabled || c.RspamdEnabled || c.MTASTSEnabled || c.DANEEnabled || c.TLSRPTEnabled
}

func LoadConfig() *Config {
	return &Config{
		ClamAVEnabled:  *flagClamAVEnabled,
		ClamAVAddress:  *flagClamAVAddress,
		ClamAVTimeout:  time.Duration(*flagClamAVTimeout) * time.Second,

		SafeBrowsingEnabled: *flagSafeBrowsingEnabled,
		SafeBrowsingAPIKey:  *flagSafeBrowsingAPIKey,
		SafeBrowsingTimeout: time.Duration(*flagSafeBrowsingTimeout) * time.Second,

		AuthCheckEnabled: *flagAuthCheckEnabled,
		AuthCheckTimeout: time.Duration(*flagAuthCheckTimeout) * time.Second,

		RateLimitEnabled:     *flagRateLimitEnabled,
		RateLimitConnPerSec:  *flagRateLimitConnPerSec,
		RateLimitConnBurst:   *flagRateLimitConnBurst,
		RateLimitMaxAuthFail: *flagRateLimitMaxAuthFail,
		RateLimitLockoutDur:  time.Duration(*flagRateLimitLockoutSec) * time.Second,

		RspamdEnabled:     *flagRspamdEnabled,
		RspamdURL:         *flagRspamdURL,
		RspamdTimeout:     time.Duration(*flagRspamdTimeout) * time.Second,
		RspamdRejectScore: *flagRspamdRejectScore,
		RspamdJunkScore:   *flagRspamdJunkScore,

		MTASTSEnabled: *flagMTASTSEnabled,
		MTASTSTimeout: time.Duration(*flagMTASTSTimeout) * time.Second,

		DANEEnabled:  *flagDANEEnabled,
		DANETimeout:  time.Duration(*flagDANETimeout) * time.Second,
		DANEResolver: *flagDANEResolver,

		TLSRPTEnabled:  *flagTLSRPTEnabled,
		TLSRPTInterval: time.Duration(*flagTLSRPTInterval) * time.Second,
	}
}
