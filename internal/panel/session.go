package panel

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

const sessionMaxUses = 30

type sessionEntry struct {
	expires time.Time
	uses    int
}

type SessionStore struct {
	mu     sync.Mutex
	ttl    time.Duration
	tokens map[string]*sessionEntry
}

func NewSessionStore(ttl time.Duration) *SessionStore {
	return &SessionStore{ttl: ttl, tokens: map[string]*sessionEntry{}}
}

func (s *SessionStore) Issue() string {
	var raw [32]byte
	_, _ = rand.Read(raw[:])
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = &sessionEntry{expires: time.Now().Add(s.ttl), uses: 0}
	return token
}

func (s *SessionStore) Valid(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.tokens[token]
	if !ok {
		return false
	}
	now := time.Now()
	if now.After(entry.expires) {
		delete(s.tokens, token)
		return false
	}
	entry.uses++
	if entry.uses > sessionMaxUses {
		delete(s.tokens, token)
		return false
	}
	return true
}
