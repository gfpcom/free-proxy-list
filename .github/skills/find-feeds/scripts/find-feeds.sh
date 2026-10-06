#!/usr/bin/env bash
set -euo pipefail

limit="${FIND_FEEDS_LIMIT:-50}"
issue_number="${FIND_FEEDS_ISSUE:-21}"
dry_run=false
candidates_file="${FIND_FEEDS_CANDIDATES_FILE:-}"
candidates_only=false

while (( $# > 0 )); do
	case "$1" in
		--dry-run) dry_run=true ;;
		--candidates-only) candidates_only=true ;;
		--candidates-file)
			(( $# >= 2 )) || { printf '%s\n' '--candidates-file requires a path.' >&2; exit 2; }
			candidates_file="$2"
			shift
			;;
		*)
			printf 'Unknown argument: %s\n' "$1" >&2
			exit 2
			;;
	esac
	shift
done

if ! [[ "$limit" =~ ^[1-9][0-9]*$ ]]; then
	printf 'FIND_FEEDS_LIMIT must be a positive integer.\n' >&2
	exit 2
fi
if (( limit > 50 )); then
	printf 'FIND_FEEDS_LIMIT must not exceed 50.\n' >&2
	exit 2
fi
if ! [[ "$issue_number" =~ ^[1-9][0-9]*$ ]]; then
	printf 'FIND_FEEDS_ISSUE must be a positive integer.\n' >&2
	exit 2
fi
if [[ "$candidates_only" == true && -z "$candidates_file" ]]; then
	printf '%s\n' '--candidates-only requires --candidates-file or FIND_FEEDS_CANDIDATES_FILE.' >&2
	exit 2
fi
if [[ -n "$candidates_file" && ! -r "$candidates_file" ]]; then
	printf 'Candidate file is not readable: %s\n' "$candidates_file" >&2
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
	"open proxy list"
	"public proxy list"
	"proxy subscription url"
	"proxy provider list"
	"free http proxy list"
	"free https proxy list"
	"free socks4 proxy list"
	"free socks5 proxy list"
	"ip port proxy list"
	"v2ray subscription"
	"v2ray nodes"
	"shadowsocks subscription"
	"ssr proxy list"
	"vless subscription"
	"vmess subscription"
	"trojan subscription"
	"xray subscription"
	"sing-box subscription"
	"hysteria2 subscription"
	"tuic subscription"
	"free nodes clash"
	"clash proxy subscription"
	"mihomo proxy list"
	"clash config nodes"
	"base64 proxy subscription"
	"http socks proxy list"
	"proxy collector"
	"proxy feed"
	"proxy pool"
	"proxy aggregator"
	"free v2ray config"
)

search_results=''
declare -A considered_repositories=()
rows=()
repositories=()

process_candidate() {
	local repository="$1"
	local key metadata pushed_at is_fork is_archived tree tree_paths paths
	key="$(tr '[:upper:]' '[:lower:]' <<< "$repository")"
	[[ -n "${considered_repositories[$key]:-}" ]] && return 0
	considered_repositories["$key"]=1
	[[ -n "${seen_repositories[$key]:-}" ]] && return 0

	metadata="$(gh api "repos/$repository" --jq '[.pushed_at, .fork, .archived] | @tsv' 2>/dev/null || true)"
	IFS=$'\t' read -r pushed_at is_fork is_archived <<< "$metadata"
	[[ -z "${pushed_at:-}" ]] && return 0
	local pushed_date="${pushed_at%%T*}"
	[[ "$pushed_date" < "$window_start" || "$pushed_date" > "$window_end" ]] && return 0
	[[ "$is_fork" == true || "$is_archived" == true ]] && return 0

	tree="$(gh api "repos/$repository/git/trees/HEAD?recursive=1" 2>/dev/null || true)"
	[[ -z "$tree" ]] && return 0
	tree_paths="$(fetch_tree_paths "$repository" <<< "$tree")" || return 0
	paths="$(jq -r '
		map(select(
			(test("\\.(txt|yaml|yml|json)$"; "i"))
			and (test("(^|/)(\\.github|docs?|tests?|fixtures?|node_modules)(/|$)"; "i") | not)
			and (test("(^|/)(package(-lock)?|tsconfig|manifest|version|info|metadata|stats|badges?)(\\.[^/]*)?\\.(json|yaml|yml)$"; "i") | not)
			and (test("(^|/)(configs?)(\\.prod)?\\.(yaml|yml|json)$"; "i") | not)
			and (test("(blocklist|blacklist|denylist|bot.?ips?)"; "i") | not)
			and (test("(^|/)(protocols|countries)/[^/]+\\.(txt|yaml|yml|json)$"; "i") or test("(^|/)(all|sub|subscriptions|proxies?|nodes?|http|https|socks|vless|vmess|trojan|shadowsocks|clash|mix|raw|result|servers?|vpn|wireguard|sing[-_.]?box|hysteria2?|hy2|tuic|providers?|feeds?|pools?|exports?)([-_.][^/]*)?\\.(txt|yaml|yml|json)$"; "i") or test("[^/]*(proxy|proxies|node|nodes|subscription|sub|http|https|socks|vless|vmess|trojan|shadowsocks|clash|mix|raw|result|server|vpn|wireguard|sing[-_.]?box|hysteria2?|hy2|tuic|provider|feed|pool|export)[^/]*\\.(txt|yaml|yml|json)$"; "i"))
		))
		| unique | .[0:3] | join(", ")
	' <<< "$tree_paths")"
	[[ -z "$paths" ]] && return 0

	rows+=("| https://github.com/$repository | $pushed_date | \`$paths\` | Pending: candidate discovered; verify reachability, content format, duplicates, and parser/transformer support before import. |")
	repositories+=("$repository")
	seen_repositories["$key"]=1
}

