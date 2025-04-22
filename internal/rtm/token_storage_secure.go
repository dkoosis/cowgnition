// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/token_storage_secure.go

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/zalando/go-keyring"
)

const (
	keyringService = "CowGnitionRTM"       // Service name for keyring.
	keyringUser    = "RTMAuthTokenDetails" // User/Account name for keyring entry. <--- Remains unexported
)

// SecureTokenStorage handles storing/retrieving tokens using the OS keychain.
type SecureTokenStorage struct {
	logger logging.Logger
}

// Ensure SecureTokenStorage implements TokenStorageInterface.
var _ TokenStorageInterface = (*SecureTokenStorage)(nil)

// NewSecureTokenStorage creates a new secure token storage instance.
func NewSecureTokenStorage(logger logging.Logger) *SecureTokenStorage {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	return &SecureTokenStorage{
		logger: logger.WithField("component", "secure_token_storage"),
	}
}

// IsAvailable checks if the OS keyring service is accessible.
func (s *SecureTokenStorage) IsAvailable() bool {
	// Try a simple test operation instead of just checking for existing items
	testKey := fmt.Sprintf("%s_test", keyringService)
	testUser := fmt.Sprintf("%s_test", keyringUser)
	testValue := fmt.Sprintf("test_value_%d", time.Now().UnixNano())

	s.logger.Debug("Testing keyring availability with test write/read",
		"service", testKey, "account", testUser)

	// First try to set a test value
	err := keyring.Set(testKey, testUser, testValue)
	if err != nil {
		s.logger.Warn("Keyring test SET failed, analyzing error type",
			"error", fmt.Sprintf("%+v", err))

		// Check for specific macOS errors that don't necessarily mean the keyring is unavailable
		errMsg := err.Error()
		if strings.Contains(errMsg, "User interaction required") ||
			strings.Contains(errMsg, "permission") ||
			strings.Contains(errMsg, "access denied") {
			s.logger.Info("Keyring requires user interaction/permissions - will attempt to use it anyway.")
			return true
		}

		// Fall back to the old behavior as a last resort
		_, getErr := keyring.Get(keyringService, keyringUser)
		if errors.Is(getErr, keyring.ErrNotFound) {
			s.logger.Info("Keyring service available but no token exists.")
			return true
		}

		s.logger.Error("Keyring service unavailable",
			"error", fmt.Sprintf("%+v", err),
			"advice", s.GetKeychainAdvice()) // Use the new method here
		return false
	}

	// Clean up test entry
	_ = keyring.Delete(testKey, testUser)
	s.logger.Info("Keyring test successful, secure storage is available.")
	return true
}

// LoadToken loads the token from the secure OS keyring.
func (s *SecureTokenStorage) LoadToken() (string, error) {
	s.logger.Info("Attempting to load authentication token from system keyring.", "service", keyringService, "account", keyringUser)
	jsonData, err := keyring.Get(keyringService, keyringUser)

	// Enhanced error logging
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			s.logger.Debug("keyring.Get: No authentication token found in system keyring (ErrNotFound).")
			return "", nil
		}
		// Log other errors from keyring.Get
		s.logger.Error("keyring.Get operation failed.", "error", fmt.Sprintf("%+v", err))
		return "", errors.Wrap(err, "failed to load token from system keyring")
	}

	s.logger.Info("Successfully retrieved token data from system keyring.")

	var data TokenData
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		s.logger.Error("Token data in keyring is corrupted and cannot be parsed, attempting deletion.", "error", err)
		_ = s.DeleteToken()
		return "", errors.Wrap(err, "failed to parse token data from secure storage")
	}

	s.logger.Debug("Successfully decoded authentication token.", "username", data.Username)
	return data.Token, nil
}

// SaveToken saves token data securely to the OS keyring.
func (s *SecureTokenStorage) SaveToken(token string, userID, username string) error {
	s.logger.Debug("Preparing to save token to system keyring.",
		"username", username, "service", keyringService, "account", keyringUser)

	if token == "" {
		err := errors.New("cannot save empty token to keyring")
		s.logger.Error("SaveToken failed.", "error", err)
		return err
	}

	data := TokenData{
		Token:     token,
		UserID:    userID,
		Username:  username,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		s.logger.Error("Failed to encode token data for secure storage.", "error", err)
		return errors.Wrap(err, "failed to encode token data for secure storage")
	}

	jsonDataString := string(jsonData)
	s.logger.Debug("Calling keyring.Set.",
		"service", keyringService,
		"account", keyringUser,
		"dataSize", len(jsonDataString))

	// Try up to 3 times with increasing delays to handle potential permission prompts
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			s.logger.Info("Retrying keyring.Set.", "attempt", attempt+1)
			time.Sleep(time.Duration(500*attempt) * time.Millisecond)
		}

		err = keyring.Set(keyringService, keyringUser, jsonDataString)
		if err == nil {
			s.logger.Info("Authentication token successfully saved to system keyring.")
			return nil
		}

		lastErr = err
		s.logger.Warn("keyring.Set failed.",
			"attempt", attempt+1,
			"error", fmt.Sprintf("%+v", err))
	}

	// All attempts failed
	s.logger.Error("Failed to save token to system keyring after multiple attempts.",
		"error", fmt.Sprintf("%+v", lastErr),
		"advice", s.GetKeychainAdvice()) // Use the new method

	return errors.Wrap(lastErr, "failed to save token to system keyring")
}

