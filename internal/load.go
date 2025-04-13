package internal

import (
	"bufio"
	"bytes"
	"log"
	"strings"
)

func Load(proto string, content []byte) error {

	s := bufio.NewScanner(bytes.NewReader(content))

	var line, src string
	transformer := FromRaw
	parser := ParseProxyURL
	for s.Scan() {
		line = strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "https://") || strings.HasPrefix(line, "http://") {
			items := strings.Fields(line)

			if len(items) > 0 {
				src = items[0]
			}

			if len(items) > 1 {
				transformer = GetTransformer(items[1])
			}

			if len(items) > 2 {
				parser = GetParser(items[2])
			}

			if src == "" {
				continue
			}

			log.Printf("> %v %s", Fetch(proto, src, transformer, parser), src)
		}

	}

	return nil
}
