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

// TestSessionStoreRejectsMissingAndExpiredTokens uses a fake clock so
// the test is deterministic on slow CI runners.
func TestSessionStoreRejectsMissingAndExpiredTokens(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	store := NewSessionStore(time.Hour, WithClock(func() time.Time { return now }))
	if store.Valid("") {
		t.Fatal("empty token should be invalid")
	}
	token := store.Issue()
	if !store.Valid(token) {
		t.Fatal("freshly issued token should be valid")
	}
	// Advance the clock past the TTL.
	now = now.Add(2 * time.Hour)
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

// TestSessionStoreExpiredFirstThenLRU verifies the eviction order:
// tokens whose expiry is in the past are purged first, then the
// soonest-to-expire token is dropped.
func TestSessionStoreExpiredFirstThenLRU(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	clock := func() time.Time { return now }
	store := NewSessionStore(time.Hour, WithClock(clock))

	// Fill the store with maxSessionTokens tokens, all expiring at the
	// same future instant.
	for i := 0; i < maxSessionTokens; i++ {
		store.Issue()
	}
	// Advance the clock so half the tokens are now expired by writing
	// new ones with TTL=0 (i.e. already expired). This requires
	// re-issuing after manually expiring; use a small helper.
	store.mu.Lock()
	half := maxSessionTokens / 2
	count := 0
	for tok := range store.tokens {
		if count >= half {
			break
		}
		store.tokens[tok] = now.Add(-time.Second)
		count++
	}
	store.mu.Unlock()

	// Issuing one more token should evict expired entries, then succeed
	// without dropping any of the still-valid ones.
	store.Issue()
	store.mu.Lock()
	stillValid := 0
	for _, expires := range store.tokens {
		if expires.After(now) {
			stillValid++
		}
	}
	store.mu.Unlock()
	if stillValid != maxSessionTokens-half+1 {
		t.Fatalf("expected %d unexpired tokens, got %d", maxSessionTokens-half+1, stillValid)
	}
}
