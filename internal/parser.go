package internal

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/cnlangzi/proxyclient"
	"github.com/cnlangzi/proxyclient/ss"
	"github.com/cnlangzi/proxyclient/xray"
)

var (
	Parsers         = map[string]Parser{}
	ParserFactories = map[string]ParserFactory{}

	ErrInvalidProxy = errors.New("gfp: invalid proxy")
)

const (
	// MaxSchemeLength defines the maximum allowed length for proxy scheme.
	// Legitimate proxy protocols (http, https, socks4, socks5, vmess, trojan, vless, ss, ssr, tg, hy, hy2)
	// are all 6 characters or less. 15 provides safe headroom for future protocols.
	MaxSchemeLength = 15
)

type Parser func(string, string) (*Proxy, error)
type ParserFactory func(string) (Parser, error)

func RegisterParser(name string, parser Parser) {
	Parsers[name] = parser
}

func RegisterParserFactory(name string, factory ParserFactory) {
	ParserFactories[name] = factory
}

func GetParser(spec string) (Parser, error) {
	name, options, hasOptions := strings.Cut(spec, ":")
	if factory, ok := ParserFactories[name]; ok {
		return factory(options)
	}
	if hasOptions {
		return nil, errors.New("parser does not accept options")
	}
	if parser, ok := Parsers[name]; ok {
		return parser, nil
	}

	return ParseProxyURL, nil
}

func init() {
	Parsers["ColonURL"] = ParseColonURL
	Parsers["SpaceURL"] = ParseSpaceURL
	Parsers["IPv4Auth"] = ParseIPv4Auth
	RegisterParserFactory("Split", newSplitParser)
}

func ParseProxyURL(proto, proxyURL string) (*Proxy, error) {
	if fields := strings.Fields(proxyURL); len(fields) > 0 {
		proxyURL = fields[0]
	}

	if !strings.Contains(proxyURL, "://") {
		proxyURL = proto + "://" + proxyURL
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	scheme := strings.ToLower(u.Scheme)

	// Validate scheme length to prevent invalid protocols
	if len(scheme) == 0 || len(scheme) > MaxSchemeLength {
		return nil, ErrInvalidProxy
	}

	// Convert hysteria to hy and hysteria2 to hy2
	switch scheme {
	case "hysteria", "hhysteria":
		scheme = "hy"
		proxyURL = "hy://" + strings.TrimPrefix(proxyURL, u.Scheme+"://")
		u, _ = url.Parse(proxyURL)
	case "hysteria2", "hhy2", "hhysteria2":
		scheme = "hy2"
		proxyURL = "hy2://" + strings.TrimPrefix(proxyURL, u.Scheme+"://")
		u, _ = url.Parse(proxyURL)
	}

	if scheme == "tg" || (strings.EqualFold(proto, "tg") && isTelegramProxyLink(u)) {
		return parseMTProtoProxyURL(u)
	}

	var it *Proxy
	switch scheme {
	case "vmess":
		vu, err := xray.ParseVmessURL(u)
		if err != nil {
			return nil, err
		}

		it = &Proxy{
			IP: vu.Host(),
		}

		port, err := strconv.Atoi(vu.Port())
		if err != nil {
			return nil, ErrInvalidProxy
		}
		it.Port = port
		it.Opaque = strings.TrimPrefix(vu.Raw().String(), "vmess://")

	case "trojan":
		vu, err := xray.ParseTrojanURL(u)
		if err != nil {
			return nil, err
		}

		it = &Proxy{
			IP: vu.Host(),
		}

		port, err := strconv.Atoi(vu.Port())
		if err != nil {
			return nil, ErrInvalidProxy
		}
		it.Port = port
		it.Opaque = strings.TrimPrefix(vu.Raw().String(), "trojan://")
	case "vless":
		vu, err := xray.ParseVlessURL(u)
		if err != nil {
			return nil, err
		}

		it = &Proxy{
			IP: vu.Host(),
		}

		port, err := strconv.Atoi(vu.Port())
		if err != nil {
			return nil, ErrInvalidProxy
		}
		it.Port = port
		it.Opaque = strings.TrimPrefix(vu.Raw().String(), "vless://")
	case "ss":
		vu, err := ss.ParseSSURL(u)
		if err != nil {
			return nil, err
		}

		it = &Proxy{
			IP: vu.Host(),
		}

		port, err := strconv.Atoi(vu.Port())
		if err != nil {
			return nil, ErrInvalidProxy
		}
		it.Port = port
		it.Opaque = strings.TrimPrefix(vu.Raw().String(), "ss://")
	case "ssr":
		vu, err := xray.ParseSSRURL(u)
		if err != nil {
			return nil, err
		}

		it = &Proxy{
			IP: vu.Host(),
		}

		port, err := strconv.Atoi(vu.Port())
		if err != nil {
			return nil, ErrInvalidProxy
		}
		it.Port = port
		it.Opaque = strings.TrimPrefix(vu.Raw().String(), "ssr://")
	default: // "http", "https", "socks4", "socks4a", "socks5", "socks5h":
		it = &Proxy{
			IP:   u.Hostname(),
			User: u.User.Username(),
		}

		port, err := strconv.Atoi(u.Port())
		if err != nil {
			return nil, ErrInvalidProxy
		}

		it.Port = port

		it.Passwd, _ = u.User.Password()
		it.Protocol = scheme
	}

	if isIPLiteralCandidate(it.IP) && net.ParseIP(it.IP) == nil {
		return nil, ErrInvalidProxy
	}
	if IsLocal(it.IP) {
		return nil, ErrInvalidProxy
	}

	if !proxyclient.IsHost(it.IP) {
		slog.Warn("gfp: invalid", slog.String("proto", proto), slog.String("proxy", proxyURL), slog.String("ip", it.IP))
		return nil, ErrInvalidProxy
	}

	it.Protocol = scheme

	return it, nil
}

func isTelegramProxyLink(u *url.URL) bool {
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return false
	}

	host := strings.ToLower(u.Hostname())
	return (host == "t.me" || host == "telegram.me") && u.Path == "/proxy"
}

