package security

import (
	"os"
	"strconv"
	"time"
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

func LoadConfig() *Config {
	return &Config{
		ClamAVEnabled:  getEnvBool("BDS_CLAMAV_ENABLED", false),
		ClamAVAddress:  getEnv("BDS_CLAMAV_ADDRESS", "unix:/var/run/clamav/clamd.ctl"),
		ClamAVTimeout:  getEnvDuration("BDS_CLAMAV_TIMEOUT", 5*time.Second),

		SafeBrowsingEnabled: getEnvBool("BDS_SAFEBROWSING_ENABLED", false),
		SafeBrowsingAPIKey:  getEnv("BDS_SAFEBROWSING_API_KEY", ""),
		SafeBrowsingTimeout: getEnvDuration("BDS_SAFEBROWSING_TIMEOUT", 5*time.Second),

		AuthCheckEnabled: getEnvBool("BDS_AUTH_CHECK_ENABLED", false),
		AuthCheckTimeout: getEnvDuration("BDS_AUTH_CHECK_TIMEOUT", 5*time.Second),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	secs, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return time.Duration(secs) * time.Second
}
