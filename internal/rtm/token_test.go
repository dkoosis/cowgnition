// File: internal/rtm/token_test.go
package rtm

import (
	"os"            // Needed for checking file existence and potentially permissions.
	"path/filepath" // Used for joining paths.
	"testing"       // Go's standard testing package.
	// Optional: You might consider using an assertion library for more expressive tests,
	// but standard 'testing' is sufficient to start.
	// "github.com/stretchr/testify/assert".
	// "github.com/stretchr/testify/require".
)

// TestNewTokenStorage_Success verifies that NewTokenStorage successfully creates
// the necessary directory structure when it doesn't exist.
func TestNewTokenStorage_Success(t *testing.T) {
	// Create a temporary directory for this test. It will be cleaned up automatically.
	tempDir := t.TempDir()
	// Define a path for the token file within a subdirectory of the temp dir.
	tokenPath := filepath.Join(tempDir, "subdir", "rtm_token.txt")
	tokenDir := filepath.Dir(tokenPath)

	// Check that the directory does *not* exist initially.
	if _, err := os.Stat(tokenDir); !os.IsNotExist(err) {
		t.Fatalf("Test setup error: Directory '%s' should not exist before test, but it does (or another error occurred: %v).", tokenDir, err)
	}

	// Call the function under test.
	storage, err := NewTokenStorage(tokenPath)

	// Assertions:.
	// 1. Check for unexpected errors.
	if err != nil {
		t.Fatalf("NewTokenStorage() returned an unexpected error: %v.", err)
	}
	// 2. Check if the returned storage is not nil.
	if storage == nil {
		t.Fatal("NewTokenStorage() returned nil storage, expected a valid instance.")
	}
	// 3. Check if the TokenPath field was set correctly.
	if storage.TokenPath != tokenPath {
		t.Errorf("NewTokenStorage() TokenPath = %q, want %q.", storage.TokenPath, tokenPath)
	}
	// 4. Verify that the directory was actually created.
	if _, err := os.Stat(tokenDir); os.IsNotExist(err) {
		t.Errorf("NewTokenStorage() failed to create the directory '%s'.", tokenDir)
	} else if err != nil {
		t.Errorf("Error checking directory status after NewTokenStorage(): %v.", err)
	}

	t.Logf("Successfully tested NewTokenStorage directory creation for path: %s.", tokenPath)
}

// TestTokenStorage_SaveLoadCycle verifies the basic functionality of saving a token
// and then loading it back correctly.
func TestTokenStorage_SaveLoadCycle(t *testing.T) {
	tempDir := t.TempDir()
	tokenPath := filepath.Join(tempDir, "token.dat")
	expectedToken := "test-auth-token-12345"

	// Create storage instance first. NewTokenStorage implicitly creates the directory.
	storage, err := NewTokenStorage(tokenPath)
	if err != nil {
		t.Fatalf("Setup failed: NewTokenStorage() returned an error: %v.", err)
	}

	// Save the token.
	err = storage.SaveToken(expectedToken)
	if err != nil {
		t.Fatalf("SaveToken() returned an unexpected error: %v.", err)
	}

	// Load the token back.
	loadedToken, err := storage.LoadToken()
	if err != nil {
		t.Fatalf("LoadToken() returned an unexpected error after save: %v.", err)
	}

	// Assert that the loaded token matches the original.
	if loadedToken != expectedToken {
		t.Errorf("LoadToken() returned token %q, want %q.", loadedToken, expectedToken)
	}

	// Optional: Verify file content directly (though LoadToken test covers this indirectly).
	// data, _ := os.ReadFile(tokenPath).
	// if string(data) != expectedToken { ... }.

	t.Logf("Successfully tested SaveToken -> LoadToken cycle.")
}

// TestLoadToken_FileNotExist verifies that LoadToken returns an empty string and no error
// when the token file does not exist.
func TestLoadToken_FileNotExist(t *testing.T) {
	tempDir := t.TempDir()
	// Path within the temp dir where the token *would* be, but we won't create it.
	tokenPath := filepath.Join(tempDir, "nonexistent_token.txt")

	// Create storage instance. NewTokenStorage creates the *directory*.
	storage, err := NewTokenStorage(tokenPath)
	if err != nil {
		t.Fatalf("Setup failed: NewTokenStorage() returned an error: %v.", err)
	}

	// Attempt to load the token (file should not exist).
	loadedToken, err := storage.LoadToken()

	// Assertions:.
	// 1. Check that no error occurred (file not existing is not an error for LoadToken).
	if err != nil {
		t.Errorf("LoadToken() returned an unexpected error when file doesn't exist: %v.", err)
	}
	// 2. Check that the returned token is empty.
	if loadedToken != "" {
		t.Errorf("LoadToken() returned token %q when file doesn't exist, want empty string.", loadedToken)
	}

	t.Logf("Successfully tested LoadToken behavior for non-existent file.")
}

// TestSaveToken_Overwrite verifies that calling SaveToken multiple times overwrites
// the existing token file with the new content.
func TestSaveToken_Overwrite(t *testing.T) {
	tempDir := t.TempDir()
	tokenPath := filepath.Join(tempDir, "overwrite_token.txt")
	initialToken := "initial-token"
	newToken := "new-overwritten-token"

	// Create storage instance.
	storage, err := NewTokenStorage(tokenPath)
	if err != nil {
		t.Fatalf("Setup failed: NewTokenStorage() returned an error: %v.", err)
	}

	// Save the initial token.
	err = storage.SaveToken(initialToken)
	if err != nil {
		t.Fatalf("SaveToken() initial save failed: %v.", err)
	}

	// Save the new token (should overwrite).
	err = storage.SaveToken(newToken)
	if err != nil {
		t.Fatalf("SaveToken() overwrite save failed: %v.", err)
	}

	// Load the token back to verify the content.
	loadedToken, err := storage.LoadToken()
	if err != nil {
		t.Fatalf("LoadToken() after overwrite failed: %v.", err)
	}

	// Assert that the loaded token is the *new* token.
	if loadedToken != newToken {
		t.Errorf("LoadToken() after overwrite returned %q, want %q.", loadedToken, newToken)
	}
	// Assert it's not the initial token anymore.
	if loadedToken == initialToken {
		t.Errorf("LoadToken() after overwrite returned the initial token %q, expected overwrite.", initialToken)
	}

	t.Logf("Successfully tested SaveToken overwrite behavior.")
}

// --- Potential Future Tests ---.

// func TestSaveToken_PermissionError(t *testing.T) {
//  // More complex: Requires setting up a directory/file with restricted permissions
//  // where WriteFile would fail. Might involve platform-specific setup or mocking os functions.
// }.

// func TestLoadToken_PermissionError(t *testing.T) {
//  // Similar complexity to the SaveToken permission error test.
// }.

// func TestTokenStorage_Concurrency(t *testing.T) {
//  // Requires running SaveToken/LoadToken concurrently using goroutines and potentially
//  // the -race flag during testing to detect race conditions.
//  // Example: Start multiple goroutines saving different tokens and loading concurrently.
//  // Check for data corruption or unexpected errors.
// }.
