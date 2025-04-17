// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/client_test.go

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// file: internal/rtm/client_test.go

func TestClient_GenerateSignature(t *testing.T) {
	// Setup logger once.
	logger := logging.GetNoopLogger()

	// Test cases including the secret for each case.
	testCases := []struct {
		name     string
		secret   string // Added secret field for each test case.
		params   map[string]string
		expected string
	}{
		{
			name:   "Simple params",
			secret: "test_secret", // Use the original secret.
			params: map[string]string{
				"method":  "rtm.test.echo",
				"api_key": "abc123",
				"format":  "json",
			},
			// Corrected expected signature calculated according to RTM spec:
			// MD5("test_secret" + "api_key" + "abc123" + "format" + "json" + "method" + "rtm.test.echo")
			expected: "ce7eb5843f9dcb6209227c72baf957bc",
		},
		{
			name:   "With auth token",
			secret: "test_secret", // Use the original secret.
			params: map[string]string{
				"method":     "rtm.lists.getList",
				"api_key":    "abc123",
				"format":     "json",
				"auth_token": "token123",
			},
			// The expected value here may also need to be updated if this test is failing
			expected: "fa17f481daca02dca3286483755718a0",
		},
		{
			name:   "RTM Example (BANANAS)",
			secret: "BANANAS", // Use the secret from the example.
			params: map[string]string{
				"abc": "baz",
				"feg": "bar",
				"yxz": "foo",
			},
			// Correct expected signature from RTM documentation:
			// MD5("BANANAS" + "abc" + "baz" + "feg" + "bar" + "yxz" + "foo")
			expected: "82044aae4dd676094f23f1ec152159ba",
		},
		{
			name:   "Custom Case - Empty Params",
			secret: "another_secret", // Use a different secret.
			params: map[string]string{},
			// Corrected expected signature calculated according to RTM spec:
			// MD5("another_secret")
			expected: "bb4a87f07bd27e737e0b4a44cfee12f3",
		},
	}

	// Run tests.
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a client instance specifically for this test case with its secret.
			client := &Client{
				config: Config{
					SharedSecret: tc.secret, // Use the secret defined in the test case.
				},
				logger: logger,
			}

			// Call the method.
			signature := client.generateSignature(tc.params)

			// Verify result.
			assert.Equal(t, tc.expected, signature, "Signature should match expected value")
		})
	}
}
func TestClient_PrepareParameters(t *testing.T) {
	// Setup.
	logger := logging.GetNoopLogger()
	client := &Client{
		config: Config{
			APIKey:       "test_key",
			SharedSecret: "test_secret",
			AuthToken:    "test_token",
		},
		logger: logger,
	}

	// Test cases.
	testCases := []struct {
		name            string
		method          string
		params          map[string]string
		expectAuthToken bool
	}{
		{
			name:            "Regular method",
			method:          "rtm.test.login",
			params:          map[string]string{},
			expectAuthToken: true,
		},
		{
			name:            "Auth method - getFrob",
			method:          "rtm.auth.getFrob",
			params:          map[string]string{},
			expectAuthToken: false,
		},
		{
			name:            "Auth method - getToken",
			method:          "rtm.auth.getToken",
			params:          map[string]string{"frob": "test_frob"},
			expectAuthToken: false,
		},
		{
			name:   "With existing params",
			method: "rtm.tasks.getList",
			params: map[string]string{
				"filter": "status:incomplete",
			},
			expectAuthToken: true,
		},
	}

	// Run tests.
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the method.
			fullParams := client.prepareParameters(tc.method, tc.params)

			// Verify common parameters.
			assert.Equal(t, tc.method, fullParams["method"], "Method should be set correctly")
			assert.Equal(t, "test_key", fullParams["api_key"], "API key should be set correctly")
			assert.Equal(t, "json", fullParams["format"], "Format should be set correctly")

			// Verify auth token.
			if tc.expectAuthToken {
				assert.Equal(t, "test_token", fullParams["auth_token"], "Auth token should be set for most methods")
			} else {
				_, hasAuthToken := fullParams["auth_token"]
				assert.False(t, hasAuthToken, "Auth token should not be set for auth methods")
			}

			// Verify signature.
			assert.NotEmpty(t, fullParams["api_sig"], "Signature should be set")

			// Verify original params are preserved.
			for k, v := range tc.params {
				assert.Equal(t, v, fullParams[k], "Original parameters should be preserved")
			}
		})
	}
}

