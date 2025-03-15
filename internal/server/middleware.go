// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"log"
	"net/http"
	"time"
)

// logMiddleware adds request logging to the server.
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("Request: %s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
		log.Printf("Response: %s %s completed in %v", r.Method, r.URL.Path, time.Since(start))
	})
}

// recoveryMiddleware adds panic recovery to prevent server crashes.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic recovered: %v", err)
				writeErrorResponse(w, http.StatusInternalServerError, "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
