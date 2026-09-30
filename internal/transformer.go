package internal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
	"gopkg.in/yaml.v3"
)

const (
	maxRegexLinkCount         = 32
	maxRegexLinkResponseBytes = 10 * 1024 * 1024
	maxCurlDepth              = 1
	maxCurlPageCount          = 32
	maxDOMFinderRows          = 1000
)

var (
	Transformers               = map[string]Transformer{}
	ProtocolFinders            = map[string]ProtocolFinder{}
	allowPrivateRegexLinkHosts = false
	errUnsafeRegexLinkRedirect = errors.New("unsafe regex link redirect")
	curlImpersonateFetch       = fetchCurlImpersonate
	regexLinkClient            = &http.Client{
		Transport: client.Transport,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if !isAllowedRegexLink(req.URL.String()) {
				return errUnsafeRegexLinkRedirect
			}
			return nil
		},
	}
)

func init() {
	Transformers["base64"] = FromBase64
	Transformers["clash"] = FromClash
	Transformers["link"] = FromLinks
	Transformers["curl"] = FromCurl
	ProtocolFinders["uri"] = findURIProxyURLs
	ProtocolFinders["dom"] = findDOMProxyURLs
}

type Transformer func(data []byte, options string) []byte

type ProtocolFinder func(data []byte, options, sourceURL string) []byte

func RegisterProtocolFinder(name string, finder ProtocolFinder) {
	ProtocolFinders[name] = finder
}

func RegisterTransformer(name string, t Transformer) {
	Transformers[name] = t
}

func GetTransformer(spec string) (Transformer, string) {
	name, options := parseTransformerSpec(spec)
	if t, ok := Transformers[name]; ok {
		return t, options
	}

	return FromRaw, ""
}

func parseTransformerSpec(spec string) (string, string) {
	name, options, _ := strings.Cut(spec, ":")
	return strings.TrimSpace(name), strings.TrimSpace(options)
}

func FromRaw(buf []byte, _ string) []byte {
	return buf
}

func FromBase64(buf []byte, _ string) []byte {
	decoded, err := base64.StdEncoding.DecodeString(string(buf))
	if err != nil {
		return buf
	}

	return decoded
}

var linkURLPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

// FromLinks extracts link-like URLs from a document, optionally filters them by
// keyword, downloads each unique URL, and applies the selected transformer to
// the downloaded content before merging it into the result.
func FromLinks(buf []byte, spec string) []byte {
	transformer, keyword := parseLinkSpec(spec)

	matches := linkURLPattern.FindAll(buf, -1)
	if len(matches) == 0 {
		return []byte{}
	}

	var result bytes.Buffer
	seen := map[string]struct{}{}
	for _, match := range matches {
		rawURL := strings.Trim(string(match), " 	\r\n\"'<>)]}")
		if keyword != "" && !strings.Contains(rawURL, keyword) {
			continue
		}
		if _, ok := seen[rawURL]; ok || !isAllowedRegexLink(rawURL) {
			continue
		}
		if len(seen) >= maxRegexLinkCount {
			break
		}
		seen[rawURL] = struct{}{}

		resp, err := regexLinkClient.Get(rawURL)
		if err != nil {
			continue
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			resp.Body.Close() // nolint: errcheck
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxRegexLinkResponseBytes+1))
		resp.Body.Close() // nolint: errcheck
		if err != nil || len(body) > maxRegexLinkResponseBytes {
			continue
		}

		result.Write(bytes.TrimSpace(transformer(body, "")))
		result.WriteByte('\n')
	}

	return result.Bytes()
}

var proxyLinkPattern = regexp.MustCompile(`(?i)\b(?:socks|socks4a?|socks5(?:a|h)?|vmess|vless|trojan|ssr?|hy2?|hysteria2?|hhysteria2?|hhy2|tuic|wireguard|anytls)://[^\s"'<>]+`)

// FromCurl finds proxy URLs on the current page or one matching linked page.
// Options use [depth-]selector[-protocol+protocol], for example 1-/servers/-ss+trojan.
func FromCurl(buf []byte, spec string) []byte {
	return fromCurl(buf, spec, "")
}

