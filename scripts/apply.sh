apply_patch() {
  local patch_file="$1"
  echo "--- Processing patch: $patch_file ---"

  # Check if patch file exists
  if [ ! -f "$patch_file" ]; then
    echo "Error: Patch file '$patch_file' not found."
    return 1
  fi

  echo "Applying patch '$patch_file' from starting directory..."

  # Apply the patch directly without changing directories
  if ! patch --ignore-whitespace --fuzz=3 -p1 < "$patch_file"; then
    patch_exit_code=$?
    echo "Error: Failed to apply patch '$patch_file' (exit code: $patch_exit_code)"
    return 1
  fi

  echo "Successfully applied patch '$patch_file'"
  return 0
}

# --- Main Script ---
# Store the directory where the script is run from
EXECUTION_DIR=$(pwd)

# If running from another directory, specify patch files with full path
if [ "$#" -gt 0 ]; then
    patch_files=("$@")
    echo "Processing specified patch file(s): ${patch_files[*]}"
else
    # No arguments provided, look for *.patch in EXECUTION_DIR
    cd "$EXECUTION_DIR"
    shopt -s nullglob
    patch_files=(*.patch)
    shopt -u nullglob

    if [ ${#patch_files[@]} -eq 0 ]; then
      echo "No *.patch files found in $EXECUTION_DIR"
      exit 0
    fi
    echo "Found ${#patch_files[@]} patch file(s) to process."
fi

# Process the patches
processed_count=0
failed_count=0

for file_to_patch in "${patch_files[@]}"; do
  # If not an absolute path, prepend the execution directory
  if [[ ! "$file_to_patch" = /* ]]; then
    file_to_patch="$EXECUTION_DIR/$file_to_patch"
  fi

  if apply_patch "$file_to_patch"; then
    processed_count=$((processed_count + 1))
  else
    failed_count=$((failed_count + 1))
    echo "--- Failed processing '$file_to_patch' ---"
  fi
  echo
done

# Summary output remains the same
