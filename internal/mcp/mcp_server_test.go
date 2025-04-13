// file: internal/mcp/mcp_server_test.go
package mcp

import (
	"context"
	"encoding/json" // Removed unused strings import after removing containsAny.
	"testing"
	"time"

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
	"github.com/stretchr/testify/assert" // Use testify/assert.
	"github.com/stretchr/testify/require"
)

// TestMCPInitializationProtocol tests the basic MCP protocol handshake
// using the in-memory transport for testing.
func TestMCPInitializationProtocol(t *testing.T) {
	// Create an in-memory transport pair.
	transportPair := transport.NewInMemoryTransportPair()
	defer transportPair.CloseChannels()

	// Set up a test context with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a minimal schema validator for testing.
	// Note: This uses a minimal schema, not the full schema.json used by the actual server.
	schemaSource := schema.SchemaSource{
		Embedded: []byte(`{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"definitions": {
				"InitializeRequest": { "type": "object", "required": ["clientInfo", "protocolVersion", "capabilities"], "properties": {"clientInfo":{}, "protocolVersion":{}, "capabilities":{}}},
				"InitializeResult": { "type": "object", "required": ["serverInfo", "protocolVersion", "capabilities"], "properties": {"serverInfo":{}, "protocolVersion":{}, "capabilities":{}}},
				"InitializedNotification": { "type": "object", "required": ["params"], "properties": {"params":{}}},
				"ListToolsRequest": { "type": "object", "properties": {"params":{}}},
				"ListToolsResult": { "type": "object", "required": ["tools"], "properties": {"tools":{}}},
				"JSONRPCRequest": { "type": "object", "required": ["jsonrpc", "method", "id"], "properties": {"jsonrpc":{}, "method":{}, "id":{}}},
				"base": {"type": "object"}
			}
		}`),
	}
	validator := schema.NewSchemaValidator(schemaSource, logging.GetNoopLogger())
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Failed to initialize schema validator.")

	// Set up the server with the in-memory transport.
	server, err := NewServer(
		config.DefaultConfig(),
		ServerOptions{
			RequestTimeout:  2 * time.Second,
			ShutdownTimeout: 1 * time.Second,
			Debug:           true,
		},
		validator, // Use the minimal validator for this test.
		time.Now(),
		logging.GetNoopLogger(),
	)
	require.NoError(t, err, "Failed to create server.")

	// Assign the server transport.
	server.transport = transportPair.ServerTransport

	// Start the server in a goroutine.
	serverErrCh := make(chan error, 1)
	go func() {
		// Use context.Background() for the server to prevent premature shutdown.
		// The test will close the transports to stop the server.
		// Pass the actual handler function (s.handleMessage).
		serverErrCh <- server.serve(context.Background(), server.handleMessage)
	}()

	// Client-side: Send an initialize request.
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
	require.NoError(t, err, "Failed to marshal initialize request.")

	// Send initialize request.
	err = transportPair.ClientTransport.WriteMessage(ctx, initializeReqBytes)
	require.NoError(t, err, "Failed to send initialize request.")

	// Receive and parse the initialize response.
	initializeRespBytes, err := transportPair.ClientTransport.ReadMessage(ctx)
	require.NoError(t, err, "Failed to receive initialize response.")

	var initializeResp map[string]interface{}
	err = json.Unmarshal(initializeRespBytes, &initializeResp)
	require.NoError(t, err, "Failed to unmarshal initialize response.")

	// Verify the response structure.
	// TODO: Consider validating initializeRespBytes against the full schema definition for InitializeResult for a more robust check.
	result, ok := initializeResp["result"].(map[string]interface{})
	require.True(t, ok, "Expected result object in response, got: %v", initializeResp)

	// Verify required fields in the initialize result.
	requiredFields := []string{"serverInfo", "protocolVersion", "capabilities"}
	for _, field := range requiredFields {
		assert.Contains(t, result, field, "Missing required field in initialize response: %s", field)
	}

	// Verify protocol version.
	protocolVersion, ok := result["protocolVersion"].(string)
	assert.True(t, ok && protocolVersion != "", "Invalid or missing protocol version: %v", result["protocolVersion"])

	// Send notifications/initialized to complete handshake.
	initializedNotif := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]interface{}{}, // Params should be an empty object, not omitted.
	}

	initializedNotifBytes, err := json.Marshal(initializedNotif)
	require.NoError(t, err, "Failed to marshal initialized notification.")

	err = transportPair.ClientTransport.WriteMessage(ctx, initializedNotifBytes)
	require.NoError(t, err, "Failed to send initialized notification.")

	// Verify connection state by sending a tools/list request.
	toolsListReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]interface{}{}, // Params should be an empty object.
	}

	toolsListReqBytes, err := json.Marshal(toolsListReq)
	require.NoError(t, err, "Failed to marshal tools/list request.")

	err = transportPair.ClientTransport.WriteMessage(ctx, toolsListReqBytes)
	require.NoError(t, err, "Failed to send tools/list request.")

	// Receive and parse the tools/list response.
	toolsListRespBytes, err := transportPair.ClientTransport.ReadMessage(ctx)
	require.NoError(t, err, "Failed to receive tools/list response.")

	var toolsListResp map[string]interface{}
	err = json.Unmarshal(toolsListRespBytes, &toolsListResp)
	require.NoError(t, err, "Failed to unmarshal tools/list response.")

	// Verify the tools response has a result with tools array.
	toolsResult, ok := toolsListResp["result"].(map[string]interface{})
	require.True(t, ok, "Expected result object in tools/list response, got: %v", toolsListResp)

	tools, ok := toolsResult["tools"].([]interface{})
	require.True(t, ok, "Expected tools array in tools/list result, got: %v", toolsResult)

	// Verify we have at least one tool defined (adjust if your default server has none).
	assert.NotEmpty(t, tools, "Expected at least one tool in tools/list response, got empty array.")

	// Clean up by closing the transports.
	err = transportPair.ClientTransport.Close()
	assert.NoError(t, err, "Failed to close client transport.")

	err = transportPair.ServerTransport.Close()
	assert.NoError(t, err, "Failed to close server transport.")

	// Check if the server reported any errors.
	select {
	case err := <-serverErrCh:
		if err != nil && !transport.IsClosedError(err) { // Use transport.IsClosedError.
			t.Errorf("Server reported unexpected error: %v", err)
		}
	case <-time.After(100 * time.Millisecond): // Shortened timeout.
		// Server might still be running if close didn't propagate immediately, that's okay.
		t.Log("Server did not immediately exit after transport close (may be expected).")
	}
}

