// file: internal/mcp/mcp_server_test.go
package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
)

// TestMCPInitializationProtocol tests the basic MCP protocol handshake
// using the in-memory transport for testing.
func TestMCPInitializationProtocol(t *testing.T) {
	// Create an in-memory transport pair
	transportPair := transport.NewInMemoryTransportPair()
	defer transportPair.CloseChannels()

	// Set up a test context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a minimal schema validator for testing
	schemaSource := schema.SchemaSource{
		// Use a minimal embedded schema for testing
		Embedded: []byte(`{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"definitions": {
				"JSONRPCRequest": {
					"properties": {
						"id": { "type": ["string", "integer"] },
						"jsonrpc": { "const": "2.0", "type": "string" },
						"method": { "type": "string" },
						"params": { "type": "object" }
					},
					"required": ["id", "jsonrpc", "method"],
					"type": "object"
				}
			}
		}`),
	}
	validator := schema.NewSchemaValidator(schemaSource, logging.GetNoopLogger())
	if err := validator.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize schema validator: %v", err)
	}

	// Set up the server with the in-memory transport
	server, err := NewServer(
		config.DefaultConfig(),
		ServerOptions{
			RequestTimeout:  2 * time.Second,
			ShutdownTimeout: 1 * time.Second,
			Debug:           true,
		},
		validator,
		time.Now(),
		logging.GetNoopLogger(),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Assign the server transport
	server.transport = transportPair.ServerTransport

	// Start the server in a goroutine
	serverErrCh := make(chan error, 1)
	go func() {
		// Use context.Background() for the server to prevent premature shutdown
		// The test will close the transports to stop the server
		serverErrCh <- server.serve(context.Background(), server.handleMessage)
	}()

	// Client-side: Send an initialize request
	initializeReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"clientInfo": map[string]interface{}{
				"name":    "TestClient",
				"version": "1.0.0",
			},
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"sampling": map[string]interface{}{},
			},
		},
	}

	initializeReqBytes, err := json.Marshal(initializeReq)
	if err != nil {
		t.Fatalf("Failed to marshal initialize request: %v", err)
	}

	// Send initialize request
	if err := transportPair.ClientTransport.WriteMessage(ctx, initializeReqBytes); err != nil {
		t.Fatalf("Failed to send initialize request: %v", err)
	}

	// Receive and parse the initialize response
	initializeRespBytes, err := transportPair.ClientTransport.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("Failed to receive initialize response: %v", err)
	}

	var initializeResp map[string]interface{}
	if err := json.Unmarshal(initializeRespBytes, &initializeResp); err != nil {
		t.Fatalf("Failed to unmarshal initialize response: %v", err)
	}

	// Verify the response structure
	result, ok := initializeResp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected result object in response, got: %v", initializeResp)
	}

	// Verify required fields in the initialize result
	requiredFields := []string{"serverInfo", "protocolVersion", "capabilities"}
	for _, field := range requiredFields {
		if _, exists := result[field]; !exists {
			t.Errorf("Missing required field in initialize response: %s", field)
		}
	}

	// Verify protocol version
	protocolVersion, ok := result["protocolVersion"].(string)
	if !ok || protocolVersion == "" {
		t.Errorf("Invalid or missing protocol version: %v", result["protocolVersion"])
	}

	// Send notifications/initialized to complete handshake
	initializedNotif := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]interface{}{},
	}

	initializedNotifBytes, err := json.Marshal(initializedNotif)
	if err != nil {
		t.Fatalf("Failed to marshal initialized notification: %v", err)
	}

	if err := transportPair.ClientTransport.WriteMessage(ctx, initializedNotifBytes); err != nil {
		t.Fatalf("Failed to send initialized notification: %v", err)
	}

	// Verify connection state by sending a tools/list request
	toolsListReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}

	toolsListReqBytes, err := json.Marshal(toolsListReq)
	if err != nil {
		t.Fatalf("Failed to marshal tools/list request: %v", err)
	}

	if err := transportPair.ClientTransport.WriteMessage(ctx, toolsListReqBytes); err != nil {
		t.Fatalf("Failed to send tools/list request: %v", err)
	}

	// Receive and parse the tools/list response
	toolsListRespBytes, err := transportPair.ClientTransport.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("Failed to receive tools/list response: %v", err)
	}

	var toolsListResp map[string]interface{}
	if err := json.Unmarshal(toolsListRespBytes, &toolsListResp); err != nil {
		t.Fatalf("Failed to unmarshal tools/list response: %v", err)
	}

	// Verify the tools response has a result with tools array
	toolsResult, ok := toolsListResp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected result object in tools/list response, got: %v", toolsListResp)
	}

	tools, ok := toolsResult["tools"].([]interface{})
	if !ok {
		t.Fatalf("Expected tools array in tools/list result, got: %v", toolsResult)
	}

	// Verify we have at least one tool defined
	if len(tools) == 0 {
		t.Errorf("Expected at least one tool in tools/list response, got empty array")
	}

	// Clean up by closing the transports
	if err := transportPair.ClientTransport.Close(); err != nil {
		t.Errorf("Failed to close client transport: %v", err)
	}

	if err := transportPair.ServerTransport.Close(); err != nil {
		t.Errorf("Failed to close server transport: %v", err)
	}

	// Check if the server reported any errors
	select {
	case err := <-serverErrCh:
		if err != nil && !transport.IsClosedError(err) {
			t.Fatalf("Server reported unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		// Server is probably still running, that's fine
	}
}

// Helper function to check if a string contains any of the given substrings
func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if strings.Contains(strings.ToLower(s), strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

// TestInvalidMethodSequence tests that the server correctly enforces
// MCP protocol sequence (e.g., initialize must happen before other methods).
func TestInvalidMethodSequence(t *testing.T) {
	// Create an in-memory transport pair
	transportPair := transport.NewInMemoryTransportPair()
	defer transportPair.CloseChannels()

	// Set up a test context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a minimal schema validator for testing
	schemaSource := schema.SchemaSource{
		// Use a minimal embedded schema for testing
		Embedded: []byte(`{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"definitions": {
				"JSONRPCRequest": {
					"properties": {
						"id": { "type": ["string", "integer"] },
						"jsonrpc": { "const": "2.0", "type": "string" },
						"method": { "type": "string" },
						"params": { "type": "object" }
					},
					"required": ["id", "jsonrpc", "method"],
					"type": "object"
				}
			}
		}`),
	}
	validator := schema.NewSchemaValidator(schemaSource, logging.GetNoopLogger())
	if err := validator.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize schema validator: %v", err)
	}

	// Set up the server with the in-memory transport
	server, err := NewServer(
		config.DefaultConfig(),
		ServerOptions{
			RequestTimeout:  2 * time.Second,
			ShutdownTimeout: 1 * time.Second,
			Debug:           true,
		},
		validator,
		time.Now(),
		logging.GetNoopLogger(),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Assign the server transport
	server.transport = transportPair.ServerTransport

	// Start the server in a goroutine
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.serve(context.Background(), server.handleMessage)
	}()

	// Client-side: Skip initialization and send a tools/list request directly
	// This should be rejected since initialize hasn't been called
	toolsListReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}

	toolsListReqBytes, err := json.Marshal(toolsListReq)
	if err != nil {
		t.Fatalf("Failed to marshal tools/list request: %v", err)
	}

	if err := transportPair.ClientTransport.WriteMessage(ctx, toolsListReqBytes); err != nil {
		t.Fatalf("Failed to send tools/list request: %v", err)
	}

	// Receive and parse the error response
	errorRespBytes, err := transportPair.ClientTransport.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("Failed to receive error response: %v", err)
	}

	var errorResp map[string]interface{}
	if err := json.Unmarshal(errorRespBytes, &errorResp); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	// Verify this is an error response
	errorObj, ok := errorResp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected error object in response, got: %v", errorResp)
	}

	// Verify error has appropriate code and message
	code, ok := errorObj["code"].(float64)
	if !ok {
		t.Fatalf("Expected numeric error code, got: %v", errorObj["code"])
	}

	// The code should be an Invalid Request error or similar
	if code > -32000 || code < -32700 {
		t.Errorf("Expected standard JSON-RPC error code, got: %v", code)
	}

	message, ok := errorObj["message"].(string)
	if !ok || message == "" {
		t.Errorf("Expected error message, got: %v", errorObj["message"])
	}

	// Verify the error message indicates something about initialization
	if !containsAny(message, []string{"initialize", "not initialized", "connection"}) {
		t.Errorf("Expected error message to mention initialization, got: %s", message)
	}

	// Clean up
	if err := transportPair.ClientTransport.Close(); err != nil {
		t.Errorf("Failed to close client transport: %v", err)
	}

	if err := transportPair.ServerTransport.Close(); err != nil {
		t.Errorf("Failed to close server transport: %v", err)
	}
}