func parseMTProtoProxyURL(u *url.URL) (*Proxy, error) {
	if strings.EqualFold(u.Scheme, "tg") {
		if !strings.EqualFold(u.Hostname(), "proxy") || (u.Path != "" && u.Path != "/") {
			return nil, ErrInvalidProxy
		}
	} else if !isTelegramProxyLink(u) {
		return nil, ErrInvalidProxy
	}

	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, ErrInvalidProxy
	}

	server := query.Get("server")
	portValue := query.Get("port")
	secret := query.Get("secret")
	if len(query["server"]) != 1 || len(query["port"]) != 1 || len(query["secret"]) != 1 ||
		server == "" || portValue == "" || !isValidMTProtoSecret(secret) {
		return nil, ErrInvalidProxy
	}

	port, err := strconv.Atoi(portValue)
	if err != nil || port < 1 || port > 65535 {
		return nil, ErrInvalidProxy
	}
	if isIPLiteralCandidate(server) && net.ParseIP(server) == nil {
		return nil, ErrInvalidProxy
	}
	if IsLocal(server) || isPrivateMTProtoServer(server) || !proxyclient.IsHost(server) {
		return nil, ErrInvalidProxy
	}

	normalizedQuery := url.Values{
		"server": {server},
		"port":   {strconv.Itoa(port)},
		"secret": {secret},
	}
	return &Proxy{
		IP:       server,
		Port:     port,
		Opaque:   "proxy?" + normalizedQuery.Encode(),
		Protocol: "tg",
	}, nil
}

func isValidMTProtoSecret(secret string) bool {
	if decoded, err := hex.DecodeString(secret); err == nil {
		switch {
		case len(decoded) == 16:
			return true
		case strings.HasPrefix(strings.ToLower(secret), "dd") && len(decoded) == 17:
			return true
		case strings.HasPrefix(strings.ToLower(secret), "ee") && len(decoded) >= 17:
			return true
		}
	}

	for _, encoding := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding} {
		if decoded, err := encoding.DecodeString(secret); err == nil && len(decoded) == 16 {
			return true
		}
	}
	return false
}

func isPrivateMTProtoServer(server string) bool {
	ip := net.ParseIP(server)
	if ip == nil {
		return false
	}
	return !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

func isIPLiteralCandidate(host string) bool {
	// Callers pass URL.Hostname output, so the port is already removed and colons indicate IPv6 syntax.
	if strings.Contains(host, ":") {
		return true
	}

	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}

	numericParts := 0
	for _, part := range parts {
		if part == "" {
			continue
		}
		numeric := true
		for _, char := range part {
			if char < '0' || char > '9' {
				numeric = false
				break
			}
		}
		if numeric {
			numericParts++
		}
	}
	return numericParts >= 3
}

