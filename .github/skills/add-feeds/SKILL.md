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

1. Read the current `sources/*.txt` files before editing. Read `CONTRIBUTING.md` and inspect the relevant loader, parser, transformer, and nearby tests. Preserve existing and uncommitted user changes.
2. Read the requested issue body and comments, or the user's supplied candidate list. Identify candidate repositories and their likely feed paths. Deduplicate by repository and by exact source URL against the current source files; do not assume that every candidate in the issue is supported.
3. Verify each candidate endpoint directly. Prefer raw feed URLs over repository pages. Check the HTTP response, inspect a bounded sample of the body, and confirm that it contains actual proxy data rather than HTML, an empty file, documentation, or a dead link. Treat repository activity and candidate paths as leads, not proof that a feed works.
4. Match the feed's content to the current implementation:
   - Standard proxy URIs can use the default parser. If a feed mixes protocols, place it in `sources/auto.txt` only when each line carries a URI scheme that the parser can infer.
   - Bare `IP:PORT` lines require the protocol-specific source file and `,,ColonURL`.
   - Space-separated host and port lines require `,,SpaceURL`.
   - Base64 content requires `,base64`; inspect the decoded output to ensure it is a supported proxy feed.
   - Clash YAML may use `,clash` only if the current transformer handles that document shape. Confirm behavior in code or tests instead of assuming support from the extension alone.
   - Use other transformers or parsers only when they are registered and their behavior matches the source. Skip unsupported formats unless the user also requested implementation support.
5. Add only reachable, non-empty feeds with identifiable protocol/format and a supported parser/transformer. Do not add a mixed or ambiguous feed to a protocol-specific file. Keep the change to new source lines and preserve file conventions.
6. Run the narrowest relevant parser/transformer tests, then `go test ./...` when available. Run `git diff --check` and inspect the final diff to verify destinations, flags, duplicates, and that unrelated changes remain untouched.
7. If the user explicitly asks to clean up issue #21, or cleanup is part of the requested task, remove only the entries for repositories whose feeds were successfully added. Fetch the latest issue/comment content, edit the original body or comment in place, preserve all unrelated text, and verify the removed repository names no longer appear there. Do not delete candidates that were skipped or unsupported. Do not modify the issue without explicit user authorization.
8. Report each added repository, destination source file, parser/transformer configuration, checks performed, skipped candidates and reasons, and whether issue cleanup succeeded.

## Guardrails

- A URL returning HTTP 200 is not sufficient: validate the response body and sample format.
- Do not claim proxy reachability or quality based only on successful feed retrieval; this workflow validates feed availability and parse compatibility.
- Avoid adding alternate endpoints that duplicate existing source URLs or source repositories without a distinct useful feed.
- Do not edit parser or transformer code just to accommodate a candidate unless the user asked for that broader change.