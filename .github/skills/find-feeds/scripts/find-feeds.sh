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
window_end="$(date -u +%F)"
if date -u -v-29d +%F >/dev/null 2>&1; then
	window_start="$(date -u -v-29d +%F)"
else
	window_start="$(date -u -d "$window_end -29 days" +%F)"
fi
source_dir="$(git rev-parse --show-toplevel)/sources"
issue_json="$(gh issue view "$issue_number" --repo "$repo_slug" --json state,body,comments)"
issue_state="$(jq -r .state <<< "$issue_json")"
issue_body="$(jq -r '.body // ""' <<< "$issue_json")"
if [[ "$issue_state" != OPEN ]]; then
	printf 'Source issue #%s is not open; refusing to update it.\n' "$issue_number" >&2
	exit 1
fi

declare -A seen_repositories=()

extract_repositories() {
	grep -Eho 'https?://(www\.)?(raw\.githubusercontent\.com|github\.com)/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+|(^|[^A-Za-z0-9_./:-])[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+' \
		| sed -E 's#https?://(www\.)?(raw\.githubusercontent\.com|github\.com)/##; s#^[^A-Za-z0-9_.-]+##' \
		| sed -E 's#^([^/]+)/([^/]+).*#\1/\2#' \
		| sed -E 's/\.git$//' \
		| tr '[:upper:]' '[:lower:]' \
		| sort -u || true
}

