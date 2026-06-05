package panel

import (
	"testing"
	"time"
)

func TestSessionStoreAcceptsIssuedTokenOnceBeforeExpiry(t *testing.T) {
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	if !store.Valid(token) {
		t.Fatal("issued token should be valid")
	}
	if !store.Valid(token) {
		t.Fatal("issued token should remain valid until expiry")
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
