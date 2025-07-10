package internal

import (
	"bufio"
	"bytes"
	"log"
	"strings"
	"fmt"
	"time"
	"regexp"
	"strconv"
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

func roundToNearestIncrement(currentHour int, increment int) int {
	if increment <= 0 || increment > 24 {
		return currentHour
	}
	
	// Generate valid hours based on increment
	var validHours []int
	for h := 0; h < 24; h += increment {
		validHours = append(validHours, h)
	}
	
	closest := 0
	minDiff := 24
	
	for _, validHour := range validHours {
		diff := currentHour - validHour
		if diff < 0 {
			diff = -diff
		}
		if diff > 12 {
			diff = 24 - diff
		}
		
		if diff < minDiff {
			minDiff = diff
			closest = validHour
		}
	}
	
	return closest
}

func applyTokenizer(url string) string {
	now := time.Now()

	hour := now.Hour()
	minute := now.Minute()
	// Handle {HH/N} tokens with regex
	hhIncrementRegex := regexp.MustCompile(`\{HH/(\d+)\}`)
	url = hhIncrementRegex.ReplaceAllStringFunc(url, func(match string) string {
		incrementStr := hhIncrementRegex.FindStringSubmatch(match)[1]
		increment, err := strconv.Atoi(incrementStr)
		if err != nil {
			return match
		}
		roundedHour := roundToNearestIncrement(hour, increment)
		return fmt.Sprintf("%02d", roundedHour)
	})

	replacements := map[string]string{
		"{YYYY}": fmt.Sprintf("%04d", now.Year()),
		"{MM}":   fmt.Sprintf("%02d", now.Month()),
		"{DD}":   fmt.Sprintf("%02d", now.Day()),
		"{HH}":   fmt.Sprintf("%02d", hour),
		"{mm}":   fmt.Sprintf("%02d", minute),
		"{M}":    fmt.Sprintf("%d", now.Month()),
	}

	for token, val := range replacements {
		url = strings.ReplaceAll(url, token, val)
	}

	return url
}

func parseLine(line string) (string, Transformer, Parser) {

	if strings.HasPrefix(line, "https://") || strings.HasPrefix(line, "http://") {
		items := strings.Split(line, ",")

		var src string
		transformer := FromRaw
		parser := ParseProxyURL

		if len(items) > 0 {
			src = applyTokenizer(strings.TrimSpace(items[0]))
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
