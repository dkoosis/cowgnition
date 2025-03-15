package auth

import (
	"os"
	"path/filepath"
	"testing"
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
