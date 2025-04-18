#!/bin/bash

# Shell script to check file line counts with warning and failure thresholds.
# Sorts output by line count (descending) and formats as requested.
# /scripts/check_file_length.sh (Modified)
# Usage: ./check_file_length.sh <warn_lines> <fail_lines> <file1> [file2] ...

# --- Configuration ---
# Color Codes
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Icons (Includes color reset after icon)
ICON_WARN=$(printf "${YELLOW}⚠${NC}") # Yellow Warning Triangle
ICON_FAIL=$(printf "${RED}✗${NC}")   # Red X Mark
ICON_OK=$(printf "${GREEN}✓${NC}")   # Green Check Mark
ICON_INFO=$(printf "${BLUE}ℹ${NC}")  # Blue Info Icon

# --- Argument Parsing ---
if [ "$#" -lt 3 ]; then
  printf "%b\n" "Usage: $0 <warn_lines> <fail_lines> <file1> [file2] ..."
  printf "%b\n" "${RED}Error: Insufficient arguments.${NC}"
  exit 2
fi

WARN_LINES="$1"
FAIL_LINES="$2"
shift 2 # Remove the line counts, "$@" now contains only filenames

# Basic validation of line count arguments
if ! [[ "$WARN_LINES" =~ ^[0-9]+$ ]] || ! [[ "$FAIL_LINES" =~ ^[0-9]+$ ]]; then
    printf "%b\n" "${RED}Error: warn_lines ('$WARN_LINES') and fail_lines ('$FAIL_LINES') must be positive integers.${NC}"
    exit 2
fi

if [ "$WARN_LINES" -ge "$FAIL_LINES" ]; then
    printf "%b\n" "${RED}Error: warn_lines (${WARN_LINES}) must be less than fail_lines (${FAIL_LINES}).${NC}"
    exit 2
fi

# --- File Checking & Data Collection ---
warning_count=0 # Counter for warnings
error_count=0   # Counter for errors
output_data="" # Variable to store lines for sorting

# Indentation for detail lines
INDENT="   "

# Process all files passed as arguments
for file in "$@"; do
    if [ ! -f "$file" ] || [ ! -r "$file" ]; then
        printf "%s%s Skipping unreadable or non-existent file %s\n" "$INDENT" "$ICON_WARN" "$file" >&2
        continue
    fi

    lines=$(wc -l < "$file" | awk '{print $1}')
    filename=$(basename "$file")

    status="ok"
    icon=""

    if [ "$lines" -gt "$FAIL_LINES" ]; then
        status="error"
        icon="$ICON_FAIL"
        ((error_count++)) # Increment error count
    elif [ "$lines" -gt "$WARN_LINES" ]; then
        status="warning"
        icon="$ICON_WARN"
        ((warning_count++)) # Increment warning count
    fi

    # If status is not "ok", store data for later sorting and printing
    # Format: lines<TAB>status<TAB>icon<TAB>filename<TAB>filepath
    if [ "$status" != "ok" ]; then
        printf -v line_data "%s\t%s\t%s\t%s\t%s\n" "$lines" "$status" "$icon" "$filename" "$file"
        output_data+="$line_data"
    fi
done

# --- Sorting & Output ---

# Check if there's anything to print
if [ -n "$output_data" ]; then
  # Sort the collected data numerically (1st column), reverse (descending)
  # Use process substitution to feed sorted data into the loop
  # IFS=$'\t' ensures tab is the delimiter for read
  while IFS=$'\t' read -r count status_code icon_code fname fpath; do
      # Add check to prevent printing for empty lines read from the sorted input
      if [ -n "$count" ]; then
        # Print in the desired format: Icon Count lines: Filename, (Full Path)
        printf "%s%s %s lines: %s, (%s)\n" "$INDENT" "$icon_code" "$count" "$fname" "$fpath"
      fi
  done < <(echo -e "$output_data" | sort -t$'\t' -k1,1nr)
fi

# --- Exit Status & Summary (Indented) ---
if [ "$error_count" -gt 0 ]; then
    # Icon included in text, whole line colored red, include error count
    printf "%s%b✗ %d errors: File length error limit exceeded.%b\n" "$INDENT" "$RED" "$error_count" "$NC"
    exit 1 # Exit with failure status
elif [ "$warning_count" -gt 0 ]; then
    # Icon included in text, whole line colored yellow, include warning count
    printf "%s%b⚠ %d warnings issued for file length, but no errors.%b\n" "$INDENT" "$YELLOW" "$warning_count" "$NC"
    exit 0 # Exit successfully even if there are warnings
else
    # No output on success if no warnings/errors, handled by Makefile target
    exit 0 # Exit with success status
fi