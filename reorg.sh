#!/bin/bash
set -e

echo "Creating directory structure..."

# Create function directories
mkdir -p test/unit/rtm test/unit/mcp test/unit/auth
mkdir -p test/integration/rtm test/integration/mcp
mkdir -p test/conformance/mcp

# Create support function directories
mkdir -p test/fixtures/rtm test/fixtures/mcp test/fixtures/common
mkdir -p test/mocks/rtm test/mocks/common
mkdir -p test/helpers/rtm test/helpers/mcp test/helpers/common

# Migrate files (example for conformance tests)
echo "Migrating conformance tests..."
for file in test/mcp/conformance/*.go; do
  basename=$(basename "$file")
  # Skip files with "live" in the name - they'll go to integration
  if [[ "$basename" != *"live"* ]]; then
    target="test/conformance/mcp/$basename"
    echo "Moving $file to $target"
    cp "$file" "$target"
    # Update package name
    sed -i 's/package conformance/package mcp/' "$target"
    # Update imports
    sed -i 's/"github.com\/cowgnition\/cowgnition\/test\/helpers"/"github.com\/cowgnition\/cowgnition\/test\/helpers\/common"/' "$target"
    sed -i 's/"github.com\/cowgnition\/cowgnition\/test\/mocks"/"github.com\/cowgnition\/cowgnition\/test\/mocks\/rtm"/' "$target"
  fi
done

# Migrate live tests to integration
echo "Migrating live tests to integration..."
for file in test/mcp/conformance/*live*.go; do
  if [ -f "$file" ]; then
    basename=$(basename "$file")
    target="test/integration/mcp/$basename"
    echo "Moving $file to $target"
    cp "$file" "$target"
    sed -i 's/package conformance/package mcp/' "$target"
    sed -i 's/"github.com\/cowgnition\/cowgnition\/test\/helpers"/"github.com\/cowgnition\/cowgnition\/test\/helpers\/common"/' "$target"
    sed -i 's/"github.com\/cowgnition\/cowgnition\/test\/mocks"/"github.com\/cowgnition\/cowgnition\/test\/mocks\/rtm"/' "$target"
  fi
done

# Migrate RTM fixtures
echo "Migrating RTM fixtures..."
for file in test/rtm/fixtures/*.go; do
  basename=$(basename "$file")
  target="test/fixtures/rtm/$basename"
  echo "Moving $file to $target"
  cp "$file" "$target"
  # Update package if needed
  # sed -i 's/package fixtures/package rtm/' "$target"
done

# Continue with other migrations...
echo "Migration script completed. Please review changes before deleting original files."
