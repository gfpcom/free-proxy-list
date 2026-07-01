package internal

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	maxRegexLinkCount         = 32
	maxRegexLinkResponseBytes = 10 * 1024 * 1024
)

var (
	Transformers               = map[string]Transformer{}
	allowPrivateRegexLinkHosts = false
	errUnsafeRegexLinkRedirect = errors.New("unsafe regex link redirect")
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
}

type Transformer func(data []byte, options string) []byte

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
		v, err := strconv.Atoi(value.Value)
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
	Proxies []ClashProxy `yaml:"proxies"`
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
		return []byte{}
	}

	var result bytes.Buffer
	for _, proxy := range config.Proxies {
		proxyURL := buildProxyURL(proxy)
		if proxyURL != "" {
			result.WriteString(proxyURL)
			result.WriteString("\n")
		}
	}

	return result.Bytes()
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