fetch_tree_paths() {
	local repository="$1"
	local initial_tree tree_sha tree_json entry_type entry_sha entry_path prefix
	local -a tree_shas prefixes paths

	initial_tree="$(cat)"
	if ! jq -e '(.tree | type) == "array"' >/dev/null <<< "$initial_tree"; then
		return 1
	fi
	if jq -e '.truncated != true' >/dev/null <<< "$initial_tree"; then
		jq -c '[.tree[] | select(.type == "blob") | .path]' <<< "$initial_tree"
		return
	fi

	tree_shas=(HEAD)
	prefixes=("")
	paths=()
	while (( ${#tree_shas[@]} > 0 )); do
		tree_sha="${tree_shas[0]}"
		prefix="${prefixes[0]}"
		tree_shas=("${tree_shas[@]:1}")
		prefixes=("${prefixes[@]:1}")
		tree_json="$(gh api "repos/$repository/git/trees/$tree_sha" 2>/dev/null || true)"
		[[ -z "$tree_json" ]] && continue
		if ! jq -e '(.tree | type) == "array"' >/dev/null <<< "$tree_json"; then
			return 1
		fi
		if [[ "$(jq -r '.truncated // false' <<< "$tree_json")" == true ]]; then
			return 1
		fi

		while IFS=$'\t' read -r entry_type entry_sha entry_path; do
			if [[ "$entry_type" == tree ]]; then
				tree_shas+=("$entry_sha")
				prefixes+=("$prefix$entry_path/")
			elif [[ "$entry_type" == blob ]]; then
				paths+=("$prefix$entry_path")
			fi
		done < <(jq -r '.tree[] | [.type, .sha, .path] | @tsv' <<< "$tree_json")
	done

	printf '%s\0' "${paths[@]}" | jq -Rsc 'split("\u0000") | map(select(length > 0))'
}

while IFS= read -r repository; do
	[[ -n "$repository" ]] && seen_repositories["$repository"]=1
done < <(find "$source_dir" -maxdepth 1 -type f -exec cat {} + | extract_repositories)

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
	result="$(gh search repos "$query pushed:$window_start..$window_end fork:false archived:false" \
		--sort updated --limit 100 \
		--json fullName,description,pushedAt,isFork,isArchived)"
	search_results+="$result"$'\n'
done

candidate_repositories="$(jq -sr --arg window_start "$window_start" --arg window_end "$window_end" '
	add | unique_by(.fullName | ascii_downcase)
	| map(select(.isFork == false and .isArchived == false and .pushedAt[:10] >= $window_start and .pushedAt[:10] <= $window_end))
	| map(select((.description // "" | test("bot[- _]?ips?|ips?[^.]{0,30}used[^.]{0,20}bots|blocklists?|blacklists?|deny lists?|proxy ips? addresses? used by bots"; "i")) | not))
	| sort_by(.pushedAt) | reverse | .[].fullName
' <<< "$search_results")"

rows=()
repositories=()
while IFS= read -r repository; do
	[[ -z "$repository" ]] && continue
	key="$(tr '[:upper:]' '[:lower:]' <<< "$repository")"
	[[ -n "${seen_repositories[$key]:-}" ]] && continue

	metadata="$(gh api "repos/$repository" --jq '[.pushed_at, .fork, .archived] | @tsv' 2>/dev/null || true)"
	IFS=$'\t' read -r pushed_at is_fork is_archived <<< "$metadata"
	pushed_date="${pushed_at%%T*}"
	[[ -z "${pushed_at:-}" || "$pushed_date" < "$window_start" || "$pushed_date" > "$window_end" ]] && continue
	[[ "$is_fork" == true || "$is_archived" == true ]] && continue

	tree="$(gh api "repos/$repository/git/trees/HEAD?recursive=1" 2>/dev/null || true)"
	[[ -z "$tree" ]] && continue
	tree_paths="$(fetch_tree_paths "$repository" <<< "$tree")" || continue
	paths="$(jq -r '
		map(select(
			(test("\\.(txt|yaml|yml|json)$"; "i"))
			and (test("(^|/)(\\.github|docs?|tests?|fixtures?|node_modules)(/|$)"; "i") | not)
			and (test("(^|/)(package(-lock)?|tsconfig|manifest|version|info|metadata|stats|badges?)(\\.[^/]*)?\\.(json|yaml|yml)$"; "i") | not)
			and (test("(^|/)(configs?)(\\.prod)?\\.(yaml|yml|json)$"; "i") | not)
			and (test("(blocklist|blacklist|denylist|bot.?ips?)"; "i") | not)
			and (test("(^|/)(protocols|countries)/[^/]+\\.(txt|yaml|yml|json)$"; "i") or test("(^|/)(all|sub|subscriptions|proxies?|nodes?|http|https|socks|vless|vmess|trojan|shadowsocks|clash|mix|raw|result|servers?)([-_.][^/]*)?\\.(txt|yaml|yml|json)$"; "i") or test("[^/]*(proxy|proxies|node|nodes|subscription|sub|http|https|socks|vless|vmess|trojan|shadowsocks|clash|mix|raw|result|server)[^/]*\\.(txt|yaml|yml|json)$"; "i"))
		))
		| unique | .[0:3] | join(", ")
	' <<< "$tree_paths")"
	[[ -z "$paths" ]] && continue

	row_number="$((${#rows[@]} + 1))"
	rows+=("| $row_number | https://github.com/$repository | ${pushed_at%%T*} | \`$paths\` |")
	repositories+=("$repository")
	seen_repositories["$key"]=1
	if (( ${#rows[@]} >= limit )); then
		break
	fi
done <<< "$candidate_repositories"

if (( ${#rows[@]} < limit )); then
	printf 'Found %s of %s requested candidates for %s through %s; issue body was not updated.\n' \
		"${#rows[@]}" "$limit" "$window_start" "$window_end" >&2
	exit 1
fi

if [[ "$dry_run" != true ]]; then
	issue_json="$(gh issue view "$issue_number" --repo "$repo_slug" --json state,body)"
	issue_state="$(jq -r .state <<< "$issue_json")"
	if [[ "$issue_state" != OPEN ]]; then
		printf 'Source issue #%s is not open; refusing to update it.\n' "$issue_number" >&2
		exit 1
	fi
	issue_body="$(jq -r '.body // ""' <<< "$issue_json")"
	declare -A latest_repositories=()
	while IFS= read -r repository; do
		[[ -n "$repository" ]] && latest_repositories["$repository"]=1
	done < <(printf '%s\n' "$issue_body" | extract_repositories)
	filtered_rows=()
	row_number=1
	for index in "${!rows[@]}"; do
		key="$(tr '[:upper:]' '[:lower:]' <<< "${repositories[$index]}")"
		[[ -n "${latest_repositories[$key]:-}" ]] && continue
		latest_repositories["$key"]=1
		filtered_rows+=("$(sed -E "s/^\\| [0-9]+ \\|/| $row_number |/" <<< "${rows[$index]}")")
		row_number=$((row_number + 1))
	done
	rows=("${filtered_rows[@]}")
	if (( ${#rows[@]} < limit )); then
		printf 'Found %s of %s requested candidates after refreshing issue #%s; issue body was not updated.\n' \
			"${#rows[@]}" "$limit" "$issue_number" >&2
		exit 1
	fi
fi

new_section="## Newly discovered repositories ($(date -u +%F))

Search window: $window_start through $window_end. Each repository was checked for a recent push, absence from sources/ and this issue, and likely text/YAML/JSON data files. Paths are extraction starting points; verify reachability, content format, duplication, and parser/transformer needs before merging.

| # | Repository | Last pushed | Candidate data path(s) |
|---:|---|---|---|
$(printf '%s\n' "${rows[@]}")

These are discovery candidates only; no entries have been merged into sources/."

body="${issue_body%$'\n'}"
if [[ -n "$body" ]]; then
	body+=$'\n\n'
fi
body+="$new_section"

if [[ "$dry_run" == true ]]; then
	printf '%s\n' "$body"
else
	gh issue edit "$issue_number" --repo "$repo_slug" --body "$body"
fi
