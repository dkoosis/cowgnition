// Package mocks provides mock implementations for external services.
// file: test/mocks/server.go
package mocks

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// RTMServer represents a mock RTM API server for testing.
// It simulates the behavior of the RTM API, allowing tests to run without relying on an external service.
type RTMServer struct {
	Server     *httptest.Server
	BaseURL    string
	Requests   RequestRecord
	Responses  map[string]string // Responses maps RTM API methods to their mock responses.
	StatusCode int               // StatusCode is the HTTP status code that the server will return.
	mu         sync.Mutex        // mu is a mutex to protect concurrent access to the server's state.
	t          *testing.T
}

// RequestRecord stores information about a received request.
// This is used to inspect the requests that the mock server received during testing.
type RequestRecord struct {
	Method string
	Path   string
	Query  string
	Body   string
}

// NewRTMServer creates a new mock RTM server for testing.
// It initializes an RTMServer instance, sets up an HTTP test server, and loads default mock responses.
//
// t *testing.T: The testing.T instance for the current test.
//
// Returns:
//
//	*RTMServer: A pointer to the newly created RTMServer instance.
func NewRTMServer(t *testing.T) *RTMServer {
	t.Helper()
	server := &RTMServer{
		Requests:   make(RequestRecord, 0),
		Responses:  make(map[string]string),
		StatusCode: http.StatusOK,
		t:          t,
	}

	// Create HTTP test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.handleRequest(w, r)
	}))

	server.Server = ts
	server.BaseURL = ts.URL

	// Load default mock responses
	server.loadDefaultResponses()

	return server
}

// Close closes the mock server.
// It shuts down the HTTP test server, releasing its resources.
func (s *RTMServer) Close() {
	s.Server.Close()
}

// loadDefaultResponses loads default responses for common RTM API methods.
// These responses are used if no custom response has been added for a method.
func (s *RTMServer) loadDefaultResponses() {
	// Default successful responses for commonly used methods
	s.AddResponse("rtm.auth.getFrob", `<rsp stat="ok"><frob>test_frob_12345</frob></rsp>`)
	s.AddResponse("rtm.auth.getToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)
	s.AddResponse("rtm.auth.checkToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)
	s.AddResponse("rtm.timelines.create", `<rsp stat="ok"><timeline>12345</timeline></rsp>`)
	s.AddResponse("rtm.lists.getList", `<rsp stat="ok"><lists><list id="1" name="Inbox" deleted="0" locked="1" archived="0" position="-1" smart="0" /><list id="2" name="Work" deleted="0" locked="0" archived="0" position="0" smart="0" /></lists></rsp>`)
	s.AddResponse("rtm.tasks.getList", `<rsp stat="ok"><tasks><list id="1"><taskseries id="1" created="2025-03-15T12:00:00Z" modified="2025-03-15T12:00:00Z" name="Test Task" source="api"><tags /><participants /><notes /><task id="1" due="" has_due_time="0" added="2025-03-15T12:00:00Z" completed="" deleted="" priority="N" postponed="0" estimate="" /></taskseries></list></tasks></rsp>`)
}

// AddResponse adds or updates a custom response for an RTM API method.
// This allows tests to define specific responses for different RTM API calls.
//
// method string: The RTM API method name (e.g., "rtm.tasks.add").
// response string: The XML response string to be returned by the mock server.
func (s *RTMServer) AddResponse(method, response string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Responses[method] = response
}

// LoadResponseFromFile loads a response from a file for an RTM API method.
// This is useful for managing complex XML responses in external files.
//
// method string: The RTM API method name.
// filePath string: The path to the file containing the XML response.
//
// Returns:
//
//	error: An error if the file cannot be read, otherwise nil.
func (s *RTMServer) LoadResponseFromFile(method, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading response file: %w", err)
	}

	s.AddResponse(method, string(data))
	return nil
}

// LoadResponsesFromDir loads all XML responses from a directory.
// Files should be named after the method they represent (e.g., rtm.auth.getFrob.xml).
// This enables loading a suite of mock responses from a directory.
//
// dir string: The path to the directory containing the XML response files.
//
// Returns:
//
//	error: An error if there are issues reading the directory or files, otherwise nil.
func (s *RTMServer) LoadResponsesFromDir(dir string) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("error reading directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".xml") {
			continue
		}

		filePath := filepath.Join(dir, file.Name())
		method := strings.TrimSuffix(file.Name(), ".xml")

		if err := s.LoadResponseFromFile(method, filePath); err != nil {
			return err
		}
	}

	return nil
}

// SetErrorResponse sets an error response for all RTM API methods.
// This is useful for simulating error conditions in the RTM API.
//
// code string: The error code to be returned in the response.
// message string: The error message to be returned in the response.
func (s *RTMServer) SetErrorResponse(code string, message string) {
	errorResp := fmt.Sprintf(`<rsp stat="fail"><err code="%s" msg="%s" /></rsp>`, code, message)
	s.AddResponse("*", errorResp)
}

