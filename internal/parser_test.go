package internal

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParser(t *testing.T) {
	u := "vless://Telegram-EXPRESSVPN_420@expressvpn_420.fast.hosting-ip.com:80/?type=ws&encryption=none&host=V2RAY_420.nettisbdaak.net&path=%2F%40EXPRESSVPN_420------%40EXPRESSVPN_420------%40EXPRESSVPN_420------%40EXPRESSVPN_420------%40EXPRESSVPN_420------%40EXPRESSVPN_420------%40EXPRESSVPN_420------%40EXPRESSVPN_420------%40EXPRESSVPN_420%3Fed%3D2048#%F0%9F%91%89%F0%9F%86%94%20%40v2ray_configs_pool%F0%9F%93%A1%F0%9F%87%BA%F0%9F%87%B8United%20States"

	proxy, err := ParseProxyURL("vless", u)

	require.NoError(t, err)
	fmt.Println(proxy)
}

func TestParseProxyURLIgnoresTrailingAnnotation(t *testing.T) {
	proxy, err := ParseProxyURL("auto", "socks5://80.76.49.48:60001      入库时间：09-30 07:50 [机房]")

	require.NoError(t, err)
	require.Equal(t, "80.76.49.48", proxy.IP)
	require.Equal(t, 60001, proxy.Port)
	require.Equal(t, "socks5", proxy.Protocol)
}

func TestParseMTProtoProxyURL(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "tg scheme",
			line: "tg://proxy?server=8.8.8.8&port=0443&secret=dd0123456789abcdef0123456789abcdef",
			want: "tg://proxy?port=443&secret=dd0123456789abcdef0123456789abcdef&server=8.8.8.8",
		},
		{
			name: "Telegram link with a hostname",
			line: "https://t.me/proxy?server=proxy.example.com&port=8443&secret=ee0123456789abcdef0123456789abcdef6578616d706c652e636f6d#Fast",
			want: "tg://proxy?port=8443&secret=ee0123456789abcdef0123456789abcdef6578616d706c652e636f6d&server=proxy.example.com",
		},
		{
			name: "Telegram alternate domain",
			line: "https://telegram.me/proxy?server=8.8.4.4&port=443&secret=AAAAAAAAAAAAAAAAAAAAAA==",
			want: "tg://proxy?port=443&secret=AAAAAAAAAAAAAAAAAAAAAA%3D%3D&server=8.8.4.4",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proxy, err := ParseProxyURL("tg", test.line)
			require.NoError(t, err)
			require.Equal(t, "tg", proxy.Protocol)
			require.Equal(t, test.want, proxy.String())
		})
	}
}

func TestParseMTProtoProxyURLRejectsInvalidLinks(t *testing.T) {
	tests := []string{
		"tg://proxy?server=8.8.8.8&port=443",
		"tg://proxy?server=8.8.8.8&port=443&secret=not-a-secret",
		"tg://other?server=8.8.8.8&port=443&secret=0123456789abcdef0123456789abcdef",
		"tg://proxy?server=8.8.8.8&port=0&secret=0123456789abcdef0123456789abcdef",
		"tg://proxy?server=8.8.8.8&port=65536&secret=0123456789abcdef0123456789abcdef",
		"tg://proxy?server=127.0.0.1&port=443&secret=0123456789abcdef0123456789abcdef",
		"tg://proxy?server=10.0.0.1&port=443&secret=0123456789abcdef0123456789abcdef",
		"tg://proxy?server=::1&port=443&secret=0123456789abcdef0123456789abcdef",
		"tg://proxy?server=fc00::1&port=443&secret=0123456789abcdef0123456789abcdef",
		"tg://proxy?server=999.8.8.8&port=443&secret=0123456789abcdef0123456789abcdef",
		"tg://proxy?server=8.8.8.8&port=443&port=444&secret=0123456789abcdef0123456789abcdef",
		"https://t.me/socks?server=8.8.8.8&port=443&secret=0123456789abcdef0123456789abcdef",
	}

	for _, line := range tests {
		t.Run(line, func(t *testing.T) {
			_, err := ParseProxyURL("tg", line)
			require.Error(t, err)
		})
	}
}

