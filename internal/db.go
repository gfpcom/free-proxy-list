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
)

// db stores deduplicated proxies indexed by an internal id.
var (
	db = make(map[string]*Proxy)
)

// Save adds a proxy to the internal database.
// It uses an MD5 of protocol+ip+port as the key to deduplicate entries.
func Save(it *Proxy) {
	h := md5.New()

	id := hex.EncodeToString(h.Sum([]byte(fmt.Sprintf("%s://%s:%v", it.Protocol, it.IP, it.Port))))

	db[id] = it
}

// WriteTo writes all collected proxies to files under the provided directory.
// Each protocol has its own file named <protocol>.txt. Badges are also generated.
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
	total := 0
	for proto, n := range counters {
		total += n
		WriteBadge(dir, proto, n)
	}

	WriteBadge(dir, "total", total)
}

// WriteBadge fetches an SVG badge for the given protocol and writes it to disk.
func WriteBadge(dir, proto string, total int) {

	resp, err := http.Get(fmt.Sprintf("https://img.shields.io/badge/%s-%v-blue", proto, total))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		file, err := os.Create(filepath.Join(dir, proto+".svg"))
		if err == nil {
			defer file.Close()
			io.Copy(file, resp.Body) // nolint: errcheck
		}
	}

}
