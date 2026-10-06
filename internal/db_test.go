package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSaveKeepsDistinctMTProtoSecrets(t *testing.T) {
	originalDB := db
	db = make(map[string]*Proxy)
	defer func() { db = originalDB }()

	Save(&Proxy{Protocol: "tg", IP: "8.8.8.8", Port: 443, Opaque: "proxy?secret=first"})
	Save(&Proxy{Protocol: "tg", IP: "8.8.8.8", Port: 443, Opaque: "proxy?secret=second"})

	require.Len(t, db, 2)
}
