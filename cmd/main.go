package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gfpcom/free-proxy-list/internal"
)

var dir string

func main() {

	flag.StringVar(&dir, "dir", ".", "work directory")
	flag.Parse()

	hostProxyCount := make(map[string]int)

	var reRepo = regexp.MustCompile(`^(?:https?://)?([^/]+)/([^/]+)/([^/]+)`) // host/user/repo

	err := fs.WalkDir(os.DirFS(filepath.Join(dir, "sources")), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("gfp: open source", slog.String("file", path), slog.Any("err", err))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		filename := d.Name()
		proto := strings.ToLower(strings.TrimSuffix(filename, filepath.Ext(filename)))
		buf, err := os.ReadFile(filepath.Join(dir, "sources", path))
		if err != nil {
			slog.Warn("gfp: read source", slog.String("file", path), slog.Any("err", err))
			return nil
		}
		log.Println("--------" + path + "-------")
		// Use Load with a callback to collect per-host statistics
		err = internal.Load(proto, buf, func(src string, count int) {
			hostKey := ""
			if strings.Contains(src, "github.com/") || strings.Contains(src, "gitlab.com/") {
				if m := reRepo.FindStringSubmatch(src); len(m) == 4 {
					hostKey = fmt.Sprintf("%s/%s/%s", m[1], m[2], m[3])
				}
			} else if strings.Contains(src, "raw.githubusercontent.com/") {
				// Convert raw.githubusercontent.com/[user]/[repo]/... to github.com/[user]/[repo]
				parts := strings.Split(src, "/")
				if len(parts) > 4 {
					hostKey = fmt.Sprintf("github.com/%s/%s", parts[3], parts[4])
				}
			}
			if hostKey == "" {
				u, err := url.Parse(src)
				if err == nil && u.Host != "" {
					hostKey = u.Host
				} else {
					if strings.HasPrefix(src, "http") {
						parts := strings.Split(src, "/")
						if len(parts) > 2 {
							hostKey = parts[2]
						}
					}
				}
			}
			if hostKey == "" {
				return
			}
			hostProxyCount[hostKey] += count
		})
		if err != nil {
			slog.Warn("gfp: read source", slog.String("file", path), slog.Any("err", err))
			return nil
		}
		log.Println("---------------------------")
		log.Println("")
		return nil
	})

	hosts := make([]string, 0, len(hostProxyCount))
	for h := range hostProxyCount {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	var md strings.Builder
	md.WriteString("# Proxy Sources\n\n")
	md.WriteString("We gratefully acknowledge the following sites for sharing open proxy data.\n\n")
	md.WriteString(fmt.Sprintf("Last updated: %s\n\n", time.Now().Format("2006-01-02 15:04:05 MST")))
	md.WriteString("| Hostname | Proxy Count |\n")
	md.WriteString("|---|---|\n")
	for _, h := range hosts {
		md.WriteString(fmt.Sprintf("| %s | %d |\n", h, hostProxyCount[h]))
	}
	os.WriteFile(filepath.Join(dir, "SOURCES.md"), []byte(md.String()), 0644)

	internal.WriteTo(filepath.Join(dir, "list"))

	if err != nil {
		panic(err)
	}
}
