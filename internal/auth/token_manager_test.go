package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTokenManager(t *testing.T) {
	// Create a temporary directory for test
	tempDir, err := os.MkdirTemp("", "token-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Test file path
	tokenPath := filepath.Join(tempDir, "test_token")
	
	// Create token manager
	tm, err := NewTokenManager(tokenPath)
	if err != nil {
		t.Fatalf("NewTokenManager() error = %v", err)
	}
	
	// Test HasToken (should be false initially)
	if tm.HasToken() {
		t.Errorf("HasToken() = true, want false for new TokenManager")
	}
	
	// Test SaveToken
	testToken := "test_token_123"
	if err := tm.SaveToken(testToken); err != nil {
		t.Errorf("SaveToken() error = %v", err)
	}
	
	// Verify file exists with correct permissions
	fileInfo, err := os.Stat(tokenPath)
	if err != nil {
		t.Errorf("Token file not created: %v", err)
	}
	
	// Check file mode (should be 0600)
	expectedMode := os.FileMode(0600)
	if fileInfo.Mode().Perm() != expectedMode {
		t.Errorf("Token file has incorrect permissions: got %v, want %v", 
			fileInfo.Mode().Perm(), expectedMode)
	}
	
	// Test HasToken (should be true now)
	if !tm.HasToken() {
		t.Errorf("HasToken() = false, want true after saving token")
	}
	
	// Test LoadToken
	loadedToken, err := tm.LoadToken()
	if err != nil {
		t.Errorf("LoadToken() error = %v", err)
	}
	
	if loadedToken != testToken {
		t.Errorf("LoadToken() = %v, want %v", loadedToken, testToken)
	}
	
	// Test DeleteToken
	if err := tm.DeleteToken(); err != nil {
		t.Errorf("DeleteToken() error = %v", err)
	}
	
	// Verify file no longer exists
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Errorf("Token file still exists after DeleteToken()")
	}
	
	// HasToken should now be false
	if tm.HasToken() {
		t.Errorf("HasToken() = true, want false after DeleteToken()")
	}
	
	// Test LoadToken on non-existent file
	_, err = tm.LoadToken()
	if err == nil {
		t.Errorf("LoadToken() should return error for non-existent file")
	}
	
	// Test DeleteToken on non-existent file (should not error)
	if err := tm.DeleteToken(); err != nil {
		t.Errorf("DeleteToken() on non-existent file returned error: %v", err)
	}
}

func TestNewTokenManagerCreatesDirectory(t *testing.T) {
	// Create a temporary directory for test
	tempDir, err := os.MkdirTemp("", "token-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create a subdirectory path that doesn't exist yet
	subDir := filepath.Join(tempDir, "subdir1", "subdir2")
	tokenPath := filepath.Join(subDir, "test_token")
	
	// Verify directory doesn't exist
	if _, err := os.Stat(subDir); !os.IsNotExist(err) {
		t.Fatalf("Test setup failed: subdirectory already exists")
	}
	
	// Create token manager, which should create the directory
	_, err = NewTokenManager(tokenPath)
	if err != nil {
		t.Fatalf("NewTokenManager() error = %v", err)
	}
	
	// Verify directory was created
	if _, err := os.Stat(subDir); os.IsNotExist(err) {
		t.Errorf("NewTokenManager() failed to create directory structure")
	}
}

func TestTokenManagerWithTilde(t *testing.T) {
	// Skip this test if running as root, as home directory expansion won't work as expected
	if os.Geteuid() == 0 {
		t.Skip("Skipping test when running as root")
	}
	
	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}
	
	// Test with path containing tilde
	tokenPath := "~/test_token_tilde"
	expandedPath := filepath.Join(homeDir, "test_token_tilde")
	
	// Create a fake token manager that doesn't actually create files
	// We're just testing path expansion, not file operations
	tm := &TokenManager{
		tokenPath: tokenPath,
	}
	
	// Verify tokenPath was expanded
	if tm.tokenPath != expandedPath {
		t.Errorf("TokenManager path expansion failed: got %v, want %v", 
			tm.tokenPath, expandedPath)
	}
}