func fromCurl(buf []byte, spec, sourceURL string) []byte {
	if finderName, options, found := strings.Cut(spec, ";"); found {
		if finder, ok := ProtocolFinders[finderName]; ok {
			return finder(buf, options, sourceURL)
		}
	}
	return ProtocolFinders["uri"](buf, spec, sourceURL)
}

func findURIProxyURLs(buf []byte, spec, sourceURL string) []byte {
	depth, selector, protocols, ok := parseCurlSpec(spec)
	if !ok {
		return []byte{}
	}

	var pages [][]byte
	if depth == 0 {
		pages = append(pages, buf)
		pages = append(pages, fetchPaginatedPages(buf, sourceURL)...)
	} else {
		pages = fetchCurlPages(buf, sourceURL, selector)
	}

	pattern := proxyLinkPattern
	if len(protocols) > 0 {
		var alternatives []string
		for _, protocol := range protocols {
			alternatives = append(alternatives, regexp.QuoteMeta(protocol))
		}
		pattern = regexp.MustCompile(`(?i)\b(?:` + strings.Join(alternatives, "|") + `)://[^\s"'<>]+`)
	}

	var result bytes.Buffer
	seen := map[string]struct{}{}
	for _, page := range pages {
		for _, match := range pattern.FindAll(page, -1) {
			proxyURL := html.UnescapeString(strings.TrimRight(string(match), ",;.)]}:"))
			if _, exists := seen[proxyURL]; exists {
				continue
			}
			seen[proxyURL] = struct{}{}
			result.WriteString(proxyURL)
			result.WriteByte('\n')
		}
	}
	return result.Bytes()
}

type domFinderConfig struct {
	depth    int
	links    string
	row      string
	template string
	fields   map[string]string
}

var domTemplateFieldPattern = regexp.MustCompile(`\{([a-zA-Z][a-zA-Z0-9_]*)\}`)
var domFieldNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)

func findDOMProxyURLs(buf []byte, options, sourceURL string) []byte {
	config, ok := parseDOMFinderOptions(options)
	if !ok {
		return []byte{}
	}

	pages := [][]byte{buf}
	if config.depth == 1 {
		pages = fetchCurlPages(buf, sourceURL, config.links)
	} else {
		pages = append(pages, fetchPaginatedPages(buf, sourceURL)...)
	}

	var result bytes.Buffer
	seen := map[string]struct{}{}
	rowCount := 0
	for _, page := range pages {
		document, err := goquery.NewDocumentFromReader(bytes.NewReader(page))
		if err != nil {
			continue
		}
		document.Find(config.row).EachWithBreak(func(_ int, row *goquery.Selection) bool {
			rowCount++
			if rowCount > maxDOMFinderRows {
				return false
			}

			values := make(map[string]string, len(config.fields))
			for name, fieldSpec := range config.fields {
				selector, attribute, hasAttribute := strings.Cut(fieldSpec, "@")
				selected := row.Find(strings.TrimSpace(selector)).First()
				value := selected.Text()
				if hasAttribute {
					var exists bool
					value, exists = selected.Attr(strings.TrimSpace(attribute))
					if !exists {
						return true
					}
				}
				value = html.UnescapeString(strings.TrimSpace(value))
				if name == "protocol" || name == "scheme" {
					value = strings.ToLower(value)
				}
				if value == "" {
					return true
				}
				values[name] = value
			}

			proxyURL := domTemplateFieldPattern.ReplaceAllStringFunc(config.template, func(field string) string {
				return values[field[1:len(field)-1]]
			})
			if strings.Contains(proxyURL, "{") || strings.Contains(proxyURL, "}") {
				return true
			}
			if _, exists := seen[proxyURL]; exists {
				return true
			}
			seen[proxyURL] = struct{}{}
			result.WriteString(proxyURL)
			result.WriteByte('\n')
			return true
		})
		if rowCount > maxDOMFinderRows {
			break
		}
	}
	return result.Bytes()
}

