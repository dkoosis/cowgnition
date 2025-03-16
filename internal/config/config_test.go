// internal/config/config_test.go

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary directory for test configs
	tempDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a valid test config file
	validConfigPath := filepath.Join(tempDir, "config.yaml")
	validConfig := `
server:
  name: "Test Server"
  port: 8080
  status_secret: "test-secret"
  dev_mode: true

rtm:
  api_key: "test-key"
  shared_secret: "test-secret"
  permission: "delete"
  token_refresh: 24

auth:
  token_path: "~/.config/test/token"
  disable_encryption: false

logging:
  level: "info"
  format: "text"
  file: ""
`
	if err := os.WriteFile(validConfigPath, []byte(validConfig), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Test valid config loading
	t.Run("ValidConfig", func(t *testing.T) {
		cfg, err := LoadConfig(validConfigPath)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}

		// Verify config values
		if cfg.Server.Name != "Test Server" {
			t.Errorf("Server.Name = %v, want %v", cfg.Server.Name, "Test Server")
		}
		if cfg.Server.Port != 8080 {
			t.Errorf("Server.Port = %v, want %v", cfg.Server.Port, 8080)
		}
		if cfg.RTM.APIKey != "test-key" {
			t.Errorf("RTM.APIKey = %v, want %v", cfg.RTM.APIKey, "test-key")
		}
		if cfg.RTM.Permission != "delete" {
			t.Errorf("RTM.Permission = %v, want %v", cfg.RTM.Permission, "delete")
		}
		if cfg.Auth.TokenPath != "~/.config/test/token" {
			t.Errorf("Auth.TokenPath = %v, want %v", cfg.Auth.TokenPath, "~/.config/test/token")
		}
		if cfg.Logging.Level != "info" {
			t.Errorf("Logging.Level = %v, want %v", cfg.Logging.Level, "info")
		}
	})

	// Test invalid config file (missing required field)
	invalidConfigPath := filepath.Join(tempDir, "invalid.yaml")
	invalidConfig := `
server:
  name: ""  # Missing required name
  port: 8080

rtm:
  api_key: "test-key"
  shared_secret: "test-secret"

auth:
  token_path: "~/.config/test/token"
`
	if err := os.WriteFile(invalidConfigPath, []byte(invalidConfig), 0644); err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}

	t.Run("InvalidConfig", func(t *testing.T) {
		_, err := LoadConfig(invalidConfigPath)
		if err == nil {
			t.Error("LoadConfig() with invalid config should return error")
		}
	})

	// Test invalid config file (invalid port)
	invalidPortPath := filepath.Join(tempDir, "invalid_port.yaml")
	invalidPortConfig := `
server:
  name: "Test Server"
  port: -1  # Invalid port

rtm:
  api_key: "test-key"
  shared_secret: "test-secret"
  permission: "delete"

auth:
  token_path: "~/.config/test/token"
`
	if err := os.WriteFile(invalidPortPath, []byte(invalidPortConfig), 0644); err != nil {
		t.Fatalf("Failed to write invalid port config: %v", err)
	}

	t.Run("InvalidPort", func(t *testing.T) {
		_, err := LoadConfig(invalidPortPath)
		if err == nil {
			t.Error("LoadConfig() with invalid port should return error")
		}
	})

	// Test nonexistent config file
	t.Run("NonexistentFile", func(t *testing.T) {
		_, err := LoadConfig(filepath.Join(tempDir, "nonexistent.yaml"))
		if err == nil {
			t.Error("LoadConfig() with nonexistent file should return error")
		}
	})

	// Test environment variable overrides
	t.Run("EnvVarOverrides", func(t *testing.T) {
		// Set environment variables
		os.Setenv("RTM_API_KEY", "env-api-key")
		os.Setenv("RTM_SHARED_SECRET", "env-shared-secret")
		os.Setenv("PORT", "9090")
		defer func() {
			os.Unsetenv("RTM_API_KEY")
			os.Unsetenv("RTM_SHARED_SECRET")
			os.Unsetenv("PORT")
		}()

		cfg, err := LoadConfig(validConfigPath)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}

		// Verify environment variables override config values
		if cfg.RTM.APIKey != "env-api-key" {
			t.Errorf("RTM.APIKey should be overridden, got %v, want %v", cfg.RTM.APIKey, "env-api-key")
		}
		if cfg.RTM.SharedSecret != "env-shared-secret" {
			t.Errorf("RTM.SharedSecret should be overridden, got %v, want %v", cfg.RTM.SharedSecret, "env-shared-secret")
		}
		if cfg.Server.Port != 9090 {
			t.Errorf("Server.Port should be overridden, got %v, want %v", cfg.Server.Port, 9090)
		}
	})

	// Test default values
	defaultConfigPath := filepath.Join(tempDir, "default.yaml")
	defaultConfig := `
server:
  name: "Test Server"

rtm:
  api_key: "test-key"
  shared_secret: "test-secret"

auth:
  token_path: "~/.config/test/token"
`
	if err := os.WriteFile(defaultConfigPath, []byte(defaultConfig), 0644); err != nil {
		t.Fatalf("Failed to write default config: %v", err)
	}

	t.Run("DefaultValues", func(t *testing.T) {
		cfg, err := LoadConfig(defaultConfigPath)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}

		// Verify default values are set
		if cfg.Server.Port != 8080 {
			t.Errorf("Default Server.Port = %v, want %v", cfg.Server.Port, 8080)
		}
		if cfg.RTM.Permission != "delete" {
			t.Errorf("Default RTM.Permission = %v, want %v", cfg.RTM.Permission, "delete")
		}
		if cfg.RTM.TokenRefresh != 24 {
			t.Errorf("Default RTM.TokenRefresh = %v, want %v", cfg.RTM.TokenRefresh, 24)
		}
		if cfg.Logging.Level != "info" {
			t.Errorf("Default Logging.Level = %v, want %v", cfg.Logging.Level, "info")
		}
		if cfg.Logging.Format != "text" {
			t.Errorf("Default Logging.Format = %v, want %v", cfg.Logging.Format, "text")
		}
	})
}

func TestExpandPath(t *testing.T) {
	// Test expanding home directory
	homePath := expandPath("~/test/path")
	homeDir, _ := os.UserHomeDir()
	expectedPath := filepath.Join(homeDir, "test/path")

	if homePath != expectedPath {
		t.Errorf("expandPath('~/test/path') = %v, want %v", homePath, expectedPath)
	}

	// Test non-home path
	normalPath := "/tmp/test/path"
	expandedPath := expandPath(normalPath)
	if expandedPath != normalPath {
		t.Errorf("expandPath('%s') = %v, want %v", normalPath, expandedPath, normalPath)
	}
}

func TestParseInt(t *testing.T) {
	testCases := []struct {
		input     string
		expected  int
		expectErr bool
	}{
		{"123", 123, false},
		{"0", 0, false},
		{"-123", -123, false},
		{"123abc", 0, true}, // Now should fail since we improved parseInt
		{"abc", 0, true},
		{"", 0, true},
	}

	for _, tc := range testCases {
		result, err := parseInt(tc.input)

		// Check error
		if (err != nil) != tc.expectErr {
			t.Errorf("parseInt(%q) error = %v, want error = %v", tc.input, err != nil, tc.expectErr)
		}

		// Check value if no error expected
		if !tc.expectErr && result != tc.expected {
			t.Errorf("parseInt(%q) = %v, want %v", tc.input, result, tc.expected)
		}
	}
}
