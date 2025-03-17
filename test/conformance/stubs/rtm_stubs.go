// test/conformance/stubs/rtm_stubs.go
package stubs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/server"
	"github.com/cowgnition/cowgnition/test/helpers"
)

func SetAuthTokenOnServer(s *server.MCPServer, token string) error {
	s.SetConfig(&config.Config{
		RTM: config.RTMConfig{
			AuthToken: token,
		},
	})
	return nil
}

func IsServerAuthenticated(ctx context.Context, client *helpers.MCPClient) bool {
	cfg := client.Server.GetConfig()
	return cfg.RTM.AuthToken != ""
}
func ReadResource(ctx context.Context, client *helpers.MCPClient, uri string) (map[string]interface{}, error) {
	if uri == "auth://rtm" {
		fakeAuthURL := "https://www.rememberthemilk.com/services/auth/?api_key=YOUR_API_KEY&perms=delete&frob=FAKE_FROB"
		fakeContent := fmt.Sprintf("authURL=%s", fakeAuthURL)

		return map[string]interface{}{
			"content": fakeContent,
		}, nil
	}
	return nil, errors.New("resource not found")
}

func ExtractAuthInfoFromContent(content string) (authURL, frob string) {
	u, err := url.Parse(content)
	if err != nil {
		return "", ""
	}

	// Check if "authURL=" is at the beginning of the string
	if strings.HasPrefix(content, "authURL=") {
		return strings.TrimPrefix(content, "authURL="), "FAKE_FROB" //Return a fake frob for the stub
	}

	if f := u.Query().Get("frob"); f != "" {
		return u.String(), f
	}

	return "", ""
}

func CallTool(ctx context.Context, client *helpers.MCPClient, toolName string, args map[string]interface{}) (map[string]interface{}, error) {
	if toolName == "authenticate" {
		return map[string]interface{}{
			"result": "success",
		}, nil
	}
	return nil, errors.New("tool not found")
}
