package panel

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

type SessionStore struct {
	mu     sync.Mutex
	ttl    time.Duration
	tokens map[string]time.Time
}

func NewSessionStore(ttl time.Duration) *SessionStore {
	return &SessionStore{ttl: ttl, tokens: map[string]time.Time{}}
}

func (s *SessionStore) Issue() string {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(fmt.Sprintf("session token: crypto/rand.Read failed: %v", err))
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = time.Now().Add(s.ttl)
	return token
}

func (s *SessionStore) Valid(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expires, ok := s.tokens[token]
	if !ok {
		return false
	}
	if time.Now().After(expires) {
		delete(s.tokens, token)
		return false
	}
	return true
}
