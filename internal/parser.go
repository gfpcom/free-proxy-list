package internal

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/cnlangzi/proxyclient/ss"
	"github.com/cnlangzi/proxyclient/xray"
)

var (
	Parsers = map[string]Parser{}

	ErrInvalidProxy = errors.New("gfp: invalid proxy")
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
}

func ParseProxyURL(proto, proxyURL string) (*Proxy, error) {
	if !strings.Contains(proxyURL, "://") {
		proxyURL = proto + "://" + proxyURL
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	scheme := strings.ToLower(u.Scheme)

	var it *Proxy
	switch scheme {
	case "http", "https", "socks4", "socks4a", "socks5", "socks5h":
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
		it.Protocol = proto

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

	}

	if it == nil {
		return nil, ErrInvalidProxy
	}

	if IsLocal(it.IP) {
		return nil, ErrInvalidProxy
	}

	it.Protocol = scheme

	return it, nil
}

func IsLocal(ip string) bool {
	return strings.HasPrefix(ip, "0.") || strings.HasPrefix(ip, "127.") || strings.HasPrefix(ip, "169.254.")
}

func IsLocalIP(ip string) bool {
	return strings.HasPrefix(ip, "0.") || strings.HasPrefix(ip, "127.") || strings.HasPrefix(ip, "169.254.")
}

func ParseColonURL(proto, proxyURL string) (*Proxy, error) {
	items := strings.Split(proxyURL, ":")

	if len(items) < 2 {
		return nil, ErrInvalidProxy
	}

	return ParseProxyURL(proto, items[0]+":"+items[1])
}
