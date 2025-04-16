#!/bin/bash

# scripts/check_go_bin_path.sh
# Checks if the Go binary path (GOBIN or default GOPATH/bin) is in the PATH.
# Exits with 1 if not found, 0 otherwise.

# Color Codes
YELLOW='\033[0;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color
ICON_INFO=$(printf "${YELLOW}ℹ${NC}") # Use printf for compatibility
ICON_FAIL=$(printf "${RED}✗${NC}")

# Determine expected Go bin path
actual_go_bin_path=$(go env GOBIN)
if [ -z "$actual_go_bin_path" ]; then
    gopath=$(go env GOPATH)
    if [ -n "$gopath" ]; then
        # Use the first path in GOPATH if multiple exist
        actual_go_bin_path="$(echo "$gopath" | cut -d: -f1)/bin"
    else
        printf "${ICON_FAIL} $(RED)Could not determine Go path (go env GOPATH failed). Cannot determine bin path.${NC}\n" >&2
        exit 1
    fi
fi

# Validate the determined path (check if directory exists)
if [ ! -d "$actual_go_bin_path" ]; then
    # This is a warning, but the main failure is if it's not in $PATH
    printf "${ICON_INFO} $(YELLOW)Warning: Determined Go bin path ('%s') does not exist or is not a directory.${NC}\n" "$actual_go_bin_path" >&2
    # Continue to check PATH anyway, as the user might need to add it first *then* install tools
fi

# Check if the path exists in the current PATH environment variable
# Escape potential special characters for grep
escaped_go_bin_path=$(printf '%s\n' "$actual_go_bin_path" | sed 's:[][\\/.^$*]:\\&:g')
if echo "$PATH" | grep -qE "(^|:)${escaped_go_bin_path}(:|$)"; then
    # Path is found, exit successfully (status 0)
    exit 0
else
    # Path not found, print error message to stderr and exit with failure (status 1)
    printf "${ICON_FAIL} $(RED)Go bin path ('%s') exists but NOT in \$PATH!${NC}\n" "$actual_go_bin_path" >&2
    printf "        ${ICON_INFO} $(YELLOW)Please ensure this path is added to your shell profile (e.g., ~/.zshrc, ~/.bashrc): export PATH=\$PATH:%s${NC}\n" "$actual_go_bin_path" >&2
    exit 1
fi