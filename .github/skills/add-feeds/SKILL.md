---
name: add-feeds
description: "Validate and add proxy-feed candidates from issue #21 or a supplied list to the correct sources/*.txt files. Use when asked to add, import, or merge proxy feeds into this repository; for discovering new repositories, use find-feeds instead."
user-invocable: true
---

# Add Feeds

Validate candidate proxy feeds against this repository's source format and parser support, then add suitable endpoints to `sources/`.

## When to Use

- The user asks to add or import feeds from issue #21.
- The user supplies proxy-feed repositories or direct feed URLs to integrate.
- The user asks to remove successfully imported repositories from the tracking issue.

Use `find-feeds` when the task is to discover new candidate repositories rather than integrate known candidates.

## Procedure

1. Check the current branch and `git status`. Start the work on a dedicated development branch based on the latest `origin/main` (for example, `git fetch origin main` followed by `git switch -c add-feeds/<short-topic> origin/main`). Do not work directly on `main`. If the worktree has existing changes, preserve them and isolate them from the new branch without discarding or accidentally committing them; if that cannot be done safely, stop and resolve the dirty worktree before proceeding.
2. Read the current `sources/*.txt` files before editing. Read `CONTRIBUTING.md` and inspect the relevant loader, parser, transformer, and nearby tests. Preserve existing and uncommitted user changes.
3. Read the requested issue body and comments, or the user's supplied candidate list. Identify candidate repositories and their likely feed paths. Deduplicate by repository and by exact source URL against the current source files; do not assume that every candidate in the issue is supported.
4. Verify each candidate endpoint directly. Prefer raw feed URLs over repository pages. Check the HTTP response, inspect a bounded sample of the body, and confirm that it contains actual proxy data rather than HTML, an empty file, documentation, or a dead link. Treat repository activity and candidate paths as leads, not proof that a feed works.
5. Match the feed's content to the current implementation:
   - Standard proxy URIs can use the default parser. If a feed mixes protocols, place it in `sources/auto.txt` only when each line carries a URI scheme that the parser can infer.
   - Bare `IP:PORT` lines without a declared protocol are registered as both HTTP and HTTPS candidates: add the identical feed URL to `sources/http.txt` and `sources/https.txt`, using `,,ColonURL` in both files. Do not infer a single protocol or place these lines in `sources/auto.txt`.
   - Space-separated host and port lines require `,,SpaceURL`.
   - Base64 content requires `,base64`; inspect the decoded output to ensure it is a supported proxy feed.
   - Clash YAML may use `,clash` only if the current transformer handles that document shape. Confirm behavior in code or tests instead of assuming support from the extension alone.
   - Use other transformers or parsers only when they are registered and their behavior matches the source. Skip unsupported formats unless the user also requested implementation support.
6. Add only reachable, non-empty feeds with a supported parser/transformer. Do not add mixed-scheme URI feeds to a protocol-specific file. For bare `IP:PORT` feeds, follow the HTTP-plus-HTTPS mapping in step 5. Keep the change to new source lines and preserve file conventions.
7. Put reviewed source entries in a TSV manifest with `protocol<TAB>source-entry` columns. Use the [add-feeds script](./scripts/add-feeds.sh) to check duplicates, fetch and preview feeds, and smoke-check transformer/parser format. The script is a preflight, not a substitute for inspecting the sample or confirming parser compatibility.
8. Run the script without `--apply` first. Review each sample and the planned source entry. Only after the formats and destinations are confirmed, pass `--apply` to append the validated entries. A failed row prevents the script from writing any entries. Duplicate URLs are checked per protocol file, so the same URL may be added once to each of `http.txt` and `https.txt`, while duplicates within either file are reported and skipped.
9. Run the narrowest relevant parser/transformer tests, then `go test ./...` when available. Run `git diff --check` and inspect the final diff to verify destinations, flags, duplicates, and that unrelated changes remain untouched.
10. Only when the user explicitly authorizes issue cleanup or explicitly requests an issue #21 task, fetch the latest issue #21 body and comments. Keep the tracking list as one table with exactly these columns: `Candidate repository / feed URL`, `Last pushed (UTC)`, `Candidate data path(s)`, and `Validation status / decision`; do not add numbering or a review-window column. Remove rows only for repositories whose feeds were fully imported. Keep partially imported and unimported candidates, updating only the final column with a concise result and specific reason (for example, `Unsupported: JSON format has no registered parser`, `Unavailable: HTTP 404`, `Empty: feed contains no proxy records`, `Deferred: bare IP:PORT feed was not validated for both HTTP and HTTPS`, or `Duplicate: overlaps an existing source`). Include the relevant format or transformer there only when it clarifies the decision. Preserve the last-pushed date, candidate paths, and all unrelated text; edit the body/comment in place, and verify imported names are absent and retained candidates have clear statuses. Do not fetch or edit issue #21 for unrelated feed-import requests. If a candidate came from outside issue #21, remove it only if its repository entry is present there.
11. Review the staged diff and stage only the intended files; never use `git add -A` or include unrelated user changes. Commit the feed changes on the development branch. **Submitting a pull request targeting `main` is required for every feed-import task:** push the branch and open the PR after validation without waiting for a separate request. Only omit the PR if the user explicitly asks not to submit one or asks to keep the changes local. The PR description must list the added repositories, destination files and parser/transformer settings, checks run, skipped candidates and reasons, and confirm which issue #21 entries were removed. Do not close issue #21 automatically. If branch pushing or PR creation is unavailable, report the blocker and do not claim it succeeded.
12. Report the development branch and PR link, added repositories and source configuration, checks performed, skipped candidates and reasons, and whether issue cleanup succeeded.

## Manifest and Run

Use one tab between the protocol filename stem and the complete source entry. Blank lines and lines beginning with `#` are ignored.

```text
auto	https://raw.githubusercontent.com/example/nodes/main/sub.txt,base64
http	https://raw.githubusercontent.com/example/proxies/main/http.txt,,ColonURL
```

By default, the script only checks and previews entries. It requires `curl` and `base64`. Requests are limited to 30 MiB by default; `ADD_FEEDS_TIMEOUT` and `ADD_FEEDS_MAX_BYTES` can adjust the request limits.

```bash
bash .github/skills/add-feeds/scripts/add-feeds.sh candidates.tsv
bash .github/skills/add-feeds/scripts/add-feeds.sh --apply candidates.tsv
```

## Guardrails

- A URL returning HTTP 200 is not sufficient: validate the response body and sample format.
- Do not claim proxy reachability or quality based only on successful feed retrieval; this workflow validates feed availability and parse compatibility.
- Avoid adding alternate endpoints that duplicate existing source URLs or source repositories without a distinct useful feed.
- Do not edit parser or transformer code just to accommodate a candidate unless the user asked for that broader change.
- Do not discard, overwrite, or include unrelated worktree changes in the development branch or PR.
- Issue #21 cleanup removes only successfully imported repository entries and must preserve the rest of the issue body/comment; never close the tracking issue as part of cleanup.
- In issue #21, never leave reviewed-but-unimported candidates looking pending: retain them and record a specific outcome and reason in the existing `Validation status / decision` column.
- When an untyped `IP:PORT` feed is selected, register it in both HTTP and HTTPS sources with `ColonURL`; never describe the missing scheme alone as a reason to skip it.