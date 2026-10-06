package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSaveNormalizesMTProtoIdentity(t *testing.T) {
	originalDB := db
	db = make(map[string]*Proxy)
	defer func() { db = originalDB }()

	Save(&Proxy{Protocol: "tg", IP: "Proxy.Example.com.", Port: 443, Opaque: "proxy?port=443&secret=0123456789ABCDEF0123456789ABCDEF&server=Proxy.Example.com."})
	Save(&Proxy{Protocol: "tg", IP: "proxy.example.com", Port: 443, Opaque: "proxy?port=0443&secret=0123456789abcdef0123456789abcdef&server=proxy.example.com"})

	require.Len(t, db, 1)
}

func TestSaveKeepsDistinctMTProtoSecrets(t *testing.T) {
	originalDB := db
	db = make(map[string]*Proxy)
	defer func() { db = originalDB }()

	Save(&Proxy{Protocol: "tg", IP: "8.8.8.8", Port: 443, Opaque: "proxy?secret=first"})
	Save(&Proxy{Protocol: "tg", IP: "8.8.8.8", Port: 443, Opaque: "proxy?secret=second"})

	require.Len(t, db, 2)
}