// SetStatusCode sets the HTTP status code for all responses.
// This allows tests to simulate different HTTP status codes from the RTM API.
//
// code int: The HTTP status code to be returned by the mock server.
func (s *RTMServer) SetStatusCode(code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.StatusCode = code
}

// GetRequests returns all recorded requests.
// This allows tests to inspect the requests received by the mock server.
//
// Returns:
//
//	RequestRecord: A slice containing all recorded RequestRecord instances.
func (s *RTMServer) GetRequests() RequestRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append(RequestRecord{}, s.Requests...)
}

// ResetRequests clears all recorded requests.
// This is useful for resetting the request log between test cases.
func (s *RTMServer) ResetRequests() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Requests = make(RequestRecord, 0)
}

// GetRequestsForMethod returns all recorded requests for a specific method.
// This filters the recorded requests to only those matching a given RTM API method.
//
// method string: The RTM API method name to filter by.
//
// Returns:
//
//	RequestRecord: A slice of RequestRecord instances that match the specified method.
func (s *RTMServer) GetRequestsForMethod(method string) RequestRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []RequestRecord
	for _, req := range s.Requests {
		if strings.Contains(req.Query, "method="+method) {
			result = append(result, req)
		}
	}
	return result
}

// handleRequest handles incoming requests to the mock server.
// It records the request details and returns a pre-configured response.
//
// w http.ResponseWriter: The http.ResponseWriter used to send the response.
// r *http.Request: The http.Request representing the incoming request.
func (s *RTMServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Read and record the request
	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body.Close()
	body := string(bodyBytes)

	s.mu.Lock()
	s.Requests = append(s.Requests, RequestRecord{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		Body:   body,
	})
	s.mu.Unlock()

	// Parse the method from the query parameters
	method := r.URL.Query().Get("method")
	if method == "" {
		// If no method is specified, return an error
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write(byte(`<rsp stat="fail"><err code="1" msg="Method not specified" /></rsp>`)); err != nil {
			log.Printf("Error writing response: %v", err)
		}
		return
	}

	// Get the response for the method
	s.mu.Lock()
	response, ok := s.Responses[method]
	if !ok {
		// If no response is defined for this method, check for a wildcard response
		response, ok = s.Responses["*"]
	}
	statusCode := s.StatusCode
	s.mu.Unlock()

	if !ok {
		// If still no response, return a default error
		w.WriteHeader(http.StatusNotImplemented)
		if _, err := w.Write(byte(fmt.Sprintf(`<rsp stat="fail"><err code="1" msg="No mock response defined for method %s" /></rsp>`, method))); err != nil {
			log.Printf("Error writing response: %v", err)
		}
		return
	}

	// Set content type and status code
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(statusCode)

	// Write the response
	if _, err := w.Write(byte(response)); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// ValidateRequest checks if a request has been made with specific characteristics.
// It allows tests to assert that requests with certain properties were received by the mock server.
//
// t *testing.T: The testing.T instance for the current test.
// method string: The RTM API method name to check for.
// validateFn func(RequestRecord) bool: A function that takes a RequestRecord and returns true if it matches the desired criteria.
//
// Returns:
//
//	bool: True if a matching request is found, false otherwise.
func (s *RTMServer) ValidateRequest(t *testing.T, method string, validateFn func(RequestRecord) bool) bool {
	t.Helper()
	requests := s.GetRequestsForMethod(method)
	if len(requests) == 0 {
		t.Errorf("No requests recorded for method %s", method)
		return false
	}

	for _, req := range requests {
		if validateFn(req) {
			return true
		}
	}

	t.Errorf("No matching request found for method %s", method)
	return false
}

// MockRTMResponseFromXML creates a mock RTM response from XML.
// This is a helper function to construct XML responses with a given status and content.
//
// statValue string: The "stat" attribute value for the <rsp> root element (e.g., "ok" or "fail").
// contentXML string: The XML content to be included within the <rsp> element.
//
// Returns:
//
//	string: A complete XML response string.
func MockRTMResponseFromXML(statValue string, contentXML string) string {
	return fmt.Sprintf(`<rsp stat="%s">%s</rsp>`, statValue, contentXML)
}

// ParseXMLResponse parses an XML response into a struct.
// This is a helper function to unmarshal XML response strings into Go data structures for testing.
//
// response string: The XML response string to parse.
// v interface{}: A pointer to the struct where the parsed data should be stored.
//
// Returns:
//
//	error: An error if the XML parsing fails, otherwise nil.
func ParseXMLResponse(response string, v interface{}) error {
	return xml.Unmarshal(byte(response), v)
}

// DocEnhanced: 2025-03-25
