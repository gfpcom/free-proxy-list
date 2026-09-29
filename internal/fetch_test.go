package internal

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetch(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping test in GitHub Actions environment")
	}

	// Fetch("", "https://github.com/mfuu/v2ray/raw/refs/heads/master/merge/merge.txt", FromRaw, "", ParseProxyURL)
	// Fetch("", "https://github.com/snakem982/proxypool/raw/refs/heads/main/source/v2ray-2.txt", FromBase64, "", ParseProxyURL)

	src, transformer, transformerOptions, parser := parseLine("https://github.com/zloi-user/hideip.me/raw/refs/heads/master/http.txt,,ColonURL")
	Fetch("http", src, transformer, transformerOptions, parser)

}

func TestValidateSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/not-found" {
			http.NotFound(w, r)
			return
		}
		if _, err := fmt.Fprintln(w, "http://8.8.8.8:8080"); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	require.NoError(t, ValidateSource("http", []byte(server.URL)))
	require.Error(t, ValidateSource("http", []byte(server.URL+"/not-found")))
	require.Error(t, ValidateSource("http", []byte("http://127.0.0.1:1/unreachable")))
	require.Error(t, ValidateSource("http", []byte("not-a-feed")))
}
