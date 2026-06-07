package openai

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"sync/atomic"
	"time"
)

var fallbackIDCounter uint64

func newOpenAIID(prefix string) string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		binary.BigEndian.PutUint64(bytes[:8], 0)
		binary.BigEndian.PutUint64(bytes[8:], atomic.AddUint64(&fallbackIDCounter, 1))
	}
	return prefix + hex.EncodeToString(bytes[:])
}

func unixNow() int64 {
	return time.Now().Unix()
}
