## TOP PRIORITY: Latest Build Errors (Tue Mar 25 18:41:23 EDT 2025)
```
Capturing build errors for documentation...
github.com/cowgnition/cowgnition/pkg/util/stringutil
# github.com/cowgnition/cowgnition/pkg/util/stringutil
pkg/util/stringutil/stringutil.go:52:55: syntax error: missing parameter type
github.com/cowgnition/cowgnition/internal/server/middleware
github.com/cowgnition/cowgnition/internal/rtm
# github.com/cowgnition/cowgnition/internal/rtm
internal/rtm/service.go:12:2: "github.com/cowgnition/cowgnition/internal/rtm/client" imported as rtm and not used
internal/rtm/service.go:17:16: undefined: client
internal/rtm/service.go:30:17: undefined: client
# github.com/cowgnition/cowgnition/internal/server/middleware
internal/server/middleware/auth.go:59:23: method AuthHandler.HandleAuthResource already declared at internal/server/middleware/auth.go:25:23
internal/server/middleware/auth.go:63:3: undefined: server
internal/server/middleware/auth.go:71:3: undefined: server
internal/server/middleware/auth.go:88:2: undefined: server
internal/server/middleware/auth.go:143:3: undefined: server
internal/server/middleware/auth.go:152:3: undefined: server
internal/server/middleware/auth.go:168:4: undefined: server
internal/server/middleware/auth.go:178:4: undefined: server
internal/server/middleware/auth.go:187:3: undefined: server
internal/server/middleware/auth.go:208:2: undefined: server
internal/server/middleware/auth.go:208:2: too many errors
test/conformance/mcp/initialize_test.go:16:2: no required module provides package github.com/cowgnition/cowgnition/test/helpers; to add it:
	go get github.com/cowgnition/cowgnition/test/helpers
# github.com/cowgnition/cowgnition/internal/rtm
internal/rtm/service.go:12:2: "github.com/cowgnition/cowgnition/internal/rtm/client" imported as rtm and not used
internal/rtm/service.go:17:16: undefined: client
internal/rtm/service.go:30:17: undefined: client
# github.com/cowgnition/cowgnition/internal/server/middleware
internal/server/middleware/auth.go:59:23: method AuthHandler.HandleAuthResource already declared at internal/server/middleware/auth.go:25:23
internal/server/middleware/auth.go:63:3: undefined: server
internal/server/middleware/auth.go:71:3: undefined: server
internal/server/middleware/auth.go:88:2: undefined: server
internal/server/middleware/auth.go:143:3: undefined: server
internal/server/middleware/auth.go:152:3: undefined: server
internal/server/middleware/auth.go:168:4: undefined: server
internal/server/middleware/auth.go:178:4: undefined: server
internal/server/middleware/auth.go:187:3: undefined: server
internal/server/middleware/auth.go:208:2: undefined: server
internal/server/middleware/auth.go:208:2: too many errors
# github.com/cowgnition/cowgnition/pkg/util/stringutil
vet: pkg/util/stringutil/stringutil.go:52:55: missing parameter type
internal/rtm/auth.go:1: : # github.com/cowgnition/cowgnition/internal/rtm
internal/rtm/service.go:12:2: "github.com/cowgnition/cowgnition/internal/rtm/client" imported as rtm and not used
internal/rtm/service.go:17:16: undefined: client
internal/rtm/service.go:30:17: undefined: client (typecheck)
// Package rtm provides client functionality for the Remember The Milk API.
internal/server/middleware/auth.go:1: : # github.com/cowgnition/cowgnition/internal/server/middleware
internal/server/middleware/auth.go:59:23: method AuthHandler.HandleAuthResource already declared at internal/server/middleware/auth.go:25:23
internal/server/middleware/auth.go:63:3: undefined: server
internal/server/middleware/auth.go:71:3: undefined: server
internal/server/middleware/auth.go:88:2: undefined: server
internal/server/middleware/auth.go:143:3: undefined: server
internal/server/middleware/auth.go:152:3: undefined: server
internal/server/middleware/auth.go:168:4: undefined: server
internal/server/middleware/auth.go:178:4: undefined: server
internal/server/middleware/auth.go:187:3: undefined: server
internal/server/middleware/auth.go:208:2: undefined: server
internal/server/middleware/auth.go:208:2: too many errors (typecheck)
// internal/server/middleware/auth.go
internal/server/server.go:13:2: could not import github.com/cowgnition/cowgnition/internal/rtm (-: # github.com/cowgnition/cowgnition/internal/rtm
internal/rtm/service.go:12:2: "github.com/cowgnition/cowgnition/internal/rtm/client" imported as rtm and not used
internal/rtm/service.go:17:16: undefined: client
internal/rtm/service.go:30:17: undefined: client) (typecheck)
	"github.com/cowgnition/cowgnition/internal/rtm"
	^
internal/server/server.go:14:2: could not import github.com/cowgnition/cowgnition/internal/server/middleware (-: # github.com/cowgnition/cowgnition/internal/server/middleware
internal/server/middleware/auth.go:59:23: method AuthHandler.HandleAuthResource already declared at internal/server/middleware/auth.go:25:23
internal/server/middleware/auth.go:63:3: undefined: server
internal/server/middleware/auth.go:71:3: undefined: server
internal/server/middleware/auth.go:88:2: undefined: server
internal/server/middleware/auth.go:143:3: undefined: server
internal/server/middleware/auth.go:152:3: undefined: server
internal/server/middleware/auth.go:168:4: undefined: server
internal/server/middleware/auth.go:178:4: undefined: server
internal/server/middleware/auth.go:187:3: undefined: server
internal/server/middleware/auth.go:208:2: undefined: server
internal/server/middleware/auth.go:208:2: too many errors) (typecheck)
	"github.com/cowgnition/cowgnition/internal/server/middleware"
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
pkg/util/stringutil/stringutil.go:1: : # github.com/cowgnition/cowgnition/pkg/util/stringutil
pkg/util/stringutil/stringutil.go:52:55: syntax error: missing parameter type (typecheck)
// Package stringutil provides string manipulation utilities used throughout the CowGnition project.
pkg/util/stringutil/stringutil.go:52:55: missing parameter type (typecheck)
func ExtractFromContent(content string, patternsstring) string {
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
```

