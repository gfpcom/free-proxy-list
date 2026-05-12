package internal

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"gopkg.in/yaml.v3"
)

var (
	Transformers = map[string]Transformer{}
)

func init() {
	Transformers["base64"] = FromBase64
	Transformers["clash"] = FromClash
}

type Transformer func([]byte) []byte

func RegisterTransformer(name string, t Transformer) {
	Transformers[name] = t
}

func GetTransformer(name string) Transformer {
	if t, ok := Transformers[name]; ok {
		return t
	}

	return FromRaw
}

func FromRaw(buf []byte) []byte {
	return buf
}

func FromBase64(buf []byte) []byte {
	decoded, err := base64.StdEncoding.DecodeString(string(buf))
	if err != nil {
		return buf
	}

	return decoded
}

// ClashConfig represents a Clash YAML configuration
type ClashConfig struct {
	Proxies []map[string]interface{} `yaml:"proxies"`
}

// FromClash parses a Clash YAML config and extracts proxy URLs
func FromClash(buf []byte) []byte {
	// Limit YAML size to prevent OOM attacks (10MB max)
	const maxYAMLSize = 10 * 1024 * 1024
	if len(buf) > maxYAMLSize {
		return []byte{}
	}

	var config ClashConfig
	if err := yaml.Unmarshal(buf, &config); err != nil {
		return []byte{}
	}

	var result bytes.Buffer
	for _, proxy := range config.Proxies {
		proxyURL := buildProxyURL(proxy)
		if proxyURL != "" {
			result.WriteString(proxyURL)
			result.WriteString("\n")
		}
	}

	return result.Bytes()
}

func buildProxyURL(proxy map[string]interface{}) string {
	proxyType, ok := proxy["type"].(string)
	if !ok {
		return ""
	}

	server, ok := proxy["server"].(string)
	if !ok {
		return ""
	}

	port, ok := proxy["port"]
	if !ok {
		return ""
	}

	portInt, err := extractPort(port)
	if err != nil {
		return ""
	}

	// Validate port range
	if portInt < 1 || portInt > 65535 {
		return ""
	}

	switch proxyType {
	case "http", "https", "socks5", "socks4":
		return fmt.Sprintf("%s://%s:%d", proxyType, server, portInt)
	case "ss":
		cipher, ok := proxy["cipher"].(string)
		if !ok || cipher == "" {
			return ""
		}
		password, ok := proxy["password"].(string)
		if !ok || password == "" {
			return ""
		}
		// Shadowsocks URL format: ss://base64(cipher:password)@server:port
		credentials := base64.URLEncoding.EncodeToString([]byte(cipher + ":" + password))
		return fmt.Sprintf("ss://%s@%s:%d", credentials, server, portInt)
	case "vmess", "vless", "trojan":
		// These protocols require complex URL formats with UUIDs, encryption params, etc.
		// Since we can't construct valid URLs without all required fields, skip them
		// to avoid creating broken proxy entries
		return ""
	default:
		return ""
	}
}

func extractPort(port interface{}) (int, error) {
	switch v := port.(type) {
	case int:
		return v, nil
	case float64:
		return int(v), nil
	case string:
		p, err := strconv.Atoi(v)
		if err != nil {
			return 0, err
		}
		return p, nil
	default:
		return 0, fmt.Errorf("unsupported port type")
	}
}
