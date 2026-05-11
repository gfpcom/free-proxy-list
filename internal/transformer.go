package internal

import (
	"bytes"
	"encoding/base64"
	"fmt"
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

	var portInt int
	switch v := port.(type) {
	case int:
		portInt = v
	case float64:
		portInt = int(v)
	default:
		return ""
	}

	switch proxyType {
	case "http", "https", "socks5", "socks4":
		return fmt.Sprintf("%s://%s:%d", proxyType, server, portInt)
	case "ss":
		cipher, _ := proxy["cipher"].(string)
		password, _ := proxy["password"].(string)
		if cipher == "" || password == "" {
			return ""
		}
		return fmt.Sprintf("ss://%s:%s@%s:%d", cipher, password, server, portInt)
	case "vmess", "vless", "trojan":
		// For complex protocols, we would need to construct the full URL
		// For now, return a basic format
		return fmt.Sprintf("%s://%s:%d", proxyType, server, portInt)
	default:
		return ""
	}
}
