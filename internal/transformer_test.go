package internal

import (
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
			expected: "ss://chacha20-ietf-poly1305:test123@9.10.11.12:443\n",
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
