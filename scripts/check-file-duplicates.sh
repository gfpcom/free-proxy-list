#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 1 ]; then
    echo "Usage: $0 <file_path>" >&2
    exit 1
fi

file="$1"
if [ ! -f "$file" ]; then
    echo "Error: File not found: $file" >&2
    exit 1
fi

declare -A seen
while IFS= read -r line || [ -n "$line" ]; do
    trimmed=$(echo "$line" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
    [ -z "$trimmed" ] && continue
    if [ -n "${seen[$trimmed]:-}" ]; then
        echo "ERROR: duplicate line found in $file:" >&2
        echo "$trimmed" >&2
        exit 1
    fi
    seen[$trimmed]=1
done < "$file"

exit 0
