// Package helpers provides testing utilities for the CowGnition MCP server.
package common

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"time"

	"github.com/cowgnition/cowgnition/internal/rtm"
	"github.com/cowgnition/cowgnition/internal/server"
)

// SimulateAuthentication sets up a server to be in an authenticated state for testing.
// It uses reflection to directly modify the RTM service's state, bypassing the normal
// authentication flow. This should ONLY be used in tests.
func SimulateAuthentication(s *server.MCPServer) error {
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