func parseDOMFinderOptions(options string) (domFinderConfig, bool) {
	config := domFinderConfig{fields: map[string]string{}}
	for _, option := range strings.Split(options, ";") {
		key, value, found := strings.Cut(strings.TrimSpace(option), "=")
		if !found || key == "" || value == "" {
			return domFinderConfig{}, false
		}
		value = strings.TrimSpace(value)
		switch key {
		case "depth":
			depth, err := strconv.Atoi(value)
			if err != nil || depth < 0 || depth > maxCurlDepth {
				return domFinderConfig{}, false
			}
			config.depth = depth
		case "links":
			config.links = value
		case "row":
			config.row = value
		case "template":
			config.template = value
		default:
			if !domFieldNamePattern.MatchString(key) {
				return domFinderConfig{}, false
			}
			if _, exists := config.fields[key]; exists {
				return domFinderConfig{}, false
			}
			selector, _, _ := strings.Cut(value, "@")
			if _, err := cascadia.Compile(strings.TrimSpace(selector)); err != nil {
				return domFinderConfig{}, false
			}
			config.fields[key] = value
		}
	}
	if config.row == "" || config.template == "" || len(config.fields) == 0 || (config.depth == 1 && config.links == "") {
		return domFinderConfig{}, false
	}
	if _, err := cascadia.Compile(config.row); err != nil {
		return domFinderConfig{}, false
	}
	fields := domTemplateFieldPattern.FindAllStringSubmatch(config.template, -1)
	if len(fields) == 0 {
		return domFinderConfig{}, false
	}
	for _, field := range fields {
		if _, exists := config.fields[field[1]]; !exists {
			return domFinderConfig{}, false
		}
	}
	return config, true
}

func parseCurlSpec(spec string) (int, string, []string, bool) {
	depth := 0
	if index := strings.IndexByte(spec, '-'); index >= 0 {
		candidate := spec[:index]
		if candidate != "" && isDecimal(candidate) {
			parsedDepth, err := strconv.Atoi(candidate)
			if err != nil || parsedDepth > maxCurlDepth {
				return 0, "", nil, false
			}
			depth = parsedDepth
			spec = spec[index+1:]
		}
	}

	selector := spec
	var protocols []string
	if isProxyScheme(strings.ToLower(spec)) {
		selector = ""
		protocols = []string{strings.ToLower(spec)}
	} else if index := strings.LastIndexByte(spec, '-'); index >= 0 {
		candidates := strings.Split(strings.ToLower(spec[index+1:]), "+")
		valid := len(candidates) > 0
		for _, candidate := range candidates {
			if !isProxyScheme(candidate) {
				valid = false
				break
			}
		}
		if valid {
			selector = spec[:index]
			protocols = candidates
		}
	}
	return depth, selector, protocols, true
}

