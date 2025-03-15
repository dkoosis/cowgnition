package server

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimiter implements a simple token bucket rate limiter
type RateLimiter struct {
	tokens      float64
	capacity    float64
	refillRate  float64
	lastRefill  time.Time
	ipLimiters  map[string]*RateLimiter
	mu          sync.Mutex
}

// NewRateLimiter creates a new rate limiter with the specified capacity and refill rate
func NewRateLimiter(capacity, refillRate float64) *RateLimiter {
	return &RateLimiter{
		tokens:     capacity,
		capacity:   capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
		ipLimiters: make(map[string]*RateLimiter),
	}
}

// Allow checks if a request can proceed based on the rate limit
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill tokens based on time elapsed
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.tokens = min(rl.capacity, rl.tokens+(elapsed*rl.refillRate))
	rl.lastRefill = now

	// Check if token is available
	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}
	return false
}

// GetIPLimiter gets or creates a per-IP rate limiter
func (rl *RateLimiter) GetIPLimiter(ip string) *RateLimiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.ipLimiters[ip]
	if !exists {
		limiter = NewRateLimiter(rl.capacity, rl.refillRate)
		rl.ipLimiters[ip] = limiter
	}
	return limiter
}

// Basic security middleware for the server
// Implements:
// - Rate limiting
// - Basic security headers
// - Request logging
// - Panic recovery
func securityMiddleware(next http.Handler) http.Handler {
	// Create a global rate limiter (10 requests/second per IP)
	limiter := NewRateLimiter(10, 10)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Header().Set("Referrer-Policy", "no-referrer")

		// Get client IP
		ip := r.RemoteAddr
		
		// Rate limit by IP
		ipLimiter := limiter.GetIPLimiter(ip)
		if !ipLimiter.Allow() {
			w.Header().Set("Retry-After", "1") // Try again in 1 second
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			log.Printf("Rate limit exceeded for IP: %s", ip)
			return
		}

		// Recover from panics
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()

		// Set a reasonable timeout for the request context
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

// Enhanced logging middleware with request details
func enhancedLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Create a wrapper for the response writer to capture status code
		rw := newResponseWriter(w)
		
		// Process the request
		next.ServeHTTP(rw, r)
		
		duration := time.Since(start)
		
		// Log request details
		log.Printf(
			"Request: %s %s | Status: %d | Duration: %v | IP: %s | User-Agent: %s",
			r.Method, r.URL.Path, rw.statusCode, duration, r.RemoteAddr, r.UserAgent(),
		)
	})
}

// responseWriter is a wrapper for http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// newResponseWriter creates a new responseWriter
func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

// WriteHeader captures the status code and delegates to the wrapped ResponseWriter
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Update the MCPServer Start method to use the security middleware
func (s *MCPServer) startWithSecurity() error {
	// Create router
	mux := http.NewServeMux()

	// Register all handlers
	s.registerHandlers(mux)

	// Add middleware chain (order matters)
	handler := enhancedLogMiddleware(securityMiddleware(mux))

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Server.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server
	log.Printf("Starting secure MCP server on port %d", s.config.Server.Port)
	if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server error: %w", err)
	}

	return nil
}

// Helper function for min of two floats
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
