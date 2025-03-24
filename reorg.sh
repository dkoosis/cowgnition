#!/bin/bash
set -e

# Helper function to create directory if it doesn't exist
create_dir() {
  if [ ! -d "$1" ]; then
    mkdir -p "$1"
    echo "Created directory: $1"
  fi
}

# Create target directories
create_dir test/unit/rtm
create_dir test/unit/mcp
create_dir test/unit/auth
create_dir test/integration/rtm
create_dir test/integration/mcp
create_dir test/conformance/mcp
create_dir test/fixtures/rtm
create_dir test/fixtures/mcp
create_dir test/fixtures/common
create_dir test/mocks/rtm
create_dir test/mocks/common
create_dir test/helpers/rtm
create_dir test/helpers/mcp
create_dir test/helpers/common

# Move to_be_organized helpers
echo "Moving helpers from to_be_organized..."
for file in test/helpers/to_be_organized/*.go; do
  if [ -f "$file" ]; then
    basename=$(basename "$file")
    # Determine destination based on file name
    if [[ "$basename" == *"rtm"* ]]; then
      target="test/helpers/rtm/$basename"
    else
      target="test/helpers/common/$basename"
    fi
    cp "$file" "$target"
    sed -i '' 's/package helpers/package common/' "$target"
    echo "Moved $file to $target"
  fi
done

# Move validators from helpers/mcp to helpers/mcp
echo "Moving MCP validators..."
cp test/helpers/mcp/validators.go test/helpers/mcp/
sed -i '' 's/package mcp/package mcp/' test/helpers/mcp/validators.go

# Move server.go mock to mocks/common
echo "Moving server mock..."
cp test/mocks/server.go test/mocks/common/
sed -i '' 's/package mocks/package common/' test/mocks/common/server.go

# Move conformance test stubs
echo "Moving conformance stubs..."
cp test/conformance/stubs/rtm_stubs.go test/helpers/rtm/
sed -i '' 's/package helpers/package rtm/' test/helpers/rtm/rtm_stubs.go

cp test/conformance/stubs/mcp/test_framework.go test/helpers/mcp/
sed -i '' 's/package mcp/package mcp/' test/helpers/mcp/test_framework.go

# Move fixtures
echo "Moving fixtures..."
cp test/fixtures/fixtures.go test/fixtures/common/
sed -i '' 's/package fixtures/package common/' test/fixtures/common/fixtures.go

# Move live tests to integration
echo "Moving live tests to integration..."
for file in test/conformance/mcp/*live*.go; do
  if [ -f "$file" ]; then
    basename=$(basename "$file")
    cp "$file" "test/integration/mcp/$basename"
    sed -i '' 's/package conformance/package mcp/' "test/integration/mcp/$basename"
    echo "Moved $file to test/integration/mcp/$basename"
  fi
done

echo "Update imports in all files..."
find test -name "*.go" -type f -exec sed -i '' 's/"github.com\/cowgnition\/cowgnition\/test\/mocks"/"github.com\/cowgnition\/cowgnition\/test\/mocks\/common"/g' {} \;
find test -name "*.go" -type f -exec sed -i '' 's/"github.com\/cowgnition\/cowgnition\/test\/helpers\/to_be_organized"/"github.com\/cowgnition\/cowgnition\/test\/helpers\/common"/g' {} \;
find test -name "*.go" -type f -exec sed -i '' 's/"github.com\/cowgnition\/cowgnition\/test\/fixtures"/"github.com\/cowgnition\/cowgnition\/test\/fixtures\/common"/g' {} \;

echo "Migration completed. Please review changes and run tests to verify functionality."
