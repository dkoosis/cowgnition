#!/bin/bash

# Shell script to check file line counts with warning and failure thresholds.
# Called from Makefile to enforce coding standards on file lengths.
# /scripts/check_file_length.sh
# Usage: ./check_file_length.sh <warn_lines> <fail_lines> <file1> [file2] ...

# --- Configuration ---
# Color Codes (copied from Makefile for consistency)
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# --- Argument Parsing ---
if [ "$#" -lt 3 ]; then
  # Use printf for consistent escape sequence handling
  printf "%b\n" "Usage: $0 <warn_lines> <fail_lines> <file1> [file2] ..."
  printf "%b\n" "${RED}Error: Insufficient arguments.${NC}"
  exit 2 # Using 2 for usage errors
fi

WARN_LINES="$1"
FAIL_LINES="$2"
shift 2 # Remove the line counts, "$@" now contains only filenames

# Basic validation of line count arguments
if ! [[ "$WARN_LINES" =~ ^[0-9]+$ ]] || ! [[ "$FAIL_LINES" =~ ^[0-9]+$ ]]; then
    # Use printf for consistent escape sequence handling
    printf "%b\n" "${RED}Error: warn_lines ('$WARN_LINES') and fail_lines ('$FAIL_LINES') must be positive integers.${NC}"
    exit 2
fi

if [ "$WARN_LINES" -ge "$FAIL_LINES" ]; then
    # Use printf for consistent escape sequence handling
    printf "%b\n" "${RED}Error: warn_lines (${WARN_LINES}) must be less than fail_lines (${FAIL_LINES}).${NC}"
    exit 2
fi

# --- File Checking Logic ---
#printf "${BLUE}▶ Checking file lengths (warn > ${WARN_LINES}, fail > ${FAIL_LINES})...${NC}\n"
warnings_found=0
errors_found=0

# Process all files passed as arguments
for file in "$@"; do
    # Check if file exists and is readable
    if [ ! -f "$file" ] || [ ! -r "$file" ]; then
        printf "${YELLOW}⚠ WARNING: Skipping unreadable or non-existent file '$file'${NC}\n"
        continue # Skip to next file
    fi

    # Get line count robustly (using awk to strip potential whitespace from wc)
    lines=$(wc -l < "$file" | awk '{print $1}')

    # Check against limits (using arithmetic comparison)
    if [ "$lines" -gt "$FAIL_LINES" ]; then
        printf "${RED}✗ ERROR: File '$file' has $lines lines (exceeds FAIL limit of ${FAIL_LINES})${NC}\n"
        errors_found=1
    elif [ "$lines" -gt "$WARN_LINES" ]; then
        printf "${YELLOW}⚠ WARNING: File '$file' has $lines lines (exceeds WARN limit of ${WARN_LINES})${NC}\n"
        warnings_found=1
    fi
done

# --- Exit Status & Summary ---
if [ "$errors_found" -eq 1 ]; then
    printf "${RED}✗ File length error limit exceeded.${NC}\n"
    exit 1 # Exit with failure status
elif [ "$warnings_found" -eq 0 ]; then
    printf "${GREEN}✓ All files checked are within the specified line limits.${NC}\n"
    exit 0 # Exit with success status
else
    printf "${YELLOW}✓ Warnings issued for file length, but no errors.${NC}\n"
    exit 0 # Exit successfully even if there are warnings
fi
