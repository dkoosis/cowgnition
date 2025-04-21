// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
)

// startCallbackServer starts a local HTTP server to receive the auth callback.
func (m *AuthManager) startCallbackServer(ctx context.Context, frob string) error {
	m.callbackMutex.Lock()
	defer m.callbackMutex.Unlock()

	// Don't start if already running
	if m.callbackServer != nil {
		return errors.New("callback server already running")
	}

	m.resultChan = make(chan error, 1)

	// Create server with security precautions
	mux := http.NewServeMux()

	// Handle the callback path
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		m.logger.Info("Received callback request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr)

		defer func() {
			if err := recover(); err != nil {
				m.logger.Error("Panic in callback handler", "error", err)
				m.callbackMutex.Lock()
				if m.resultChan != nil {
					m.resultChan <- errors.Errorf("panic in callback: %v", err)
				}
				m.callbackMutex.Unlock()

				// Return error to browser
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()

		// Verify request
		if r.Method != http.MethodGet {
			m.logger.Warn("Invalid method in callback", "method", r.Method)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check state parameter for CSRF protection
		receivedState := r.URL.Query().Get("state")
		m.logger.Debug("Checking callback state parameter",
			"received", receivedState,
			"expected", m.state)

		if receivedState != m.state {
			m.logger.Warn("Invalid state in callback",
				"received", receivedState,
				"expected", m.state)
			http.Error(w, "Invalid state parameter", http.StatusBadRequest)

			m.callbackMutex.Lock()
			if m.resultChan != nil {
				m.resultChan <- errors.New("CSRF protection failed: invalid state")
			}
			m.callbackMutex.Unlock()
			return
		}

		// Complete authentication with the frob
		m.logger.Info("Completing auth from callback", "frob", frob)
		err := m.service.CompleteAuth(ctx, frob)
		if err != nil {
			m.logger.Error("Failed to complete auth in callback", "error", err)

			// Show user-friendly error page
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			// FIX: Check error from fmt.Fprintf
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

		// Get auth state to verify and get username
		authState, stateErr := m.service.GetAuthState(ctx)
		if stateErr != nil {
			m.logger.Error("Failed to verify auth state after completion", "error", stateErr)

			// Show user-friendly error page
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			// FIX: Check error from fmt.Fprintf
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
			m.logger.Error("Auth completion succeeded but user not authenticated")

			// Show user-friendly error page
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			// FIX: Check error from fmt.Fprintf
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

		// Success page
		w.Header().Set("Content-Type", "text/html")
		// FIX: Check error from fmt.Fprintf
		_, _ = fmt.Fprintf(w, "<html><body><h1>Authentication Successful!</h1>"+
			"<p>You are now authenticated as: %s</p>"+
			"<p>You can close this window and return to the application.</p>"+
			"</body></html>",
			authState.Username)

		// Signal success to main thread
		m.callbackMutex.Lock()
		if m.resultChan != nil {
			m.resultChan <- nil
		}
		m.callbackMutex.Unlock()
	})

	// Create server with timeout settings
	addr := fmt.Sprintf("%s:%d", m.options.CallbackHost, m.options.CallbackPort)
	m.callbackServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	// Start server in separate goroutine
	go func() {
		m.logger.Info("Starting callback server", "address", addr)
		if err := m.callbackServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Error("Callback server error", "error", err)

			m.callbackMutex.Lock()
			if m.resultChan != nil {
				m.resultChan <- err
			}
			m.callbackMutex.Unlock()
		}
	}()

	return nil
}

// stopCallbackServer gracefully shuts down the callback server.
func (m *AuthManager) stopCallbackServer() {
	m.callbackMutex.Lock()
	defer m.callbackMutex.Unlock()

	if m.callbackServer == nil {
		return
	}

	m.logger.Info("Stopping callback server")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.callbackServer.Shutdown(ctx); err != nil {
		m.logger.Warn("Callback server shutdown error", "error", err)
	}

	m.callbackServer = nil
}
