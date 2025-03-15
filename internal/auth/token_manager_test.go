package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTokenManager(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "token_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir) // Clean up after test

	// Create token path
	tokenPath := filepath.Join(tempDir, "test_token")

	// Create token manager
	tm, err := NewTokenManager(tokenPath)
	if err != nil {
		t.Fatalf("Failed to create token manager: %v", err)
	}

	// Disable encryption for predictable test results
	tm.DisableEncryption()

	// Test initial state - should have no token
	if tm.HasToken() {
		t.Errorf("New token manager should not have a token")
	}

	// Test saving token
	testToken := "test_auth_token_12345"
	if saveErr := tm.SaveToken(testToken); saveErr != nil {
		t.Errorf("Failed to save token: %v", saveErr)
	}

	// Check that token exists
	if !tm.HasToken() {
		t.Errorf("Token manager should have a token after saving")
	}

	// Test loading token
	loadedToken, loadErr := tm.LoadToken()
	if loadErr != nil {
		t.Errorf("Failed to load token: %v", loadErr)
	}
	if loadedToken != testToken {
		t.Errorf("Loaded token does not match saved token: got %s, want %s", loadedToken, testToken)
	}

	// Test file info
	fileInfo, infoErr := tm.GetTokenFileInfo()
	if infoErr != nil {
		t.Errorf("Failed to get token file info: %v", infoErr)
	}
	if fileInfo.Size() == 0 {
		t.Errorf("Token file size should not be zero")
	}

	// Verify file mod time is recent
	if time.Since(fileInfo.ModTime()) > time.Minute {
		t.Errorf("Token file modification time is too old")
	}

	// Test deleting token
	if deleteErr := tm.DeleteToken(); deleteErr != nil {
		t.Errorf("Failed to delete token: %v", deleteErr)
	}

	// Check that token is gone
	if tm.HasToken() {
		t.Errorf("Token manager should not have a token after deletion")
	}
	if _, statErr := os.Stat(tokenPath); !os.IsNotExist(statErr) {
		t.Errorf("Token file should not exist after deletion")
	}
}

func TestTokenManagerCreateDirectory(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, createErr := os.MkdirTemp("", "token_test")
	if createErr != nil {
		t.Fatalf("Failed to create temp dir: %v", createErr)
	}
	defer os.RemoveAll(tempDir) // Clean up after test

	// Create a nested path that doesn't exist yet
	subDir := filepath.Join(tempDir, "nested", "dirs")
	tokenPath := filepath.Join(subDir, "test_token")

	// Check that directory doesn't exist
	if _, statErr := os.Stat(subDir); !os.IsNotExist(statErr) {
		t.Errorf("Test setup failed: nested directory should not exist")
	}

	// Create token manager - should create the directory
	tm, createTmErr := NewTokenManager(tokenPath)
	if createTmErr != nil {
		t.Fatalf("Failed to create token manager with nested path: %v", createTmErr)
	}

	// Disable encryption for testing
	tm.DisableEncryption()

	// Check that directory was created
	if _, statErr := os.Stat(subDir); os.IsNotExist(statErr) {
		t.Errorf("Token manager failed to create directory structure")
	}

	// Test saving token in nested directory
	testToken := "test_auth_token_in_nested_dir"
	if saveErr := tm.SaveToken(testToken); saveErr != nil {
		t.Errorf("Failed to save token in nested directory: %v", saveErr)
	}

	// Check that token exists and can be loaded
	loadedToken, loadErr := tm.LoadToken()
	if loadErr != nil {
		t.Errorf("Failed to load token from nested directory: %v", loadErr)
	}
	if loadedToken != testToken {
		t.Errorf("Loaded token from nested directory does not match: got %s, want %s", loadedToken, testToken)
	}
}

func TestTokenEncryption(t *testing.T) {
	// Skip if running in CI environment
	if os.Getenv("CI") != "" {
		t.Skip("Skipping encryption test in CI environment")
	}

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "token_encryption_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tokenPath := filepath.Join(tempDir, "encrypted_token")

	// Create token manager with encryption enabled
	tm, err := NewTokenManager(tokenPath)
	if err != nil {
		t.Fatalf("Failed to create token manager: %v", err)
	}

	// Test saving and loading encrypted token
	testToken := "super_secret_token_12345"
	if err := tm.SaveToken(testToken); err != nil {
		t.Fatalf("Failed to save encrypted token: %v", err)
	}

	// Read the raw file to verify it's encrypted
	rawData, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("Failed to read token file: %v", err)
	}

	// The encrypted data should not contain the plain token
	if string(rawData) == testToken {
		t.Errorf("Token appears to be stored unencrypted")
	}

	// Load the token and verify decryption works
	loadedToken, err := tm.LoadToken()
	if err != nil {
		t.Fatalf("Failed to load encrypted token: %v", err)
	}

	if loadedToken != testToken {
		t.Errorf("Loaded encrypted token does not match: got %s, want %s", loadedToken, testToken)
	}
}

func TestGenerateTokenFilename(t *testing.T) {
	testCases := []struct {
		apiKey     string
		wantPrefix string
	}{
		{"abcdef1234567890", "rtm_token_abcdef12"},
		{"short", "rtm_token_short"},
		{"", "rtm_token_"},
	}

	for _, tc := range testCases {
		filename := GenerateTokenFilename(tc.apiKey)

		// Check that filename starts with expected prefix
		if len(filename) < len(tc.wantPrefix) || filename[:len(tc.wantPrefix)] != tc.wantPrefix {
			t.Errorf("GenerateTokenFilename(%q) = %q, want prefix %q",
				tc.apiKey, filename, tc.wantPrefix)
		}

		// Check that filename contains a hash component
		if len(tc.apiKey) > 0 && len(filename) <= len(tc.wantPrefix)+1 {
			t.Errorf("GenerateTokenFilename(%q) = %q, expected hash component after prefix",
				tc.apiKey, filename)
		}
	}
}
