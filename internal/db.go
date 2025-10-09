package internal

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	db = make(map[string]*Proxy)
)

func Save(it *Proxy) {
	h := md5.New()
	id := hex.EncodeToString(h.Sum([]byte(fmt.Sprintf("%s://%s:%v", it.Protocol, it.IP, it.Port))))
	db[id] = it
}

func WriteTo(dir string) {
	files := make(map[string]*os.File)
	defer func() {
		for _, f := range files {
			f.Sync() // nolint: errcheck
			f.Close()
		}
	}()

	// Get all keys and sort them
	keys := make([]string, 0, len(db))
	for k := range db {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	counters := make(map[string]int)

	// Iterate through sorted keys
	for _, key := range keys {
		it := db[key]
		file, ok := files[it.Protocol]
		if !ok {
			file, _ = os.Create(filepath.Join(dir, it.Protocol+".txt"))
			files[it.Protocol] = file
		}

		c, ok := counters[it.Protocol]
		if !ok {
			counters[it.Protocol] = 1
		} else {
			counters[it.Protocol] = c + 1
		}

		file.WriteString(it.String() + "\n") // nolint: errcheck
	}

	// Generate total.svg and update README.md
	WriteTotalAndUpdateReadme(dir, counters)
}

func WriteTotalAndUpdateReadme(dir string, counters map[string]int) {
	// Calculate total
	total := 0
	protocols := make([]string, 0, len(counters))
	for proto, count := range counters {
		protocols = append(protocols, proto)
		total += count
	}
	sort.Strings(protocols)

	timestamp := time.Now().UTC().Format("2006-01-02 15:04:05 UTC")

	// Generate total.svg using shields.io
	// e.g. https://img.shields.io/badge/total-1234-blue
	svgURL := fmt.Sprintf("https://img.shields.io/badge/total-%d-blue", total)
	resp, err := httpGet(svgURL)
	if err == nil && resp != nil {
		defer resp.Close()
		// write to list/total.svg
		outPath := filepath.Join(dir, "total.svg")
		_ = os.WriteFile(outPath, resp.Bytes(), 0644) // nolint: errcheck
	}

	// Build table content for README replacement
	var tableContent strings.Builder
	for _, proto := range protocols {
		count := counters[proto]
		url := fmt.Sprintf("https://github.com/gfpcom/free-proxy-list/wiki/lists/%s.txt", proto)
		tableContent.WriteString(fmt.Sprintf("| %s | %d | %s |\n",
			strings.ToUpper(proto),
			count,
			url))
	}

	// Update README.md
	readmePath := filepath.Join(dir, "..", "README.md")
	readmeContent, err := os.ReadFile(readmePath)
	if err == nil {
		newSection := fmt.Sprintf(`
Last Updated: %s

**Total Proxies: %d**

Click on your preferred proxy type to get the latest list. These links always point to the most recently updated proxy files.

| Protocol | Count | Download |
|----------|-------|----------|
%s`, timestamp, total, tableContent.String())

		content := string(readmeContent)
		startMarker := "<!-- BEGIN PROXY LIST -->"
		endMarker := "<!-- END PROXY LIST -->"

		startIdx := strings.Index(content, startMarker)
		endIdx := strings.Index(content, endMarker)

		if startIdx != -1 && endIdx != -1 {
			before := content[:startIdx+len(startMarker)]
			after := content[endIdx:]
			newContent := before + "\n" + newSection + "\n" + after
			_ = os.WriteFile(readmePath, []byte(newContent), 0644) // nolint: errcheck
		}
	}
}

// httpGet fetches URL and returns a small wrapper with the body bytes and Close()
type respWrap struct {
	b []byte
}

func (r *respWrap) Close() error  { return nil }
func (r *respWrap) Bytes() []byte { return r.b }

func httpGet(url string) (*respWrap, error) {
	// Use simple http.Get but avoid adding net/http import at top if not present; import locally
	resp, err := http.DefaultClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &respWrap{b: data}, nil
}
