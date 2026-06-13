package panel

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

const maxSessionTokens = 1024

// SessionStoreOption configures a [SessionStore] at construction.
// Options are unexported; they are consumed only by [NewSessionStore].
type SessionStoreOption func(*SessionStore)

// WithClock replaces the time source used by the store. Intended for
// deterministic tests that need to fast-forward expiry without sleeping.
func WithClock(now func() time.Time) SessionStoreOption {
	return func(s *SessionStore) { s.now = now }
}

type SessionStore struct {
	mu     sync.Mutex
	ttl    time.Duration
	now    func() time.Time
	tokens map[string]time.Time
}

func NewSessionStore(ttl time.Duration, opts ...SessionStoreOption) *SessionStore {
	s := &SessionStore{
		ttl:    ttl,
		now:    time.Now,
		tokens: map[string]time.Time{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *SessionStore) Issue() string {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(fmt.Sprintf("session token: crypto/rand.Read failed: %v", err))
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	s.makeRoomLocked()
	s.tokens[token] = s.now().Add(s.ttl)
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
	if s.now().After(expires) {
		delete(s.tokens, token)
		return false
	}
	return true
}

// makeRoomLocked ensures the store is below maxSessionTokens by first
// purging expired entries and then evicting the entry that expires
// soonest. Callers must hold s.mu.
func (s *SessionStore) makeRoomLocked() {
	now := s.now()
	for token, expires := range s.tokens {
		if now.After(expires) {
			delete(s.tokens, token)
		}
	}
	if len(s.tokens) < maxSessionTokens {
		return
	}
	var victim string
	var victimExpiry time.Time
	for token, expires := range s.tokens {
		if victim == "" || expires.Before(victimExpiry) {
			victim = token
			victimExpiry = expires
		}
	}
	if victim != "" {
		delete(s.tokens, victim)
	}
}
