// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/client_test.go

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClient_GenerateSignature verifies the RTM API signature generation logic.
func TestClient_GenerateSignature(t *testing.T) {
	// Setup logger once.
	logger := logging.GetNoopLogger()

	// Test cases including the secret for each case.
	testCases := []struct {
		name     string            // Test case name.
		secret   string            // RTM shared secret for this case.
		params   map[string]string // Input parameters for signing.
		expected string            // Expected MD5 signature hash.
	}{
		{
			name:   "Simple params",
			secret: "test_secret",
			params: map[string]string{
				"method":  "rtm.test.echo",
				"api_key": "abc123",
				"format":  "json",
			},
			expected: "ce7eb5843f9dcb6209227c72baf957bc",
		},
		{
			name:   "With auth token 🔒",
			secret: "test_secret",
			params: map[string]string{
				"method":     "rtm.lists.getList",
				"api_key":    "abc123",
				"format":     "json",
				"auth_token": "token123",
			},
			expected: "fa17f481daca02dca3286483755718a0",
		},
		{
			name:   "RTM Example (BANANAS) 🍌",
			secret: "BANANAS",
			params: map[string]string{
				"abc": "baz",
				"feg": "bar",
				"yxz": "foo",
			},
			expected: "82044aae4dd676094f23f1ec152159ba",
		},
		{
			name:     "Custom Case - Empty Params",
			secret:   "another_secret",
			params:   map[string]string{},
			expected: "bb4a87f07bd27e737e0b4a44cfee12f3",
		},
	}

	// Run tests using subtests.
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing signature generation for case: %s.", tc.name)
			// Create a client instance specifically for this test case with its secret.
			client := &Client{
				config: Config{
					SharedSecret: tc.secret,
				},
				logger: logger,
			}

			// Call the method under test.
			t.Logf("Generating signature with secret '%s' and params: %v.", tc.secret, tc.params)
			signature := client.generateSignature(tc.params)
			t.Logf("Generated signature: %s.", signature)

			// Verify the result.
			assert.Equal(t, tc.expected, signature, "Generated signature '%s' didn't match expected '%s'. Udderly wrong.", signature, tc.expected)
			if t.Failed() {
				t.Logf("Signature mismatch details: Params=%v, Secret=%s.", tc.params, tc.secret)
			} else {
				t.Logf("Signature matched expectations. Moo-velous.")
			}
		})
	}
}

// TestClient_PrepareParameters ensures standard parameters are added correctly before signing.
func TestClient_PrepareParameters(t *testing.T) {
	// Setup a test client instance.
	logger := logging.GetNoopLogger()
	client := &Client{
		config: Config{
			APIKey:       "test_key",
			SharedSecret: "test_secret",
			AuthToken:    "test_token",
		},
		logger: logger,
	}
	t.Logf("Testing parameter preparation with API Key '%s' and Auth Token '%s'.", client.config.APIKey, client.config.AuthToken)

	// Test cases defining different method types and parameter sets.
	testCases := []struct {
		name            string            // Name of the test case.
		method          string            // RTM method being called.
		params          map[string]string // Original parameters provided by the caller.
		expectAuthToken bool              // Whether the auth token should be included for this method.
	}{
		{
			name:            "Regular method - Expecting auth token",
			method:          "rtm.test.login",
			params:          map[string]string{},
			expectAuthToken: true,
		},
		{
			name:            "Auth method (getFrob) - Should skip auth token",
			method:          methodGetFrob, // Use constant.
			params:          map[string]string{},
			expectAuthToken: false,
		},
		{
			name:            "Auth method (getToken) - Should skip auth token",
			method:          methodGetToken, // Use constant.
			params:          map[string]string{"frob": "test_frob"},
			expectAuthToken: false,
		},
		{
			name:   "Method with existing params - Ensuring preservation",
			method: methodGetTasks, // Use constant.
			params: map[string]string{
				"filter": "status:incomplete",
			},
			expectAuthToken: true,
		},
	}

	// Run tests using subtests.
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Preparing parameters for method: %s with initial params: %v.", tc.method, tc.params)
			// Call the method under test.
			fullParams := client.prepareParameters(tc.method, tc.params)
			t.Logf("Prepared parameters: %v.", fullParams)

			// Verify common parameters are always present.
			assert.Equal(t, tc.method, fullParams["method"], "Field 'method' should be '%s'.", tc.method)
			assert.Equal(t, "test_key", fullParams["api_key"], "Field 'api_key' should be 'test_key'.")
			assert.Equal(t, "json", fullParams["format"], "Field 'format' should be 'json'.")

			// Verify auth token presence based on expectation.
			if tc.expectAuthToken {
				assert.Equal(t, "test_token", fullParams["auth_token"], "Auth token should be present and correct for method '%s'.", tc.method)
			} else {
				_, hasAuthToken := fullParams["auth_token"]
				assert.False(t, hasAuthToken, "Auth token should NOT be present for auth method '%s'. It's like a cow trying to hide behind a fence post.", tc.method)
			}

			// Verify signature was generated and added.
			assert.NotEmpty(t, fullParams["api_sig"], "API signature 'api_sig' should be generated and not empty.")

			// Verify original parameters passed in are preserved.
			for k, v := range tc.params {
				assert.Equal(t, v, fullParams[k], "Original parameter '%s' should be preserved with value '%s'.", k, v)
			}
			t.Logf("Parameter preparation successful for method '%s'.", tc.method)
		})
	}
}

