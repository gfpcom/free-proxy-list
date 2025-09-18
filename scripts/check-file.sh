#!/usr/bin/env bash
# Usage: check-file.sh <file>
# Check duplicate lines in a file, exit on first duplicate found
# Returns: 0=no duplicates, 1=duplicate found

if [ $# -ne 1 ]; then
  echo "Usage: $0 <file>"
  exit 2
fi

# Process each line:
# 1. Remove \r (dos2unix)
# 2. Trim whitespace
# 3. Skip empty lines (including whitespace-only)
declare -A seen
while IFS= read -r line; do
  # Clean the line
  line=$(echo "$line" | tr -d '\r' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
  
  # Skip empty lines
  [ -z "$line" ] && continue
  
  # Check for duplicate
  if [ -n "${seen[$line]}" ]; then
    echo "$line"
    exit 1
  fi
  seen[$line]=1
done < "$1"

exit 0