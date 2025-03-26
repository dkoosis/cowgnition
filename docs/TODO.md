## TOP PRIORITY: Latest Build Errors (Tue Mar 25 20:58:23 EDT 2025)
```
Capturing build errors for documentation...
internal/server/handlers_mcp.go:13:2: no required module provides package github.com/cowgnition/cowgnition/internal/middleware; to add it:
	go get github.com/cowgnition/cowgnition/internal/middleware
internal/server/handlers_mcp.go:13:2: no required module provides package github.com/cowgnition/cowgnition/internal/middleware; to add it:
	go get github.com/cowgnition/cowgnition/internal/middleware
internal/server/utils.go:59:6: formatTimeComponent redeclared in this block (typecheck)
func formatTimeComponent(t time.Time) string {
     ^
internal/server/resources_mcp.go:189:6: other declaration of formatTimeComponent (typecheck)
func formatTimeComponent(t time.Time) string {
     ^
internal/server/utils.go:69:6: formatDate redeclared in this block (typecheck)
func formatDate(dateStr string) string {
     ^
internal/server/resources_mcp.go:153:6: other declaration of formatDate (typecheck)
func formatDate(dueDate string) string {
     ^
internal/server/handlers.go:108:12: s.handleSetPriorityTool undefined (type *Server has no field or method handleSetPriorityTool) (typecheck)
		return s.handleSetPriorityTool(args)
		         ^
internal/server/handlers.go:110:12: s.handleAddTagsTool undefined (type *Server has no field or method handleAddTagsTool) (typecheck)
		return s.handleAddTagsTool(args)
		         ^
internal/server/handlers.go:112:12: s.handleLogoutTool undefined (type *Server has no field or method handleLogoutTool) (typecheck)
		return s.handleLogoutTool(args)
		         ^
internal/server/handlers.go:114:12: s.handleAuthStatusTool undefined (type *Server has no field or method handleAuthStatusTool) (typecheck)
		return s.handleAuthStatusTool(args)
		         ^
internal/server/handlers_mcp.go:203:5: s.handleAuthResource undefined (type *Server has no field or method handleAuthResource) (typecheck)
		s.handleAuthResource(w, r)
		  ^
internal/server/handlers_mcp.go:225:20: s.handleListsResource undefined (type *Server has no field or method handleListsResource) (typecheck)
		content, err = s.handleListsResource()
		                 ^
internal/server/resources_mcp.go:37:17: undefined: formatTasksSummary (typecheck)
	sb.WriteString(formatTasksSummary(totalTasks, completedTasks))
	               ^
internal/server/resources_mcp.go:117:30: undefined: formatTaskPriority (typecheck)
	priority, prioritySymbol := formatTaskPriority(task.Priority)
	                            ^
internal/server/resources_mcp.go:126:27: undefined: formatTaskDueDate (typecheck)
	dueDate, dueDateColor := formatTaskDueDate(task.Due)
	                         ^
internal/server/resources_mcp.go:139:14: undefined: formatTaskMetadata (typecheck)
	metadata := formatTaskMetadata(priority, dueDate, dueDateColor, ts.Tags.Tag)
	            ^
internal/server/tools_mcp.go:155:53: undefined: formatDueDate (typecheck)
	return fmt.Sprintf("Due date has been set to %s.", formatDueDate(dueDate, hasDueTime)), nil
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