func isDecimal(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func isProxyScheme(scheme string) bool {
	switch scheme {
	case "http", "https", "socks", "socks4", "socks4a", "socks5", "socks5a", "socks5h", "vmess", "vless", "trojan", "ss", "ssr", "hy", "hy2", "hysteria", "hysteria2", "hhysteria", "hhysteria2", "hhy2", "tuic", "wireguard", "anytls":
		return true
	default:
		return false
	}
}

func fetchCurlPages(root []byte, sourceURL, selector string) [][]byte {
	baseURL, err := url.Parse(sourceURL)
	if err != nil {
		return nil
	}
	links := extractHTMLLinks(root, baseURL)
	queue := make([]string, 0, len(links))
	for _, link := range links {
		if selector == "" || strings.Contains(strings.ToLower(link), strings.ToLower(selector)) {
			queue = append(queue, link)
		}
	}
	queue = append(queue, extractPaginationLinks(root, baseURL)...)
	seen := map[string]struct{}{sourceURL: {}}
	var pages [][]byte
	for len(queue) > 0 && len(pages) < maxCurlPageCount {
		rawURL := queue[0]
		queue = queue[1:]
		if _, exists := seen[rawURL]; exists || !isAllowedRegexLink(rawURL) {
			continue
		}
		seen[rawURL] = struct{}{}

		body, err := curlImpersonateFetch(rawURL)
		if err != nil {
			continue
		}
		pages = append(pages, body)
		pageURL, err := url.Parse(rawURL)
		if err == nil {
			queue = append(queue, extractPaginationLinks(body, pageURL)...)
		}
	}
	return pages
}

func fetchPaginatedPages(root []byte, sourceURL string) [][]byte {
	baseURL, err := url.Parse(sourceURL)
	if err != nil {
		return nil
	}
	queue := extractPaginationLinks(root, baseURL)
	seen := map[string]struct{}{}
	seen[sourceURL] = struct{}{}
	var pages [][]byte
	for len(queue) > 0 && len(pages) < maxCurlPageCount {
		rawURL := queue[0]
		queue = queue[1:]
		if _, exists := seen[rawURL]; exists || !isAllowedRegexLink(rawURL) {
			continue
		}
		seen[rawURL] = struct{}{}

		body, err := curlImpersonateFetch(rawURL)
		if err != nil {
			continue
		}
		pages = append(pages, body)
		pageURL, err := url.Parse(rawURL)
		if err == nil {
			queue = append(queue, extractPaginationLinks(body, pageURL)...)
		}
	}
	return pages
}

func extractPaginationLinks(body []byte, baseURL *url.URL) []string {
	document, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	var links []string
	seen := map[string]struct{}{}
	document.Find("a[href]").Each(func(_ int, anchor *goquery.Selection) {
		href, exists := anchor.Attr("href")
		if !exists || strings.TrimSpace(href) == "" {
			return
		}
		class, _ := anchor.Attr("class")
		ariaLabel, _ := anchor.Attr("aria-label")
		title, _ := anchor.Attr("title")
		rel, _ := anchor.Attr("rel")
		label := strings.ToLower(strings.Join(strings.Fields(anchor.Text()), " "))
		attributes := strings.ToLower(strings.Join([]string{class, ariaLabel, title, rel}, " "))
		if strings.Contains(attributes, "disabled") {
			return
		}

		isNext := false
		for _, token := range strings.Fields(strings.ToLower(rel)) {
			if token == "next" {
				isNext = true
			}
		}
		for _, token := range strings.Fields(strings.ToLower(class)) {
			if token == "next" || token == "next-page" || token == "pagination-next" {
				isNext = true
			}
		}
		if strings.Contains(strings.ToLower(ariaLabel+" "+title), "next") || label == "next" || label == "next page" || label == "older" || label == "›" || label == "»" || label == "→" {
			isNext = true
		}

		reference, err := url.Parse(strings.TrimSpace(href))
		if err != nil {
			return
		}
		resolved := reference
		if baseURL != nil {
			resolved = baseURL.ResolveReference(reference)
		}
		if !isNext && !isNumberedPaginationLink(anchor, resolved) {
			return
		}
		link := resolved.String()
		if _, duplicate := seen[link]; duplicate {
			return
		}
		seen[link] = struct{}{}
		links = append(links, link)
	})
	return links
}

func isNumberedPaginationLink(anchor *goquery.Selection, target *url.URL) bool {
	label := strings.TrimSpace(anchor.Text())
	if _, err := strconv.Atoi(label); err != nil {
		return false
	}
	query := target.Query()
	for _, key := range []string{"page", "p", "paged"} {
		if query.Get(key) != "" {
			return true
		}
	}
	if regexp.MustCompile(`(?i)(?:^|/)page/\d+/?$`).MatchString(target.Path) {
		return true
	}
	for ancestor := anchor; ancestor.Length() > 0; ancestor = ancestor.Parent() {
		class, _ := ancestor.Attr("class")
		id, _ := ancestor.Attr("id")
		role, _ := ancestor.Attr("role")
		if strings.Contains(strings.ToLower(class+" "+id+" "+role), "pagination") || strings.EqualFold(role, "navigation") {
			return true
		}
	}
	return false
}

func fetchCurlImpersonate(rawURL string) ([]byte, error) {
	if !isAllowedRegexLink(rawURL) {
		return nil, fmt.Errorf("curl-impersonate target is not allowed")
	}

	currentURL := rawURL
	for redirects := 0; redirects <= 5; redirects++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		command := exec.CommandContext(ctx, curlImpersonateBinary(),
			"--silent", "--show-error", "--compressed",
			"--max-redirs", "0", "--connect-timeout", "10", "--max-time", "20",
			"--max-filesize", strconv.Itoa(maxRegexLinkResponseBytes),
			"--proto", "=http,https",
			"--write-out", "\n%{http_code}:%{redirect_url}", currentURL,
		)
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		err := command.Run()
		cancel()
		if err != nil {
			return nil, fmt.Errorf("curl-impersonate fetch failed: %w", err)
		}

		output := stdout.Bytes()
		metadataStart := bytes.LastIndexByte(output, '\n')
		if metadataStart < 0 {
			return nil, fmt.Errorf("curl-impersonate returned no HTTP status")
		}
		metadata := strings.SplitN(string(output[metadataStart+1:]), ":", 2)
		if len(metadata) != 2 {
			return nil, fmt.Errorf("curl-impersonate returned invalid response metadata")
		}
		status, err := strconv.Atoi(metadata[0])
		if err != nil {
			return nil, fmt.Errorf("curl-impersonate returned invalid HTTP status")
		}
		if status >= http.StatusMultipleChoices && status < 400 {
			if redirects == 5 || metadata[1] == "" {
				return nil, fmt.Errorf("curl-impersonate redirect limit exceeded")
			}
			baseURL, _ := url.Parse(currentURL)
			redirectURL, err := url.Parse(metadata[1])
			if err != nil {
				return nil, fmt.Errorf("curl-impersonate returned invalid redirect URL")
			}
			currentURL = baseURL.ResolveReference(redirectURL).String()
			if !isAllowedRegexLink(currentURL) {
				return nil, errUnsafeRegexLinkRedirect
			}
			continue
		}
		if status < http.StatusOK || status >= http.StatusMultipleChoices {
			return nil, fmt.Errorf("curl-impersonate returned HTTP %d", status)
		}
		body := output[:metadataStart]
		if len(body) > maxRegexLinkResponseBytes {
			return nil, fmt.Errorf("curl-impersonate response exceeds size limit")
		}
		return body, nil
	}
	return nil, fmt.Errorf("curl-impersonate redirect limit exceeded")
}

