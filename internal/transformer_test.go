package internal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestFromMTProtoExtractsProxyLinks(t *testing.T) {
	input := []byte(`updated: 2026-10-05 https://t.me/proxy?server=proxy.example.com&amp;port=443&amp;secret=0123456789abcdef0123456789abcdef, tg://proxy?server=8.8.8.8&port=443&secret=0123456789abcdef0123456789abcdef#fast;https://t.me/proxy?server=other.example.com&port=443&secret=0123456789abcdef0123456789abcdef`)

	got := string(FromMTProto(input, ""))
	want := "https://t.me/proxy?server=proxy.example.com&port=443&secret=0123456789abcdef0123456789abcdef\n" +
		"tg://proxy?server=8.8.8.8&port=443&secret=0123456789abcdef0123456789abcdef#fast\n" +
		"https://t.me/proxy?server=other.example.com&port=443&secret=0123456789abcdef0123456789abcdef\n"
	if got != want {
		t.Fatalf("FromMTProto() = %q, want %q", got, want)
	}
}

func TestFromClash(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "basic clash config with http proxy",
			input: `proxies:
  - name: "http-proxy"
    type: http
    server: 1.2.3.4
    port: 8080`,
			expected: "http://1.2.3.4:8080\n",
		},
		{
			name: "clash config with multiple proxies",
			input: `proxies:
  - name: "http-proxy"
    type: http
    server: 1.2.3.4
    port: 8080
  - name: "socks5-proxy"
    type: socks5
    server: 5.6.7.8
    port: 1080`,
			expected: "http://1.2.3.4:8080\nsocks5://5.6.7.8:1080\n",
		},
		{
			name: "clash config with ss proxy",
			input: `proxies:
  - name: "ss-proxy"
    type: ss
    server: 9.10.11.12
    port: 443
    cipher: chacha20-ietf-poly1305
    password: "test123"`,
			expected: "ss://" + base64.StdEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:test123")) + "@9.10.11.12:443\n",
		},
		{
			name: "clash config with no proxies",
			input: `port: 7890
socks-port: 7891`,
			expected: "",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name: "invalid YAML",
			input: `proxies:
  - name: "test"
    type: http
    server: 1.2.3.4
    port: invalid syntax {{{`,
			expected: "",
		},
		{
			name: "port string with non-digit suffix",
			input: `proxies:
  - name: "http-proxy"
    type: http
    server: 1.2.3.4
    port: "8080?"`,
			expected: "http://1.2.3.4:8080\n",
		},
		{
			name: "missing server field",
			input: `proxies:
  - name: "http-proxy"
    type: http
    port: 8080`,
			expected: "",
		},
		{
			name: "missing port field",
			input: `proxies:
  - name: "http-proxy"
    type: http
    server: 1.2.3.4`,
			expected: "",
		},
		{
			name: "missing type field",
			input: `proxies:
  - name: "http-proxy"
    server: 1.2.3.4
    port: 8080`,
			expected: "",
		},
		{
			name: "port out of range high",
			input: `proxies:
  - name: "http-proxy"
    type: http
    server: 1.2.3.4
    port: 99999`,
			expected: "",
		},
		{
			name: "port out of range low",
			input: `proxies:
  - name: "http-proxy"
    type: http
    server: 1.2.3.4
    port: 0`,
			expected: "",
		},
		{
			name: "ss proxy missing cipher",
			input: `proxies:
  - name: "ss-proxy"
    type: ss
    server: 1.2.3.4
    port: 443
    password: "test123"`,
			expected: "",
		},
		{
			name: "ss proxy missing password",
			input: `proxies:
  - name: "ss-proxy"
    type: ss
    server: 1.2.3.4
    port: 443
    cipher: chacha20-ietf-poly1305`,
			expected: "",
		},
		{
			name: "https proxy",
			input: `proxies:
  - name: "https-proxy"
    type: https
    server: 5.6.7.8
    port: 8443`,
			expected: "https://5.6.7.8:8443\n",
		},
		{
			name: "socks4 proxy",
			input: `proxies:
  - name: "socks4-proxy"
    type: socks4
    server: 9.9.9.9
    port: 1080`,
			expected: "socks4://9.9.9.9:1080\n",
		},
		{
			name: "http proxy with auth credentials",
			input: `proxies:
  - name: "auth-proxy"
    type: http
    server: 1.2.3.4
    port: 8080
    username: "user1"
    password: "pass1"`,
			expected: "http://user1:pass1@1.2.3.4:8080\n",
		},
		{
			name: "http proxy with special chars in auth",
			input: `proxies:
  - name: "auth-proxy"
    type: http
    server: 1.2.3.4
    port: 8080
    username: "user1"
    password: "p@ss"`,
			expected: "http://user1:p%40ss@1.2.3.4:8080\n",
		},
		{
			name: "http proxy with username only",
			input: `proxies:
  - name: "auth-proxy"
    type: http
    server: 1.2.3.4
    port: 8080
    username: "user1"`,
			expected: "http://user1@1.2.3.4:8080\n",
		},
		{
			name: "http proxy with space in password",
			input: `proxies:
  - name: "space-pass"
    type: http
    server: 1.2.3.4
    port: 8080
    username: "user1"
    password: "foo bar"`,
			expected: "http://user1:foo%20bar@1.2.3.4:8080\n",
		},
		{
			name: "empty server rejected",
			input: `proxies:
  - name: "empty-server"
    type: http
    server: ""
    port: 8080`,
			expected: "",
		},
		{
			name: "whitespace server rejected",
			input: `proxies:
  - name: "space-server"
    type: http
    server: "  "
    port: 8080`,
			expected: "",
		},
		{
			name: "float64 port",
			input: `proxies:
  - name: "float-port"
    type: http
    server: 1.2.3.4
    port: 8080.0`,
			expected: "http://1.2.3.4:8080\n",
		},
		{
			name: "non-integer float port rejected",
			input: `proxies:
  - name: "float-port"
    type: http
    server: 1.2.3.4
    port: 8080.9`,
			expected: "",
		},
		{
			name: "bool port skipped gracefully",
			input: `proxies:
  - name: "valid"
    type: http
    server: 1.2.3.4
    port: 8080
  - name: "bool-port"
    type: http
    server: 5.6.7.8
    port: true
  - name: "valid2"
    type: socks5
    server: 9.10.11.12
    port: 1080`,
			expected: "http://1.2.3.4:8080\nsocks5://9.10.11.12:1080\n",
		},
		{
			name: "mixed valid and invalid proxies",
			input: `proxies:
  - name: "valid"
    type: http
    server: 1.2.3.4
    port: 8080
  - name: "invalid"
    type: http
    server: 5.6.7.8
    port: 99999
  - name: "valid2"
    type: socks5
    server: 9.10.11.12
    port: 1080`,
			expected: "http://1.2.3.4:8080\nsocks5://9.10.11.12:1080\n",
		},
		{
			name: "malformed proxy does not discard valid proxies",
			input: `proxies:
  - name: "valid-before"
    type: http
    server: 1.2.3.4
    port: 8080
  - name: "invalid-port"
    type: http
    server: 5.6.7.8
    port: "44x3?"
  - name: "valid-after"
    type: socks5
    server: 9.10.11.12
    port: 1080`,
			expected: "http://1.2.3.4:8080\nsocks5://9.10.11.12:1080\n",
		},
		{
			name: "malformed inline proxy does not discard valid proxies",
			input: `proxies:
  - {name: before, type: http, server: 1.2.3.4, port: 8080}
  - {name: "broken-"2001:db8::1", type: vmess, server: 5.6.7.8, port: 443}
  - {name: after, type: socks5, server: 9.10.11.12, port: 1080}`,
			expected: "http://1.2.3.4:8080\nsocks5://9.10.11.12:1080\n",
		},
		{
			name: "IPv6 server",
			input: `proxies:
  - name: "ipv6"
    type: http
    server: "::1"
    port: 8080`,
			expected: "http://[::1]:8080\n",
		},
		{
			// 10MB is the maxYAMLSize boundary: input passes the size gate and
			// reaches the YAML parser, which fails on non-YAML content, so output is empty.
			name:     "input at 10MB size limit reaches parser and yields empty output on parse failure",
			input:    strings.Repeat("x", 10*1024*1024),
			expected: "",
		},
		{
			// 11MB exceeds maxYAMLSize: input is rejected by the size gate
			// before YAML parsing, so output is empty without a parse attempt.
			name:     "input above 10MB size limit rejected by size gate before parsing",
			input:    strings.Repeat("x", 11*1024*1024), // 11MB
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(FromClash([]byte(tt.input), ""))
			if result != tt.expected {
				t.Errorf("FromClash() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestVmessURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "basic vmess",
			input: `proxies:
  - name: "vmess-test"
    type: vmess
    server: example.com
    port: 443
    uuid: "75a0885f-0ca5-42a4-8651-391cf8193154"
    alterId: 0
    cipher: auto
    tls: true`,
		},
		{
			name: "vmess with ws transport",
			input: `proxies:
  - name: "vmess-ws"
    type: vmess
    server: ws.example.com
    port: 2096
    uuid: "75a0885f-0ca5-42a4-8651-391cf8193154"
    alterId: 0
    cipher: auto
    network: ws
    tls: true
    servername: ws.example.com
    ws-opts:
      path: /path
      headers:
        Host: ws.example.com`,
		},
		{
			name: "vmess with grpc transport",
			input: `proxies:
  - name: "vmess-grpc"
    type: vmess
    server: grpc.example.com
    port: 443
    uuid: "75a0885f-0ca5-42a4-8651-391cf8193154"
    alterId: 0
    network: grpc
    tls: true
    grpc-opts:
      grpc-service-name: myservice`,
		},
		{
			name: "vmess with h2 transport",
			input: `proxies:
  - name: "vmess-h2"
    type: vmess
    server: h2.example.com
    port: 443
    uuid: "75a0885f-0ca5-42a4-8651-391cf8193154"
    alterId: 0
    network: h2
    tls: true
    h2-opts:
      path: /h2path
      host:
        - h2.example.com`,
		},
		{
			name: "vmess missing uuid skipped",
			input: `proxies:
  - name: "vmess-no-uuid"
    type: vmess
    server: example.com
    port: 443`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(FromClash([]byte(tt.input), ""))
			if tt.name == "vmess missing uuid skipped" {
				if result != "" {
					t.Errorf("expected empty result for missing uuid, got %q", result)
				}
				return
			}

			if result == "" {
				t.Fatal("expected non-empty vmess URL")
			}

			line := strings.TrimSpace(result)
			if !strings.HasPrefix(line, "vmess://") {
				t.Fatalf("expected vmess:// prefix, got %q", line)
			}

			// Decode and verify the JSON structure
			encoded := strings.TrimPrefix(line, "vmess://")
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Fatalf("base64 decode failed: %v", err)
			}

			var config map[string]interface{}
			if err := json.Unmarshal(decoded, &config); err != nil {
				t.Fatalf("JSON unmarshal failed: %v", err)
			}

			// Verify required fields per proxyclient VmessConfig
			if config["v"] != "2" {
				t.Errorf("expected v=2, got %v", config["v"])
			}
			if config["id"] != "75a0885f-0ca5-42a4-8651-391cf8193154" {
				t.Errorf("expected uuid, got %v", config["id"])
			}
			if _, ok := config["add"]; !ok {
				t.Error("missing 'add' field")
			}
			if _, ok := config["port"]; !ok {
				t.Error("missing 'port' field")
			}

			// Check transport-specific fields
			switch tt.name {
			case "vmess with ws transport":
				if config["net"] != "ws" {
					t.Errorf("expected net=ws, got %v", config["net"])
				}
				if config["path"] != "/path" {
					t.Errorf("expected path=/path, got %v", config["path"])
				}
				if config["host"] != "ws.example.com" {
					t.Errorf("expected host=ws.example.com, got %v", config["host"])
				}
				if config["tls"] != "tls" {
					t.Errorf("expected tls=tls, got %v", config["tls"])
				}
			case "vmess with grpc transport":
				if config["net"] != "grpc" {
					t.Errorf("expected net=grpc, got %v", config["net"])
				}
				if config["path"] != "myservice" {
					t.Errorf("expected path=myservice, got %v", config["path"])
				}
			case "vmess with h2 transport":
				if config["net"] != "h2" {
					t.Errorf("expected net=h2, got %v", config["net"])
				}
				if config["path"] != "/h2path" {
					t.Errorf("expected path=/h2path, got %v", config["path"])
				}
				if config["host"] != "h2.example.com" {
					t.Errorf("expected host=h2.example.com, got %v", config["host"])
				}
			case "basic vmess":
				if config["tls"] != "tls" {
					t.Errorf("expected tls=tls, got %v", config["tls"])
				}
				if config["security"] != "auto" {
					t.Errorf("expected security=auto, got %v", config["security"])
				}
			}
		})
	}
}

func TestVlessURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		checkURL func(t *testing.T, rawURL string)
	}{
		{
			name: "basic vless",
			input: `proxies:
  - name: "vless-test"
    type: vless
    server: example.com
    port: 443
    uuid: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
    tls: true
    servername: example.com`,
			checkURL: func(t *testing.T, rawURL string) {
				u, err := parseVlessTestURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				if u.User.Username() != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
					t.Errorf("expected uuid in userinfo, got %s", u.User.Username())
				}
				if u.Host != "example.com:443" {
					t.Errorf("expected example.com:443, got %s", u.Host)
				}
				q := u.Query()
				if q.Get("encryption") != "none" {
					t.Errorf("expected encryption=none, got %s", q.Get("encryption"))
				}
				if q.Get("security") != "tls" {
					t.Errorf("expected security=tls, got %s", q.Get("security"))
				}
				if q.Get("sni") != "example.com" {
					t.Errorf("expected sni=example.com, got %s", q.Get("sni"))
				}
			},
		},
		{
			name: "vless with ws and flow",
			input: `proxies:
  - name: "vless-ws"
    type: vless
    server: ws.example.com
    port: 443
    uuid: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
    tls: true
    flow: xtls-rprx-vision
    network: ws
    ws-opts:
      path: /vless
      headers:
        Host: cdn.example.com`,
			checkURL: func(t *testing.T, rawURL string) {
				u, err := parseVlessTestURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				q := u.Query()
				if q.Get("type") != "ws" {
					t.Errorf("expected type=ws, got %s", q.Get("type"))
				}
				if q.Get("flow") != "xtls-rprx-vision" {
					t.Errorf("expected flow, got %s", q.Get("flow"))
				}
				if q.Get("path") != "/vless" {
					t.Errorf("expected path=/vless, got %s", q.Get("path"))
				}
				if q.Get("host") != "cdn.example.com" {
					t.Errorf("expected host=cdn.example.com, got %s", q.Get("host"))
				}
			},
		},
		{
			name: "vless with reality",
			input: `proxies:
  - name: "vless-reality"
    type: vless
    server: reality.example.com
    port: 443
    uuid: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
    client-fingerprint: chrome
    reality-opts:
      public-key: abc123publickey
      short-id: deadbeef`,
			checkURL: func(t *testing.T, rawURL string) {
				u, err := parseVlessTestURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				q := u.Query()
				if q.Get("security") != "reality" {
					t.Errorf("expected security=reality, got %s", q.Get("security"))
				}
				if q.Get("pbk") != "abc123publickey" {
					t.Errorf("expected pbk, got %s", q.Get("pbk"))
				}
				if q.Get("sid") != "deadbeef" {
					t.Errorf("expected sid, got %s", q.Get("sid"))
				}
				if q.Get("fp") != "chrome" {
					t.Errorf("expected fp=chrome, got %s", q.Get("fp"))
				}
			},
		},
		{
			name: "vless with grpc",
			input: `proxies:
  - name: "vless-grpc"
    type: vless
    server: grpc.example.com
    port: 443
    uuid: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
    tls: true
    network: grpc
    grpc-opts:
      grpc-service-name: mygrpc`,
			checkURL: func(t *testing.T, rawURL string) {
				u, err := parseVlessTestURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				q := u.Query()
				if q.Get("type") != "grpc" {
					t.Errorf("expected type=grpc, got %s", q.Get("type"))
				}
				if q.Get("serviceName") != "mygrpc" {
					t.Errorf("expected serviceName=mygrpc, got %s", q.Get("serviceName"))
				}
			},
		},
		{
			name: "vless with h2 transport",
			input: `proxies:
  - name: "vless-h2"
    type: vless
    server: h2.example.com
    port: 443
    uuid: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
    tls: true
    network: h2
    h2-opts:
      path: /h2path
      host:
        - h2.example.com`,
			checkURL: func(t *testing.T, rawURL string) {
				u, err := parseVlessTestURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				q := u.Query()
				if q.Get("type") != "h2" {
					t.Errorf("expected type=h2, got %s", q.Get("type"))
				}
				if q.Get("path") != "/h2path" {
					t.Errorf("expected path=/h2path, got %s", q.Get("path"))
				}
				if q.Get("host") != "h2.example.com" {
					t.Errorf("expected host=h2.example.com, got %s", q.Get("host"))
				}
			},
		},
		{
			name: "vless missing uuid skipped",
			input: `proxies:
  - name: "vless-no-uuid"
    type: vless
    server: example.com
    port: 443`,
			checkURL: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(FromClash([]byte(tt.input), ""))

			if tt.checkURL == nil {
				if result != "" {
					t.Errorf("expected empty result, got %q", result)
				}
				return
			}

			line := strings.TrimSpace(result)
			if !strings.HasPrefix(line, "vless://") {
				t.Fatalf("expected vless:// prefix, got %q", line)
			}
			tt.checkURL(t, line)
		})
	}
}

func TestTrojanURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		checkURL func(t *testing.T, rawURL string)
	}{
		{
			name: "basic trojan",
			input: `proxies:
  - name: "trojan-test"
    type: trojan
    server: trojan.example.com
    port: 443
    password: "mysecret"
    sni: trojan.example.com`,
			checkURL: func(t *testing.T, rawURL string) {
				u, err := parseTrojanTestURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				if u.User.Username() != "mysecret" {
					t.Errorf("expected password in userinfo, got %s", u.User.Username())
				}
				if u.Host != "trojan.example.com:443" {
					t.Errorf("expected trojan.example.com:443, got %s", u.Host)
				}
				q := u.Query()
				if q.Get("security") != "tls" {
					t.Errorf("expected security=tls, got %s", q.Get("security"))
				}
				if q.Get("sni") != "trojan.example.com" {
					t.Errorf("expected sni, got %s", q.Get("sni"))
				}
			},
		},
		{
			name: "trojan with special password",
			input: `proxies:
  - name: "trojan-special"
    type: trojan
    server: trojan.example.com
    port: 443
    password: "p@ss/w0rd"
    servername: trojan.example.com`,
			checkURL: func(t *testing.T, rawURL string) {
				u, err := parseTrojanTestURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				// Password should be URL-encoded, so when parsed it comes back decoded
				if u.User.Username() != "p@ss/w0rd" {
					t.Errorf("expected decoded password p@ss/w0rd, got %s", u.User.Username())
				}
			},
		},
		{
			name: "trojan with ws transport",
			input: `proxies:
  - name: "trojan-ws"
    type: trojan
    server: ws.example.com
    port: 443
    password: "secret"
    network: ws
    ws-opts:
      path: /trojan
      headers:
        Host: cdn.example.com`,
			checkURL: func(t *testing.T, rawURL string) {
				u, err := parseTrojanTestURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				q := u.Query()
				if q.Get("type") != "ws" {
					t.Errorf("expected type=ws, got %s", q.Get("type"))
				}
				if q.Get("path") != "/trojan" {
					t.Errorf("expected path=/trojan, got %s", q.Get("path"))
				}
				if q.Get("host") != "cdn.example.com" {
					t.Errorf("expected host=cdn.example.com, got %s", q.Get("host"))
				}
			},
		},
		{
			name: "trojan with grpc",
			input: `proxies:
  - name: "trojan-grpc"
    type: trojan
    server: grpc.example.com
    port: 443
    password: "secret"
    network: grpc
    grpc-opts:
      grpc-service-name: trojangrpc`,
			checkURL: func(t *testing.T, rawURL string) {
				u, err := parseTrojanTestURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				q := u.Query()
				if q.Get("type") != "grpc" {
					t.Errorf("expected type=grpc, got %s", q.Get("type"))
				}
				if q.Get("serviceName") != "trojangrpc" {
					t.Errorf("expected serviceName=trojangrpc, got %s", q.Get("serviceName"))
				}
			},
		},
		{
			name: "trojan with servername fallback",
			input: `proxies:
  - name: "trojan-sn"
    type: trojan
    server: trojan.example.com
    port: 443
    password: "secret"
    servername: fallback.example.com`,
			checkURL: func(t *testing.T, rawURL string) {
				u, err := parseTrojanTestURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				q := u.Query()
				if q.Get("sni") != "fallback.example.com" {
					t.Errorf("expected sni=fallback.example.com, got %s", q.Get("sni"))
				}
			},
		},
		{
			name: "trojan with h2 transport",
			input: `proxies:
  - name: "trojan-h2"
    type: trojan
    server: h2.example.com
    port: 443
    password: "secret"
    network: h2
    h2-opts:
      path: /h2trojan
      host:
        - h2.example.com`,
			checkURL: func(t *testing.T, rawURL string) {
				u, err := parseTrojanTestURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				q := u.Query()
				if q.Get("type") != "h2" {
					t.Errorf("expected type=h2, got %s", q.Get("type"))
				}
				if q.Get("path") != "/h2trojan" {
					t.Errorf("expected path=/h2trojan, got %s", q.Get("path"))
				}
				if q.Get("host") != "h2.example.com" {
					t.Errorf("expected host=h2.example.com, got %s", q.Get("host"))
				}
			},
		},
		{
			name: "trojan with reality",
			input: `proxies:
  - name: "trojan-reality"
    type: trojan
    server: reality.example.com
    port: 443
    password: "secret"
    client-fingerprint: chrome
    reality-opts:
      public-key: realpubkey
      short-id: aabb`,
			checkURL: func(t *testing.T, rawURL string) {
				u, err := parseTrojanTestURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				q := u.Query()
				if q.Get("security") != "reality" {
					t.Errorf("expected security=reality, got %s", q.Get("security"))
				}
				if q.Get("pbk") != "realpubkey" {
					t.Errorf("expected pbk=realpubkey, got %s", q.Get("pbk"))
				}
				if q.Get("sid") != "aabb" {
					t.Errorf("expected sid=aabb, got %s", q.Get("sid"))
				}
				if q.Get("fp") != "chrome" {
					t.Errorf("expected fp=chrome, got %s", q.Get("fp"))
				}
			},
		},
		{
			name: "trojan with space in password",
			input: `proxies:
  - name: "trojan-space"
    type: trojan
    server: example.com
    port: 443
    password: "foo bar"`,
			checkURL: func(t *testing.T, rawURL string) {
				u, err := parseTrojanTestURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				// url.User encodes space as %20, not +
				if u.User.Username() != "foo bar" {
					t.Errorf("expected 'foo bar', got %s", u.User.Username())
				}
			},
		},
		{
			name: "trojan missing password skipped",
			input: `proxies:
  - name: "trojan-no-pass"
    type: trojan
    server: example.com
    port: 443`,
			checkURL: nil,
		},
		{
			name: "trojan IPv6 server",
			input: `proxies:
  - name: "trojan-ipv6"
    type: trojan
    server: "2001:db8::1"
    port: 443
    password: "secret"`,
			checkURL: func(t *testing.T, rawURL string) {
				u, err := parseTrojanTestURL(rawURL)
				if err != nil {
					t.Fatal(err)
				}
				if u.Host != "[2001:db8::1]:443" {
					t.Errorf("expected [2001:db8::1]:443, got %s", u.Host)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(FromClash([]byte(tt.input), ""))

			if tt.checkURL == nil {
				if result != "" {
					t.Errorf("expected empty result, got %q", result)
				}
				return
			}

			line := strings.TrimSpace(result)
			if !strings.HasPrefix(line, "trojan://") {
				t.Fatalf("expected trojan:// prefix, got %q", line)
			}
			tt.checkURL(t, line)
		})
	}
}

