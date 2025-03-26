// Package helpers provides testing utilities for the CowGnition MCP server.
package rtm

import (
	"fmt"
	"reflect"

	"github.com/dkoosis/cowgnition/internal/server"
)

// SetAuthTokenOnServer attempts to set the authentication token directly on the server's RTM service.
// This uses reflection for testing purposes.
func SetAuthTokenOnServer(s *server.Server, token string) error {
	// Get RTM service.
	rtmService := s.GetRTMService()
	if rtmService == nil {
		return fmt.Errorf("cannot get RTM service from server")
	}

	// Use reflection to access the RTM service client.
	value := reflect.ValueOf(rtmService).Elem()

	// Try to find the client field.
	clientField := value.FieldByName("client")
	if !clientField.IsValid() {
		return fmt.Errorf("RTM service has no client field")
	}

	// Check if client is accessible.
	if !clientField.CanInterface() {
		return fmt.Errorf("RTM service client field is not accessible")
	}

	// Get the client.
	clientObj := clientField.Interface()

	// Check if the client has a SetAuthToken method.
	clientValue := reflect.ValueOf(clientObj)
	setTokenMethod := clientValue.MethodByName("SetAuthToken")
	if !setTokenMethod.IsValid() {
		return fmt.Errorf("RTM client has no SetAuthToken method")
	}

	// Call SetAuthToken with the token.
	setTokenMethod.Call([]reflect.Value{reflect.ValueOf(token)})

	// Also try to set the authStatus field to indicate authentication.
	authStatusField := value.FieldByName("authStatus")
	if authStatusField.IsValid() && authStatusField.CanSet() {
		// Status 3 is StatusAuthenticated in our RTM package.
		authStatusField.SetInt(3)
	}

	// Check if authentication worked.
	if IsAuthenticated(NewMCPClient(nil, s)) {
		return nil
	}

	return fmt.Errorf("failed to authenticate server using reflection")
}
