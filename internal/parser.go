package internal

import (
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
	Parsers = map[string]Parser{}

	ErrInvalidProxy = errors.New("gfp: invalid proxy")
)

const (
	// MaxSchemeLength defines the maximum allowed length for proxy scheme.
	// Legitimate proxy protocols (http, https, socks4, socks5, vmess, trojan, vless, ss, ssr, hy, hy2)
	// are all 6 characters or less. 15 provides safe headroom for future protocols.
	MaxSchemeLength = 15
)

type Parser func(string, string) (*Proxy, error)

func RegisterParser(name string, parser Parser) {
	Parsers[name] = parser
}

func GetParser(name string) Parser {
	if parser, ok := Parsers[name]; ok {
		return parser
	}

	return ParseProxyURL
}

func init() {
	Parsers["ColonURL"] = ParseColonURL
	Parsers["SpaceURL"] = ParseSpaceURL
	Parsers["IPv4Auth"] = ParseIPv4Auth
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
	return strings.HasPrefix(ip, "0.") || strings.HasPrefix(ip, "127.") || strings.HasPrefix(ip, "169.254.")
}

func ParseColonURL(proto, proxyURL string) (*Proxy, error) {
	items := strings.Split(proxyURL, ":")

	if len(items) < 2 {
		return nil, ErrInvalidProxy
	}

	return ParseProxyURL(proto, items[0]+":"+items[1])
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
