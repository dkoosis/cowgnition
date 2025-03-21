// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

// logMiddleware adds request logging to the server.
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response interceptor to capture status code
		wi := &responseInterceptor{ResponseWriter: w, statusCode: http.StatusOK}

		// Log the request
		log.Printf("Request: %s %s %s", r.Method, r.URL.Path, r.RemoteAddr)

		// Call the next handler
		next.ServeHTTP(wi, r)

		// Log the response
		duration := time.Since(start)
		log.Printf("Response: %s %s %d %s in %v",
			r.Method, r.URL.Path, wi.statusCode, http.StatusText(wi.statusCode), duration)
	})
}

// recoveryMiddleware adds panic recovery to prevent server crashes.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Log the panic with stack trace
				stack := debug.Stack()
				log.Printf("PANIC: %v\n%s", err, stack)

				// Return a 500 error
				writeErrorResponse(w, http.StatusInternalServerError,
					fmt.Sprintf("Internal server error: %v", err))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers for development scenarios.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add CORS headers for development
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// responseInterceptor wraps an http.ResponseWriter to capture the status code.
type responseInterceptor struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code before passing it to the wrapped ResponseWriter.
func (wi *responseInterceptor) WriteHeader(statusCode int) {
	wi.statusCode = statusCode
	wi.ResponseWriter.WriteHeader(statusCode)
}
