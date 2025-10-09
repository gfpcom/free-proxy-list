package internal

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
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

	// Generate STATS.md and update README.md
	WriteStats(dir, counters)
}

func WriteStats(dir string, counters map[string]int) {
	// Get sorted protocols for consistent ordering
	protocols := make([]string, 0, len(counters))
	total := 0
	for proto, count := range counters {
		protocols = append(protocols, proto)
		total = total + count
	}
	sort.Strings(protocols)

	timestamp := time.Now().UTC().Format("2006-01-02 15:04:05 UTC")

	// Generate table content
	var tableContent strings.Builder
	for _, proto := range protocols {
		count := counters[proto]
		url := fmt.Sprintf("https://github.com/gfpcom/free-proxy-list/wiki/lists/%s.txt", proto)
		tableContent.WriteString(fmt.Sprintf("| %s | %d | %s |\n",
			strings.ToUpper(proto),
			count,
			url))
	}

	// Write STATS.md
	statsFile, err := os.Create(filepath.Join(dir, "STATS.md"))
	if err != nil {
		return
	}
	defer statsFile.Close()

	statsContent := fmt.Sprintf(`# Proxy Statistics

Last Updated: %s

**Total Proxies: %d**

| Protocol | Count | Download |
|----------|-------|----------|
%s`, timestamp, total, tableContent.String())

	statsFile.WriteString(statsContent)

	// Update README.md
	readmePath := filepath.Join(dir, "..", "README.md")
	readmeContent, err := os.ReadFile(readmePath)
	if err != nil {
		return
	}

	// Prepare new section content
	newSection := fmt.Sprintf(`
Last Updated: %s

**Total Proxies: %d**

Click on your preferred proxy type to get the latest list. These links always point to the most recently updated proxy files.

| Protocol | Count | Download |
|----------|-------|----------|
%s`, timestamp, total, tableContent.String())

	// Find and replace the section in README.md
	content := string(readmeContent)
	startMarker := "<!-- BEGIN PROXY LIST -->"
	endMarker := "<!-- END PROXY LIST -->"

	startIdx := strings.Index(content, startMarker)
	endIdx := strings.Index(content, endMarker)

	if startIdx != -1 && endIdx != -1 {
		// Keep the markers and replace content between them
		before := content[:startIdx+len(startMarker)]
		after := content[endIdx:]

		// Add newlines for better readability
		newContent := before + "\n" + newSection + "\n" + after
		os.WriteFile(readmePath, []byte(newContent), 0644)
	}
}