func TestMixedProtocols(t *testing.T) {
	input := `proxies:
  - name: "http"
    type: http
    server: 1.2.3.4
    port: 8080
  - name: "vmess"
    type: vmess
    server: vmess.example.com
    port: 443
    uuid: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
    alterId: 0
    cipher: auto
    tls: true
  - name: "vless"
    type: vless
    server: vless.example.com
    port: 443
    uuid: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
    tls: true
  - name: "trojan"
    type: trojan
    server: trojan.example.com
    port: 443
    password: "secret"
  - name: "ss"
    type: ss
    server: ss.example.com
    port: 443
    cipher: aes-256-gcm
    password: "sspass"
  - name: "socks5"
    type: socks5
    server: 5.6.7.8
    port: 1080`

	result := string(FromClash([]byte(input), ""))
	lines := strings.Split(strings.TrimSpace(result), "\n")

	if len(lines) != 6 {
		t.Fatalf("expected 6 proxy URLs, got %d: %v", len(lines), lines)
	}

	if !strings.HasPrefix(lines[0], "http://") {
		t.Errorf("line 0: expected http://, got %s", lines[0])
	}
	if !strings.HasPrefix(lines[1], "vmess://") {
		t.Errorf("line 1: expected vmess://, got %s", lines[1])
	}
	if !strings.HasPrefix(lines[2], "vless://") {
		t.Errorf("line 2: expected vless://, got %s", lines[2])
	}
	if !strings.HasPrefix(lines[3], "trojan://") {
		t.Errorf("line 3: expected trojan://, got %s", lines[3])
	}
	if !strings.HasPrefix(lines[4], "ss://") {
		t.Errorf("line 4: expected ss://, got %s", lines[4])
	}
	if !strings.HasPrefix(lines[5], "socks5://") {
		t.Errorf("line 5: expected socks5://, got %s", lines[5])
	}
}

