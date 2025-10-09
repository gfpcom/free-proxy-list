#!/bin/bash

# Build wiki Home.md with numeric total and list links
total=$(cat list/*.txt 2>/dev/null | wc -l | tr -d ' ')
[ -n "$total" ] || total=0
cat > wiki/Home.md << EOF
# Proxy Lists

**Total Proxies:** $total

This wiki contains the latest protocol-specific proxy lists under the `lists/` directory.

## Available Lists

$(for f in list/*.txt; do [ -f "$f" ] && echo "* [$(basename "$f")](lists/$(basename "$f"))"; done)
EOF

# Replace wiki history with a single commit and push
cd ../wiki
rm -rf .git
git init
git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git remote add origin https://github.com/gfpcom/free-proxy-list.wiki.git
git add .
if [ -n "$(git status --porcelain)" ]; then \
    timestamp=$(date -u +"%Y-%m-%d %H:%M:%S UTC"); \
    git commit -m "Update proxy lists - $timestamp"; \
    git branch -M master; \
    git push --force origin master; \
fi