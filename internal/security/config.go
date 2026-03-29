package security

import (
	"time"

	"github.com/mustafakarli/bdsmail/config"
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

func LoadConfig(env config.EnvMap) *Config {
	return &Config{
		ClamAVEnabled:  env.GetBool("BDS_CLAMAV_ENABLED", false),
		ClamAVAddress:  env.Get("BDS_CLAMAV_ADDRESS", "unix:/var/run/clamav/clamd.ctl"),
		ClamAVTimeout:  env.GetDuration("BDS_CLAMAV_TIMEOUT", 5*time.Second),

		SafeBrowsingEnabled: env.GetBool("BDS_SAFEBROWSING_ENABLED", false),
		SafeBrowsingAPIKey:  env.Get("BDS_SAFEBROWSING_API_KEY", ""),
		SafeBrowsingTimeout: env.GetDuration("BDS_SAFEBROWSING_TIMEOUT", 5*time.Second),

		AuthCheckEnabled: env.GetBool("BDS_AUTH_CHECK_ENABLED", false),
		AuthCheckTimeout: env.GetDuration("BDS_AUTH_CHECK_TIMEOUT", 5*time.Second),

		RateLimitEnabled:     env.GetBool("BDS_RATELIMIT_ENABLED", true),
		RateLimitConnPerSec:  env.GetFloat("BDS_RATELIMIT_CONN_PER_SEC", 10.0),
		RateLimitConnBurst:   env.GetInt("BDS_RATELIMIT_CONN_BURST", 20),
		RateLimitMaxAuthFail: env.GetInt("BDS_RATELIMIT_MAX_AUTH_FAIL", 5),
		RateLimitLockoutDur:  env.GetDuration("BDS_RATELIMIT_LOCKOUT_SEC", 15*time.Minute),

		RspamdEnabled:     env.GetBool("BDS_RSPAMD_ENABLED", true),
		RspamdURL:         env.Get("BDS_RSPAMD_URL", "http://localhost:11333"),
		RspamdTimeout:     env.GetDuration("BDS_RSPAMD_TIMEOUT", 10*time.Second),
		RspamdRejectScore: env.GetFloat("BDS_RSPAMD_REJECT_SCORE", 15.0),
		RspamdJunkScore:   env.GetFloat("BDS_RSPAMD_JUNK_SCORE", 6.0),

		MTASTSEnabled: env.GetBool("BDS_MTASTS_ENABLED", true),
		MTASTSTimeout: env.GetDuration("BDS_MTASTS_TIMEOUT", 10*time.Second),

		DANEEnabled:  env.GetBool("BDS_DANE_ENABLED", true),
		DANETimeout:  env.GetDuration("BDS_DANE_TIMEOUT", 5*time.Second),
		DANEResolver: env.Get("BDS_DANE_RESOLVER", "1.1.1.1:53"),

		TLSRPTEnabled:      env.GetBool("BDS_TLSRPT_ENABLED", true),
		TLSRPTInterval:     env.GetDuration("BDS_TLSRPT_INTERVAL", 86400*time.Second),
		TLSRPTSenderDomain: env.Get("BDS_TLSRPT_SENDER_DOMAIN", env.Get("BDS_DOMAINS", "localhost")),
	}
}