// TestMCPMethodNotFound tests that the server correctly handles
// requests for non-existent methods.
func TestMCPMethodNotFound(t *testing.T) {
	// Create an in-memory transport pair
	transportPair := transport.NewInMemoryTransportPair()
	defer transportPair.CloseChannels()

	// Set up a test context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a minimal schema validator for testing
	schemaSource := schema.SchemaSource{
		// Use a minimal embedded schema for testing
		Embedded: []byte(`{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"definitions": {
				"JSONRPCRequest": {
					"properties": {
						"id": { "type": ["string", "integer"] },
						"jsonrpc": { "const": "2.0", "type": "string" },
						"method": { "type": "string" },
						"params": { "type": "object" }
					},
					"required": ["id", "jsonrpc", "method"],
					"type": "object"
				}
			}
		}`),
	}
	validator := schema.NewSchemaValidator(schemaSource, logging.GetNoopLogger())
	if err := validator.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize schema validator: %v", err)
	}

	// Set up the server with the in-memory transport
	server, err := NewServer(
		config.DefaultConfig(),
		ServerOptions{
			RequestTimeout:  2 * time.Second,
			ShutdownTimeout: 1 * time.Second,
			Debug:           true,
		},
		validator,
		time.Now(),
		logging.GetNoopLogger(),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Assign the server transport
	server.transport = transportPair.ServerTransport

	// Start the server in a goroutine
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.serve(context.Background(), server.handleMessage)
	}()

	// Initialize the connection properly first
	initializeReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"clientInfo": map[string]interface{}{
				"name":    "TestClient",
				"version": "1.0.0",
			},
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
		},
	}

	initializeReqBytes, err := json.Marshal(initializeReq)
	if err != nil {
		t.Fatalf("Failed to marshal initialize request: %v", err)
	}

	if err := transportPair.ClientTransport.WriteMessage(ctx, initializeReqBytes); err != nil {
		t.Fatalf("Failed to send initialize request: %v", err)
	}

	// Read and discard the initialize response
	_, err = transportPair.ClientTransport.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("Failed to receive initialize response: %v", err)
	}

	// Send notifications/initialized to complete handshake
	initializedNotif := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]interface{}{},
	}

	initializedNotifBytes, err := json.Marshal(initializedNotif)
	if err != nil {
		t.Fatalf("Failed to marshal initialized notification: %v", err)
	}

	if err := transportPair.ClientTransport.WriteMessage(ctx, initializedNotifBytes); err != nil {
		t.Fatalf("Failed to send initialized notification: %v", err)
	}

	// Now send a request for a non-existent method
	nonExistentReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "non_existent_method",
		"params":  map[string]interface{}{},
	}

	nonExistentReqBytes, err := json.Marshal(nonExistentReq)
	if err != nil {
		t.Fatalf("Failed to marshal non-existent method request: %v", err)
	}

	if err := transportPair.ClientTransport.WriteMessage(ctx, nonExistentReqBytes); err != nil {
		t.Fatalf("Failed to send non-existent method request: %v", err)
	}

	// Receive and parse the error response
	errorRespBytes, err := transportPair.ClientTransport.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("Failed to receive error response: %v", err)
	}

	var errorResp map[string]interface{}
	if err := json.Unmarshal(errorRespBytes, &errorResp); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	// Verify this is an error response
	errorObj, ok := errorResp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected error object in response, got: %v", errorResp)
	}

	// Verify error has appropriate code and message
	code, ok := errorObj["code"].(float64)
	if !ok {
		t.Fatalf("Expected numeric error code, got: %v", errorObj["code"])
	}

	// The code should be Method Not Found (-32601)
	if code != -32601 {
		t.Errorf("Expected Method Not Found error code (-32601), got: %v", code)
	}

	// Clean up
	if err := transportPair.ClientTransport.Close(); err != nil {
		t.Errorf("Failed to close client transport: %v", err)
	}

	if err := transportPair.ServerTransport.Close(); err != nil {
		t.Errorf("Failed to close server transport: %v", err)
	}
}