func TestParseIPv4Auth(t *testing.T) {
	parser, err := GetParser("IPv4Auth")
	require.NoError(t, err)
	proxy, err := parser("http", "104.207.38.226:3129:proxy-user:proxy-pass")

	require.NoError(t, err)
	require.Equal(t, "104.207.38.226", proxy.IP)
	require.Equal(t, 3129, proxy.Port)
	require.Equal(t, "proxy-user", proxy.User)
	require.Equal(t, "proxy-pass", proxy.Passwd)
	require.Equal(t, "http", proxy.Protocol)
}

func TestParseSplitColumns(t *testing.T) {
	commaParser, err := GetParser("Split:separator=comma;host=1;port=3")
	require.NoError(t, err)
	spaceParser, err := GetParser("Split:separator=space;host=0;port=1")
	require.NoError(t, err)
	pipeParser, err := GetParser("Split:separator=|;host=1;port=3")
	require.NoError(t, err)

	for _, protocol := range []string{"http", "https"} {
		for _, test := range []struct {
			parser Parser
			line   string
		}{
			{parser: commaParser, line: "US,8.8.8.8,Example Inc,443,metadata"},
			{parser: spaceParser, line: "8.8.8.8 443 US Example Inc"},
			{parser: pipeParser, line: "US|8.8.8.8|Example Inc|443|metadata"},
		} {
			proxy, err := test.parser(protocol, test.line)

			require.NoError(t, err)
			require.Equal(t, "8.8.8.8", proxy.IP)
			require.Equal(t, 443, proxy.Port)
			require.Equal(t, protocol, proxy.Protocol)
		}
	}
}

func TestParseSplitEndpointAndProtocol(t *testing.T) {
	parser, err := GetParser("Split:separator=comma;endpoint=0;protocol=1")
	require.NoError(t, err)

	tests := []struct {
		protocol string
		line     string
		wantIP   string
		wantPort int
	}{
		{protocol: "http", line: "191.101.1.116:80, HTTP, 16ms", wantIP: "191.101.1.116", wantPort: 80},
		{protocol: "https", line: "191.101.1.116:443, HTTPS, 16ms", wantIP: "191.101.1.116", wantPort: 443},
		{protocol: "socks4", line: "47.88.94.79:1080, SOCKS4, 86ms", wantIP: "47.88.94.79", wantPort: 1080},
		{protocol: "socks5", line: "47.88.94.79:1080, SOCKS5, 86ms", wantIP: "47.88.94.79", wantPort: 1080},
	}
	for _, test := range tests {
		t.Run(test.protocol, func(t *testing.T) {
			proxy, err := parser(test.protocol, test.line)
			require.NoError(t, err)
			require.Equal(t, test.wantIP, proxy.IP)
			require.Equal(t, test.wantPort, proxy.Port)
			require.Equal(t, test.protocol, proxy.Protocol)
		})
	}

	_, err = parser("http", "47.88.94.79:1080, SOCKS4, 86ms")
	require.Error(t, err)
	_, err = parser("http", "47.88.94.79:invalid, HTTP, 86ms")
	require.Error(t, err)
}

func TestSplitParserOptions(t *testing.T) {
	for _, test := range []struct {
		spec string
	}{
		{spec: "Split"},
		{spec: "Split:host=0;port=1"},
		{spec: "Split:separator=comma;host=-1;port=1"},
		{spec: "Split:separator=comma;host=one;port=1"},
		{spec: "Split:separator=comma;host=0;port=1;other=2"},
		{spec: "Split:separator=comma;host=0;host=1;port=2"},
		{spec: "Split:separator=comma;host=0;port=1;separator=|"},
		{spec: "Split:separator=multi;host=0;port=1"},
		{spec: "Split:separator=comma;endpoint=0;endpoint=1"},
		{spec: "Split:separator=comma;endpoint=0;protocol=0"},
		{spec: "Split:separator=comma;endpoint=0;protocol=1;host=2;port=3"},
		{spec: "Split:separator=comma;host=0;port=1;protocol=2"},
	} {
		t.Run(test.spec, func(t *testing.T) {
			_, err := GetParser(test.spec)
			require.Error(t, err)
		})
	}
}

