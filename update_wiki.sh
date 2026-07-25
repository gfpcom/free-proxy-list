#!/usr/bin/env bash

# Build wiki Home.md with numeric total and list links
total=$(cat list/*.txt 2>/dev/null | wc -l | tr -d ' ')
[ -n "$total" ] || total=0
cat > ../wiki/Home.md << EOF
# Proxy Lists

**Total Proxies:** $total

This wiki contains the latest protocol-specific proxy lists under the `lists/` directory.

## Available Lists

$(for f in list/*.txt; do [ -f "$f" ] && echo "* [$(basename "$f")](lists/$(basename "$f"))"; done)
EOF
