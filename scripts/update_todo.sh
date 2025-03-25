#!/bin/bash
# scripts/update_todo.sh

# Create temp file for build output
build_output=$(mktemp)

# Only capture the build errors for documentation without disrupting normal pretty output
{
  # Run a separate, silent build just to capture errors
  echo "Capturing build errors for documentation..."
  go build -v ./... 2>&1 || true
  go vet ./... 2>&1 || true

  # Check for linters
  if command -v golangci-lint &> /dev/null; then
    golangci-lint run --timeout=1m 2>&1 || true
  fi
} > "$build_output" 2>/dev/null

TODO_PATH="docs/TODO.md"

# Update TODO if there were errors
if [[ -s "$build_output" ]]; then
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
fi

# Update tree.txt
if command -v tree &> /dev/null; then
  tree -F > "scripts/tree.txt" 2>/dev/null
fi

# Clean up
rm "$build_output"

# Exit successfully
exit 0
