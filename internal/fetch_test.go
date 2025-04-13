package internal

import (
	"os"
	"testing"
)

func TestFet(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping test in GitHub Actions environment")
	}

	// Fetch("", "https://github.com/mfuu/v2ray/raw/refs/heads/master/merge/merge.txt", FromRaw, ParseProxyURL)
	// Fetch("", "https://github.com/snakem982/proxypool/raw/refs/heads/main/source/v2ray-2.txt", FromBase64, ParseProxyURL)

	src, transformer, parser := parseLine("https://github.com/zloi-user/hideip.me/raw/refs/heads/master/http.txt,,ColonURL")
	Fetch("http", src, transformer, parser)

}