// DeleteToken removes the token from the secure OS keyring.
func (s *SecureTokenStorage) DeleteToken() error {
	s.logger.Debug("Deleting authentication token from system keyring.")

	err := keyring.Delete(keyringService, keyringUser)

	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			s.logger.Debug("No token to delete - system keyring entry not found.")
			return nil
		}
		s.logger.Error("Failed to delete token from system keyring.", "error", err)
		return errors.Wrap(err, "failed to delete token from system keyring")
	}

	s.logger.Info("Authentication token successfully deleted from system keyring.")
	return nil
}

// GetTokenData loads the full token data from the secure storage.
func (s *SecureTokenStorage) GetTokenData() (*TokenData, error) {
	jsonData, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, nil
		}
		s.logger.Error("Failed to load token data from keyring.", "error", err)
		return nil, errors.Wrap(err, "failed to load token data from keyring")
	}

	var data TokenData
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		s.logger.Error("Failed to parse token data from secure storage, attempting deletion.", "error", err)
		_ = s.DeleteToken()
		return nil, errors.Wrap(err, "failed to parse token data from secure storage")
	}

	return &data, nil
}

// UpdateToken updates the stored token data. Re-uses SaveToken as keyring.Set often overwrites.
func (s *SecureTokenStorage) UpdateToken(token string, userID, username string) error {
	s.logger.Debug("Updating token in secure storage.", "username", username)
	return s.SaveToken(token, userID, username)
}

// --- Keychain Diagnostics Methods ---

// GetKeychainServiceName returns the service name used for storing tokens in the keychain.
func (s *SecureTokenStorage) GetKeychainServiceName() string {
	return keyringService
}

// GetKeychainUserName returns the user/account name used for storing tokens in the keychain.
func (s *SecureTokenStorage) GetKeychainUserName() string {
	return keyringUser
}

// DiagnoseKeychain performs a series of tests (set, get, delete) on the OS keychain.
// It returns a map containing the results of each test operation.
func (s *SecureTokenStorage) DiagnoseKeychain() map[string]interface{} {
	results := make(map[string]interface{})
	testKey := fmt.Sprintf("%s_diag_test", s.GetKeychainServiceName())
	testUser := "RTMAuthTokenDetails_diag_test" // Keep user part consistent if needed elsewhere
	testValue := fmt.Sprintf("diag_test_value_%d", time.Now().UnixNano())

	s.logger.Debug("Running keychain diagnostic tests.", "service", testKey, "user", testUser)

	// Test SET operation
	setErr := keyring.Set(testKey, testUser, testValue)
	results["set_success"] = (setErr == nil)
	if setErr != nil {
		results["set_error"] = setErr.Error()
		s.logger.Warn("Keychain diagnostic SET failed.", "error", setErr)
		return results // Stop if set fails
	}
	s.logger.Debug("Keychain diagnostic SET successful.")

	// Test GET operation (only if set succeeded)
	getValue, getErr := keyring.Get(testKey, testUser)
	results["get_success"] = (getErr == nil)
	if getErr != nil {
		results["get_error"] = getErr.Error()
		s.logger.Warn("Keychain diagnostic GET failed.", "error", getErr)
	} else {
		results["get_value_match"] = (getValue == testValue)
		if getValue != testValue {
			s.logger.Warn("Keychain diagnostic GET value mismatch.", "expected", testValue, "got", getValue)
		} else {
			s.logger.Debug("Keychain diagnostic GET successful and value matched.")
		}
	}

	// Test DELETE operation (clean up test entry)
	deleteErr := keyring.Delete(testKey, testUser)
	results["delete_success"] = (deleteErr == nil)
	if deleteErr != nil {
		results["delete_error"] = deleteErr.Error()
		s.logger.Warn("Keychain diagnostic DELETE failed.", "error", deleteErr)
	} else {
		s.logger.Debug("Keychain diagnostic DELETE successful.")
	}

	return results
}

// GetKeychainAdvice provides user-friendly troubleshooting advice for common macOS keychain issues.
func (s *SecureTokenStorage) GetKeychainAdvice() string {
	var advice strings.Builder
	advice.WriteString("Common MacOS Keychain Troubleshooting Advice:\n")
	advice.WriteString("1. Check for Dialogs: Look for any keychain permission dialogs popping up that might need your password or approval.\n")
	advice.WriteString("2. Unlock Keychain: Open 'Keychain Access' (in Applications > Utilities). Make sure the 'login' keychain (usually top-left) is unlocked (no padlock icon).\n")
	advice.WriteString("3. Remove Old Entries: In Keychain Access, search for 'CowGnitionRTM'. If you find any entries, delete them.\n")
	advice.WriteString("4. Restart Application: Quit CowGnition (and potentially Claude Desktop if it launched CowGnition) and start it again.\n")
	advice.WriteString("5. Grant Permission: If a keychain access prompt appears when the app runs, enter your login password and select 'Always Allow' if available.")
	return advice.String()
}