// TestClient_CallMethod tests the full API call cycle using a mock HTTP server.
func TestClient_CallMethod(t *testing.T) {
	// --- Mock Server Setup ---
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Basic request verification.
		err := r.ParseForm()
		require.NoError(t, err, "[Mock Server] Failed to parse incoming form data.")

		apiKey := r.Form.Get("api_key")
		apiSig := r.Form.Get("api_sig")
		method := r.Form.Get("method")

		t.Logf("[Mock Server] Received request: Method=%s, APIKey=%s, Sig=%s.", method, apiKey, apiSig)
		require.Equal(t, "test_key", apiKey, "[Mock Server] API key in request must match 'test_key'.")
		require.NotEmpty(t, apiSig, "[Mock Server] API signature must be present in request.")

		// Check the method and respond accordingly.
		w.Header().Set("Content-Type", "application/json") // Set content type for all responses.
		switch method {
		case "rtm.test.echo":
			t.Logf("[Mock Server] Responding to echo request.")
			responseMap := map[string]interface{}{
				"rsp": map[string]interface{}{
					"stat": "ok",
				},
			}
			for k, v := range r.Form {
				if len(v) > 0 && k != "stat" {
					responseMap["rsp"].(map[string]interface{})[k] = v[0]
				}
			}
			jsonBytes, marshalErr := json.Marshal(responseMap)
			require.NoError(t, marshalErr, "[Mock Server] Failed to marshal echo response.")
			_, writeErr := w.Write(jsonBytes)
			if writeErr != nil {
				t.Logf("[Mock Server] Failed to write echo response: %v.", writeErr)
			}

		case "rtm.test.error":
			t.Logf("[Mock Server] Responding with a simulated API error (stat: fail).")
			// Note: Returning 200 OK but with RTM error status.
			_, writeErr := w.Write([]byte(`{"rsp":{"stat":"fail","err":{"code":"112","msg":"Method not found"}}}`))
			if writeErr != nil {
				t.Logf("[Mock Server] Failed to write stat:fail error response: %v.", writeErr)
			}

		case methodCheckToken: // Use constant.
			t.Logf("[Mock Server] Responding to checkToken request with success.")
			// Use fmt.Fprintf for efficient formatted writing.
			_, writeErr := fmt.Fprintf(w, `{
				"rsp": {
					"stat": "ok",
					"auth": {
						"token": "%s",
						"perms": "delete",
						"user": {
							"id": "user123",
							"username": "testuser",
							"fullname": "Test User"
						}
					}
				}
			}`, r.Form.Get("auth_token")) // Echo back the token for verification.
			if writeErr != nil {
				t.Logf("[Mock Server] Failed to write checkToken response: %v.", writeErr)
			}

		default: // Handles unknown methods like rtm.cows.moo
			t.Logf("[Mock Server] Responding with HTTP 400 for unknown method '%s'.", method)
			// Unknown method should ideally get an HTTP error status from a real API gateway,
			// or potentially a 200 OK with stat:fail if the endpoint itself is valid but method isn't.
			// We'll simulate the HTTP 400 here.
			w.WriteHeader(http.StatusBadRequest) // Return 400 Bad Request.
			_, writeErr := w.Write([]byte(`{"rsp":{"stat":"fail","err":{"code":"112","msg":"Method not found"}}}`))
			if writeErr != nil {
				t.Logf("[Mock Server] Failed to write default HTTP 400 error response: %v.", writeErr)
			}
		}
	}))
	defer server.Close()
	t.Logf("Mock RTM server started at URL: %s.", server.URL)

	// --- Test Client Setup ---
	logger := logging.GetNoopLogger()
	client := &Client{
		config: Config{
			APIKey:       "test_key",
			SharedSecret: "test_secret",
			APIEndpoint:  server.URL, // Point to mock server.
			HTTPClient:   &http.Client{Timeout: 5 * time.Second},
			AuthToken:    "test_token_123", // Add a token for relevant tests.
		},
		logger: logger,
	}

	// --- Test Cases ---
	// Subtest: Echo/Success/ReturnsOK
	t.Run("Echo_Success_ReturnsOK", func(t *testing.T) {
		ctx := context.Background()
		params := map[string]string{"test_param": "test_value", "another_param": "value2"}
		t.Logf("Calling 'rtm.test.echo' with params: %v.", params)

		result, err := client.CallMethod(ctx, "rtm.test.echo", params)

		require.NoError(t, err, "CallMethod for 'rtm.test.echo' should succeed.")
		require.NotNil(t, result, "Result from 'rtm.test.echo' should not be nil.")
		t.Logf("Received successful response for echo: %s.", string(result))

		// Unmarshal and check specific fields for robustness against formatting changes.
		var respData map[string]map[string]interface{}
		err = json.Unmarshal(result, &respData)
		require.NoError(t, err, "Failed to unmarshal successful echo response JSON.")

		rsp, ok := respData["rsp"]
		require.True(t, ok, "Response JSON must contain 'rsp' field.")

		assert.Equal(t, "ok", rsp["stat"], "Response status should be 'ok'.")
		assert.Equal(t, "test_value", rsp["test_param"], "Response should echo back 'test_param'.")
		assert.Equal(t, "value2", rsp["another_param"], "Response should echo back 'another_param'.")
		assert.Equal(t, "rtm.test.echo", rsp["method"], "Response should echo back the method.")
		t.Logf("Echo test passed. The server heard our moo.")
	})

	// Subtest: APIErrorResponse/ReturnsError
	t.Run("APIErrorResponse_ReturnsError", func(t *testing.T) {
		ctx := context.Background()
		params := map[string]string{}
		t.Logf("Calling 'rtm.test.error', expecting an RTM API level error (stat: fail).")

		result, err := client.CallMethod(ctx, "rtm.test.error", params)

		require.Error(t, err, "CallMethod for 'rtm.test.error' should return an error.")
		assert.Nil(t, result, "Result should be nil when an RTM API error occurs.")
		t.Logf("Received expected error: %v.", err)

		// Check the error message reflects the RTM error.
		assert.Contains(t, err.Error(), "RTM API Error:", "Error message should indicate an RTM API Error.")
		assert.Contains(t, err.Error(), "Method not found", "Error message should contain the RTM error message 'Method not found'.")
		// Check for RTM error code if needed (requires error type assertion).
		t.Logf("API error test passed. Correctly handled the server saying 'nope' (stat: fail).")
	})

	// Subtest: AuthCheck/Success/ReturnsOK
	t.Run("AuthCheck_Success_ReturnsOK", func(t *testing.T) {
		ctx := context.Background()
		params := map[string]string{}
		t.Logf("Calling '%s' to verify authentication token.", methodCheckToken)

		result, err := client.CallMethod(ctx, methodCheckToken, params)

		require.NoError(t, err, "CallMethod for '%s' should succeed.", methodCheckToken)
		require.NotNil(t, result, "Result from '%s' should not be nil.", methodCheckToken)
		t.Logf("Received successful response for auth check: %s.", string(result))

		// Unmarshal and check specific fields.
		var respData checkTokenRsp // Use the specific struct type.
		err = json.Unmarshal(result, &respData)
		require.NoError(t, err, "Failed to unmarshal successful auth check response JSON.")

		assert.Equal(t, "ok", respData.Rsp.Stat, "Auth check response status should be 'ok'.")
		require.NotNil(t, respData.Rsp.Auth.User, "Auth check response should contain user info.")
		assert.Equal(t, "testuser", respData.Rsp.Auth.User.Username, "Auth check response should contain the username.")
		assert.Equal(t, "test_token_123", respData.Rsp.Auth.Token, "Auth check response should echo back the token used.")
		t.Logf("Auth check test passed. We're authenticated in this pasture.")
	})

	// Subtest: UnknownMethod/HTTPError/ReturnsError
	t.Run("UnknownMethod_HTTPError_ReturnsError", func(t *testing.T) {
		ctx := context.Background()
		params := map[string]string{}
		unknownMethod := "rtm.cows.moo"
		t.Logf("Calling unknown method '%s', expecting HTTP 400 error.", unknownMethod)

		result, err := client.CallMethod(ctx, unknownMethod, params)

		require.Error(t, err, "CallMethod for unknown method '%s' should return an error.", unknownMethod)
		assert.Nil(t, result, "Result should be nil for unknown method resulting in HTTP error.")
		t.Logf("Received expected error for unknown method: %v.", err)

		// Verify the error message indicates an HTTP status error, not just the RTM error message.
		assert.Contains(t, err.Error(), "API returned non-200 status: 400", "Error message should indicate HTTP 400 Bad Request.")
		// Do NOT assert contains "Method not found" here, as that's in the body which callMethod doesn't parse on HTTP error.
		t.Logf("Unknown method test passed. Correctly handled HTTP 400 error.")
	})
}
