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
