// Package rtm provides client functionality for the Remember The Milk API.
// file: internal/rtm/rate_limiter.go
package rtm

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter for API requests.
// It enforces a rate limit while allowing for controlled bursts of traffic.
type RateLimiter struct {
	rate       float64    // Rate limit in requests per second
	burstLimit int        // Maximum burst size allowed
	tokens     float64    // Current number of tokens in the bucket
	lastUpdate time.Time  // Time of last token update
	mu         sync.Mutex // Mutex to protect concurrent access
}

// NewRateLimiter creates a new rate limiter with the specified rate and burst limit.
// Rate is specified in requests per second. Burst limit is the maximum number of
// requests that can be made concurrently before being limited.
func NewRateLimiter(rate float64, burstLimit int) *RateLimiter {
	return &RateLimiter{
		rate:       rate,
		burstLimit: burstLimit,
		tokens:     float64(burstLimit), // Start with a full bucket
		lastUpdate: time.Now(),
	}
}

// Wait blocks until a token is available or the context is canceled.
// It returns an error if the context is canceled or if the rate limit
// is exceeded beyond the allowed threshold.
func (l *RateLimiter) Wait(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Calculate time since last update
	now := time.Now()
	elapsed := now.Sub(l.lastUpdate).Seconds()
	l.lastUpdate = now

	// Add tokens based on elapsed time, up to burst limit
	l.tokens += elapsed * l.rate
	if l.tokens > float64(l.burstLimit) {
		l.tokens = float64(l.burstLimit)
	}

	// Check if we have a token available
	if l.tokens >= 1 {
		// Consume a token
		l.tokens -= 1
		return nil
	}

	// Calculate wait time needed to get a token
	waitTime := time.Duration((1 - l.tokens) / l.rate * float64(time.Second))

	// If wait time is too long (more than 5 seconds), reject the request
	// This prevents excessive delays and resource consumption
	if waitTime > 5*time.Second {
		return fmt.Errorf("rate limit exceeded: try again later")
	}

	// Wait for a token to become available or context to be canceled
	select {
	case <-time.After(waitTime):
		// Token is now available, consume it
		l.tokens = 0
		return nil
	case <-ctx.Done():
		// Context was canceled while waiting
		return ctx.Err()
	}
}

// SetRateLimit updates the rate limit and burst limit.
// This allows for dynamic adjustment of rate limiting parameters.
func (l *RateLimiter) SetRateLimit(rate float64, burstLimit int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.rate = rate
	l.burstLimit = burstLimit

	// Cap current tokens to new burst limit
	if l.tokens > float64(burstLimit) {
		l.tokens = float64(burstLimit)
	}
}
