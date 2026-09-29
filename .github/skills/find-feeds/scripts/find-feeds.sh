#!/usr/bin/env bash
set -euo pipefail

limit="${FIND_FEEDS_LIMIT:-50}"
issue_number="${FIND_FEEDS_ISSUE:-21}"
dry_run=false

for argument in "$@"; do
	case "$argument" in
		--dry-run) dry_run=true ;;
		*) printf 'Unknown argument: %s\n' "$argument" >&2; exit 2 ;;
	esac
done

if ! [[ "$limit" =~ ^[1-9][0-9]*$ ]]; then
	printf 'FIND_FEEDS_LIMIT must be a positive integer.\n' >&2
	exit 2
fi
if ! [[ "$issue_number" =~ ^[1-9][0-9]*$ ]]; then
	printf 'FIND_FEEDS_ISSUE must be a positive integer.\n' >&2
	exit 2
fi

command -v gh >/dev/null
command -v jq >/dev/null
gh auth status >/dev/null

repo_slug="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
cutoff="$(date -u -d '1 month ago' +%F)"
source_dir="$(git rev-parse --show-toplevel)/sources"
issue_json="$(gh issue view "$issue_number" --repo "$repo_slug" --json state,body,comments)"
issue_state="$(jq -r .state <<< "$issue_json")"
if [[ "$issue_state" != OPEN ]]; then
	printf 'Source issue #%s is not open; refusing to post.\n' "$issue_number" >&2
	exit 1
fi

declare -A seen_repositories=()

extract_repositories() {
	grep -Eho 'https?://(raw\.githubusercontent\.com|github\.com)/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+' \
		| sed -E 's#https?://(raw\.githubusercontent\.com|github\.com)/([^/]+)/([^/]+).*#\2/\3#' \
		| sed -E 's/\.git$//' \
		| tr '[:upper:]' '[:lower:]' \
		| sort -u || true
}

while IFS= read -r repository; do
	[[ -n "$repository" ]] && seen_repositories["$repository"]=1
done < <(find "$source_dir" -maxdepth 1 -type f -print0 | xargs -0 grep -hEo 'https?://(raw\.githubusercontent\.com|github\.com)/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+' | extract_repositories)

while IFS= read -r repository; do
	[[ -n "$repository" ]] && seen_repositories["$repository"]=1
done < <(jq -r '[.body, (.comments[].body)] | join("\n")' <<< "$issue_json" | extract_repositories)

queries=(
	"proxy list"
	"free proxy"
	"v2ray subscription"
	"v2ray nodes"
	"free nodes clash"
	"http socks proxy list"
	"proxy collector"
	"free v2ray config"
)

search_results=''
for query in "${queries[@]}"; do
	result="$(gh search repos "$query pushed:>=$cutoff fork:false archived:false" \
		--sort updated --limit 100 \
		--json fullName,description,pushedAt,isFork,isArchived)"
	search_results+="$result"$'\n'
done

candidate_repositories="$(jq -sr --arg cutoff "$cutoff" '
	add | unique_by(.fullName | ascii_downcase)
	| map(select(.isFork == false and .isArchived == false and .pushedAt[:10] >= $cutoff))
	| map(select((.description // "" | test("bot[- _]?ips?|ips?[^.]{0,30}used[^.]{0,20}bots|blocklists?|blacklists?|deny lists?|proxy ips? addresses? used by bots"; "i")) | not))
	| sort_by(.pushedAt) | reverse | .[].fullName
' <<< "$search_results")"

rows=()
while IFS= read -r repository; do
	[[ -z "$repository" ]] && continue
	key="$(tr '[:upper:]' '[:lower:]' <<< "$repository")"
	[[ -n "${seen_repositories[$key]:-}" ]] && continue

	metadata="$(gh api "repos/$repository" --jq '[.pushed_at, .fork, .archived] | @tsv' 2>/dev/null || true)"
	IFS=$'\t' read -r pushed_at is_fork is_archived <<< "$metadata"
	[[ -z "${pushed_at:-}" || "${pushed_at%%T*}" < "$cutoff" ]] && continue
	[[ "$is_fork" == true || "$is_archived" == true ]] && continue

	tree="$(gh api "repos/$repository/git/trees/HEAD?recursive=1" 2>/dev/null || true)"
	[[ -z "$tree" ]] && continue
	paths="$(jq -r '
		[.tree[] | select(.type == "blob") | .path
		 | select(test("\\.(txt|yaml|yml|json)$"; "i"))
		 | select(test("(^|/)(\\.github|docs?|tests?|fixtures?|node_modules)(/|$)"; "i") | not)
		 | select(test("(^|/)(package(-lock)?|tsconfig|manifest|version|info|metadata|stats|badges?)(\\.[^/]*)?\\.(json|yaml|yml)$"; "i") | not)
		 | select(test("(^|/)(configs?)(\\.prod)?\\.(yaml|yml|json)$"; "i") | not)
		 | select(test("(blocklist|blacklist|denylist|bot.?ips?)"; "i") | not)
		 | select(test("(^|/)(protocols|countries)/[^/]+\\.(txt|yaml|yml|json)$"; "i") or test("(^|/)(all|sub|subscriptions|proxies?|nodes?|http|https|socks|vless|vmess|trojan|shadowsocks|clash|mix|raw|result|servers?)([-_.][^/]*)?\\.(txt|yaml|yml|json)$"; "i") or test("[^/]*(proxy|proxies|node|nodes|subscription|sub|http|https|socks|vless|vmess|trojan|shadowsocks|clash|mix|raw|result|server)[^/]*\\.(txt|yaml|yml|json)$"; "i"))]
		| unique | .[0:3] | join(", ")
	' <<< "$tree")"
	[[ -z "$paths" ]] && continue

	row_number="$((${#rows[@]} + 1))"
	rows+=("| $row_number | https://github.com/$repository | ${pushed_at%%T*} | \`$paths\` |")
	seen_repositories["$key"]=1
	if (( ${#rows[@]} >= limit )); then
		break
	fi
done <<< "$candidate_repositories"

if (( ${#rows[@]} < limit )); then
	printf 'Found %s of %s requested candidates since %s; no issue comment was posted.\n' \
		"${#rows[@]}" "$limit" "$cutoff" >&2
	exit 1
fi

body="## Additional $limit repository leads ($(date -u +%F))

Search window: $cutoff through $(date -u +%F). Each repository was checked for a recent push, absence from sources/ and this issue, and likely text/YAML/JSON data files. Paths are extraction starting points; verify reachability, content format, duplication, and parser/transformer needs before merging.

| # | Repository | Last pushed | Candidate data path(s) |
|---:|---|---|---|
$(printf '%s\n' "${rows[@]}")

These are discovery candidates only; no entries have been merged into sources/."

if [[ "$dry_run" == true ]]; then
	printf '%s\n' "$body"
else
	gh issue comment "$issue_number" --repo "$repo_slug" --body "$body"
fi
