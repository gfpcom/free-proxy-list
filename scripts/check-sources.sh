#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SRC_DIR="$ROOT_DIR/sources"
CHECK_SCRIPT="$ROOT_DIR/scripts/check-file-duplicates.sh"

if [ ! -d "$SRC_DIR" ]; then
    echo "Error: No sources directory found at $SRC_DIR" >&2
    exit 1
fi

if [ ! -x "$CHECK_SCRIPT" ]; then
    echo "Error: check-file-duplicates.sh not found or not executable" >&2
    exit 1
fi

shopt -s nullglob
for f in "$SRC_DIR"/*.txt; do
    [ -f "$f" ] || continue
    echo "Checking $f..."
    if ! "$CHECK_SCRIPT" "$f"; then
        exit 1
    fi
done

echo "All files checked, no duplicates found."
exit 0