// parseVlessTestURL parses a vless:// URL for test assertions.
func parseVlessTestURL(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}

// parseTrojanTestURL parses a trojan:// URL for test assertions.
func parseTrojanTestURL(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}

func TestFromLinksDownloadsKeywordMatchesAndAppliesTransformer(t *testing.T) {
	allowPrivateRegexLinkHosts = true
	defer func() { allowPrivateRegexLinkHosts = false }()

	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.Path]++
		switch r.URL.Path {
		case "/base64-fn0618.txt":
			encoded := base64.StdEncoding.EncodeToString([]byte("http://1.2.3.4:8080\n"))
			_, _ = w.Write([]byte(encoded))
		case "/clash-fn0618.yaml":
			_, _ = w.Write([]byte("proxies:\n  - name: socks\n    type: socks5\n    server: 5.6.7.8\n    port: 1080\n"))
		case "/other.txt":
			_, _ = w.Write([]byte("http://9.9.9.9:9090\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	readme := []byte("sources:\n- " + server.URL + "/base64-fn0618.txt\n- " + server.URL + "/other.txt\n- " + server.URL + "/base64-fn0618.txt\n")
	transformer, transformerOptions := GetTransformer("link:base64-fn0618")

	got := string(transformer(readme, transformerOptions))
	want := "http://1.2.3.4:8080\n"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if requests["/base64-fn0618.txt"] != 1 {
		t.Fatalf("expected duplicate keyword link to be fetched once, got %d requests", requests["/base64-fn0618.txt"])
	}
	if requests["/other.txt"] != 0 {
		t.Fatalf("expected non-keyword link not to be fetched, got %d requests", requests["/other.txt"])
	}

	clashTransformer, clashTransformerOptions := GetTransformer("link:clash-fn0618")
	got = string(clashTransformer([]byte("source: "+server.URL+"/clash-fn0618.yaml\n"), clashTransformerOptions))
	want = "socks5://5.6.7.8:1080\n"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestFromListDownloadsAndTransformsEachURL(t *testing.T) {
	allowPrivateRegexLinkHosts = true
	defer func() { allowPrivateRegexLinkHosts = false }()

	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.Path]++
		switch r.URL.Path {
		case "/first.yaml":
			_, _ = w.Write([]byte("proxies:\n  - name: http\n    type: http\n    server: 1.2.3.4\n    port: 8080\n"))
		case "/second.yaml":
			_, _ = w.Write([]byte("proxies:\n  - name: socks\n    type: socks5\n    server: 5.6.7.8\n    port: 1080\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	list := "# Clash sources\n" + server.URL + "/first.yaml\n\n" + server.URL + "/second.yaml # second source\n" + server.URL + "/first.yaml\n"
	transformer, options := GetTransformer("list:clash")
	got := string(transformer([]byte(list), options))
	want := "http://1.2.3.4:8080\nsocks5://5.6.7.8:1080\n"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if requests["/first.yaml"] != 1 || requests["/second.yaml"] != 1 {
		t.Fatalf("expected each unique list URL to be fetched once, got %#v", requests)
	}

	for _, line := range strings.Fields(got) {
		if _, err := ParseProxyURL("auto", line); err != nil {
			t.Errorf("transformed proxy line %q was rejected by the normal parser: %v", line, err)
		}
	}
}

func TestFromListPassesNestedTransformerOptions(t *testing.T) {
	allowPrivateRegexLinkHosts = true
	defer func() { allowPrivateRegexLinkHosts = false }()

	var baseURL string
	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.Path]++
		switch r.URL.Path {
		case "/index.txt":
			_, _ = fmt.Fprintf(w, "%s/match-keyword.txt\n%s/other.txt\n", baseURL, baseURL)
		case "/match-keyword.txt":
			_, _ = w.Write([]byte("http://1.2.3.4:8080\n"))
		case "/other.txt":
			_, _ = w.Write([]byte("http://5.6.7.8:8080\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	baseURL = server.URL

	transformer, options := GetTransformer("list:link:keyword")
	got := string(transformer([]byte(server.URL+"/index.txt\n"), options))
	if got != "http://1.2.3.4:8080\n" {
		t.Fatalf("expected only the keyword-matching child feed, got %q", got)
	}
	if requests["/match-keyword.txt"] != 1 || requests["/other.txt"] != 0 {
		t.Fatalf("expected nested keyword filtering before fetch, got %#v", requests)
	}
}

func TestFromLinksAppliesKeywordBeforeFanOutLimit(t *testing.T) {
	allowPrivateRegexLinkHosts = true
	defer func() { allowPrivateRegexLinkHosts = false }()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte("http://1.2.3.4:8080\n"))
	}))
	defer server.Close()

	links := make([]string, 0, maxRegexLinkCount+1)
	for i := 0; i < maxRegexLinkCount; i++ {
		links = append(links, fmt.Sprintf("%s/asset-%d.css", server.URL, i))
	}
	links = append(links, server.URL+"/proxy-fn0618.txt")

	transformer, transformerOptions := GetTransformer("link:fn0618")
	got := string(transformer([]byte(strings.Join(links, "\n")), transformerOptions))
	if got != "http://1.2.3.4:8080\n" {
		t.Fatalf("expected keyword link after non-keyword matches to be fetched, got %q", got)
	}
	if requests != 1 {
		t.Fatalf("expected only keyword link to be fetched, got %d requests", requests)
	}
}

func TestRegexLinkRedirectsRejectPrivateTargets(t *testing.T) {
	allowPrivateRegexLinkHosts = false

	req, err := http.NewRequest(http.MethodGet, "http://169.254.169.254/latest/meta-data", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := regexLinkClient.CheckRedirect(req, nil); err == nil {
		t.Fatal("expected private redirect target to be rejected")
	}
}

func TestFromLinksRejectsPrivateTargets(t *testing.T) {
	allowPrivateRegexLinkHosts = false

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte("http://1.2.3.4:8080\n"))
	}))
	defer server.Close()

	transformer, transformerOptions := GetTransformer("link:private")
	got := string(transformer([]byte("source: "+server.URL+"/private.txt\n"), transformerOptions))
	if got != "" {
		t.Fatalf("expected private target to be skipped, got %q", got)
	}
	if requests != 0 {
		t.Fatalf("expected private target not to be requested, got %d requests", requests)
	}
}

