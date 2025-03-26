#!/bin/bash

# ==============================================================================
# list_internal_deps.sh
#
# Description:
#   Lists internal Go package dependencies within the current Go module.
#   Outputs dependencies in the format "source/package -> target/package".
#   The common module path prefix is removed for brevity.
#
# Requires:
#   - Go toolchain ('go')
#   - jq ('jq') command-line JSON processor
#
# Usage:
#   ./list_internal_deps.sh [direct|all]
#
# Arguments:
#   direct  : Show only direct internal imports (Default).
#   all     : Show all internal dependencies (direct and transitive).
#
# Example:
#   ./list_internal_deps.sh       # Shows direct dependencies
#   ./list_internal_deps.sh all   # Shows all dependencies
# ==============================================================================

# --- Configuration & Argument Parsing ---
MODE="direct" # Default mode
if [[ "$1" == "all" ]]; then
  MODE="all"
  echo "# Mode: Listing ALL internal dependencies (direct & transitive)..." >&2
elif [[ -n "$1" && "$1" != "direct" ]]; then
  echo "Error: Invalid argument '$1'. Usage: $0 [direct|all]" >&2
  exit 1
elif [[ "$1" == "direct" ]]; then
   echo "# Mode: Listing DIRECT internal dependencies..." >&2
else
   echo "# Mode: Listing DIRECT internal dependencies (default)..." >&2
fi

# --- Dependency Checks ---
if ! command -v go &> /dev/null; then
    echo "Error: 'go' command not found. Please install the Go toolchain." >&2
    exit 1
fi

if ! command -v jq &> /dev/null; then
    echo "Error: 'jq' command not found. Please install jq." >&2
    echo "(e.g., brew install jq, sudo apt install jq, etc.)" >&2
    exit 1
fi

# --- Define Common JQ Filter Logic ---
# This part extracts module/package paths and creates shortened versions
JQ_FILTER_COMMON='
    .Module.Path as $mod |                   # Store module path (e.g., "github.com/dkoosis/cowgnition")
    ($mod + "/") as $mod_prefix |          # Create prefix to remove (e.g., "github.com/dkoosis/cowgnition/")
    .ImportPath as $pkg_full |             # Get full source package path
    ($pkg_full | sub("^"+$mod_prefix; "")) as $pkg_short # Create shortened source path (e.g., "cmd/server")
'

# --- Define JQ Filter based on Mode ---
if [[ "$MODE" == "direct" ]]; then
  # Filter for DIRECT imports
  JQ_FILTER="$JQ_FILTER_COMMON |
    .Imports[] |                           # Iterate through direct imports
    select(startswith($mod)) |             # Select only imports within our module
    . as $dep_full |                       # Get full dependency path
    ($dep_full | sub("^"+$mod_prefix; "")) as $dep_short | # Create shortened dependency path
    \"\\(\$pkg_short) -> \\(\$dep_short)\"   # Format output string
  "
else # MODE == "all"
  # Filter for ALL dependencies (direct & transitive)
  JQ_FILTER="$JQ_FILTER_COMMON |
    .Deps[] |                              # Iterate through ALL dependencies
    select(startswith($mod) and . != $pkg_full) | # Select internal deps, excluding self-references
    . as $dep_full |
    ($dep_full | sub("^"+$mod_prefix; "")) as $dep_short |
    \"\\(\$pkg_short) -> \\(\$dep_short)\"
  "
fi

# --- Execute the command ---
# Use 'set -o pipefail' to ensure script exits if 'go list' fails
set -o pipefail

go list -json ./... | jq -r "$JQ_FILTER" | sort -u

# Check the exit status of the pipe
PIPE_STATUS=${PIPESTATUS[0]}
if [[ $PIPE_STATUS -ne 0 ]]; then
  echo "Error: 'go list' command failed with status $PIPE_STATUS." >&2
  exit $PIPE_STATUS
fi

exit 0
