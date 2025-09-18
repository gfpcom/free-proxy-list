package internal

import "encoding/base64"

// Transformers holds registered data transformers.
var (
	Transformers = map[string]Transformer{}
)

func init() {
	Transformers["base64"] = FromBase64
}

// Transformer converts raw response bytes into transformed bytes for parsing.
type Transformer func([]byte) []byte

// RegisterTransformer registers a Transformer under the given name.
func RegisterTransformer(name string, t Transformer) {
	Transformers[name] = t
}

// GetTransformer returns the named Transformer or FromRaw if none exists.
func GetTransformer(name string) Transformer {
	if t, ok := Transformers[name]; ok {
		return t
	}

	return FromRaw
}

// FromRaw returns the input unchanged.
func FromRaw(buf []byte) []byte {
	return buf
}

// FromBase64 decodes base64-encoded input; on error it returns the original buffer.
func FromBase64(buf []byte) []byte {
	decoded, err := base64.StdEncoding.DecodeString(string(buf))
	if err != nil {
		return buf
	}

	return decoded
}