func curlImpersonateBinary() string {
	if binary := strings.TrimSpace(os.Getenv("CURL_IMPERSONATE_BIN")); binary != "" {
		return binary
	}
	if binary, err := exec.LookPath("curl_chrome116"); err == nil {
		return binary
	}
	if home := os.Getenv("HOME"); home != "" {
		binary := filepath.Join(home, ".local", "bin", "curl_chrome116")
		if info, err := os.Stat(binary); err == nil && !info.IsDir() {
			return binary
		}
	}
	return "curl_chrome116"
}

func extractHTMLLinks(body []byte, baseURL *url.URL) []string {
	document, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	var links []string
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && (node.Data == "a" || node.Data == "link" || node.Data == "iframe") {
			for _, attribute := range node.Attr {
				if attribute.Key != "href" && attribute.Key != "src" {
					continue
				}
				reference, err := url.Parse(strings.TrimSpace(attribute.Val))
				if err != nil {
					continue
				}
				resolved := reference
				if baseURL != nil {
					resolved = baseURL.ResolveReference(reference)
				}
				links = append(links, resolved.String())
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	return links
}

func parseLinkSpec(spec string) (Transformer, string) {
	if spec == "" {
		return FromRaw, ""
	}
	if t, ok := Transformers[spec]; ok {
		return t, ""
	}
	for name, t := range Transformers {
		prefix := name + "-"
		if strings.HasPrefix(spec, prefix) {
			return t, strings.TrimPrefix(spec, prefix)
		}
	}
	return FromRaw, spec
}

func isAllowedRegexLink(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if allowPrivateRegexLinkHosts {
		return true
	}

	host := u.Hostname()
	if strings.EqualFold(host, "localhost") || IsLocal(host) {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !IsLocal(ip.String()) && isPublicIP(ip)
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return false
		}
	}
	return true
}

func isPublicIP(ip net.IP) bool {
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified()
}

// FlexPort handles YAML port values that may be int, float, or string.
type FlexPort int

func (p *FlexPort) UnmarshalYAML(value *yaml.Node) error {
	switch value.Tag {
	case "!!int":
		v, err := strconv.Atoi(value.Value)
		if err != nil {
			return err
		}
		*p = FlexPort(v)
		return nil
	case "!!float":
		v, err := strconv.ParseFloat(value.Value, 64)
		if err != nil {
			return err
		}
		// Reject NaN, Inf, non-integer floats, and out-of-range values before
		// casting. int(f) on NaN/Inf is implementation-defined and silently
		// truncates fractional parts (8080.9 -> 8080), so each case is checked
		// explicitly before the cast.
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("port must be a finite number, got %v", v)
		}
		if v != math.Trunc(v) {
			return fmt.Errorf("port must be an integer, got %v", v)
		}
		if v < 1 || v > 65535 {
			return fmt.Errorf("port must be in range [1, 65535], got %v", v)
		}
		*p = FlexPort(int(v))
		return nil
	case "!!str":
		port := strings.TrimFunc(value.Value, func(r rune) bool {
			return r < '0' || r > '9'
		})
		v, err := strconv.Atoi(port)
		if err != nil {
			return err
		}
		*p = FlexPort(v)
		return nil
	}
	// Unsupported types (bool, null, etc.) default to 0; port range check rejects them.
	return nil
}

