package panel

import (
	"testing"
	"time"
)

func TestSessionStoreAcceptsIssuedToken(t *testing.T) {
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	if !store.Valid(token) {
		t.Fatal("issued token should be valid")
	}
}

func TestSessionStoreRejectsMissingAndExpiredTokens(t *testing.T) {
	store := NewSessionStore(time.Nanosecond)
	if store.Valid("") {
		t.Fatal("empty token should be invalid")
	}
	token := store.Issue()
	time.Sleep(time.Millisecond)
	if store.Valid(token) {
		t.Fatal("expired token should be invalid")
	}
}

func TestSessionStoreCapsAndEvictsExpiredOnIssue(t *testing.T) {
	store := NewSessionStore(time.Hour)
	issued := make([]string, 0, maxSessionTokens+10)
	for i := 0; i < maxSessionTokens+10; i++ {
		issued = append(issued, store.Issue())
	}
	store.mu.Lock()
	size := len(store.tokens)
	store.mu.Unlock()
	if size > maxSessionTokens {
		t.Fatalf("token map size = %d, want <= %d", size, maxSessionTokens)
	}
	// The most recent tokens should still be valid; the oldest should be evicted.
	if !store.Valid(issued[len(issued)-1]) {
		t.Fatal("most recent token should be valid")
	}
	firstValid := false
	for i := 0; i < 10; i++ {
		if store.Valid(issued[i]) {
			firstValid = true
			break
		}
	}
	if firstValid {
		t.Fatal("expected early issued tokens to be evicted under cap pressure")
	}
}
