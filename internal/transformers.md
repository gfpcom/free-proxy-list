# Transformer Reference

Transformers convert a downloaded source response into proxy lines before the configured parser runs. Source entries use this format:

```text
<url>,<transformer>[:<options>],<parser>
```

The transformer and parser are optional. The default transformer is raw (unchanged bytes); the default parser is `ParseProxyURL`. The loader substitutes date/time URL tokens before fetching the source.

## `raw`

Returns the response body unchanged. It is the default when the transformer column is omitted. Unknown transformer names also fall back to raw, so misspellings do not currently produce a configuration error.

```text
https://example.net/proxies.txt
```

## `mtproto`

Extracts Telegram MTProto proxy links from text or HTML-like feed responses. It recognizes `tg://proxy`, `https://t.me/proxy`, and `https://telegram.me/proxy` links and emits one link per line, so feeds that put several records on one line can be parsed. The source protocol should be `tg`; the parser validates the server, port, and required secret, then stores each record as a `tg://proxy` URI.

```text
https://example.net/telegram-proxies.txt,mtproto
```

## `base64`

Decodes the entire response using standard Base64. If decoding fails, it returns the original bytes unchanged, which will normally result in parser rejection. Base64URL and recursive decoding are not supported.

```text
https://example.net/subscription.txt,base64
```

## `json`

Maps records in a JSON response to proxy URI lines. Options are semicolon-separated `key=value` pairs. `path` selects records; use `$` for the root, `.field` for object properties, `[N]` for an array index, and `[*]` to select every array item. Field mappings are relative to each selected record and support the same property/index selectors.

Map an existing URI field or a single record that is already an `IP:PORT` endpoint:

```text
https://example.net/proxies.json,json:path=$[*];uri=proxy
https://example.net/http.json,json:path=$.proxies[*];endpoint=$
```

Or assemble an endpoint from record fields. If `scheme` is omitted, the configured source protocol is supplied by the normal parser:

```text
https://example.net/proxies.json,json:path=$.data.proxies[*];scheme=protocol;host=ip;port=port
https://example.net/http-proxies.json,json:path=$[*];host=ip;port=port
```

Exactly one mapping is required: `uri`, `endpoint`, or `host` plus `port` and an optional `scheme`. URI and endpoint fields must be strings; endpoints must be valid `IP:PORT` values. Host and scheme fields must be strings; ports may be integer JSON numbers or digit-only strings from 1 through 65535. Records with missing or invalid values are skipped. Invalid transformer options fail source validation. JSON is limited to 10 MiB and 100,000 selected records; malformed, trailing, or oversized JSON produces no output.

## `clash`

Reads a Clash YAML document and converts its `proxies` entries into proxy URI lines. It supports `http`, `https`, `socks4`, `socks5`, `ss`, `vmess`, `vless`, and `trojan`. Invalid entries and unsupported types are skipped. The converter maps supported TLS, Reality, SNI, fingerprint, WebSocket, gRPC, and HTTP/2 fields. Input is limited to 10 MiB, and ports must be integers from 1 through 65535.

```text
https://example.net/config.yaml,clash
```

## `link`

Finds HTTP(S) links in the response, optionally selects links by a case-sensitive substring, fetches each unique child URL, transforms its body, and combines the results. It is intended for index pages or README files linking to actual feed files.

```text
https://example.net/README.md,link:base64-fn0618
```

In this example, only links containing `fn0618` are fetched, and each child response is decoded with `base64` before merging. Other examples:

```text
https://example.net/index.html,link:clash-provider
https://example.net/index.html,link:fn0618
```

The first applies the `clash` transformer to matching children; the second uses raw child bodies and filters links by `fn0618`. The child transformer name is separated from its keyword with `-`.

At most 32 unique child links are fetched, and each child response is limited to 10 MiB. Only public HTTP(S) targets are allowed; unsafe redirect destinations are rejected. A failed child fetch is skipped without discarding other results.

## `list`

Reads a plain-text source list with one HTTP(S) URL per line, downloads each unique URL, transforms each response, and combines the results. Blank lines and lines beginning with `#` are ignored. Use this for source lists whose entries point to proxy subscriptions instead of containing proxy records themselves. Specify the child transformer after `list:`:

```text
https://example.net/clash-subscriptions.txt,list:clash
```

This downloads the listed subscriptions, converts each Clash YAML response to proxy URI lines, and lets the configured parser process the merged output. `list:base64` can be used for Base64-encoded child feeds; omitting the option leaves child responses unchanged. The same 32-URL, 10 MiB response, public HTTP(S) target, redirect, and failure limits as `link` apply.

## `curl`

`curl` uses `curl_chrome116` from curl-impersonate for both the root page and child-page requests. Install it on Linux with:

```sh
make install-curl-impersonate
```

The executable can be selected with `CURL_IMPERSONATE_BIN`. Otherwise the program checks `PATH`, then `~/.local/bin/curl_chrome116`.

`curl` accepts an optional built-in protocol finder. If omitted, the `uri` finder is used:

```text
https://example.net/,curl:<finder>;<options>
```

### URI finder

The default `uri` finder searches HTML/text bytes for supported proxy URI schemes. It supports a depth, an optional link URL substring selector, and an optional scheme filter:

```text
https://openkeys.net/,curl:1-/key/-vless
https://example.net/,curl:1-/servers/-ss+trojan+vless
```

Depth `0` scans the root response. Depth `1` scans matching child links selected from `a[href]`, `link[href]`, and `iframe[src]`; relative links are resolved against the page URL. The selector match is case-insensitive. A final suffix filters schemes, joined with `+`; without a suffix, all recognized proxy schemes are searched. Recognized schemes include `socks`, `socks4`, `socks4a`, `socks5`, `socks5a`, `socks5h`, `ss`, `ssr`, `vmess`, `vless`, `trojan`, `hy`, `hy2`, `hysteria`, `hysteria2`, `tuic`, `wireguard`, and `anytls`. `http` and `https` may be selected explicitly.

The crawler automatically follows detected pagination links for both the root page and fetched child pages. It recognizes `rel="next"`, common next-page classes/labels, numbered links in pagination/navigation containers, numeric `page`, `p`, or `paged` query parameters, and `/page/<number>` paths. Pagination is followed even at depth `0`; depth controls ordinary child-link fetching. Duplicate page URLs are fetched once.

### DOM finder

Use `dom` when a proxy record is split across elements instead of containing a ready-made URI. The finder chooses rows, extracts configured fields from within each row, then fills a URI template.

Options are semicolon-separated `key=value` pairs. `row`, `template`, and at least one field are required. Field names are identifiers and are referenced as `{field}` in the template. Each field value is read from the selected element's text; add `@attribute` to read an HTML attribute instead.

| Option | Meaning |
| --- | --- |
| `row` | CSS selector for each record. Required. |
| `template` | URI template, such as `{protocol}://{host}:{port}`. Required. |
| `depth` | `0` for the root page (default), `1` to also fetch selector-matched child links. Pagination is followed automatically at either depth. |
| `links` | Case-insensitive substring for child link URLs; required when `depth=1`. |
| other keys | Field name and CSS selector, optionally followed by `@attribute`. |

Example for the mixed-protocol table on freeproxylist.ru:

```text
https://freeproxylist.ru/en/?page=1,curl:dom;row=tbody.table-proxy-list tr;protocol=td:nth-child(4);host=th.tblport;port=td.tblport;template={protocol}://{host}:{port}
```

An attribute-based field can be configured as `host=.host@data-ip`. The protocol/scheme field is lowercased. Rows with missing fields are skipped; assembled URLs are deduplicated. CSS selectors and template placeholders are validated before scanning. Processing is limited to 1,000 rows across fetched pages.

### Curl safety limits

The crawler supports at most depth `1`, follows at most 32 unique pages, limits each response to 10 MiB, uses connection/request timeouts, and allows HTTP(S) only. Public-address checks apply before requests and at each redirect; private, loopback, link-local, and unspecified targets are rejected. The root source URL and every child page use curl-impersonate.

## Adding a transformer or finder

Register byte transformations in the `Transformers` map. `curl` protocol finders use the `ProtocolFinder` signature:

```go
type ProtocolFinder func(data []byte, options, sourceURL string) []byte
```

Register a finder with `RegisterProtocolFinder`. A finder should validate its settings, bound its work, and return newline-separated proxy URI candidates for the standard parser.