func TestSplitParserRejectsInvalidLines(t *testing.T) {
	parser, err := GetParser("Split:separator=comma;host=0;port=1")
	require.NoError(t, err)
	tests := []string{
		"8.8.8.8",
		"8.8.8.8,invalid,US,provider",
		"8.8.8.8,0,US,provider",
		"8.8.8.8,65536,US,provider",
		"127.0.0.1,443,US,provider",
	}

	for _, line := range tests {
		t.Run(line, func(t *testing.T) {
			_, err := parser("http", line)
			require.Error(t, err)
		})
	}
}

func TestParseLineConfiguredParser(t *testing.T) {
	_, _, _, parser, err := parseLine("https://feed.example/proxies.csv,,Split:separator=comma;host=2;port=3")
	require.NoError(t, err)

	proxy, err := parser("http", "US,Provider,8.8.8.8,443")
	require.NoError(t, err)
	require.Equal(t, "8.8.8.8", proxy.IP)
	require.Equal(t, 443, proxy.Port)
}

func TestValidateSourceRejectsInvalidParserOptions(t *testing.T) {
	err := ValidateSource("http", []byte("https://feed.example/proxies.txt,,Split:separator=comma;host=0"))
	require.ErrorContains(t, err, "split parser requires either an endpoint column or host and port column mappings")
}

func TestParseIPv4AuthRejectsInvalidLines(t *testing.T) {
	tests := []string{
		"104.207.38.226:3129:proxy-user",
		"not-an-ip:3129:proxy-user:proxy-pass",
		"104.207.38.226:0:proxy-user:proxy-pass",
		"104.207.38.226:65536:proxy-user:proxy-pass",
		"104.207.38.226:3129::proxy-pass",
		"104.207.38.226:3129:proxy-user:",
		"127.0.0.1:3129:proxy-user:proxy-pass",
		"::ffff:8.8.8.8:3129:proxy-user:proxy-pass",
		"::ffff:127.0.0.1:3129:proxy-user:proxy-pass",
	}

	for _, line := range tests {
		t.Run(line, func(t *testing.T) {
			_, err := ParseIPv4Auth("http", line)
			require.Error(t, err)
		})
	}
}

func TestParseProxyURLValidatesIPHosts(t *testing.T) {
	tests := []struct {
		name    string
		address string
		valid   bool
	}{
		{name: "valid IPv4", address: "8.8.8.8", valid: true},
		{name: "valid IPv6", address: "2001:4860:4860::8888", valid: true},
		{name: "hostname", address: "proxy.example.com", valid: true},
		{name: "masked IPv4", address: "166.142.X.211"},
		{name: "invalid IPv4 octet", address: "166.142.999.211"},
		{name: "invalid IPv6", address: "2001:db8::xyz"},
		{name: "IPv6 loopback", address: "::1"},
		{name: "IPv6 link-local", address: "fe80::1"},
		{name: "IPv6 private", address: "fd00::1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseProxyURL("https", "https://"+net.JoinHostPort(test.address, "9443"))
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestProxyStringFormatsIPv6Endpoints(t *testing.T) {
	tests := []struct {
		name  string
		proxy Proxy
		want  string
	}{
		{
			name:  "without credentials",
			proxy: Proxy{Protocol: "http", IP: "2001:4860:4860::8888", Port: 8080},
			want:  "http://[2001:4860:4860::8888]:8080",
		},
		{
			name:  "with username",
			proxy: Proxy{Protocol: "http", IP: "2001:4860:4860::8888", Port: 8080, User: "user"},
			want:  "http://user@[2001:4860:4860::8888]:8080",
		},
		{
			name:  "with credentials",
			proxy: Proxy{Protocol: "http", IP: "2001:4860:4860::8888", Port: 8080, User: "user", Passwd: "pass"},
			want:  "http://user:pass@[2001:4860:4860::8888]:8080",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, test.proxy.String())
		})
	}
}

func TestIsIPLiteralCandidate(t *testing.T) {
	tests := []struct {
		host     string
		isIPLike bool
	}{
		{host: "166.142.88.211", isIPLike: true},
		{host: "166.142.X.211", isIPLike: true},
		{host: "166.X.X.211", isIPLike: false},
		{host: "proxy.example.com", isIPLike: false},
		{host: "2001:db8::1", isIPLike: true},
	}

	for _, test := range tests {
		t.Run(test.host, func(t *testing.T) {
			require.Equal(t, test.isIPLike, isIPLiteralCandidate(test.host))
		})
	}
}
