---
name: find-feeds
description: "Discover new GitHub repositories that publish free proxy lists or VPN node subscriptions. Use when the user says execute find-feeds, asks to find recent proxy feeds, or asks to register new proxy-source repositories in the source issue."
user-invocable: true
---

# Find Feeds

Discover candidate repositories for this project's `sources/` lists and register them in the existing source-tracking issue. Use two complementary discovery lanes: the reusable scripted search for known feed patterns, and AI-led heuristic mining for useful sources the script's query list does not anticipate. Do not treat the script's query list as the boundary of discovery.

## Procedure

1. Establish the most recent 30 UTC calendar days, including today and the preceding 29 days. Read the current `sources/` files and source-tracking issue body/comments. Deduplicate repository identities case-insensitively and exclude anything already present in either place.
2. **AI heuristic lane — unanticipated patterns.** Inspect the script's query list, then independently explore GitHub beyond those fixed queries; do not just repeat its searches. Use repository and code search with broad, changing terms, inspect descriptions/topics/READMEs and recent commit context, follow links to upstream publishers and aggregators, and search code or filenames for feed artifacts. Pivot searches from each useful hit: distinctive filenames, protocol labels, subscription URL patterns, config keys, and publishers mentioned in documentation. Look for less obvious naming, formats, protocols, language variants, and publishing styles, such as sing-box, Hysteria/Hy2, TUIC, WireGuard, subscription converters/providers, generated daily lists, raw endpoints, and protocol-neutral node collections. Treat each search result as a lead, not proof. Save discovered GitHub repository identities (one `owner/repo` or GitHub repository URL per line) to a temporary candidate file; do not add that file to the repository.
3. **Scripted lane — known patterns and shared validation.** Run the reusable search script with the AI-discovered candidate file. It checks those candidates first, then searches for known proxy-list, protocol, subscription, feed, pool, and aggregator patterns to fill any remaining room under the shared candidate limit. Use `--candidates-only` only when running the AI lane on its own. The script is the common final gate for both lanes: exclude forks, archived repositories, existing source/issue entries, stale pushes, inaccessible repositories, and trees without likely data files (`txt`, `yaml`, `yml`, or `json`) whose paths suggest proxy, node, subscription, protocol, or aggregated config data. Exclude workflow, docs, fixture, and dependency metadata files.
5. Confirm each retained repository's latest push is within the date window. Gather the repository, last-push date, and up to three candidate data paths.
6. By default, register at most 50 total candidates from both lanes in the existing table in issue #21 in the current GitHub repository. Do not add feeds to `sources/`; the issue is a handoff for later reachability, content-format, duplicate, and parser/transformer validation. Never create a comment or pull request for discovery results.
   Use exactly these columns: `Candidate repository / feed URL`, `Last pushed (UTC)`, `Candidate data path(s)`, and `Validation status / decision`. Do not add a row number, a review-window column, or a separate table for each search. Format push dates as `YYYY-MM-DD`, list up to three candidate paths, and mark every new row `Pending: candidate discovered; verify reachability, content format, duplicates, and parser/transformer support before import.`
7. If no eligible candidates are found, do not update the issue body; report that none were found and the blocker. If GitHub search or repository inspection is unavailable, report the specific blocker rather than implying the search was exhaustive.

## Run

Use [the discovery script](./scripts/find-feeds.sh) for the planned search:

```bash
bash .github/skills/find-feeds/scripts/find-feeds.sh
```

For a full discovery run, save repositories found during AI-led exploration to a temporary file, then validate those first and let the planned search fill any remaining slots:

```bash
bash .github/skills/find-feeds/scripts/find-feeds.sh \
  --candidates-file /path/to/ai-feed-candidates.txt
```

Candidate files accept one GitHub `owner/repo` identity or `https://github.com/owner/repo` URL per line; blank lines and lines beginning with `#` are ignored. To process only AI-discovered candidates:

```bash
bash .github/skills/find-feeds/scripts/find-feeds.sh \
  --candidates-file /path/to/ai-feed-candidates.txt \
  --candidates-only
```

Requirements: authenticated `gh`, `jq`, and a checkout of this repository. `FIND_FEEDS_ISSUE` overrides the default issue number; `FIND_FEEDS_LIMIT` sets a total limit from 1 to 50; `FIND_FEEDS_CANDIDATES_FILE` supplies a candidate file without the CLI option. Pass `--dry-run` to print the complete proposed issue body without updating it. Running the script without a candidate file performs only the planned search; a full discovery run also requires the independent AI-led GitHub exploration described above.

Treat results as leads, not validated proxy endpoints. Report the search cutoff in the completion summary, not as a table column. Candidate paths belong in the table; clearly state that feeds still need reachability, format, duplication, and parser/transformer checks.