// TestInvalidMethodSequence tests that the server correctly enforces
// MCP protocol sequence (e.g., initialize must happen before other methods).
func TestInvalidMethodSequence(t *testing.T) {
	// Create an in-memory transport pair.
	transportPair := transport.NewInMemoryTransportPair()
	defer transportPair.CloseChannels()

	// Set up a test context with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a minimal schema validator for testing.
	schemaSource := schema.SchemaSource{
		Embedded: []byte(`{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"definitions": {
				"ListToolsRequest": { "type": "object", "properties": {"params":{}}},
				"JSONRPCRequest": { "type": "object", "required": ["jsonrpc", "method", "id"], "properties": {"jsonrpc":{}, "method":{}, "id":{}}}
			}
		}`),
	}
	validator := schema.NewSchemaValidator(schemaSource, logging.GetNoopLogger())
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Failed to initialize schema validator.")

	// Set up the server with the in-memory transport.
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
	require.NoError(t, err, "Failed to create server.")

	// Assign the server transport.
	server.transport = transportPair.ServerTransport

	// Start the server in a goroutine.
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.serve(context.Background(), server.handleMessage) // Pass actual handler.
	}()

	// Client-side: Skip initialization and send a tools/list request directly.
	// This should be rejected since initialize hasn't been called.
	toolsListReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}

	toolsListReqBytes, err := json.Marshal(toolsListReq)
	require.NoError(t, err, "Failed to marshal tools/list request.")

	err = transportPair.ClientTransport.WriteMessage(ctx, toolsListReqBytes)
	require.NoError(t, err, "Failed to send tools/list request.")

	// Receive and parse the error response.
	errorRespBytes, err := transportPair.ClientTransport.ReadMessage(ctx)
	require.NoError(t, err, "Failed to receive error response.")

	var errorResp map[string]interface{}
	err = json.Unmarshal(errorRespBytes, &errorResp)
	require.NoError(t, err, "Failed to unmarshal error response.")

	// Verify this is an error response.
	errorObj, ok := errorResp["error"].(map[string]interface{})
	require.True(t, ok, "Expected error object in response, got: %v", errorResp)

	// Verify error has the specific code for sequence errors (-32601 Method Not Found based on mapping).
	code, ok := errorObj["code"].(float64) // JSON numbers unmarshal to float64.
	require.True(t, ok, "Expected numeric error code, got: %v", errorObj["code"])

	// Corrected Assertion: Check for the specific error code mapped in mcp_server.go.
	assert.Equal(t, float64(transport.JSONRPCMethodNotFound), code, "Expected Method Not Found error code (-32601) for sequence violation.")

	// Optionally check the message or data for more context if needed, but code is primary.
	message, ok := errorObj["message"].(string)
	require.True(t, ok, "Expected string error message, got: %v", errorObj["message"])
	// Message might be generic like "Method not found." or specific like "Connection initialization required.".
	// Check the mapping logic in mcp_server.go's mapErrorToJSONRPCComponents.
	assert.Equal(t, "Connection initialization required.", message, "Expected specific error message for sequence violation.")

	errorData, hasData := errorObj["data"].(map[string]interface{})
	if assert.True(t, hasData, "Expected data field in error response.") {
		assert.Contains(t, errorData["detail"], "protocol sequence error", "Error data detail should mention sequence error.")
		assert.Equal(t, "uninitialized", errorData["state"], "Error data should indicate the state was 'uninitialized'.")
	}

	// Clean up.
	err = transportPair.ClientTransport.Close()
	assert.NoError(t, err, "Failed to close client transport.")

	err = transportPair.ServerTransport.Close()
	assert.NoError(t, err, "Failed to close server transport.")
}

