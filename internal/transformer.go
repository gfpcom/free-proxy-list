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

// FlexPort handles YAML port values that may be int, float, or string.
type FlexPort int

func (p *FlexPort) UnmarshalYAML(value *yaml.Node) error {
	switch value.Tag {
	case "!!int":
		v, err := strconv.Atoi(value.Value)
		if err != nil {
			return err
		}
		*p = FlexPort(v)
		return nil
	case "!!float":
		v, err := strconv.ParseFloat(value.Value, 64)
		if err != nil {
			return err
		}
		*p = FlexPort(int(v))
		return nil
	case "!!str":
		v, err := strconv.Atoi(value.Value)
		if err != nil {
			return err
		}
		*p = FlexPort(v)
		return nil
	}
	// Unsupported types (bool, null, etc.) default to 0; port range check rejects them.
	return nil
}

// ClashProxy represents a single proxy entry in a Clash config.
type ClashProxy struct {
	Type     string   `yaml:"type"`
	Server   string   `yaml:"server"`
	Port     FlexPort `yaml:"port"`
	Cipher   string   `yaml:"cipher,omitempty"`
	Password string   `yaml:"password,omitempty"`
	Username string   `yaml:"username,omitempty"`
}

// ClashConfig represents a Clash YAML configuration.
type ClashConfig struct {
	Proxies []ClashProxy `yaml:"proxies"`
}

// FromClash parses a Clash YAML config and extracts proxy URLs.
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

func buildProxyURL(proxy ClashProxy) string {
	if proxy.Type == "" || proxy.Server == "" {
		return ""
	}

	port := int(proxy.Port)

	// Validate port range
	if port < 1 || port > 65535 {
		return ""
	}

	switch proxy.Type {
	case "http", "https", "socks5", "socks4":
		if proxy.Username != "" && proxy.Password != "" {
			return fmt.Sprintf("%s://%s:%s@%s:%d", proxy.Type, proxy.Username, proxy.Password, proxy.Server, port)
		}
		return fmt.Sprintf("%s://%s:%d", proxy.Type, proxy.Server, port)
	case "ss":
		if proxy.Cipher == "" || proxy.Password == "" {
			return ""
		}
		// Shadowsocks URL format: ss://base64(cipher:password)@server:port
		credentials := base64.StdEncoding.EncodeToString([]byte(proxy.Cipher + ":" + proxy.Password))
		return fmt.Sprintf("ss://%s@%s:%d", credentials, proxy.Server, port)
	case "vmess", "vless", "trojan":
		// These protocols require complex URL formats with UUIDs, encryption params, etc.
		// Since we can't construct valid URLs without all required fields, skip them
		// to avoid creating broken proxy entries
		return ""
	default:
		return ""
	}
}
