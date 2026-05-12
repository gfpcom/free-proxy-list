package internal

import (
	"encoding/base64"
	"strings"
	"testing"
)

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
			expected: "ss://" + base64.URLEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:test123")) + "@9.10.11.12:443\n",
		},
		{
			name: "clash config with no proxies",
			input: `port: 7890
socks-port: 7891`,
			expected: "",
		},
		{
			name: "empty input",
			input: "",
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
			name: "port as string",
			input: `proxies:
  - name: "http-proxy"
    type: http
    server: 1.2.3.4
    port: "8080"`,
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
			name: "vmess proxy skipped",
			input: `proxies:
  - name: "vmess-proxy"
    type: vmess
    server: 1.2.3.4
    port: 443`,
			expected: "",
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
			name: "oversized YAML rejected",
			input: strings.Repeat("x", 11*1024*1024), // 11MB
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(FromClash([]byte(tt.input)))
			if result != tt.expected {
				t.Errorf("FromClash() = %q, want %q", result, tt.expected)
			}
		})
	}
}