func TestFromLinksLimitsFanOutAndBodySize(t *testing.T) {
	allowPrivateRegexLinkHosts = true
	defer func() { allowPrivateRegexLinkHosts = false }()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/oversize.txt" {
			_, _ = w.Write([]byte(strings.Repeat("x", maxRegexLinkResponseBytes+1)))
			return
		}
		_, _ = w.Write([]byte("http://1.2.3.4:8080\n"))
	}))
	defer server.Close()

	links := make([]string, 0, maxRegexLinkCount+2)
	for i := 0; i < maxRegexLinkCount+1; i++ {
		links = append(links, fmt.Sprintf("%s/fn0618-%d.txt", server.URL, i))
	}
	links[0] = server.URL + "/oversize.txt?tag=fn0618"

	transformer, transformerOptions := GetTransformer("link:fn0618")
	got := string(transformer([]byte(strings.Join(links, "\n")), transformerOptions))
	if strings.Contains(got, strings.Repeat("x", 32)) {
		t.Fatalf("expected oversized response to be skipped")
	}
	if requests != maxRegexLinkCount {
		t.Fatalf("expected at most %d requests, got %d", maxRegexLinkCount, requests)
	}
}

func TestFromCurlDepthOneSelectorAndProtocolFinder(t *testing.T) {
	useHTTPFetcherForCurlTest(t)
	allowPrivateRegexLinkHosts = true
	defer func() { allowPrivateRegexLinkHosts = false }()

	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.Path]++
		switch r.URL.Path {
		case "/feed-vless":
			_, _ = w.Write([]byte("vless://uuid@example.com:443\nvmess://uuid@example.com:443\n"))
		case "/other":
			_, _ = w.Write([]byte("vless://ignored@example.com:443\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, _, options, _, err := parseLine(server.URL + ",curl:1-feed-vless-vless")
	if err != nil {
		t.Fatal(err)
	}
	page := []byte(`<a href="/feed-vless">feed</a><a href="/other">other</a>vless://current@example.com:443`)
	got := string(fromCurl(page, options, server.URL))
	if got != "vless://uuid@example.com:443\n" {
		t.Fatalf("expected only linked vless URL, got %q", got)
	}
	if requests["/feed-vless"] != 1 || requests["/other"] != 0 {
		t.Fatalf("expected only selected link to be fetched once, got %#v", requests)
	}
}

func TestFromCurlDepthZeroScansCurrentPageOnly(t *testing.T) {
	useHTTPFetcherForCurlTest(t)
	allowPrivateRegexLinkHosts = true
	defer func() { allowPrivateRegexLinkHosts = false }()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte("vless://linked@example.com:443\n"))
	}))
	defer server.Close()

	_, _, options, _, err := parseLine(server.URL + ",curl:0-feed-vless-vless")
	if err != nil {
		t.Fatal(err)
	}
	got := string(fromCurl([]byte(`<a href="/feed-vless">feed</a>vless://current@example.com:443`), options, server.URL))
	if got != "vless://current@example.com:443\n" {
		t.Fatalf("expected current-page vless URL, got %q", got)
	}
	if requests != 0 {
		t.Fatalf("expected depth zero not to fetch links, got %d requests", requests)
	}
}

