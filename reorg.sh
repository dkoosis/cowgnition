#!/bin/bash

# Create new directories
mkdir -p test/mcp/conformance
mkdir -p test/mcp/helpers
mkdir -p test/rtm/fixtures
mkdir -p test/rtm/mocks
mkdir -p test/rtm/helpers
mkdir -p test/common/helpers
mkdir -p internal/rtm/client
mkdir -p internal/server/api
mkdir -p internal/server/mcp
mkdir -p internal/server/middleware

# Move MCP test files
mv test/conformance/mcp_*_test.go test/mcp/conformance/
mv test/conformance/json_rpc_error_validation_test.go test/mcp/conformance/
mv test/conformance/protocol_validators.go test/mcp/helpers/validators.go
mv test/conformance/resource_validator.go test/mcp/helpers/
mv test/conformance/rtm_live_test_framework.go test/rtm/helpers/

# Move helper files
mv test/helpers/mcp_client.go test/mcp/helpers/client.go
mv test/helpers/rtm_test_client.go test/rtm/helpers/client.go
mv test/helpers/auth_stub.go test/common/helpers/
mv test/helpers/test_config.go test/common/helpers/
mv test/helpers/rtm_helpers.go test/rtm/helpers/
mv test/helpers/rtm_live_helpers.go test/rtm/helpers/

# Move fixtures and mocks
mv test/fixtures/rtm/* test/rtm/fixtures/
mv test/mocks/rtm_server.go test/rtm/mocks/server.go

# Rename auth files
mv internal/auth/auth_manager.go internal/auth/manager.go
mv internal/auth/token_manager.go internal/auth/token.go

# Move and rename RTM client files
mv internal/rtm/client.go internal/rtm/client/client.go
mv internal/rtm/client_auth.go internal/rtm/client/auth.go
mv internal/rtm/client_lists.go internal/rtm/client/lists.go
mv internal/rtm/client_tasks.go internal/rtm/client/tasks.go
mv internal/rtm/client_tags.go internal/rtm/client/tags.go
mv internal/rtm/transport.go internal/rtm/client/transport.go
mv internal/rtm/client_test.go internal/rtm/client/client_test.go
mv internal/rtm/rate_limiter.go internal/rtm/client/rate_limiter.go

# Move and rename server files
mv internal/server/mcp_handlers.go internal/server/mcp/handlers.go
mv internal/server/resources.go internal/server/mcp/resources.go
mv internal/server/tools.go internal/server/mcp/tools.go
mv internal/server/auth_handlers.go internal/server/middleware/auth.go
mv internal/server/middleware.go internal/server/middleware/middleware.go
mv internal/server/handlers_api.go internal/server/api/handlers.go

# Rename MCP conformance test files
cd test/mcp/conformance
for f in mcp_*_endpoint_test.go; do
  newname=$(echo "$f" | sed 's/mcp_\(.*\)_endpoint_test.go/\1_test.go/')
  mv "$f" "$newname"
done
mv mcp_error_response_test.go errors_test.go
mv mcp_authenticated_resources_test.go auth_test.go
mv mcp_conformance_test.go conformance_test.go
mv mcp_live_resource_test.go live_resource_test.go
mv mcp_read_resource_live_test.go read_resource_live_test.go
cd -

# Clean up empty directories
find . -type d -empty -delete
