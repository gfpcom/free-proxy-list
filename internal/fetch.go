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

func Fetch(proto, src string, transformer Transformer, transformerOptions string, parser Parser) int {
	resp, err := client.Get(src)
	if err != nil {
		return 0
	}
	defer resp.Body.Close() // nolint: errcheck
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return 0
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0
	}

	return saveTransformedProxies(proto, transformer(buf, transformerOptions), parser)
}

func FetchCurl(proto, src, transformerOptions string, parser Parser) int {
	buf, err := curlImpersonateFetch(src)
	if err != nil {
		return 0
	}
	return saveTransformedProxies(proto, fromCurl(buf, transformerOptions, src), parser)
}

func saveTransformedProxies(proto string, data []byte, parser Parser) int {
	var total int
	s := bufio.NewScanner(bytes.NewReader(data))
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