// TestMCPMethodNotFound tests that the server correctly handles
// requests for non-existent methods.
func TestMCPMethodNotFound(t *testing.T) {
	// Create an in-memory transport pair.
	transportPair := transport.NewInMemoryTransportPair()
	defer transportPair.CloseChannels()

	// Set up a test context with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a minimal schema validator for testing.
	// Include definitions needed for initialize and the error case.
	schemaSource := schema.SchemaSource{
		Embedded: []byte(`{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"definitions": {
				"InitializeRequest": { "type": "object", "required": ["clientInfo", "protocolVersion", "capabilities"], "properties": {"clientInfo":{}, "protocolVersion":{}, "capabilities":{}}},
				"InitializeResult": { "type": "object", "required": ["serverInfo", "protocolVersion", "capabilities"], "properties": {"serverInfo":{}, "protocolVersion":{}, "capabilities":{}}},
				"InitializedNotification": { "type": "object", "required": ["params"], "properties": {"params":{}}},
				"NonExistentRequest": { "type": "object", "properties": {"params":{}}}, // Schema for the request itself
				"JSONRPCRequest": { "type": "object", "required": ["jsonrpc", "method", "id"], "properties": {"jsonrpc":{}, "method":{}, "id":{}}},
				"base": {"type": "object"}
			}
		}`),
	}
	validator := schema.NewSchemaValidator(schemaSource, logging.GetNoopLogger())
	err := validator.Initialize(ctx)
	require.NoError(t, err, "Failed to initialize schema validator.")

	// Set up the server with the in-memory transport.
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
	require.NoError(t, err, "Failed to create server.")

	// Assign the server transport.
	server.transport = transportPair.ServerTransport

	// Start the server in a goroutine.
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.serve(context.Background(), server.handleMessage) // Pass actual handler.
	}()

	// Initialize the connection properly first.
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
	require.NoError(t, err, "Failed to marshal initialize request.")

	err = transportPair.ClientTransport.WriteMessage(ctx, initializeReqBytes)
	require.NoError(t, err, "Failed to send initialize request.")

	// Read and discard the initialize response.
	_, err = transportPair.ClientTransport.ReadMessage(ctx)
	require.NoError(t, err, "Failed to receive initialize response.")

	// Send notifications/initialized to complete handshake.
	initializedNotif := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]interface{}{},
	}

	initializedNotifBytes, err := json.Marshal(initializedNotif)
	require.NoError(t, err, "Failed to marshal initialized notification.")

	err = transportPair.ClientTransport.WriteMessage(ctx, initializedNotifBytes)
	require.NoError(t, err, "Failed to send initialized notification.")

	// Now send a request for a non-existent method.
	nonExistentReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "non_existent_method",
		"params":  map[string]interface{}{},
	}

	nonExistentReqBytes, err := json.Marshal(nonExistentReq)
	require.NoError(t, err, "Failed to marshal non-existent method request.")

	err = transportPair.ClientTransport.WriteMessage(ctx, nonExistentReqBytes)
	require.NoError(t, err, "Failed to send non-existent method request.")

	// Receive and parse the error response.
	errorRespBytes, err := transportPair.ClientTransport.ReadMessage(ctx)
	require.NoError(t, err, "Failed to receive error response.")

	var errorResp map[string]interface{}
	err = json.Unmarshal(errorRespBytes, &errorResp)
	require.NoError(t, err, "Failed to unmarshal error response.")

	// Verify this is an error response.
	errorObj, ok := errorResp["error"].(map[string]interface{})
	require.True(t, ok, "Expected error object in response, got: %v", errorResp)

	// Verify error has appropriate code and message.
	code, ok := errorObj["code"].(float64) // JSON numbers unmarshal to float64.
	require.True(t, ok, "Expected numeric error code, got: %v", errorObj["code"])

	// The code should be Method Not Found (-32601).
	assert.Equal(t, float64(transport.JSONRPCMethodNotFound), code, "Expected Method Not Found error code (-32601).")

	// Clean up.
	err = transportPair.ClientTransport.Close()
	assert.NoError(t, err, "Failed to close client transport.")

	err = transportPair.ServerTransport.Close()
	assert.NoError(t, err, "Failed to close server transport.")
}

// Removed helper function containsAny as it's no longer needed.
