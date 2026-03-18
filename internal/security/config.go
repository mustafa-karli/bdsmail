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
}

func (c *Config) AnyEnabled() bool {
	return c.ClamAVEnabled || c.SafeBrowsingEnabled || c.AuthCheckEnabled
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
	}
}
