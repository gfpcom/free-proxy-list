package internal

import (
	"errors"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"github.com/cnlangzi/proxyclient"
	"github.com/cnlangzi/proxyclient/ss"
	"github.com/cnlangzi/proxyclient/xray"
)

// Parsers contains named parser implementations used to parse custom lines.
var (
	Parsers = map[string]Parser{}

	// ErrInvalidProxy is returned when a proxy URL cannot be parsed or is
	// rejected by validation rules.
	ErrInvalidProxy = errors.New("gfp: invalid proxy")
)

// Parser is a function that parses a single line into a Proxy object.
type Parser func(string, string) (*Proxy, error)

// RegisterParser registers a Parser under the given name.
func RegisterParser(name string, parser Parser) {
	Parsers[name] = parser
}

// GetParser returns the parser registered under name or the default
// ParseProxyURL if no parser is found.
func GetParser(name string) Parser {
	if parser, ok := Parsers[name]; ok {
		return parser
	}

	return ParseProxyURL
}

func init() {
	Parsers["ColonURL"] = ParseColonURL
	Parsers["SpaceURL"] = ParseSpaceURL
}

// ParseProxyURL parses a proxy URL into a Proxy struct and performs basic
// validation. It supports many schemes such as vmess, vless, ss, ssr, trojan,
// and standard http/socks proxies.
func ParseProxyURL(proto, proxyURL string) (*Proxy, error) {
	if !strings.Contains(proxyURL, "://") {
		proxyURL = proto + "://" + proxyURL
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	scheme := strings.ToLower(u.Scheme)

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

// IsLocal returns true when ip is a link-local or localhost address and should
// be rejected.
func IsLocal(ip string) bool {
	return strings.HasPrefix(ip, "0.") || strings.HasPrefix(ip, "127.") || strings.HasPrefix(ip, "169.254.")
}

// ParseColonURL accepts `ip:port` style lines and delegates to ParseProxyURL.
func ParseColonURL(proto, proxyURL string) (*Proxy, error) {
	items := strings.Split(proxyURL, ":")

	if len(items) < 2 {
		return nil, ErrInvalidProxy
	}

	return ParseProxyURL(proto, items[0]+":"+items[1])
}

// ParseSpaceURL accepts `ip port` style lines and delegates to ParseProxyURL.
func ParseSpaceURL(proto, proxyURL string) (*Proxy, error) {
	items := strings.Split(proxyURL, " ")

	if len(items) < 2 {
		return nil, ErrInvalidProxy
	}

	return ParseProxyURL(proto, items[0]+":"+items[1])
}
