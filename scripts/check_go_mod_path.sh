#!/bin/bash

# scripts/check_go_mod_path.sh
# Checks if the module path in go.mod matches the expected path provided as an argument.
# Exits with 1 if mismatch or error, 0 otherwise.

# Color Codes (consistent with your other script)
YELLOW='\033[0;33m'
RED='\033[0;31m'
GREEN='\033[0;32m'
BOLD='\033[1m'
NC='\033[0m' # No Color
# Use printf for better compatibility, especially inside scripts
ICON_INFO=$(printf "${YELLOW}ℹ${NC}")
ICON_FAIL=$(printf "${RED}✗${NC}")
ICON_OK=$(printf "${GREEN}✓${NC}")

EXPECTED_PATH="$1"

# --- Input Validation ---

# Check if expected path argument was provided
if [ -z "$EXPECTED_PATH" ]; then
    # Error message to stderr
    printf "${ICON_FAIL} $(RED)Error: No expected module path provided to the script.${NC}\n" >&2
    exit 1
fi

# Check if go.mod exists in the current directory
if [ ! -f "go.mod" ]; then
    # Error message to stderr
    printf "${ICON_FAIL} $(RED)Error: go.mod file not found in the current directory.${NC}\n" >&2
    exit 1
fi

# --- Logic ---

# Extract the actual module path (using grep + awk, confirmed to work for you)
# Ensure grep exits non-zero if no match, handle potential awk errors
ACTUAL_PATH=$(grep '^module ' go.mod | awk '{print $2}')
GREP_AWK_STATUS=$? # Capture exit status of the pipe (usually the last command, awk)

# Check if extraction worked (non-zero status or empty result)
if [ $GREP_AWK_STATUS -ne 0 ] || [ -z "$ACTUAL_PATH" ]; then
    # Error message to stderr
    printf "${ICON_FAIL} $(RED)Error: Could not find/parse 'module' line starting go.mod!${NC}\n" >&2
    exit 1
fi

# Compare actual path with expected path
if [ "$ACTUAL_PATH" == "$EXPECTED_PATH" ]; then
    # Paths match, exit successfully (status 0)
    # No success message here; Makefile will print it if script exits 0
    exit 0
else
    # Paths mismatch, print detailed error message to stderr and exit with failure (status 1)
    printf "${ICON_FAIL} $(RED)Error: Incorrect module path in go.mod!${NC}\n" >&2
    printf "        ${ICON_INFO} Expected: '%s'${NC}\n" "$EXPECTED_PATH" >&2
    printf "        ${ICON_INFO} Found   : '%s'${NC}\n" "$ACTUAL_PATH" >&2
    printf "        ${ICON_INFO} $(YELLOW)Please fix the 'module' directive in go.mod.${NC}\n" >&2
    exit 1
fi