func TestClient_CallMethod(t *testing.T) {
	// Create a test server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request.
		err := r.ParseForm()
		// Use require inside tests for fatal errors
		require.NoError(t, err, "Failed to parse form data")

		// Verify basic parameters.
		assert.Equal(t, "test_key", r.Form.Get("api_key"), "API key should be set")
		assert.NotEmpty(t, r.Form.Get("api_sig"), "Signature should be set")

		// Check the method and respond accordingly.
		switch r.Form.Get("method") {
		case "rtm.test.echo":
			// Construct the response JSON including *all* parameters.
			responseMap := map[string]interface{}{
				"rsp": map[string]interface{}{
					"stat": "ok",
				},
			}
			// Add all form parameters to the response under rsp.
			for k, v := range r.Form {
				if len(v) > 0 {
					// Ensure we don't overwrite the stat field.
					if k != "stat" {
						responseMap["rsp"].(map[string]interface{})[k] = v[0]
					}
				}
			}
			w.Header().Set("Content-Type", "application/json")
			jsonBytes, marshalErr := json.Marshal(responseMap)
			require.NoError(t, marshalErr, "Failed to marshal echo response")
			// Corrected: Check w.Write error
			if _, writeErr := w.Write(jsonBytes); writeErr != nil {
				t.Logf("httptest server failed to write echo response: %v", writeErr)
			}

		case "rtm.test.error":
			// Return an error response.
			w.Header().Set("Content-Type", "application/json")
			// Corrected: Check w.Write error
			if _, writeErr := w.Write([]byte(`{"rsp":{"stat":"fail","err":{"code":"112","msg":"Method not found"}}}`)); writeErr != nil {
				t.Logf("httptest server failed to write error response: %v", writeErr)
			}

		case "rtm.auth.checkToken":
			// Return a valid auth check response.
			w.Header().Set("Content-Type", "application/json")
			// Corrected: Check w.Write error
			if _, writeErr := w.Write([]byte(`{
				"rsp": {
					"stat": "ok",
					"auth": {
						"token": "token123",
						"user": {
							"id": "user123",
							"username": "testuser",
							"fullname": "Test User"
						}
					}
				}
			}`)); writeErr != nil {
				t.Logf("httptest server failed to write checkToken response: %v", writeErr)
			}

		default:
			// Unknown method.
			w.WriteHeader(http.StatusBadRequest)
			// Corrected: Check w.Write error
			if _, writeErr := w.Write([]byte(`{"rsp":{"stat":"fail","err":{"code":"112","msg":"Method not found"}}}`)); writeErr != nil {
				t.Logf("httptest server failed to write default error response: %v", writeErr)
			}
		}
	}))
	defer server.Close()

	// Create client with test server URL.
	logger := logging.GetNoopLogger()
	client := &Client{
		config: Config{
			APIKey:       "test_key",
			SharedSecret: "test_secret",
			APIEndpoint:  server.URL,
			HTTPClient:   &http.Client{Timeout: 5 * time.Second},
		},
		logger: logger,
	}

	// Test cases.
	t.Run("Success - Echo", func(t *testing.T) {
		ctx := context.Background()
		params := map[string]string{"test_param": "test_value", "another_param": "value2"} // Add more params.

		// Call the method.
		result, err := client.CallMethod(ctx, "rtm.test.echo", params)

		// Verify.
		require.NoError(t, err, "CallMethod should not return error on success")
		require.NotNil(t, result, "Result should not be nil")
		// Check specifically for the echoed params within the "rsp" object.
		assert.Contains(t, string(result), `"test_param":"test_value"`, "Result should contain echoed parameter")
		assert.Contains(t, string(result), `"another_param":"value2"`, "Result should contain another echoed parameter")
		// Ensure other standard params are also present if needed for assertion.
		assert.Contains(t, string(result), `"method":"rtm.test.echo"`, "Result should contain method")
	})
	t.Run("Error - API Error", func(t *testing.T) {
		ctx := context.Background()
		params := map[string]string{}

		// Call the method.
		result, err := client.CallMethod(ctx, "rtm.test.error", params)

		// Verify.
		require.Error(t, err, "CallMethod should return error for API error")
		assert.Contains(t, err.Error(), "Method not found", "Error should contain API error message")
		assert.Nil(t, result, "Result should be nil for error")
	})

	t.Run("Success - Auth Check", func(t *testing.T) {
		ctx := context.Background()
		params := map[string]string{}

		// Call the method.
		result, err := client.CallMethod(ctx, "rtm.auth.checkToken", params)

		// Verify.
		require.NoError(t, err, "CallMethod should not return error for successful auth check")
		require.NotNil(t, result, "Result should not be nil")
		assert.Contains(t, string(result), "testuser", "Result should contain username")
		assert.Contains(t, string(result), "token123", "Result should contain token")
	})
}
