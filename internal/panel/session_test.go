package panel

import (
	"testing"
	"time"
)

func TestSessionStoreAcceptsIssuedTokenUpToLimit(t *testing.T) {
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	for i := 0; i < sessionMaxUses; i++ {
		if !store.Valid(token) {
			t.Fatalf("issued token should be valid at use %d/%d", i+1, sessionMaxUses)
		}
	}
	if store.Valid(token) {
		t.Fatal("issued token should be invalidated after exceeding use limit")
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
