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
	var transformer Transformer
	var parser Parser
	for s.Scan() {
		line = strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "https://") || strings.HasPrefix(line, "http://") {
			src, transformer, parser = parseLine(line)

			if src == "" {
				continue
			}

			log.Printf("> %v %s", Fetch(proto, src, transformer, parser), src)
		}

	}

	return nil
}

func parseLine(line string) (string, Transformer, Parser) {

	if strings.HasPrefix(line, "https://") || strings.HasPrefix(line, "http://") {
		items := strings.Split(line, ",")

		var src string
		transformer := FromRaw
		parser := ParseProxyURL

		if len(items) > 0 {
			src = strings.TrimSpace(items[0])
		}

		if len(items) > 1 {
			transformer = GetTransformer(strings.TrimSpace(items[1]))
		}

		if len(items) > 2 {
			parser = GetParser(strings.TrimSpace(items[2]))
		}

		return src, transformer, parser
	}

	return "", nil, nil
}
