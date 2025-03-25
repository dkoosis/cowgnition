#!/bin/bash
# scripts/update_todo.sh

# Create temp files for build output and script output
build_output=$(mktemp)
script_output=$(mktemp)

# Run build commands and capture output without showing it in the terminal
{
  echo "Running build and checks..."
  go build -v ./... 2>&1 || true
  go vet ./... 2>&1 || true

  # Check for golangci-lint and run if available
  if command -v golangci-lint &> /dev/null; then
    golangci-lint run --timeout=1m 2>&1 || true
  else
    echo "golangci-lint not found, skipping lint checks"
  fi
} > "$build_output" 2>&1

TODO_PATH="docs/TODO.md"

# Check if there were any errors/warnings
if [[ -s "$build_output" ]]; then
  {
    echo "Updating $TODO_PATH with build errors..."

    # Create temp file for new TODO.md
    new_todo=$(mktemp)

    # Write build errors section with TOP PRIORITY designation
    echo "## TOP PRIORITY: Latest Build Errors ($(date))" > "$new_todo"
    echo '```' >> "$new_todo"
    cat "$build_output" >> "$new_todo"
    echo '```' >> "$new_todo"
    echo "" >> "$new_todo"

    # Append rest of original TODO.md, excluding any previous build errors section
    if grep -q "^## TOP PRIORITY: Latest Build Errors" "$TODO_PATH"; then
      sed -n '/^## TOP PRIORITY: Latest Build Errors/,/^## [^T]/!p; /^## [^T]/,$p' "$TODO_PATH" >> "$new_todo"
    else
      cat "$TODO_PATH" >> "$new_todo"
    fi

    # Replace original TODO.md
    mv "$new_todo" "$TODO_PATH"
    echo "Build errors found and added to TODO.md"
  } > "$script_output" 2>&1
else
  echo "No build errors to add to $TODO_PATH" > "$script_output"
fi

# Update tree.txt without showing output
{
  if command -v tree &> /dev/null; then
    echo "Updating tree.txt..."
    tree -F > "scripts/tree.txt"
  else
    echo "Tree command not found. Install with 'brew install tree' or 'apt-get install tree'"
  fi
} >> "$script_output" 2>&1

# Clean up
rm "$build_output"
rm "$script_output"

# Return success status for the Makefile
exit 0
