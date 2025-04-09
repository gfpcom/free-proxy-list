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

func Fetch(proto, src, parser string) int {
	var total int
	resp, err := client.Get(src)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	buf, _ := io.ReadAll(resp.Body)

	s := bufio.NewScanner(bytes.NewReader(buf))

	var line string

	for s.Scan() {
		line = strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}

		p := GetParser(parser)

		it, err := p(proto, line)
		if err == nil {
			Save(it)
			total++
		}
	}

	return total
}
