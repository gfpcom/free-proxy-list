package internal

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	client = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxConnsPerHost:     10,
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			Proxy: http.ProxyFromEnvironment,
		},
	}
)

// Fetch retrieves the data from src, applies transformer and parser to each
// non-empty line, stores parsed proxies via Save and returns the count of
// successfully parsed proxies.
// transformer and parser may be nil depending on callers but are expected
// to be valid functions when provided.
func Fetch(proto, src string, transformer Transformer, parser Parser) int {
	var total int
	resp, err := client.Get(src)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	buf, _ := io.ReadAll(resp.Body)

	s := bufio.NewScanner(bytes.NewReader(transformer(buf)))

	var line string

	for s.Scan() {
		line = strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}

		it, err := parser(proto, line)
		if err == nil {
			Save(it)
			total++
		}
	}

	return total
}