func IsLocal(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return strings.HasPrefix(ip, "0.") || strings.HasPrefix(ip, "127.") || strings.HasPrefix(ip, "169.254.")
	}
	if ipv4 := parsedIP.To4(); ipv4 != nil {
		ip = ipv4.String()
		return strings.HasPrefix(ip, "0.") || strings.HasPrefix(ip, "127.") || strings.HasPrefix(ip, "169.254.")
	}
	return parsedIP.IsLoopback() || parsedIP.IsLinkLocalUnicast() || parsedIP.IsPrivate() || parsedIP.IsUnspecified()
}

func ParseColonURL(proto, proxyURL string) (*Proxy, error) {
	items := strings.Split(proxyURL, ":")

	if len(items) < 2 {
		return nil, ErrInvalidProxy
	}

	return ParseProxyURL(proto, items[0]+":"+items[1])
}

func newSplitParser(options string) (Parser, error) {
	separator := ""
	var hostColumn, portColumn int
	hasSeparator, hasHost, hasPort := false, false, false
	for _, option := range strings.Split(options, ";") {
		key, value, ok := strings.Cut(option, "=")
		if !ok {
			return nil, errors.New("split parser options must be key=value pairs")
		}
		switch key {
		case "separator":
			if hasSeparator {
				return nil, errors.New("split parser separator is configured more than once")
			}
			separator, hasSeparator = value, true
			continue
		}
		index, err := strconv.Atoi(value)
		if err != nil || index < 0 {
			return nil, errors.New("split parser column indexes must be non-negative integers")
		}
		switch key {
		case "host":
			if hasHost {
				return nil, errors.New("split parser host column is configured more than once")
			}
			hostColumn, hasHost = index, true
		case "port":
			if hasPort {
				return nil, errors.New("split parser port column is configured more than once")
			}
			portColumn, hasPort = index, true
		default:
			return nil, errors.New("split parser supports only separator, host, and port options")
		}
	}
	if !hasSeparator || separator == "" {
		return nil, errors.New("split parser requires a separator")
	}
	if !hasHost || !hasPort {
		return nil, errors.New("split parser requires host and port column mappings")
	}
	switch separator {
	case "comma":
		separator = ","
	case "space", "whitespace":
		separator = " "
	case "tab":
		separator = "\t"
	default:
		if len(separator) != 1 {
			return nil, errors.New("split parser separator must be comma, whitespace, tab, or one character")
		}
	}

	return func(proto, line string) (*Proxy, error) {
		var record []string
		if separator == " " {
			record = strings.Fields(line)
		} else {
			record = strings.Split(line, separator)
		}
		if hostColumn >= len(record) || portColumn >= len(record) {
			return nil, ErrInvalidProxy
		}
		host := strings.TrimSpace(record[hostColumn])
		portValue, err := strconv.Atoi(strings.TrimSpace(record[portColumn]))
		if host == "" || err != nil || portValue < 1 || portValue > 65535 {
			return nil, ErrInvalidProxy
		}
		return ParseProxyURL(proto, net.JoinHostPort(host, strconv.Itoa(portValue)))
	}, nil
}

func ParseIPv4Auth(proto, proxyLine string) (*Proxy, error) {
	items := strings.SplitN(strings.TrimSpace(proxyLine), ":", 4)
	if len(items) != 4 || items[2] == "" || items[3] == "" {
		return nil, ErrInvalidProxy
	}

	ip := items[0]
	parsedIP := net.ParseIP(ip)
	if strings.Contains(ip, ":") || parsedIP == nil || parsedIP.To4() == nil || IsLocal(ip) || !proxyclient.IsHost(ip) {
		return nil, ErrInvalidProxy
	}

	port, err := strconv.Atoi(items[1])
	if err != nil || port < 1 || port > 65535 {
		return nil, ErrInvalidProxy
	}

	return &Proxy{
		IP:       ip,
		Port:     port,
		User:     items[2],
		Passwd:   items[3],
		Protocol: strings.ToLower(proto),
	}, nil
}

func ParseSpaceURL(proto, proxyURL string) (*Proxy, error) {
	items := strings.Split(proxyURL, " ")

	if len(items) < 2 {
		return nil, ErrInvalidProxy
	}

	return ParseProxyURL(proto, items[0]+":"+items[1])
}
