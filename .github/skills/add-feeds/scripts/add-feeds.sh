#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'EOF'
Usage: add-feeds.sh [--apply] <manifest.tsv>

Manifest rows contain a source filename stem and a complete source entry,
separated by one tab. Without --apply, the script only validates and previews.

Examples:
  http<TAB>https://example.com/http.txt,,ColonURL
  auto<TAB>https://example.com/subscription.txt,base64
EOF
}

apply=false
case "${1:-}" in
	-h|--help) usage; exit 0 ;;
	--apply) apply=true; shift ;;
	--dry-run) shift ;;
esac

if [[ $# -ne 1 || ! -f "$1" ]]; then
	usage >&2
	exit 2
fi
manifest="$1"

command -v curl >/dev/null
command -v base64 >/dev/null

root_dir="$(git rev-parse --show-toplevel)"
source_dir="$root_dir/sources"
timeout_seconds="${ADD_FEEDS_TIMEOUT:-20}"
max_bytes="${ADD_FEEDS_MAX_BYTES:-31457280}"
if ! [[ "$timeout_seconds" =~ ^[1-9][0-9]*$ && "$max_bytes" =~ ^[1-9][0-9]*$ ]]; then
	printf 'ADD_FEEDS_TIMEOUT and ADD_FEEDS_MAX_BYTES must be positive integers.\n' >&2
	exit 2
fi

temp_dir="$(mktemp -d)"
trap 'rm -rf "$temp_dir"' EXIT

declare -a validated_entries=()
declare -A seen_urls=()
has_errors=false

source_url_exists() {
	local protocol="$1"
	local target_url="$2"
	local source_file="$source_dir/$protocol.txt"
	local source_line
	[[ -f "$source_file" ]] || return 1
	while IFS= read -r source_line || [[ -n "$source_line" ]]; do
		[[ "${source_line%%,*}" == "$target_url" ]] && return 0
	done < "$source_file"
	return 1
}

matches_expected_format() {
	local sample_file="$1"
	local parser_name="$2"
	local transformer_name="$3"
	case "$transformer_name" in
		clash)
			grep -Eiq '^[[:space:]]*proxies[[:space:]]*:' "$sample_file"
			;;
		link)
			grep -Eiq "https?://[^[:space:]<>\"']+" "$sample_file"
			;;
		curl)
			grep -Eiq '<(!doctype|html|a|input)([[:space:]>])' "$sample_file"
			;;
		*)
			case "$parser_name" in
				ColonURL)
					grep -Eq '^[[:space:]]*([[:alnum:]_.-]+):[0-9]{1,5}([[:space:]]|$)' "$sample_file"
					;;
				SpaceURL)
					grep -Eq '^[[:space:]]*[^[:space:]]+[[:space:]]+[0-9]{1,5}([[:space:]]|$)' "$sample_file"
					;;
				*)
					grep -Eiq '^[[:space:]]*(https?|tg|socks4a?|socks5a?|ssr?|vmess|vless|trojan|hy2?|hysteria2?|anytls|tuic)://[^[:space:]]+' "$sample_file"
					;;
			esac
			;;
	esac
}