read_candidate_file() {
	local line owner repository
	while IFS= read -r line || [[ -n "$line" ]]; do
		[[ "$line" =~ ^[[:space:]]*(#|$) ]] && continue
		line="${line#"${line%%[![:space:]]*}"}"
		line="${line%"${line##*[![:space:]]}"}"
		if [[ "$line" =~ ^https?://(www\.)?github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)(\.git)?/?$ ]]; then
			owner="${BASH_REMATCH[2]}"
			repository="${BASH_REMATCH[3]}"
			printf '%s/%s\n' "$owner" "${repository%.git}"
		elif [[ "$line" =~ ^([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)(\.git)?$ ]]; then
			owner="${BASH_REMATCH[1]}"
			repository="${BASH_REMATCH[2]}"
			printf '%s/%s\n' "$owner" "${repository%.git}"
		else
			printf 'Invalid candidate on line: %s\n' "$line" >&2
			return 1
		fi
	done < "$candidates_file"
}

if [[ -n "$candidates_file" ]]; then
	candidate_list="$(read_candidate_file)" || exit 2
	while IFS= read -r repository; do
		[[ -z "$repository" ]] && continue
		process_candidate "$repository"
		if (( ${#rows[@]} >= limit )); then
			break
		fi
	done <<< "$candidate_list"
fi

if [[ "$candidates_only" != true ]] && (( ${#rows[@]} < limit )); then
	for query in "${queries[@]}"; do
		result="$(gh search repos "$query pushed:$window_start..$window_end fork:false archived:false" \
			--sort updated --limit 100 \
			--json fullName,description,pushedAt,isFork,isArchived)"
		search_results+="$result"$'\n'
		candidate_repositories="$(jq -sr --arg window_start "$window_start" --arg window_end "$window_end" '
			add | unique_by(.fullName | ascii_downcase)
			| map(select(.isFork == false and .isArchived == false and .pushedAt[:10] >= $window_start and .pushedAt[:10] <= $window_end))
			| map(select((.description // "" | test("bot[- _]?ips?|ips?[^.]{0,30}used[^.]{0,20}bots|blocklists?|blacklists?|deny lists?|proxy ips? addresses? used by bots"; "i")) | not))
			| sort_by(.pushedAt) | reverse | .[].fullName
		' <<< "$search_results")"

		while IFS= read -r repository; do
			[[ -z "$repository" ]] && continue
			process_candidate "$repository"
			if (( ${#rows[@]} >= limit )); then
				break 2
			fi
		done <<< "$candidate_repositories"
	done
fi

if (( ${#rows[@]} == 0 )); then
	printf 'Found no eligible candidates for %s through %s; issue body was not updated.\n' \
		"$window_start" "$window_end" >&2
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
	for index in "${!rows[@]}"; do
		key="$(tr '[:upper:]' '[:lower:]' <<< "${repositories[$index]}")"
		[[ -n "${latest_repositories[$key]:-}" ]] && continue
		latest_repositories["$key"]=1
		filtered_rows+=("${rows[$index]}")
	done
	rows=("${filtered_rows[@]}")
	if (( ${#rows[@]} == 0 )); then
		printf 'No new candidates remain after refreshing issue #%s; issue body was not updated.\n' \
			"$issue_number" >&2
		exit 1
	fi
fi

append_rows_to_table() {
	local body="$1"
	shift
	local line output="" in_table=false inserted=false found=false
	local expected_header='| Candidate repository / feed URL | Last pushed (UTC) | Candidate data path(s) | Validation status / decision |'

	while IFS= read -r line || [[ -n "$line" ]]; do
		if [[ "$line" == "$expected_header" ]]; then
			in_table=true
			found=true
		fi
		if [[ "$in_table" == true && "$line" != \|* ]]; then
			printf -v output '%s%s\n' "$output" "$(printf '%s\n' "$@")"
			inserted=true
			in_table=false
		fi
		output+="$line"$'\n'
	done <<< "$body"

	if [[ "$in_table" == true ]]; then
		printf -v output '%s%s\n' "$output" "$(printf '%s\n' "$@")"
		inserted=true
	fi
	if [[ "$found" != true || "$inserted" != true ]]; then
		printf 'Issue body is missing the expected candidate table; refusing to add rows.\n' >&2
		return 1
	fi
	printf '%s' "${output%$'\n'}"
}

if ! issue_body="$(append_rows_to_table "$issue_body" "${rows[@]}")"; then
	exit 1
fi
if [[ "$dry_run" == true ]]; then
	printf '%s\n' "$issue_body"
else
	gh issue edit "$issue_number" --repo "$repo_slug" --body "$issue_body"
fi