// ClashWSHeaders represents WebSocket headers in Clash config.
type ClashWSHeaders struct {
	Host string `yaml:"Host,omitempty"`
}

// ClashWSOpts represents WebSocket transport options.
type ClashWSOpts struct {
	Path    string         `yaml:"path,omitempty"`
	Headers ClashWSHeaders `yaml:"headers,omitempty"`
}

// ClashGRPCOpts represents gRPC transport options.
type ClashGRPCOpts struct {
	ServiceName string `yaml:"grpc-service-name,omitempty"`
}

// ClashH2Opts represents HTTP/2 transport options.
type ClashH2Opts struct {
	Path string   `yaml:"path,omitempty"`
	Host []string `yaml:"host,omitempty"`
}

// ClashRealityOpts represents Reality TLS options.
type ClashRealityOpts struct {
	PublicKey string `yaml:"public-key,omitempty"`
	ShortID   string `yaml:"short-id,omitempty"`
}

// ClashProxy represents a single proxy entry in a Clash config.
type ClashProxy struct {
	Name              string            `yaml:"name,omitempty"`
	Type              string            `yaml:"type"`
	Server            string            `yaml:"server"`
	Port              FlexPort          `yaml:"port"`
	Cipher            string            `yaml:"cipher,omitempty"`
	Password          string            `yaml:"password,omitempty"`
	Username          string            `yaml:"username,omitempty"`
	UUID              string            `yaml:"uuid,omitempty"`
	AlterID           int               `yaml:"alterId,omitempty"`
	Network           string            `yaml:"network,omitempty"`
	TLS               bool              `yaml:"tls,omitempty"`
	ServerName        string            `yaml:"servername,omitempty"`
	SNI               string            `yaml:"sni,omitempty"`
	Flow              string            `yaml:"flow,omitempty"`
	ClientFingerprint string            `yaml:"client-fingerprint,omitempty"`
	WSOpts            *ClashWSOpts      `yaml:"ws-opts,omitempty"`
	GRPCOpts          *ClashGRPCOpts    `yaml:"grpc-opts,omitempty"`
	H2Opts            *ClashH2Opts      `yaml:"h2-opts,omitempty"`
	RealityOpts       *ClashRealityOpts `yaml:"reality-opts,omitempty"`
}

// ClashConfig represents a Clash YAML configuration.
type ClashConfig struct {
	Proxies []yaml.Node `yaml:"proxies"`
}

// FromClash parses a Clash YAML config and extracts proxy URLs.
func FromClash(buf []byte, _ string) []byte {
	// Limit YAML size to prevent OOM attacks (10MB max)
	const maxYAMLSize = 10 * 1024 * 1024
	if len(buf) > maxYAMLSize {
		return []byte{}
	}

	var config ClashConfig
	if err := yaml.Unmarshal(buf, &config); err != nil {
		config.Proxies = parseInlineClashProxies(buf)
		if len(config.Proxies) == 0 {
			return []byte{}
		}
	}

	var result bytes.Buffer
	for _, proxyNode := range config.Proxies {
		var proxy ClashProxy
		if err := proxyNode.Decode(&proxy); err != nil {
			continue
		}

		proxyURL := buildProxyURL(proxy)
		if proxyURL != "" {
			result.WriteString(proxyURL)
			result.WriteString("\n")
		}
	}

	return result.Bytes()
}

func parseInlineClashProxies(buf []byte) []yaml.Node {
	var proxies []yaml.Node
	inProxies := false
	for _, line := range strings.Split(string(buf), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !inProxies {
			inProxies = line == "proxies:"
			continue
		}
		if !strings.HasPrefix(line, "- {") {
			return nil
		}

		var document yaml.Node
		if err := yaml.Unmarshal([]byte(strings.TrimPrefix(line, "- ")), &document); err != nil {
			continue
		}
		if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
			continue
		}
		proxies = append(proxies, *document.Content[0])
	}
	return proxies
}