func TestFromCurlDefaultFinderIgnoresWebsiteLinks(t *testing.T) {
	page := []byte("https://example.com/page\nvless://uuid@example.com:443\n")
	if got := string(FromCurl(page, "")); got != "vless://uuid@example.com:443\n" {
		t.Fatalf("expected only proxy protocols by default, got %q", got)
	}
	if got := string(FromCurl(page, "https")); got != "https://example.com/page\n" {
		t.Fatalf("expected explicit https finder to return website URL, got %q", got)
	}
}

func TestFromCurlFindsMultipleProtocolsInInputValues(t *testing.T) {
	page := []byte(`<input value="ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYXNz@1.2.3.4:443#node"><input value="trojan://uuid@example.com:443?security=tls&amp;sni=example.com"><input value="vless://uuid@example.com:443?type=tcp"><input value="tg://proxy?server=8.8.8.8&amp;port=443&amp;secret=0123456789abcdef0123456789abcdef">`)
	got := string(FromCurl(page, ""))
	want := "ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpwYXNz@1.2.3.4:443#node\ntrojan://uuid@example.com:443?security=tls&sni=example.com\nvless://uuid@example.com:443?type=tcp\ntg://proxy?server=8.8.8.8&port=443&secret=0123456789abcdef0123456789abcdef\n"
	if got != want {
		t.Fatalf("expected both input proxy URLs, got %q", got)
	}
}

func TestFromCurlFiltersMultipleProtocolFinders(t *testing.T) {
	page := []byte("ss://method:password@example.com:443#ss\ntrojan://password@example.com:443#trojan\nvless://uuid@example.com:443\n")
	got := string(FromCurl(page, "/servers/-ss+trojan"))
	want := "ss://method:password@example.com:443#ss\ntrojan://password@example.com:443#trojan\n"
	if got != want {
		t.Fatalf("expected only selected proxy protocols, got %q", got)
	}
}

func TestFromCurlFiltersMTProtoFinder(t *testing.T) {
	page := []byte("tg://proxy?server=8.8.8.8&port=443&secret=0123456789abcdef0123456789abcdef\nhttps://example.com")
	got := string(FromCurl(page, "tg"))
	want := "tg://proxy?server=8.8.8.8&port=443&secret=0123456789abcdef0123456789abcdef\n"
	if got != want {
		t.Fatalf("expected only MTProto URLs, got %q", got)
	}
}

func TestDOMProtocolFinderBuildsMixedProxyURLs(t *testing.T) {
	page := []byte(`<table><tbody class="table-proxy-list">
<tr><th class="tblport">187.188.131.169</th><td class="tblport">1080</td><td>Mexico</td><td class="protocol"><a href="/protocol/socks">SOCKS</a></td></tr>
<tr><th class="tblport">199.34.230.5</th><td class="tblport">80</td><td>United States</td><td class="protocol"><a href="/protocol/http">HTTP</a></td></tr>
</tbody></table>`)
	options := "dom;row=tbody.table-proxy-list tr;protocol=td.protocol;host=th.tblport;port=td.tblport;template={protocol}://{host}:{port}"
	got := string(FromCurl(page, options))
	want := "socks://187.188.131.169:1080\nhttp://199.34.230.5:80\n"
	if got != want {
		t.Fatalf("expected DOM fields to assemble mixed proxy URLs, got %q", got)
	}
}

func TestDOMProtocolFinderReadsAttributesAndRejectsInvalidSelectors(t *testing.T) {
	page := []byte(`<div class="row"><span class="host" data-ip="192.0.2.1"></span><span class="port" data-port="8080"></span><span class="protocol" data-scheme="http"></span></div>`)
	options := "dom;row=.row;protocol=.protocol@data-scheme;host=.host@data-ip;port=.port@data-port;template={protocol}://{host}:{port}"
	if got := string(FromCurl(page, options)); got != "http://192.0.2.1:8080\n" {
		t.Fatalf("expected selected attributes to form a proxy URL, got %q", got)
	}
	if got := FromCurl(page, "dom;row=[invalid;host=.host;template={host}"); len(got) != 0 {
		t.Fatalf("expected invalid selector configuration to be ignored, got %q", got)
	}
}

func TestCurlAutomaticallyFollowsPagination(t *testing.T) {
	useHTTPFetcherForCurlTest(t)
	allowPrivateRegexLinkHosts = true
	defer func() { allowPrivateRegexLinkHosts = false }()

	requests := map[string]int{}
	pageHTML := func(page int) string {
		navigation := ""
		if page < 3 {
			navigation = fmt.Sprintf(`<nav class="pagination"><a href="?page=%d">%d</a><a class="pagination__arrow next" rel="next" href="?page=%d">Next</a></nav>`, page+1, page+1, page+1)
		}
		return fmt.Sprintf(`<div class="entry"><span class="protocol">http</span><span class="host">192.0.2.%d</span><span class="port">80%d</span></div>vless://uuid%d@example.com:443%s`, page, page, page, navigation)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, err := strconv.Atoi(r.URL.Query().Get("page"))
		if err != nil || page < 1 {
			page = 1
		}
		requests[r.URL.Query().Get("page")]++
		_, _ = fmt.Fprint(w, pageHTML(page))
	}))
	defer server.Close()

	rootURL := server.URL + "/?page=1"
	root := []byte(pageHTML(1))
	uriResult := string(fromCurl(root, "", rootURL))
	wantURIs := "vless://uuid1@example.com:443\nvless://uuid2@example.com:443\nvless://uuid3@example.com:443\n"
	if uriResult != wantURIs {
		t.Fatalf("URI finder pagination mismatch: got %q, want %q", uriResult, wantURIs)
	}

	domOptions := "dom;row=.entry;protocol=.protocol;host=.host;port=.port;template={protocol}://{host}:{port}"
	domResult := string(fromCurl(root, domOptions, rootURL))
	wantDOM := "http://192.0.2.1:801\nhttp://192.0.2.2:802\nhttp://192.0.2.3:803\n"
	if domResult != wantDOM {
		t.Fatalf("DOM finder pagination mismatch: got %q, want %q", domResult, wantDOM)
	}
	if requests["2"] != 2 || requests["3"] != 2 || requests["4"] != 0 {
		t.Fatalf("expected pages 2 and 3 once per finder and no page 4, got %#v", requests)
	}
}

