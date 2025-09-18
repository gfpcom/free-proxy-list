#!/usr/bin/env bash
# Usage: check-sources.sh
# Check all txt files under sources/ for duplicate lines

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
CHECK_SCRIPT="$ROOT_DIR/scripts/check-file.sh"

if [ ! -x "$CHECK_SCRIPT" ]; then
  echo "Error: $CHECK_SCRIPT not found or not executable"
  exit 1
fi

for f in "$ROOT_DIR/sources"/*.txt; do
  [ -f "$f" ] || continue
  filename=$(basename "$f")
  
  duplicate=$("$CHECK_SCRIPT" "$f")
  if [ $? -eq 1 ]; then
    echo "ERROR: duplicate found in $filename:"
    echo "$duplicate"
    exit 1
  fi
done

exit 0