// hostPort formats server:port, handling IPv6 addresses correctly via net.JoinHostPort.
func hostPort(server string, port int) string {
	return net.JoinHostPort(server, strconv.Itoa(port))
}

func buildProxyURL(proxy ClashProxy) string {
	if proxy.Type == "" || strings.TrimSpace(proxy.Server) == "" {
		return ""
	}

	port := int(proxy.Port)

	// Validate port range
	if port < 1 || port > 65535 {
		return ""
	}

	switch proxy.Type {
	case "http", "https", "socks5", "socks4":
		u := &url.URL{
			Scheme: proxy.Type,
			Host:   hostPort(proxy.Server, port),
		}
		if proxy.Username != "" {
			if proxy.Password != "" {
				u.User = url.UserPassword(proxy.Username, proxy.Password)
			} else {
				u.User = url.User(proxy.Username)
			}
		}
		return u.String()
	case "ss":
		if proxy.Cipher == "" || proxy.Password == "" {
			return ""
		}
		// Shadowsocks URL format: ss://base64(cipher:password)@server:port
		// Uses standard base64 (RFC 4648) per SIP008 spec.
		credentials := base64.StdEncoding.EncodeToString([]byte(proxy.Cipher + ":" + proxy.Password))
		return fmt.Sprintf("ss://%s@%s", credentials, hostPort(proxy.Server, port))
	case "vmess":
		return buildVmessURL(proxy)
	case "vless":
		return buildVlessURL(proxy)
	case "trojan":
		return buildTrojanURL(proxy)
	default:
		return ""
	}
}

// buildVmessURL constructs a vmess:// URL from Clash YAML proxy fields.
// Format: vmess://base64(JSON) matching proxyclient VmessConfig struct.
func buildVmessURL(proxy ClashProxy) string {
	if proxy.UUID == "" {
		return ""
	}

	network := proxy.Network
	if network == "" {
		network = "tcp"
	}

	config := map[string]interface{}{
		"v":    "2",
		"ps":   proxy.Name,
		"add":  proxy.Server,
		"port": int(proxy.Port),
		"id":   proxy.UUID,
		"aid":  proxy.AlterID,
		"net":  network,
		"type": "none",
		"host": "",
		"path": "",
		"tls":  "",
	}

	if proxy.TLS {
		config["tls"] = "tls"
	}
	if proxy.SNI != "" {
		config["sni"] = proxy.SNI
	} else if proxy.ServerName != "" {
		config["sni"] = proxy.ServerName
	}
	if proxy.Cipher != "" {
		config["security"] = proxy.Cipher
	}
	if proxy.ClientFingerprint != "" {
		config["fp"] = proxy.ClientFingerprint
	}

	// Transport-specific settings
	switch network {
	case "ws":
		if proxy.WSOpts != nil {
			if proxy.WSOpts.Path != "" {
				config["path"] = proxy.WSOpts.Path
			}
			if proxy.WSOpts.Headers.Host != "" {
				config["host"] = proxy.WSOpts.Headers.Host
			}
		}
	case "grpc":
		if proxy.GRPCOpts != nil && proxy.GRPCOpts.ServiceName != "" {
			config["path"] = proxy.GRPCOpts.ServiceName
		}
	case "h2":
		if proxy.H2Opts != nil {
			if proxy.H2Opts.Path != "" {
				config["path"] = proxy.H2Opts.Path
			}
			if len(proxy.H2Opts.Host) > 0 {
				config["host"] = proxy.H2Opts.Host[0]
			}
		}
	}

	jsonBytes, err := json.Marshal(config)
	if err != nil {
		return ""
	}

	encoded := base64.StdEncoding.EncodeToString(jsonBytes)
	return "vmess://" + encoded
}

