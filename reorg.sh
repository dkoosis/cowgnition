#!/bin/bash
set -e

echo "Fixing package declarations in test/conformance/mcp..."

# Fix inconsistent package declarations in test/conformance/mcp
find test/conformance/mcp -name "*.go" -type f -exec grep -l "^package conformance" {} \; | while read file; do
  echo "Fixing package in $file"
  sed -i '' 's/^package conformance/package mcp/' "$file"
done

# Check for and fix RTM client import issues
echo "Checking for RTM client import issues..."
if grep -r --include="*.go" "undefined: Client" . || grep -r --include="*.go" "undefined: NewClient" .; then
  echo "Potential RTM client references found. Check internal/rtm/service.go to ensure it imports:"
  echo "  \"github.com/cowgnition/cowgnition/internal/rtm/client\""
  echo "And update references to use client.Client and client.NewClient"
fi

# Check for validator duplication
echo "Checking for validator function duplication..."
grep -r --include="*.go" "func ValidateMCPResource" ./test
grep -r --include="*.go" "func ValidateMCPTool" ./test
grep -r --include="*.go" "func ValidateResourceResponse" ./test
grep -r --include="*.go" "func ValidateToolResponse" ./test
grep -r --include="*.go" "func ValidateErrorResponse" ./test

echo "Script completed. Now run 'go test ./test/...' to verify fixes."