func useHTTPFetcherForCurlTest(t *testing.T) {
	t.Helper()
	previousFetcher := curlImpersonateFetch
	curlImpersonateFetch = func(rawURL string) (curlFetchResponse, error) {
		response, err := regexLinkClient.Get(rawURL)
		if err != nil {
			return curlFetchResponse{}, err
		}
		defer response.Body.Close() // nolint: errcheck
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return curlFetchResponse{}, fmt.Errorf("unexpected test response status: %d", response.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, maxRegexLinkResponseBytes+1))
		return curlFetchResponse{body: body, finalURL: response.Request.URL.String()}, err
	}
	t.Cleanup(func() { curlImpersonateFetch = previousFetcher })
}

func TestCurlImpersonateFetchRunsConfiguredBinary(t *testing.T) {
	allowPrivateRegexLinkHosts = true
	defer func() { allowPrivateRegexLinkHosts = false }()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	const body = "vless://uuid@example.com:443\n"
	tempDir := t.TempDir()
	argsFile := filepath.Join(tempDir, "args.txt")
	binary := filepath.Join(tempDir, "curl_chrome116")
	script := "#!/bin/sh\nprintf '%s' \"$*\" > \"$CURL_IMPERSONATE_ARGS_FILE\"\nprintf '%s' \"$CURL_IMPERSONATE_TEST_BODY\"\nprintf '200:' >&2\n"
	if err := os.WriteFile(binary, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CURL_IMPERSONATE_BIN", binary)
	t.Setenv("CURL_IMPERSONATE_ARGS_FILE", argsFile)
	t.Setenv("CURL_IMPERSONATE_TEST_BODY", body)

	got, err := fetchCurlImpersonate(server.URL)
	if err != nil {
		t.Fatalf("fetchCurlImpersonate returned error: %v", err)
	}
	if string(got.body) != body {
		t.Fatalf("expected body %q, got %q", body, got.body)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--proto =http,https") || !strings.HasSuffix(string(args), server.URL) {
		t.Fatalf("expected protocol restriction and target URL in command args, got %q", args)
	}
}

func TestLoadCurlUsesImpersonatingFetcherForRoot(t *testing.T) {
	previousFetcher := curlImpersonateFetch
	requestedURLs := []string{}
	curlImpersonateFetch = func(rawURL string) (curlFetchResponse, error) {
		requestedURLs = append(requestedURLs, rawURL)
		return curlFetchResponse{body: []byte("vless://uuid@example.com:443\n"), finalURL: rawURL}, nil
	}
	t.Cleanup(func() { curlImpersonateFetch = previousFetcher })

	const parserName = "testCurlImpersonatingFetch"
	previousParser, hadParser := Parsers[parserName]
	RegisterParser(parserName, func(_, _ string) (*Proxy, error) {
		return &Proxy{Protocol: "vless", IP: "example.com", Port: 443}, nil
	})
	t.Cleanup(func() {
		if hadParser {
			Parsers[parserName] = previousParser
		} else {
			delete(Parsers, parserName)
		}
	})

	if err := Load("vless", []byte("https://feed.example.com/,curl:0-vless,"+parserName)); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(requestedURLs) != 1 || requestedURLs[0] != "https://feed.example.com/" {
		t.Fatalf("expected the impersonating fetcher to fetch the root URL, got %#v", requestedURLs)
	}
}

// TestFlexPortFloatRejection locks the contract that float ports must be
// integral, finite, and within the valid TCP/UDP range. Catches CodeRabbit's
// concern that int(f) silently truncates non-integer floats and accepts
// out-of-range or non-finite values.
func TestFlexPortFloatRejection(t *testing.T) {
	tests := []struct {
		name      string
		yamlPort  string // raw YAML scalar for the port field
		wantEmpty bool   // FromClash returns empty when port is invalid
	}{
		{name: "integer port accepted", yamlPort: "8080", wantEmpty: false},
		{name: "float equal to integer accepted", yamlPort: "8080.0", wantEmpty: false},
		{name: "non-integer float rejected", yamlPort: "8080.5", wantEmpty: true},
		{name: "fractional float rejected", yamlPort: "8080.7", wantEmpty: true},
		{name: "negative float rejected", yamlPort: "-1.0", wantEmpty: true},
		{name: "out-of-range float rejected", yamlPort: "99999.0", wantEmpty: true},
		{name: "NaN rejected", yamlPort: ".nan", wantEmpty: true},
		{name: "positive infinity rejected", yamlPort: ".inf", wantEmpty: true},
		{name: "negative infinity rejected", yamlPort: "-.inf", wantEmpty: true},
		{name: "zero rejected", yamlPort: "0", wantEmpty: true},
		{name: "negative integer rejected", yamlPort: "-1", wantEmpty: true},
		{name: "above 65535 rejected", yamlPort: "65536", wantEmpty: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := "proxies:\n  - name: test\n    type: http\n    server: example.com\n    port: " + tt.yamlPort + "\n"
			result := string(FromClash([]byte(yaml), ""))
			if tt.wantEmpty && result != "" {
				t.Errorf("port %q: expected empty result, got %q", tt.yamlPort, result)
			}
			if !tt.wantEmpty && result == "" {
				t.Errorf("port %q: expected non-empty result, got empty", tt.yamlPort)
			}
		})
	}
}
