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
	parser := GetParser("IPv4Auth")
	proxy, err := parser("http", "104.207.38.226:3129:proxy-user:proxy-pass")

	require.NoError(t, err)
	require.Equal(t, "104.207.38.226", proxy.IP)
	require.Equal(t, 3129, proxy.Port)
	require.Equal(t, "proxy-user", proxy.User)
	require.Equal(t, "proxy-pass", proxy.Passwd)
	require.Equal(t, "http", proxy.Protocol)
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
