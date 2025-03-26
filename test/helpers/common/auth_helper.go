// Package common provides common testing utilities for the CowGnition MCP server.
package common

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"time"

	"github.com/dkoosis/cowgnition/internal/rtm"
	"github.com/dkoosis/cowgnition/internal/server"
)

// SimulateAuthentication sets up a server to be in an authenticated state for testing.
// It uses reflection to directly modify the RTM service's state, bypassing the normal
// authentication flow. This should ONLY be used in tests.
func SimulateAuthentication(s *server.Server) error {
	// Get RTM service from the server
	rtmService := s.GetRTMService()
	if rtmService == nil {
		log.Println("Warning: Failed to get RTM service from server")
		return nil
	}

	// Use reflection to access and modify the service's internal state
	serviceValue := reflect.ValueOf(rtmService).Elem()

	// Set authentication status to authenticated (StatusAuthenticated = 3)
	authStatusField := serviceValue.FieldByName("authStatus")
	if authStatusField.IsValid() && authStatusField.CanSet() {
		authStatusField.SetInt(3) // 3 is StatusAuthenticated in rtm.Status
		log.Println("Set authentication status to authenticated")
	}

	// Set a dummy token on the client
	clientField := serviceValue.FieldByName("client")
	if clientField.IsValid() && clientField.CanInterface() {
		client, ok := clientField.Interface().(*rtm.Client)
		if ok {
			client.SetAuthToken("test_token_for_conformance_tests")
			log.Println("Set test authentication token on client")
		}
	}

	// Set last refresh time to now
	lastRefreshField := serviceValue.FieldByName("lastRefresh")
	if lastRefreshField.IsValid() && lastRefreshField.CanSet() {
		now := reflect.ValueOf(time.Now())
		if lastRefreshField.Type() == now.Type() {
			lastRefreshField.Set(now)
			log.Println("Set lastRefresh time")
		}
	}

	return nil
}

// IsAuthenticated checks if the server is currently authenticated
// by trying to access a protected resource.
func IsAuthenticated(client *MCPClient) bool {
	if client == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		client.BaseURL+"/mcp/read_resource?name="+url.QueryEscape("tasks://all"), nil)
	if err != nil {
		return false
	}

	resp, err := client.Client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// If we can access this resource, the server is authenticated
	return resp.StatusCode == http.StatusOK
}

// SetAuthTokenOnServer attempts to set the authentication token directly on the server's RTM service.
// This uses reflection for testing purposes.
func SetAuthTokenOnServer(s *server.Server, token string) error {
	// Get RTM service
	rtmService := s.GetRTMService()
	if rtmService == nil {
		return fmt.Errorf("cannot get RTM service from server")
	}

	// Use reflection to access the RTM service client
	value := reflect.ValueOf(rtmService).Elem()

	// Try to find the client field
	clientField := value.FieldByName("client")
	if !clientField.IsValid() {
		return fmt.Errorf("RTM service has no client field")
	}

	// Check if client is accessible
	if !clientField.CanInterface() {
		return fmt.Errorf("RTM service client field is not accessible")
	}

	// Get the client
	clientObj := clientField.Interface()

	// Check if the client has a SetAuthToken method
	clientValue := reflect.ValueOf(clientObj)
	setTokenMethod := clientValue.MethodByName("SetAuthToken")
	if !setTokenMethod.IsValid() {
		return fmt.Errorf("RTM client has no SetAuthToken method")
	}

	// Call SetAuthToken with the token
	setTokenMethod.Call([]reflect.Value{reflect.ValueOf(token)})

	// Also try to set the authStatus field to indicate authentication
	authStatusField := value.FieldByName("authStatus")
	if authStatusField.IsValid() && authStatusField.CanSet() {
		// Status 3 is StatusAuthenticated in our RTM package
		authStatusField.SetInt(3)
	}

	// Check if authentication worked
	client := NewMCPClient(nil, s)
	defer client.Close()

	if IsAuthenticated(client) {
		return nil
	}

	return fmt.Errorf("failed to authenticate server using reflection")
}
