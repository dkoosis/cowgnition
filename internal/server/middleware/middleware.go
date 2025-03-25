// internal/server/middleware/middleware.go
package middleware

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/cowgnition/cowgnition/internal/server/httputils"
)

// LogMiddleware adds request logging to the server.
func LogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response interceptor to capture status code
		wi := &ResponseInterceptor{ResponseWriter: w, statusCode: http.StatusOK}

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

// RecoveryMiddleware adds panic recovery to prevent server crashes.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Log the panic with stack trace
				stack := debug.Stack()
				log.Printf("PANIC: %v\n%s", err, stack)

				// Return a 500 error
				httputils.WriteStandardErrorResponse(w, httputils.InternalError,
					"Internal server error: recovered from panic",
					map[string]interface{}{
						"panic_value": fmt.Sprintf("%v", err),
					})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CorsMiddleware adds CORS headers for development scenarios.
func CorsMiddleware(next http.Handler) http.Handler {
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

// ResponseInterceptor wraps an http.ResponseWriter to capture the status code.
type ResponseInterceptor struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code before passing it to the wrapped ResponseWriter.
func (wi *ResponseInterceptor) WriteHeader(statusCode int) {
	wi.statusCode = statusCode
	wi.ResponseWriter.WriteHeader(statusCode)
}
