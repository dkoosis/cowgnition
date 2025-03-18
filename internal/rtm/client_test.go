package rtm

import (
	"encoding/xml"
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
	requiredParams := string{
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
		_, err := w.Write(byte(response)) // Check for write errors
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

func TestDo(t *testing.T) {
	// Test successful API call
	t.Run("Success", func(t *testing.T) {
		mockResp := `<rsp stat="ok"><echo>test</echo></rsp>`
		server := setupMockServer(t, "rtm.test.echo", mockResp)
		defer server.Close()

		client := NewClient("api_key_123", "shared_secret_abc")
		client.baseURL = server.URL

		params := url.Values{}
		params.Set("method", "rtm.test.echo")

		var result struct {
			Echo string `xml:"echo"`
		}
		_, err := client.Do(params, &result)

		if err != nil {
			t.Errorf("Do() returned unexpected error: %v", err)
		}

		if result.Echo != "test" {
			t.Errorf("Do() result.Echo = %v, want %v", result.Echo, "test")
		}
	})

	// Test API error
	t.Run("APIError", func(t *testing.T) {
		mockResp := `<rsp stat="fail"><err code="123" msg="Test error"/></rsp>`
		server := setupMockServer(t, "rtm.test.echo", mockResp)
		defer server.Close()

		client := NewClient("api_key_123", "shared_secret_abc")
		client.baseURL = server.URL

		params := url.Values{}
		params.Set("method", "rtm.test.echo")

		_, err := client.Do(params, nil)

		if err == nil {
			t.Error("Do() should return an error for API fail response")
		}

		apiErr, ok := err.(APIError)
		if !ok {
			t.Errorf("Do() error type = %T, want APIError", err)
		}

		if apiErr.Code != 123 {
			t.Errorf("Do() APIError.Code = %v, want %v", apiErr.Code, 123)
		}

		if apiErr.Message != "Test error" {
			t.Errorf("Do() APIError.Message = %v, want %v", apiErr.Message, "Test error")
		}
	})

	// Test HTTP error
	t.Run("HTTPError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := NewClient("api_key_123", "shared_secret_abc")
		client.baseURL = server.URL

		params := url.Values{}
		params.Set("method", "rtm.test.echo")

		_, err := client.Do(params, nil)

		if err == nil {
			t.Error("Do() should return an error for HTTP 500 response")
		}

		// Check if the error message contains the HTTP status
		if !strings.Contains(err.Error(), "HTTP status: 500") {
			t.Errorf("Do() error message should contain HTTP status, got: %v", err)
		}
	})

	// Test XML unmarshalling error
	t.Run("UnmarshalError", func(t *testing.T) {
		mockResp := `<rsp stat="ok"><invalid-xml></rsp>` // Invalid XML
		server := setupMockServer(t, "rtm.test.echo", mockResp)
		defer server.Close()

		client := NewClient("api_key_123", "shared_secret_abc")
		client.baseURL = server.URL

		params := url.Values{}
		params.Set("method", "rtm.test.echo")

		var result struct {
			Echo string `xml:"echo"`
		}
		_, err := client.Do(params, &result)

		if err == nil {
			t.Error("Do() should return an error for invalid XML response")
		}

		if _, ok := err.(*xml.SyntaxError); !ok {
			t.Errorf("Do() error type = %T, want xml.SyntaxError", err)
		}
	})

	// Test successful API call with POST
	t.Run("SuccessPOST", func(t *testing.T) {
		mockResp := `<rsp stat="ok"><echo>test</echo></rsp>`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("Expected POST request, got %s", r.Method)
				return
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(byte(mockResp))
			if err != nil {
				t.Fatalf("Error writing response: %v", err)
			}
		}))
		defer server.Close()

		client := NewClient("api_key_123", "shared_secret_abc")
		client.baseURL = server.URL
		client.usePOST = true // Enable POST

		params := url.Values{}
		params.Set("method", "rtm.test.echo")

		var result struct {
			Echo string `xml:"echo"`
		}
		_, err := client.Do(params, &result)

		if err != nil {
			t.Errorf("Do() returned unexpected error: %v", err)
		}

		if result.Echo != "test" {
			t.Errorf("Do() result.Echo = %v, want %v", result.Echo, "test")
		}
	})

	// Test successful file upload
	t.Run("FileUploadSuccess", func(t *testing.T) {
		mockResp := `<rsp stat="ok"><photoid>12345</photoid></rsp>`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("Expected POST request, got %s", r.Method)
				return
			}

			// Check Content-Type for multipart/form-data
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				t.Errorf("Expected multipart/form-data Content-Type, got %s", r.Header.Get("Content-Type"))
				return
			}

			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(byte(mockResp))
			if err != nil {
				t.Fatalf("Error writing response: %v", err)
			}
		}))
		defer server.Close()

		client := NewClient("api_key_123", "shared_secret_abc")
		client.baseURL = server.URL
		client.usePOST = true // Ensure POST is used

		params := url.Values{}
		params.Set("method", "rtm.photos.upload")

		fileContent := "test file content"
		fileName := "test.txt"

		file := strings.NewReader(fileContent)

		result, err := client.Upload(params, "photo", fileName, file)

		if err != nil {
			t.Errorf("Upload() returned unexpected error: %v", err)
		}

		photoID, ok := result["photoid"].(string)
		if !ok || photoID != "12345" {
			t.Errorf("Upload() result[photoid] = %v, want %v", result["photoid"], "12345")
		}
	})

	// Test file upload with API error
	t.Run("FileUploadAPIError", func(t *testing.T) {
		mockResp := `<rsp stat="fail"><err code="123" msg="Upload failed"/></rsp>`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(byte(mockResp))
			if err != nil {
				t.Fatalf("Error writing response: %v", err)
			}
		}))
		defer server.Close()

		client := NewClient("api_key_123", "shared_secret_abc")
		client.baseURL = server.URL
		client.usePOST = true

		params := url.Values{}
		params.Set("method", "rtm.photos.upload")

		fileContent := "test file content"
		fileName := "test.txt"
		file := strings.NewReader(fileContent)

		_, err := client.Upload(params, "photo", fileName, file)

		if err == nil {
			t.Error("Upload() should return an error for API fail response")
		}

		apiErr, ok := err.(APIError)
		if !ok {
			t.Errorf("Upload() error type = %T, want APIError", err)
		}

		if apiErr.Code != 123 {
			t.Errorf("Upload() APIError.Code = %v, want %v", apiErr.Code, 123)
		}

		if apiErr.Message != "Upload failed" {
			t.Errorf("Upload() APIError.Message = %v, want %v", apiErr.Message, "Upload failed")
		}
	})

	// Test file upload with HTTP error
	t.Run("FileUploadHTTPError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := NewClient("api_key_123", "shared_secret_abc")
		client.baseURL = server.URL
		client.usePOST = true

		params := url.Values{}
		params.Set("method", "rtm.photos.upload")

		fileContent := "test file content"
		fileName := "test.txt"
		file := strings.NewReader(fileContent)

		_, err := client.Upload(params, "photo", fileName, file)

		if err == nil {
			t.Error("Upload() should return an error for HTTP 500 response")
		}

		if !strings.Contains(err.Error(), "HTTP status: 500") {
			t.Errorf("Upload() error message should contain HTTP status, got: %v", err)
		}
	})

	// Test file upload with invalid XML response
	t.Run("FileUploadUnmarshalError", func(t *testing.T) {
		mockResp := `<rsp stat="ok"><invalid-xml></rsp>` // Invalid XML
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(byte(mockResp))
			if err != nil {
				t.Fatalf("Error writing response: %v", err)
			}
		}))
		defer server.Close()

		client := NewClient("api_key_123", "shared_secret_abc")
		client.baseURL = server.URL
		client.usePOST = true

		params := url.Values{}
		params.Set("method", "rtm.photos.upload")

		fileContent := "test file content"
		fileName := "test.txt"
		file := strings.NewReader(fileContent)

		_, err := client.Upload(params, "photo", fileName, file)

		if err == nil {
			t.Error("Upload() should return an error for invalid XML response")
		}

		if _, ok := err.(*xml.SyntaxError); !ok {
			t.Errorf("Upload() error type = %T, want xml.SyntaxError", err)
		}
	})
}
