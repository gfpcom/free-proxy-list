package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

const (
	maxJSONSize    = 10 * 1024 * 1024
	maxJSONRecords = 100000
)

type jsonTransformerConfig struct {
	path   []string
	uri    []string
	host   []string
	port   []string
	scheme []string
}

func FromJSON(data []byte, options string) []byte {
	if len(data) == 0 || len(data) > maxJSONSize {
		return nil
	}

	config, err := parseJSONTransformerOptions(options)
	if err != nil {
		return nil
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil
	}

	records, ok := resolveJSONPath(document, config.path)
	if !ok {
		return nil
	}
	if len(records) == 1 {
		if array, ok := records[0].([]any); ok {
			records = array
		}
	}
	if len(records) > maxJSONRecords {
		return nil
	}

	var output bytes.Buffer
	for _, record := range records {
		line, ok := transformJSONRecord(record, config)
		if !ok {
			continue
		}
		output.WriteString(line)
		output.WriteByte('\n')
	}
	return output.Bytes()
}

func validateJSONTransformerOptions(options string) error {
	_, err := parseJSONTransformerOptions(options)
	return err
}

func parseJSONTransformerOptions(options string) (jsonTransformerConfig, error) {
	var config jsonTransformerConfig
	settings := make(map[string]string)
	for _, item := range strings.Split(options, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(item), "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" || value == "" {
			return config, fmt.Errorf("expected non-empty key=value options")
		}
		switch key {
		case "path", "uri", "host", "port", "scheme":
		default:
			return config, fmt.Errorf("unknown option %q", key)
		}
		if _, exists := settings[key]; exists {
			return config, fmt.Errorf("duplicate option %q", key)
		}
		settings[key] = value
	}

	if settings["path"] == "" {
		return config, fmt.Errorf("path is required")
	}
	var ok bool
	if config.path, ok = parseJSONPath(settings["path"], true); !ok {
		return config, fmt.Errorf("invalid path %q", settings["path"])
	}
	if uri := settings["uri"]; uri != "" {
		if settings["host"] != "" || settings["port"] != "" || settings["scheme"] != "" {
			return config, fmt.Errorf("uri cannot be combined with host, port, or scheme")
		}
		if config.uri, ok = parseJSONPath(uri, false); !ok {
			return config, fmt.Errorf("invalid uri field path %q", uri)
		}
		return config, nil
	}
	if settings["host"] == "" || settings["port"] == "" {
		return config, fmt.Errorf("host and port are required when uri is not set")
	}
	if config.host, ok = parseJSONPath(settings["host"], false); !ok {
		return config, fmt.Errorf("invalid host field path %q", settings["host"])
	}
	if config.port, ok = parseJSONPath(settings["port"], false); !ok {
		return config, fmt.Errorf("invalid port field path %q", settings["port"])
	}
	if scheme := settings["scheme"]; scheme != "" {
		if config.scheme, ok = parseJSONPath(scheme, false); !ok {
			return config, fmt.Errorf("invalid scheme field path %q", scheme)
		}
	}
	return config, nil
}

func parseJSONPath(path string, rooted bool) ([]string, bool) {
	if !rooted {
		path = "$." + path
	}
	if !strings.HasPrefix(path, "$") {
		return nil, false
	}
	path = path[1:]
	var selectors []string
	for len(path) > 0 {
		switch path[0] {
		case '.':
			path = path[1:]
			end := 0
			for end < len(path) && isJSONPathKeyChar(path[end], end == 0) {
				end++
			}
			if end == 0 {
				return nil, false
			}
			selectors = append(selectors, path[:end])
			path = path[end:]
		case '[':
			end := strings.IndexByte(path, ']')
			if end < 2 {
				return nil, false
			}
			index := path[1:end]
			if index != "*" {
				if index == "" {
					return nil, false
				}
				for _, digit := range index {
					if digit < '0' || digit > '9' {
						return nil, false
					}
				}
			}
			selectors = append(selectors, "["+index+"]")
			path = path[end+1:]
		default:
			return nil, false
		}
	}
	return selectors, true
}

func isJSONPathKeyChar(char byte, first bool) bool {
	if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char == '_' {
		return true
	}
	return !first && (char >= '0' && char <= '9' || char == '-')
}

func resolveJSONPath(value any, selectors []string) ([]any, bool) {
	values := []any{value}
	for _, selector := range selectors {
		var next []any
		for _, current := range values {
			switch {
			case strings.HasPrefix(selector, "["):
				array, ok := current.([]any)
				if !ok {
					continue
				}
				index := selector[1 : len(selector)-1]
				if index == "*" {
					if len(array) > maxJSONRecords-len(next) {
						return nil, false
					}
					next = append(next, array...)
				} else if i, err := strconv.Atoi(index); err == nil && i < len(array) {
					if len(next) == maxJSONRecords {
						return nil, false
					}
					next = append(next, array[i])
				}
			default:
				object, ok := current.(map[string]any)
				if !ok {
					continue
				}
				if field, found := object[selector]; found {
					if len(next) == maxJSONRecords {
						return nil, false
					}
					next = append(next, field)
				}
			}
		}
		values = next
		if len(values) == 0 {
			return nil, false
		}
	}
	return values, true
}

func transformJSONRecord(record any, config jsonTransformerConfig) (string, bool) {
	if len(config.uri) > 0 {
		value, ok := resolveJSONPath(record, config.uri)
		if !ok || len(value) != 1 {
			return "", false
		}
		text, ok := value[0].(string)
		text = strings.TrimSpace(text)
		return text, ok && text != "" && !strings.ContainsAny(text, "\r\n")
	}

	hostValue, ok := resolveJSONPath(record, config.host)
	if !ok || len(hostValue) != 1 {
		return "", false
	}
	host, ok := hostValue[0].(string)
	host = strings.TrimSpace(host)
	if !ok || host == "" || strings.ContainsAny(host, "/?#@ \t\r\n") {
		return "", false
	}
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")

	portValue, ok := resolveJSONPath(record, config.port)
	if !ok || len(portValue) != 1 {
		return "", false
	}
	port, ok := jsonPort(portValue[0])
	if !ok {
		return "", false
	}
	endpoint := net.JoinHostPort(host, strconv.Itoa(port))

	if len(config.scheme) == 0 {
		return endpoint, true
	}
	schemeValue, ok := resolveJSONPath(record, config.scheme)
	if !ok || len(schemeValue) != 1 {
		return "", false
	}
	scheme, ok := schemeValue[0].(string)
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if !ok || scheme == "" {
		return "", false
	}
	for i, char := range scheme {
		switch {
		case char >= 'a' && char <= 'z':
		case i > 0 && (char >= '0' && char <= '9' || char == '+' || char == '.' || char == '-'):
		default:
			return "", false
		}
	}
	return scheme + "://" + endpoint, true
}

func jsonPort(value any) (int, bool) {
	var text string
	switch value := value.(type) {
	case json.Number:
		text = value.String()
	case string:
		text = strings.TrimSpace(value)
	default:
		return 0, false
	}
	if text == "" {
		return 0, false
	}
	for _, char := range text {
		if char < '0' || char > '9' {
			return 0, false
		}
	}
	port, err := strconv.Atoi(text)
	return port, err == nil && port > 0 && port <= 65535
}
