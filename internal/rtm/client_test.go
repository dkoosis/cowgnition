package rtm

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGenerateSignature(t *testing.T) {
	client := NewClient("api_key_123", "shared_secret_abc")

	// Test case 1: Basic parameters
	params := url.Values{}
	params.Set("method", "rtm.test.echo")
	params.Set("api_key", "api_key_123")
	params.Set("name", "value")

	expected := "8a31ec665d5ef04129be58635a7543c1" // Updated expected hash
	actual := client.generateSignature(params)

	if actual != expected {
		t.Errorf("generateSignature() = %v, want %v", actual, expected)
	}

	// Test case 2: Different order of parameters should yield same signature
	params = url.Values{}
	params.Set("name", "value")
	params.Set("api_key", "api_key_123")
	params.Set("method", "rtm.test.echo")

	actual = client.generateSignature(params)

	if actual != expected {
		t.Errorf("generateSignature() with reordered params = %v, want %v", actual, expected)
	}
}

func TestGetAuthURL(t *testing.T) {
	client := NewClient("api_key_123", "shared_secret_abc")

	url := client.GetAuthURL("test_frob", "delete")

	// Check that URL contains the expected parts
	if url == "" {
		t.Error("GetAuthURL() returned empty string")
	}

	if !strings.HasPrefix(url, authURL) {
		t.Errorf("GetAuthURL() should start with %s, got %s", authURL, url)
	}

	// Check that params are included
	requiredParams := []string{
		"api_key=api_key_123",
		"perms=delete",
		"frob=test_frob",
		"api_sig=",
	}

	for _, param := range requiredParams {
		if !strings.Contains(url, param) {
			t.Errorf("GetAuthURL() should contain %s, got %s", param, url)
		}
	}
}

// Mock RTM API response for testing.
func setupMockServer(t *testing.T, expectedMethod string, response string) *httptest.Server {
	t.Helper() // Added this line to fix the linting issue

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET request, got %s", r.Method)
			return // Add return to stop processing on error
		}

		query := r.URL.Query()
		method := query.Get("method")
		if method != expectedMethod {
			t.Errorf("Expected method %s, got %s", expectedMethod, method)
			return // Add return to stop processing on error
		}

		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(response)) // Check for write errors
		if err != nil {
			t.Fatalf("Error writing response: %v", err)
		}
	}))
}

func TestGetFrob(t *testing.T) {
	// Setup mock server
	mockResp := `<rsp stat="ok">
        <frob>test_frob_123</frob>
    </rsp>`
	server := setupMockServer(t, "rtm.auth.getFrob", mockResp)
	defer server.Close()

	// Create client with baseURL pointing to mock server
	// Pass server.URL directly to the client.
	client := NewClient("api_key_123", "shared_secret_abc")
	client.baseURL = server.URL // Directly set the client's baseURL

	// Test GetFrob
	frob, err := client.GetFrob()

	if err != nil {
		t.Errorf("GetFrob() returned unexpected error: %v", err)
	}

	if frob != "test_frob_123" {
		t.Errorf("GetFrob() = %v, want %v", frob, "test_frob_123")
	}
}

func TestGetToken(t *testing.T) {
	// Setup mock server
	mockResp := `<rsp stat="ok">
        <auth>
            <token>test_token_abc</token>
            <perms>delete</perms>
            <user id="123" username="test_user" fullname="Test User" />
        </auth>
    </rsp>`
	server := setupMockServer(t, "rtm.auth.getToken", mockResp)
	defer server.Close()

	// Create client, passing the mock server URL directly
	client := NewClient("api_key_123", "shared_secret_abc")
	client.baseURL = server.URL

	// Test GetToken
	token, err := client.GetToken("test_frob_123")

	if err != nil {
		t.Errorf("GetToken() returned unexpected error: %v", err)
	}

	if token != "test_token_abc" {
		t.Errorf("GetToken() = %v, want %v", token, "test_token_abc")
	}

	// Check that token was saved in client
	if client.authToken != "test_token_abc" {
		t.Errorf("GetToken() should set client.authToken to %v, got %v", "test_token_abc", client.authToken)
	}
}

func TestCheckToken(t *testing.T) {
	// Setup mock server with valid response
	mockResp := `<rsp stat="ok">
        <auth>
            <token>test_token_abc</token>
            <perms>delete</perms>
            <user id="123" username="test_user" fullname="Test User" />
        </auth>
    </rsp>`
	server := setupMockServer(t, "rtm.auth.checkToken", mockResp)
	defer server.Close()

	// Create client, passing the mock server URL directly
	client := NewClient("api_key_123", "shared_secret_abc")
	client.SetAuthToken("test_token_abc")
	client.baseURL = server.URL // Set the baseURL

	// Test CheckToken
	valid, err := client.CheckToken()

	if err != nil {
		t.Errorf("CheckToken() returned unexpected error: %v", err)
	}

	if !valid {
		t.Errorf("CheckToken() = %v, want %v", valid, true)
	}

	// Setup mock server with error response
	mockRespErr := `<rsp stat="fail">
        <err code="98" msg="Login failed / Invalid auth token" />
    </rsp>`
	serverErr := setupMockServer(t, "rtm.auth.checkToken", mockRespErr)
	defer serverErr.Close()

	// Create a *new* client for the error case.  This is important
	// to avoid state leaking between tests.
	clientErr := NewClient("api_key_123", "shared_secret_abc")
	clientErr.SetAuthToken("test_token_abc") // Use a consistent token
	clientErr.baseURL = serverErr.URL        // Set baseURL to the error server

	// Test CheckToken with invalid token
	valid, _ = clientErr.CheckToken() // Use blank identifier to ignore the error

	// We expect valid to be false, but don't necessarily expect an error
	// since the API might just return a "fail" status
	if valid {
		t.Errorf("CheckToken() with invalid token = %v, want %v", valid, false)
	}
}

func TestResponseGetError(t *testing.T) {
	// Test with error in response
	resp := Response{
		Status: statusFail,
		Error: &struct {
			Code    string `xml:"code,attr"`
			Message string `xml:"msg,attr"`
		}{
			Code:    "123",
			Message: "Test error",
		},
	}

	code, msg := resp.GetError()

	if code != "123" {
		t.Errorf("Response.GetError() code = %v, want %v", code, "123")
	}

	if msg != "Test error" {
		t.Errorf("Response.GetError() message = %v, want %v", msg, "Test error")
	}

	// Test with no error
	resp = Response{
		Status: statusOK,
		Error:  nil,
	}

	code, msg = resp.GetError()

	if code != "" {
		t.Errorf("Response.GetError() with no error code = %v, want %v", code, "")
	}

	if msg != "" {
		t.Errorf("Response.GetError() with no error message = %v, want %v", msg, "")
	}
}
