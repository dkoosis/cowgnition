// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
// file: internal/rtm/auth_manager_callback.go
package rtm

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
)

// startCallbackServer starts a local HTTP server to receive the auth callback.
func (m *AuthManager) startCallbackServer(_ context.Context, frob string) error { // Renamed ctx to _.
	m.callbackMutex.Lock()
	defer m.callbackMutex.Unlock()

	// Don't start if already running.
	if m.callbackServer != nil {
		return errors.New("callback server already running")
	}

	m.resultChan = make(chan error, 1)

	// Create server with security precautions.
	mux := http.NewServeMux()

	// Add handlers for multiple potential callback paths.
	// RTM might redirect to any of these.
	mux.HandleFunc("/auth/callback", m.createCallbackHandler(frob))
	mux.HandleFunc("/", m.createCallbackHandler(frob))         // Root path.
	mux.HandleFunc("/callback", m.createCallbackHandler(frob)) // Simple callback path.

	// Create server with timeout settings.
	addr := fmt.Sprintf("%s:%d", m.options.CallbackHost, m.options.CallbackPort)
	m.callbackServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	// Start server in separate goroutine.
	go func() {
		m.logger.Info("Starting callback server.", "address", addr)
		if err := m.callbackServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Error("Callback server error.", "error", err)

			m.callbackMutex.Lock()
			if m.resultChan != nil {
				m.resultChan <- err
			}
			m.callbackMutex.Unlock()
		}
	}()

	return nil
}

// createCallbackHandler creates a handler function for auth callbacks.
func (m *AuthManager) createCallbackHandler(frob string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.logger.Info("Received callback request.",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"remote", r.RemoteAddr)

		defer func() {
			if err := recover(); err != nil {
				m.logger.Error("Panic in callback handler.", "error", err)
				m.callbackMutex.Lock()
				if m.resultChan != nil {
					m.resultChan <- errors.Errorf("panic in callback: %v", err)
				}
				m.callbackMutex.Unlock()

				// Return error to browser.
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()

		// Verify request.
		if r.Method != http.MethodGet {
			m.logger.Warn("Invalid method in callback.", "method", r.Method)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check request parameters.
		// RTM might include frob in the callback URL.
		callbackFrob := r.URL.Query().Get("frob")

		// Prioritize frob from URL if available, otherwise use the one we started with.
		frobToUse := frob
		if callbackFrob != "" {
			m.logger.Info("Using frob from callback URL.", "frob", callbackFrob)
			frobToUse = callbackFrob
		}

		if frobToUse == "" {
			m.logger.Error("No frob available for auth completion.")
			http.Error(w, "Missing frob parameter", http.StatusBadRequest)

			m.callbackMutex.Lock()
			if m.resultChan != nil {
				m.resultChan <- errors.New("no frob available for auth completion")
			}
			m.callbackMutex.Unlock()
			return
		}

		// Add a small delay to ensure RTM servers have processed the auth.
		time.Sleep(1 * time.Second)

		// Create a context for the auth operations initiated by this callback.
		// Use a background context as the original request context might expire.
		callbackCtx := context.Background() // Use background context.

		// Complete authentication with the frob.
		m.logger.Info("Completing auth from callback.", "frob", frobToUse)
		err := m.service.CompleteAuth(callbackCtx, frobToUse) // Pass callbackCtx.
		if err != nil {
			m.logger.Error("Failed to complete auth in callback.", "error", err)

			// Show user-friendly error page.
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, "<html><body><h1>Authentication Failed</h1>"+
				"<p>Error: %s</p>"+
				"<p>Please try again or contact support.</p>"+
				"</body></html>",
				err.Error())

			m.callbackMutex.Lock()
			if m.resultChan != nil {
				m.resultChan <- err
			}
			m.callbackMutex.Unlock()
			return
		}

		// Get auth state to verify and get username.
		authState, stateErr := m.service.GetAuthState(callbackCtx) // Pass callbackCtx.
		if stateErr != nil {
			m.logger.Error("Failed to verify auth state after completion.", "error", stateErr)

			// Show user-friendly error page.
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, "<html><body><h1>Authentication Verification Failed</h1>"+
				"<p>Error: %s</p>"+
				"<p>Authentication may have been successful, but we couldn't verify it.</p>"+
				"</body></html>",
				stateErr.Error())

			m.callbackMutex.Lock()
			if m.resultChan != nil {
				m.resultChan <- stateErr
			}
			m.callbackMutex.Unlock()
			return
		}

		if authState == nil || !authState.IsAuthenticated {
			m.logger.Error("Auth completion succeeded but user not authenticated.")

			// Show user-friendly error page.
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, "<html><body><h1>Authentication Failed</h1>"+
				"<p>The authorization process completed, but you are not authenticated.</p>"+
				"<p>Please try again or contact support.</p>"+
				"</body></html>")

			m.callbackMutex.Lock()
			if m.resultChan != nil {
				m.resultChan <- errors.New("auth completion succeeded but user not authenticated")
			}
			m.callbackMutex.Unlock()
			return
		}

		// Success page.
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, "<html><body><h1>Authentication Successful!</h1>"+
			"<p>You are now authenticated as: %s</p>"+
			"<p>You can close this window and return to the application.</p>"+
			"</body></html>",
			authState.Username)

		// Signal success to main thread.
		m.callbackMutex.Lock()
		if m.resultChan != nil {
			m.resultChan <- nil
		}
		m.callbackMutex.Unlock()
	}
}

// stopCallbackServer gracefully shuts down the callback server.
func (m *AuthManager) stopCallbackServer() {
	m.callbackMutex.Lock()
	defer m.callbackMutex.Unlock()

	if m.callbackServer != nil {
		m.logger.Info("Stopping callback server.")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // Timeout for shutdown.
		defer cancel()
		if err := m.callbackServer.Shutdown(shutdownCtx); err != nil {
			m.logger.Error("Error shutting down callback server.", "error", err)
		}
		m.callbackServer = nil
	}

	// Close result channel if it exists.
	if m.resultChan != nil {
		close(m.resultChan)
		m.resultChan = nil
	}
}