// buildVlessURL constructs a vless:// URL from Clash YAML proxy fields.
// Format: vless://uuid@host:port?encryption=none&type=tcp&security=tls&...#name
// Matches proxyclient ParseVlessURL expected format.
func buildVlessURL(proxy ClashProxy) string {
	if proxy.UUID == "" {
		return ""
	}

	hp := hostPort(proxy.Server, int(proxy.Port))

	params := url.Values{}
	params.Set("encryption", "none")

	network := proxy.Network
	if network == "" {
		network = "tcp"
	}
	params.Set("type", network)

	if proxy.TLS {
		params.Set("security", "tls")
	}
	if proxy.SNI != "" {
		params.Set("sni", proxy.SNI)
	} else if proxy.ServerName != "" {
		params.Set("sni", proxy.ServerName)
	}
	if proxy.Flow != "" {
		params.Set("flow", proxy.Flow)
	}
	if proxy.ClientFingerprint != "" {
		params.Set("fp", proxy.ClientFingerprint)
	}

	switch network {
	case "ws":
		if proxy.WSOpts != nil {
			if proxy.WSOpts.Path != "" {
				params.Set("path", proxy.WSOpts.Path)
			}
			if proxy.WSOpts.Headers.Host != "" {
				params.Set("host", proxy.WSOpts.Headers.Host)
			}
		}
	case "grpc":
		if proxy.GRPCOpts != nil && proxy.GRPCOpts.ServiceName != "" {
			params.Set("serviceName", proxy.GRPCOpts.ServiceName)
		}
	case "h2":
		if proxy.H2Opts != nil {
			if proxy.H2Opts.Path != "" {
				params.Set("path", proxy.H2Opts.Path)
			}
			if len(proxy.H2Opts.Host) > 0 {
				params.Set("host", proxy.H2Opts.Host[0])
			}
		}
	}

	// Reality overrides TLS security
	if proxy.RealityOpts != nil {
		params.Set("security", "reality")
		if proxy.RealityOpts.PublicKey != "" {
			params.Set("pbk", proxy.RealityOpts.PublicKey)
		}
		if proxy.RealityOpts.ShortID != "" {
			params.Set("sid", proxy.RealityOpts.ShortID)
		}
	}

	u := &url.URL{
		Scheme:   "vless",
		User:     url.User(proxy.UUID),
		Host:     hp,
		RawQuery: params.Encode(),
		Fragment: proxy.Name,
	}

	return u.String()
}

// buildTrojanURL constructs a trojan:// URL from Clash YAML proxy fields.
// Format: trojan://password@host:port?security=tls&type=tcp&...#name
// Matches proxyclient ParseTrojanURL expected format.
func buildTrojanURL(proxy ClashProxy) string {
	if proxy.Password == "" {
		return ""
	}

	hp := hostPort(proxy.Server, int(proxy.Port))

	params := url.Values{}

	network := proxy.Network
	if network == "" {
		network = "tcp"
	}
	params.Set("type", network)

	// Trojan checks both "sni" and "servername" in Clash configs
	if proxy.SNI != "" {
		params.Set("sni", proxy.SNI)
	} else if proxy.ServerName != "" {
		params.Set("sni", proxy.ServerName)
	}
	if proxy.ClientFingerprint != "" {
		params.Set("fp", proxy.ClientFingerprint)
	}

	switch network {
	case "ws":
		if proxy.WSOpts != nil {
			if proxy.WSOpts.Path != "" {
				params.Set("path", proxy.WSOpts.Path)
			}
			if proxy.WSOpts.Headers.Host != "" {
				params.Set("host", proxy.WSOpts.Headers.Host)
			}
		}
	case "grpc":
		if proxy.GRPCOpts != nil && proxy.GRPCOpts.ServiceName != "" {
			params.Set("serviceName", proxy.GRPCOpts.ServiceName)
		}
	case "h2":
		if proxy.H2Opts != nil {
			if proxy.H2Opts.Path != "" {
				params.Set("path", proxy.H2Opts.Path)
			}
			if len(proxy.H2Opts.Host) > 0 {
				params.Set("host", proxy.H2Opts.Host[0])
			}
		}
	}

	// Reality overrides default TLS security
	if proxy.RealityOpts != nil {
		params.Set("security", "reality")
		if proxy.RealityOpts.PublicKey != "" {
			params.Set("pbk", proxy.RealityOpts.PublicKey)
		}
		if proxy.RealityOpts.ShortID != "" {
			params.Set("sid", proxy.RealityOpts.ShortID)
		}
	} else {
		params.Set("security", "tls")
	}

	u := &url.URL{
		Scheme:   "trojan",
		User:     url.User(proxy.Password),
		Host:     hp,
		RawQuery: params.Encode(),
		Fragment: proxy.Name,
	}

	return u.String()
}
