// internal/logging/logger_test.go
package logging

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestGetLogger(t *testing.T) {
	// Get a logger for a component
	logger := GetLogger("test")
	if logger == nil {
		t.Fatal("GetLogger returned nil")
	}
}

func TestLogOutput(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer

	// Initialize logging with the buffer
	InitLogging(LevelDebug, &buf)

	// Get a logger and log a message
	logger := GetLogger("test_component")
	logger.Info("test message", "key1", "value1", "key2", 123)

	// Parse the JSON log entry
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	// Verify log fields
	if logEntry["msg"] != "test message" {
		t.Errorf("Expected msg to be 'test message', got %v", logEntry["msg"])
	}

	if logEntry["component"] != "test_component" {
		t.Errorf("Expected component to be 'test_component', got %v", logEntry["component"])
	}

	if logEntry["key1"] != "value1" {
		t.Errorf("Expected key1 to be 'value1', got %v", logEntry["key1"])
	}

	if int(logEntry["key2"].(float64)) != 123 {
		t.Errorf("Expected key2 to be 123, got %v", logEntry["key2"])
	}
}

func TestIsDebugEnabled(t *testing.T) {
	// Set level to INFO
	SetLevel(LevelInfo)
	if IsDebugEnabled() {
		t.Error("IsDebugEnabled should return false when level is INFO")
	}

	// Set level to DEBUG
	SetLevel(LevelDebug)
	if !IsDebugEnabled() {
		t.Error("IsDebugEnabled should return true when level is DEBUG")
	}
}
