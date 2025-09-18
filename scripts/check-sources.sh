#!/usr/bin/env bash#!/usr/bin/env bash

# Usage: check-sources.shset -euo pipefail

# Check all txt files under sources/ for duplicate lines

# Stop on first file containing duplicatesROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

SRC_DIR="$ROOT_DIR/sources"

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"CHECK_SCRIPT="$ROOT_DIR/scripts/check-file-duplicates.sh"

CHECKER="$ROOT_DIR/scripts/check-file.sh"

if [ ! -d "$SRC_DIR" ]; then

# Ensure checker script exists and is executable    echo "Error: No sources directory found at $SRC_DIR" >&2

if [ ! -x "$CHECKER" ]; then    exit 1

  echo "Error: $CHECKER not found or not executable"fi

  exit 1

fiif [ ! -x "$CHECK_SCRIPT" ]; then

    echo "Error: check-file-duplicates.sh not found or not executable" >&2

for f in "$ROOT_DIR/sources"/*.txt; do    exit 1

  [ -f "$f" ] || continuefi

  

  # Get filename without pathshopt -s nullglob

  filename=$(basename "$f")for f in "$SRC_DIR"/*.txt; do

      [ -f "$f" ] || continue

  # Check this file    echo "Checking $f..."

  duplicate=$("$CHECKER" "$f")    if ! "$CHECK_SCRIPT" "$f"; then

  if [ $? -eq 1 ]; then        exit 1

    echo "ERROR: duplicate found in $filename:"    fi

    echo "$duplicate"done

    exit 1

  fiecho "All files checked, no duplicates found."

doneexit 0


exit 0