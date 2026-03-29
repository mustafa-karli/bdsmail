package security

import (
	"log"
	"net"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter provides per-IP connection rate limiting and brute-force protection.
type RateLimiter struct {
	mu          sync.Mutex
	connLimiter map[string]*rate.Limiter
	authFails   map[string]*authTracker

	connRate    rate.Limit
	connBurst   int
	maxAuthFail int
	lockoutDur  time.Duration
}

type authTracker struct {
	failures int
	lockedAt time.Time
}

func NewRateLimiter(cfg *Config) *RateLimiter {
	rl := &RateLimiter{
		connLimiter: make(map[string]*rate.Limiter),
		authFails:   make(map[string]*authTracker),
		connRate:    rate.Limit(cfg.RateLimitConnPerSec),
		connBurst:   cfg.RateLimitConnBurst,
		maxAuthFail: cfg.RateLimitMaxAuthFail,
		lockoutDur:  cfg.RateLimitLockoutDur,
	}
	go rl.cleanup()
	return rl
}

// AllowConnection checks if the IP is within its connection rate limit.
func (rl *RateLimiter) AllowConnection(ip net.IP) bool {
	if ip == nil {
		return true
	}
	key := ip.String()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, ok := rl.connLimiter[key]
	if !ok {
		limiter = rate.NewLimiter(rl.connRate, rl.connBurst)
		rl.connLimiter[key] = limiter
	}
	return limiter.Allow()
}

// IsLockedOut returns true if the IP has exceeded the max auth failure threshold.
func (rl *RateLimiter) IsLockedOut(ip net.IP) bool {
	if ip == nil {
		return false
	}
	key := ip.String()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	tracker, ok := rl.authFails[key]
	if !ok {
		return false
	}
	if tracker.failures >= rl.maxAuthFail {
		if time.Since(tracker.lockedAt) < rl.lockoutDur {
			return true
		}
		// Lockout expired — reset
		delete(rl.authFails, key)
		return false
	}
	return false
}

// RecordAuthFailure records a failed authentication attempt for the given IP.
func (rl *RateLimiter) RecordAuthFailure(ip net.IP) {
	if ip == nil {
		return
	}
	key := ip.String()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	tracker, ok := rl.authFails[key]
	if !ok {
		tracker = &authTracker{}
		rl.authFails[key] = tracker
	}
	tracker.failures++
	if tracker.failures >= rl.maxAuthFail {
		tracker.lockedAt = time.Now()
		log.Printf("security: IP %s locked out after %d failed auth attempts", key, tracker.failures)
	}
}

// RecordAuthSuccess clears the failure counter for the given IP.
func (rl *RateLimiter) RecordAuthSuccess(ip net.IP) {
	if ip == nil {
		return
	}
	key := ip.String()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.authFails, key)
}

// cleanup periodically evicts stale entries to prevent unbounded memory growth.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		// Evict auth trackers that have expired lockouts
		for key, tracker := range rl.authFails {
			if tracker.failures >= rl.maxAuthFail && time.Since(tracker.lockedAt) > 2*rl.lockoutDur {
				delete(rl.authFails, key)
			}
		}
		// Evict connection limiters not used recently (no tokens consumed = stale)
		// rate.Limiter doesn't expose last-used time, so we periodically reset the map.
		// This is acceptable: new connections will simply get a fresh limiter.
		if len(rl.connLimiter) > 10000 {
			rl.connLimiter = make(map[string]*rate.Limiter)
		}
		rl.mu.Unlock()
	}
}
