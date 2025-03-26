## TOP PRIORITY: Latest Build Errors (Tue Mar 25 21:09:14 EDT 2025)
```
Capturing build errors for documentation...
github.com/cowgnition/cowgnition/internal/server/mcp
github.com/cowgnition/cowgnition/internal/server
# github.com/cowgnition/cowgnition/internal/server/mcp
internal/server/mcp/handlers.go:17:10: undefined: Server
internal/server/mcp/handlers.go:93:10: undefined: Server
internal/server/mcp/handlers.go:180:10: undefined: Server
internal/server/mcp/handlers.go:274:10: undefined: Server
internal/server/mcp/handlers.go:314:10: undefined: Server
internal/server/mcp/handlers.go:373:10: undefined: Server
internal/server/mcp/handlers.go:389:10: undefined: Server
internal/server/mcp/resources.go:14:10: undefined: Server
internal/server/mcp/resources.go:43:24: undefined: Server
internal/server/mcp/resources.go:242:10: undefined: Server
internal/server/mcp/resources.go:242:10: too many errors
# github.com/cowgnition/cowgnition/internal/server
internal/server/utils.go:59:6: formatTimeComponent redeclared in this block
	internal/server/resources_mcp.go:189:6: other declaration of formatTimeComponent
internal/server/utils.go:69:6: formatDate redeclared in this block
	internal/server/resources_mcp.go:153:6: other declaration of formatDate
internal/server/handlers_mcp.go:203:5: s.handleAuthResource undefined (type *Server has no field or method handleAuthResource)
internal/server/handlers_mcp.go:338:44: cannot use s (variable of type *Server) as httputils.ServerInterface value in argument to middleware.NewAuthHandler: *Server does not implement httputils.ServerInterface (wrong type for method GetRTMService)
		have GetRTMService() *"github.com/cowgnition/cowgnition/internal/rtm".Service
		want GetRTMService() httputils.RTMServiceInterface
internal/server/tools_mcp.go:156:53: undefined: formatDueDate
test/conformance/mcp/initialize_test.go:16:2: no required module provides package github.com/cowgnition/cowgnition/test/helpers; to add it:
	go get github.com/cowgnition/cowgnition/test/helpers
# github.com/cowgnition/cowgnition/internal/server
internal/server/utils.go:59:6: formatTimeComponent redeclared in this block
	internal/server/resources_mcp.go:189:6: other declaration of formatTimeComponent
internal/server/utils.go:69:6: formatDate redeclared in this block
	internal/server/resources_mcp.go:153:6: other declaration of formatDate
internal/server/handlers_mcp.go:203:5: s.handleAuthResource undefined (type *Server has no field or method handleAuthResource)
internal/server/handlers_mcp.go:338:44: cannot use s (variable of type *Server) as httputils.ServerInterface value in argument to middleware.NewAuthHandler: *Server does not implement httputils.ServerInterface (wrong type for method GetRTMService)
		have GetRTMService() *"github.com/cowgnition/cowgnition/internal/rtm".Service
		want GetRTMService() httputils.RTMServiceInterface
internal/server/tools_mcp.go:156:53: undefined: formatDueDate
# github.com/cowgnition/cowgnition/internal/server/mcp
vet: internal/server/mcp/handlers.go:17:10: undefined: Server
internal/server/errors.go:1: : # github.com/cowgnition/cowgnition/internal/server
internal/server/utils.go:59:6: formatTimeComponent redeclared in this block
	internal/server/resources_mcp.go:189:6: other declaration of formatTimeComponent
internal/server/utils.go:69:6: formatDate redeclared in this block
	internal/server/resources_mcp.go:153:6: other declaration of formatDate
internal/server/handlers_mcp.go:203:5: s.handleAuthResource undefined (type *Server has no field or method handleAuthResource)
internal/server/handlers_mcp.go:338:44: cannot use s (variable of type *Server) as httputils.ServerInterface value in argument to middleware.NewAuthHandler: *Server does not implement httputils.ServerInterface (wrong type for method GetRTMService)
		have GetRTMService() *"github.com/cowgnition/cowgnition/internal/rtm".Service
		want GetRTMService() httputils.RTMServiceInterface
internal/server/tools_mcp.go:156:53: undefined: formatDueDate (typecheck)
// Package server defines the core server-side logic for the Cowgnition MCP server.
cmd/server/commands.go:15:2: could not import github.com/cowgnition/cowgnition/internal/server (-: # github.com/cowgnition/cowgnition/internal/server
internal/server/utils.go:59:6: formatTimeComponent redeclared in this block
	internal/server/resources_mcp.go:189:6: other declaration of formatTimeComponent
internal/server/utils.go:69:6: formatDate redeclared in this block
	internal/server/resources_mcp.go:153:6: other declaration of formatDate
internal/server/handlers_mcp.go:203:5: s.handleAuthResource undefined (type *Server has no field or method handleAuthResource)
internal/server/handlers_mcp.go:338:44: cannot use s (variable of type *Server) as httputils.ServerInterface value in argument to middleware.NewAuthHandler: *Server does not implement httputils.ServerInterface (wrong type for method GetRTMService)
		have GetRTMService() *"github.com/cowgnition/cowgnition/internal/rtm".Service
		want GetRTMService() httputils.RTMServiceInterface
internal/server/tools_mcp.go:156:53: undefined: formatDueDate) (typecheck)
	"github.com/cowgnition/cowgnition/internal/server"
	^
cmd/server/main.go:18:2: version redeclared in this block (typecheck)
	version    = "dev"
	^
cmd/server/commands.go:20:2: other declaration of version (typecheck)
	version     = "dev"
	^
cmd/server/commands.go:157:2: declared and not used: s (typecheck)
	s, err := server.NewServer(cfg)
	^
cmd/server/main.go:41:14: undefined: RegisterCommands (typecheck)
	commands := RegisterCommands()
	            ^
cmd/server/main.go:149:19: invalid composite literal type string (typecheck)
	standardPaths := string{
	                 ^
internal/server/api/handlers.go:8:2: could not import github.com/cowgnition/cowgnition/internal/server (-: # github.com/cowgnition/cowgnition/internal/server
internal/server/utils.go:59:6: formatTimeComponent redeclared in this block
	internal/server/resources_mcp.go:189:6: other declaration of formatTimeComponent
internal/server/utils.go:69:6: formatDate redeclared in this block
	internal/server/resources_mcp.go:153:6: other declaration of formatDate
internal/server/handlers_mcp.go:203:5: s.handleAuthResource undefined (type *Server has no field or method handleAuthResource)
internal/server/handlers_mcp.go:338:44: cannot use s (variable of type *Server) as httputils.ServerInterface value in argument to middleware.NewAuthHandler: *Server does not implement httputils.ServerInterface (wrong type for method GetRTMService)
		have GetRTMService() *"github.com/cowgnition/cowgnition/internal/rtm".Service
		want GetRTMService() httputils.RTMServiceInterface
internal/server/tools_mcp.go:156:53: undefined: formatDueDate) (typecheck)
	"github.com/cowgnition/cowgnition/internal/server"
	^
internal/server/mcp/handlers.go:1: : # github.com/cowgnition/cowgnition/internal/server/mcp
internal/server/mcp/handlers.go:17:10: undefined: Server
internal/server/mcp/handlers.go:93:10: undefined: Server
internal/server/mcp/handlers.go:180:10: undefined: Server
internal/server/mcp/handlers.go:274:10: undefined: Server
internal/server/mcp/handlers.go:314:10: undefined: Server
internal/server/mcp/handlers.go:373:10: undefined: Server
internal/server/mcp/handlers.go:389:10: undefined: Server
internal/server/mcp/resources.go:14:10: undefined: Server
internal/server/mcp/resources.go:43:24: undefined: Server
internal/server/mcp/resources.go:242:10: undefined: Server
internal/server/mcp/resources.go:242:10: too many errors (typecheck)
// Package server implements the Model Context Protocol server for RTM integration.
test/helpers/common/auth_helper.go:14:2: could not import github.com/cowgnition/cowgnition/internal/server (-: # github.com/cowgnition/cowgnition/internal/server
internal/server/utils.go:59:6: formatTimeComponent redeclared in this block
	internal/server/resources_mcp.go:189:6: other declaration of formatTimeComponent
internal/server/utils.go:69:6: formatDate redeclared in this block
	internal/server/resources_mcp.go:153:6: other declaration of formatDate
internal/server/handlers_mcp.go:203:5: s.handleAuthResource undefined (type *Server has no field or method handleAuthResource)
internal/server/handlers_mcp.go:338:44: cannot use s (variable of type *Server) as httputils.ServerInterface value in argument to middleware.NewAuthHandler: *Server does not implement httputils.ServerInterface (wrong type for method GetRTMService)
		have GetRTMService() *"github.com/cowgnition/cowgnition/internal/rtm".Service
		want GetRTMService() httputils.RTMServiceInterface
internal/server/tools_mcp.go:156:53: undefined: formatDueDate) (typecheck)
	"github.com/cowgnition/cowgnition/internal/server"
	^
test/helpers/common/auth_stub.go:19:6: SimulateAuthentication redeclared in this block (typecheck)
func SimulateAuthentication(s *server.Server) error {
     ^
test/helpers/common/auth_helper.go:20:6: other declaration of SimulateAuthentication (typecheck)
func SimulateAuthentication(s *server.Server) error {
     ^
test/helpers/common/auth_stub.go:62:6: IsAuthenticated redeclared in this block (typecheck)
func IsAuthenticated(client *MCPClient) bool {
     ^
test/helpers/common/auth_helper.go:63:6: other declaration of IsAuthenticated (typecheck)
func IsAuthenticated(client *MCPClient) bool {
     ^
test/helpers/common/client.go:17:20: undefined: rtm.Client (typecheck)
	client       *rtm.Client
	                  ^
test/helpers/common/client.go:155:42: undefined: rtm.Client (typecheck)
func (c *RTMTestClient) GetClient() *rtm.Client {
                                         ^
test/helpers/common/auth_helper.go:41:47: undefined: rtm.Client (typecheck)
		client, ok := clientField.Interface().(*rtm.Client)
		                                            ^
test/helpers/common/auth_helper.go:131:12: undefined: NewMCPClient (typecheck)
	client := NewMCPClient(nil, s)
	          ^
test/helpers/common/client.go:42:16: undefined: rtm.NewClient (typecheck)
	client := rtm.NewClient(apiKey, sharedSecret)
	              ^
test/helpers/rtm/rtm_live_test_framework.go:24:19: undefined: helpers (typecheck)
	Client          *helpers.MCPClient
	                 ^
test/helpers/rtm/rtm_live_test_framework.go:25:19: undefined: helpers (typecheck)
	RTMClient       *helpers.RTMTestClient
	                 ^
test/helpers/rtm/rtm_live_test_framework.go:26:19: undefined: helpers (typecheck)
	TestConfig      *helpers.TestConfig
	                 ^
test/helpers/rtm/rtm_live_helpers.go:16:48: undefined: MCPClient (typecheck)
func ReadResource(ctx context.Context, client *MCPClient, resourceName string) (map[string]interface{}, error) {
                                               ^
test/helpers/rtm/rtm_live_helpers.go:48:44: undefined: MCPClient (typecheck)
func CallTool(ctx context.Context, client *MCPClient, toolName string, args map[string]interface{}) (map[string]interface{}, error) {
                                           ^
test/helpers/rtm/rtm_helpers.go:55:5: undefined: IsAuthenticated (typecheck)
	if IsAuthenticated(NewMCPClient(nil, s)) {
	   ^
test/helpers/rtm/rtm_live_test_framework.go:160:19: undefined: ExtractAuthInfoFromContent (typecheck)
	authURL, frob := ExtractAuthInfoFromContent(content)
	                 ^
```