line_number=0
while IFS= read -r manifest_line || [[ -n "$manifest_line" ]]; do
	line_number=$((line_number + 1))
	manifest_line="${manifest_line%$'\r'}"
	[[ -z "$manifest_line" || "$manifest_line" == \#* ]] && continue

	if [[ "$manifest_line" != *$'\t'* || "${manifest_line#*$'\t'}" == *$'\t'* ]]; then
		printf 'Line %s: expected exactly two tab-separated columns.\n' "$line_number" >&2
		has_errors=true
		continue
	fi
	protocol="${manifest_line%%$'\t'*}"
	source_entry="${manifest_line#*$'\t'}"
	if ! [[ "$protocol" =~ ^[a-z0-9][a-z0-9_-]*$ ]] || [[ ! -f "$source_dir/$protocol.txt" ]]; then
		printf 'Line %s: unsupported or missing source file for protocol "%s".\n' "$line_number" "$protocol" >&2
		has_errors=true
		continue
	fi
	if [[ -z "$source_entry" ]]; then
		printf 'Line %s: source entry is empty.\n' "$line_number" >&2
		has_errors=true
		continue
	fi

	url="${source_entry%%,*}"
	transformer_spec=""
	parser_name=""
	if [[ "$source_entry" == *,* ]]; then
		remainder="${source_entry#*,}"
		transformer_spec="${remainder%%,*}"
		if [[ "$remainder" == *,* ]]; then
			parser_name="${remainder#*,}"
			if [[ "$parser_name" == *,* ]]; then
				printf 'Line %s: source entries support at most URL, transformer, parser.\n' "$line_number" >&2
				has_errors=true
				continue
			fi
		fi
	fi
	if [[ "$url" != https://* && "$url" != http://* ]]; then
		printf 'Line %s: feed URL must use HTTP or HTTPS.\n' "$line_number" >&2
		has_errors=true
		continue
	fi

	transformer_name="${transformer_spec%%:*}"
	case "$transformer_name" in
		""|raw|base64|mtproto|json|clash|link|curl) ;;
		*)
			printf 'Line %s: transformer "%s" is not registered by this project.\n' "$line_number" "$transformer_name" >&2
			has_errors=true
			continue
			;;
	esac
	parser_type="${parser_name%%:*}"
	case "$parser_type" in
		""|ColonURL|SpaceURL|Split) ;;
		*)
			printf 'Line %s: parser "%s" is not registered by this project.\n' "$line_number" "$parser_type" >&2
			has_errors=true
			continue
			;;
	esac
	if [[ -n "$transformer_spec" && "$transformer_name" == link && "$transformer_spec" == link: ]]; then
		printf 'Line %s: link transformer options cannot be empty.\n' "$line_number" >&2
		has_errors=true
		continue
	fi

	source_key="$protocol"$'\t'"$url"
	if [[ -n "${seen_urls[$source_key]:-}" ]] || source_url_exists "$protocol" "$url"; then
		printf 'Line %s: duplicate source URL for %s, skipped: %s\n' "$line_number" "$protocol" "$url"
		continue
	fi
	seen_urls["$source_key"]=1

	body_file="$temp_dir/body"
	fetch_client=curl
	if [[ "$transformer_name" == curl ]]; then
		fetch_client="${CURL_IMPERSONATE_BIN:-}"
		if [[ -z "$fetch_client" ]]; then
			fetch_client="$(command -v curl_chrome116 || true)"
		fi
		if [[ -z "$fetch_client" && -x "${HOME:-}/.local/bin/curl_chrome116" ]]; then
			fetch_client="${HOME}/.local/bin/curl_chrome116"
		fi
		if [[ -z "$fetch_client" ]]; then
			printf 'Line %s: curl transformer requires curl_chrome116.\n' "$line_number" >&2
			has_errors=true
			continue
		fi
	fi
	if [[ "$transformer_name" != curl ]]; then
		if ! "$fetch_client" --fail --location --silent --show-error \
			--proto '=http,https' --proto-redir '=http,https' \
			--connect-timeout 10 --max-time "$timeout_seconds" --max-filesize "$max_bytes" \
			--output "$body_file" "$url"; then
			printf 'Line %s: feed request failed: %s\n' "$line_number" "$url" >&2
			has_errors=true
			continue
		fi
		if [[ ! -s "$body_file" ]]; then
			printf 'Line %s: feed response is empty: %s\n' "$line_number" "$url" >&2
			has_errors=true
			continue
		fi
	fi

	preview_file="$body_file"
	if [[ "$transformer_name" == base64 ]]; then
		preview_file="$temp_dir/decoded"
		if ! base64 --decode "$body_file" > "$preview_file" 2>/dev/null || [[ ! -s "$preview_file" ]]; then
			printf 'Line %s: response is not valid non-empty Base64 content.\n' "$line_number" >&2
			has_errors=true
			continue
		fi
	fi
	if [[ "$transformer_name" == curl || "$transformer_name" == json || "$transformer_name" == mtproto || "$parser_type" == Split ]]; then
		validation_dir="$temp_dir/transformer-validation-$line_number"
		mkdir -p "$validation_dir/sources"
		printf '%s\n' "$source_entry" > "$validation_dir/sources/$protocol.txt"
		if ! go run "$root_dir/cmd" -dir "$validation_dir" -dry-run; then
			printf 'Line %s: configured transformer/parser produced no valid proxies.\n' "$line_number" >&2
			has_errors=true
			continue
		fi
	fi
	if [[ "$transformer_name" != curl && "$transformer_name" != json && "$transformer_name" != mtproto && "$parser_type" != Split ]] && ! matches_expected_format "$preview_file" "$parser_type" "$transformer_name"; then
		printf 'Line %s: response sample does not match the configured format.\n' "$line_number" >&2
		has_errors=true
		continue
	fi

	printf 'Validated %s: %s\n' "$protocol" "$source_entry"
	if [[ "$transformer_name" != curl && "$transformer_name" != json ]]; then
		sed -n '1,4p' "$preview_file" | cut -c1-180 | sed 's/^/  /'
	fi
	if [[ "$transformer_name" == link ]]; then
		printf '  Note: inspect linked child feeds and their formats manually.\n'
	fi
	validated_entries+=("$protocol"$'\t'"$source_entry")
done < "$manifest"

if [[ "$has_errors" == true ]]; then
	printf 'No entries were written because at least one manifest row failed validation.\n' >&2
	exit 1
fi
if (( ${#validated_entries[@]} == 0 )); then
	printf 'No new feed entries to add.\n'
	exit 0
fi

if [[ "$apply" == false ]]; then
	printf 'Dry run only: %s entries passed preflight. Re-run with --apply to append them.\n' "${#validated_entries[@]}"
	exit 0
fi

for validated_entry in "${validated_entries[@]}"; do
	protocol="${validated_entry%%$'\t'*}"
	source_entry="${validated_entry#*$'\t'}"
	destination="$source_dir/$protocol.txt"
	if [[ -s "$destination" && "$(tail -c 1 "$destination" | wc -l)" -eq 0 ]]; then
		printf '\n' >> "$destination"
	fi
	printf '%s\n' "$source_entry" >> "$destination"
	printf 'Added %s to %s\n' "$source_entry" "$destination"
done