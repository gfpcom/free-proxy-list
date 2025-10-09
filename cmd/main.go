package main

import (
	"flag"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/gfpcom/free-proxy-list/internal"
)

var dir string

func main() {

	flag.StringVar(&dir, "dir", ".", "work directory")
	flag.Parse()

	os.MkdirAll(filepath.Join(dir, "list"), 0755) // nolint: errcheck


	err := fs.WalkDir(os.DirFS(filepath.Join(dir, "sources")), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("gfp: open source", slog.String("file", path), slog.Any("err", err))
			return nil
		}

		if d.IsDir() {
			return nil
		}

		// Get filename without extension
		filename := d.Name()
		proto := strings.ToLower(strings.TrimSuffix(filename, filepath.Ext(filename)))

		buf, err := os.ReadFile(filepath.Join(dir, "sources", path))
		if err != nil {
			slog.Warn("gfp: read source", slog.String("file", path), slog.Any("err", err))
			return nil
		}

		log.Println("--------" + path + "-------")
		err = internal.Load(proto, buf)
		if err != nil {
			slog.Warn("gfp: read source", slog.String("file", path), slog.Any("err", err))
			return nil
		}
		log.Println("---------------------------")
		log.Println("")

		return nil
	})


	internal.WriteTo(filepath.Join(dir, "list"))

	if err != nil {
		panic(err)
	}
}
