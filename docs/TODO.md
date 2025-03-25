## TOP PRIORITY: Latest Build Errors (Tue Mar 25 17:53:27 EDT 2025)

```
Capturing build errors for documentation...
found packages middleware (auth.go) and server (middleware.go) in /Users/davidkoosis/projects/cowgnition/internal/server/middleware
found packages middleware (auth.go) and server (middleware.go) in /Users/davidkoosis/projects/cowgnition/internal/server/middleware
internal/rtm/auth.go:1: : # github.com/cowgnition/cowgnition/internal/rtm
internal/rtm/service.go:12:2: "github.com/cowgnition/cowgnition/internal/rtm/client" imported as rtm and not used
internal/rtm/service.go:17:16: undefined: client
internal/rtm/service.go:30:17: undefined: client (typecheck)
// Package rtm provides client functionality for the Remember The Milk API.
internal/server/server.go:13:2: could not import github.com/cowgnition/cowgnition/internal/rtm (-: # github.com/cowgnition/cowgnition/internal/rtm
internal/rtm/service.go:12:2: "github.com/cowgnition/cowgnition/internal/rtm/client" imported as rtm and not used
internal/rtm/service.go:17:16: undefined: client
internal/rtm/service.go:30:17: undefined: client) (typecheck)
	"github.com/cowgnition/cowgnition/internal/rtm"
	^
internal/server/handlers.go:14:4: s.handleMCPInitialize undefined (type *MCPServer has no field or method handleMCPInitialize) (typecheck)
	s.handleMCPInitialize(w, r)
	  ^
internal/server/handlers.go:19:4: s.handleMCPListResources undefined (type *MCPServer has no field or method handleMCPListResources) (typecheck)
	s.handleMCPListResources(w, r)
	  ^
internal/server/handlers.go:24:4: s.handleMCPReadResource undefined (type *MCPServer has no field or method handleMCPReadResource) (typecheck)
	s.handleMCPReadResource(w, r)
	  ^
internal/server/handlers.go:29:4: s.handleMCPListTools undefined (type *MCPServer has no field or method handleMCPListTools) (typecheck)
	s.handleMCPListTools(w, r)
	  ^
internal/server/handlers.go:34:4: s.handleMCPCallTool undefined (type *MCPServer has no field or method handleMCPCallTool) (typecheck)
	s.handleMCPCallTool(w, r)
	  ^
internal/server/handlers.go:39:4: s.handleMCPSendNotification undefined (type *MCPServer has no field or method handleMCPSendNotification) (typecheck)
	s.handleMCPSendNotification(w, r)
	  ^
internal/server/handlers.go:98:12: s.handleAddTaskTool undefined (type *MCPServer has no field or method handleAddTaskTool) (typecheck)
		return s.handleAddTaskTool(args)
		         ^
internal/server/handlers.go:100:12: s.handleCompleteTaskTool undefined (type *MCPServer has no field or method handleCompleteTaskTool) (typecheck)
		return s.handleCompleteTaskTool(args)
		         ^
internal/server/handlers.go:102:12: s.handleUncompleteTaskTool undefined (type *MCPServer has no field or method handleUncompleteTaskTool) (typecheck)
		return s.handleUncompleteTaskTool(args)
		         ^
internal/server/handlers.go:104:12: s.handleDeleteTaskTool undefined (type *MCPServer has no field or method handleDeleteTaskTool) (typecheck)
		return s.handleDeleteTaskTool(args)
		         ^
internal/server/handlers.go:106:12: s.handleSetDueDateTool undefined (type *MCPServer has no field or method handleSetDueDateTool) (typecheck)
		return s.handleSetDueDateTool(args)
		         ^
internal/server/handlers.go:108:12: s.handleSetPriorityTool undefined (type *MCPServer has no field or method handleSetPriorityTool) (typecheck)
		return s.handleSetPriorityTool(args)
		         ^
internal/server/handlers.go:110:12: s.handleAddTagsTool undefined (type *MCPServer has no field or method handleAddTagsTool) (typecheck)
		return s.handleAddTagsTool(args)
		         ^
internal/server/handlers.go:112:12: s.handleLogoutTool undefined (type *MCPServer has no field or method handleLogoutTool) (typecheck)
		return s.handleLogoutTool(args)
		         ^
internal/server/handlers.go:114:12: s.handleAuthStatusTool undefined (type *MCPServer has no field or method handleAuthStatusTool) (typecheck)
		return s.handleAuthStatusTool(args)
		         ^
internal/server/server.go:88:13: undefined: logMiddleware (typecheck)
	handler := logMiddleware(recoveryMiddleware(corsMiddleware(mux)))
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
internal/server/middleware/auth.go:1: : found packages middleware (auth.go) and server (middleware.go) in internal/server/middleware (typecheck)
// file: internal/server/middleware/auth.go
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
```

# Task List & Build Errors (Tue Mar 25 14:34:14 EDT 2025)
