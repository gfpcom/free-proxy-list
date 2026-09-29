---
name: find-feeds
description: "Discover new GitHub repositories that publish free proxy lists or VPN node subscriptions. Use when the user says execute find-feeds, asks to find recent proxy feeds, or asks to register new proxy-source repositories in the source issue."
user-invocable: true
---

# Find Feeds

Discover candidate repositories for this project's `sources/` lists and register them in the existing source-tracking issue.

## Procedure

1. Read the current `sources/` files and the source-tracking issue body and comments. Deduplicate repository identities case-insensitively.
2. Search GitHub for public proxy lists, HTTP/SOCKS feeds, V2Ray/Xray configs, and free-node subscriptions pushed within the last UTC calendar month.
3. Exclude forks, archived repositories, repositories already in `sources/`, and repositories already mentioned in the issue.
4. Inspect each candidate's repository tree. Keep candidates with likely data files (`txt`, `yaml`, `yml`, or `json`) whose paths suggest proxy, node, subscription, protocol, or aggregated config data. Exclude workflow, docs, fixture, and dependency metadata files.
5. Confirm each repository's latest push is within the date window. Gather the repository, last-push date, and candidate data paths.
6. By default, require 50 candidates and add them as a new comment to issue #21 in the current GitHub repository. Do not add feeds to `sources/`; the issue is a handoff for later extraction and parser/transformer validation.
7. If GitHub search/API results cannot provide the requested number, do not post a partial list. Report the count found and the blocker.

## Run

Use [the discovery script](./scripts/find-feeds.sh):

```bash
bash .github/skills/find-feeds/scripts/find-feeds.sh
```

Requirements: authenticated `gh`, `jq`, and a checkout of this repository. `FIND_FEEDS_ISSUE` overrides the default issue number; `FIND_FEEDS_LIMIT` overrides the default of 50. Pass `--dry-run` to print the proposed issue comment without posting it.

Treat results as leads, not validated proxy endpoints. Include the search cutoff and candidate file paths, and clearly state that feeds still need reachability, format, duplication, and parser/transformer checks.
