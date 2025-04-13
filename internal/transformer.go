package internal

import "encoding/base64"

var (
	Transformers = map[string]Transformer{}
)

func init() {
	Transformers["base64"] = FromBase64
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
