## TOP PRIORITY: Latest Build Errors (Tue Mar 25 18:04:44 EDT 2025)
```
Capturing build errors for documentation...
package github.com/cowgnition/cowgnition/cmd/server
	imports github.com/cowgnition/cowgnition/internal/server from commands.go
	imports github.com/cowgnition/cowgnition/internal/server/middleware from server.go
	imports github.com/cowgnition/cowgnition/internal/server from auth.go: import cycle not allowed
package github.com/cowgnition/cowgnition/cmd/server
	imports github.com/cowgnition/cowgnition/internal/server from commands.go
	imports github.com/cowgnition/cowgnition/internal/server/middleware from server.go
	imports github.com/cowgnition/cowgnition/internal/server from auth.go: import cycle not allowed
internal/rtm/auth.go:1: : # github.com/cowgnition/cowgnition/internal/rtm
internal/rtm/service.go:12:2: "github.com/cowgnition/cowgnition/internal/rtm/client" imported as rtm and not used
internal/rtm/service.go:17:16: undefined: client
internal/rtm/service.go:30:17: undefined: client (typecheck)
// Package rtm provides client functionality for the Remember The Milk API.
internal/server/middleware/auth.go:12:2: could not import github.com/cowgnition/cowgnition/internal/server (-: import cycle not allowed: import stack: [github.com/cowgnition/cowgnition/cmd/server github.com/cowgnition/cowgnition/internal/server github.com/cowgnition/cowgnition/internal/server/middleware github.com/cowgnition/cowgnition/internal/server]) (typecheck)
	"github.com/cowgnition/cowgnition/internal/server"
	^
internal/server/middleware/middleware.go:49:5: declared and not used: context (typecheck)
				context := map[string]interface{}{
				^
internal/server/errors.go:1: : import cycle not allowed: import stack: [github.com/cowgnition/cowgnition/cmd/server github.com/cowgnition/cowgnition/internal/server github.com/cowgnition/cowgnition/internal/server/middleware github.com/cowgnition/cowgnition/internal/server] (typecheck)
// Package server defines the core server-side logic for the Cowgnition MCP server.
cmd/server/commands.go:16:2: could not import github.com/cowgnition/cowgnition/internal/server (-: import cycle not allowed: import stack: [github.com/cowgnition/cowgnition/cmd/server github.com/cowgnition/cowgnition/internal/server github.com/cowgnition/cowgnition/internal/server/middleware github.com/cowgnition/cowgnition/internal/server]) (typecheck)
	"github.com/cowgnition/cowgnition/internal/server"
	^
cmd/server/commands.go:24:19: undefined: argsstring (typecheck)
	Run         func(argsstring) error
	                 ^
cmd/server/commands.go:62:19: undefined: argsstring (typecheck)
func serveCommand(argsstring) error {
                  ^
cmd/server/commands.go:157:21: undefined: _string (typecheck)
func versionCommand(_string) error {
                    ^
cmd/server/commands.go:167:19: undefined: argsstring (typecheck)
func checkCommand(argsstring) error {
                  ^
cmd/server/commands.go:70:21: undefined: args (typecheck)
	if err := fs.Parse(args); err != nil {
	                   ^
cmd/server/commands.go:173:21: undefined: args (typecheck)
	if err := fs.Parse(args); err != nil {
	                   ^
cmd/server/commands.go:228:21: undefined: args (typecheck)
	if err := fs.Parse(args); err != nil {
	                   ^
cmd/server/main.go:149:19: invalid composite literal type string (typecheck)
	standardPaths := string{
	                 ^
internal/server/api/handlers.go:8:2: could not import github.com/cowgnition/cowgnition/internal/server (-: import cycle not allowed: import stack: [github.com/cowgnition/cowgnition/cmd/server github.com/cowgnition/cowgnition/internal/server github.com/cowgnition/cowgnition/internal/server/middleware github.com/cowgnition/cowgnition/internal/server]) (typecheck)
	"github.com/cowgnition/cowgnition/internal/server"
	^
internal/server/mcp/resources.go:9:2: could not import github.com/cowgnition/cowgnition/internal/rtm (-: # github.com/cowgnition/cowgnition/internal/rtm
internal/rtm/service.go:12:2: "github.com/cowgnition/cowgnition/internal/rtm/client" imported as rtm and not used
internal/rtm/service.go:17:16: undefined: client
internal/rtm/service.go:30:17: undefined: client) (typecheck)
	"github.com/cowgnition/cowgnition/internal/rtm"
	^
internal/server/mcp/handlers.go:17:10: undefined: MCPServer (typecheck)
func (s *MCPServer) handleMCPInitialize(w http.ResponseWriter, r *http.Request) {
         ^
internal/server/mcp/handlers.go:93:10: undefined: MCPServer (typecheck)
func (s *MCPServer) handleMCPListResources(w http.ResponseWriter, r *http.Request) {
         ^
internal/server/mcp/handlers.go:180:10: undefined: MCPServer (typecheck)
func (s *MCPServer) handleMCPReadResource(w http.ResponseWriter, r *http.Request) {
         ^
internal/server/mcp/handlers.go:19:3: undefined: writeStandardErrorResponse (typecheck)
		writeStandardErrorResponse(w, MethodNotFound,
		^
internal/server/mcp/handlers.go:28:3: undefined: writeStandardErrorResponse (typecheck)
		writeStandardErrorResponse(w, ParseError,
		^
internal/server/mcp/handlers.go:88:2: undefined: writeJSONResponse (typecheck)
	writeJSONResponse(w, http.StatusOK, response)
	^
internal/server/mcp/handlers.go:95:3: undefined: writeStandardErrorResponse (typecheck)
		writeStandardErrorResponse(w, MethodNotFound,
		^
internal/server/mcp/handlers.go:122:2: undefined: writeJSONResponse (typecheck)
	writeJSONResponse(w, http.StatusOK, response)
	^
internal/server/mcp/handlers.go:269:2: undefined: writeJSONResponse (typecheck)
	writeJSONResponse(w, http.StatusOK, response)
	^
internal/server/mcp/handlers.go:394:9: undefined: formatTags (typecheck)
	return formatTags(tags), nil
	       ^
internal/server/mcp/resources.go:171:15: undefined: formatDate (typecheck)
	formatted := formatDate(dueDate)
	             ^
test/helpers/common/auth_helper.go:13:2: could not import github.com/cowgnition/cowgnition/internal/rtm (-: # github.com/cowgnition/cowgnition/internal/rtm
internal/rtm/service.go:12:2: "github.com/cowgnition/cowgnition/internal/rtm/client" imported as rtm and not used
internal/rtm/service.go:17:16: undefined: client
internal/rtm/service.go:30:17: undefined: client) (typecheck)
	"github.com/cowgnition/cowgnition/internal/rtm"
	^
test/helpers/common/auth_stub.go:19:6: SimulateAuthentication redeclared in this block (typecheck)
func SimulateAuthentication(s *server.MCPServer) error {
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
test/helpers/common/auth_helper.go:131:12: undefined: NewMCPClient (typecheck)
	client := NewMCPClient(nil, s)
	          ^
test/helpers/rtm/rtm_live_test_framework.go:25:19: undefined: helpers (typecheck)
	Client          *helpers.MCPClient
	                 ^
test/helpers/rtm/rtm_live_test_framework.go:26:19: undefined: helpers (typecheck)
	RTMClient       *helpers.RTMTestClient
	                 ^
test/helpers/rtm/rtm_live_test_framework.go:27:19: undefined: helpers (typecheck)
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
test/helpers/rtm/rtm_live_test_framework.go:161:19: undefined: ExtractAuthInfoFromContent (typecheck)
	authURL, frob := ExtractAuthInfoFromContent(content)
	                 ^
```

