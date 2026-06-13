package panel

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSessionStoreConcurrentIssueAndValid runs many goroutines
// concurrently issuing tokens and validating them. With -race this
// catches any unprotected access to the internal map.
func TestSessionStoreConcurrentIssueAndValid(t *testing.T) {
	store := NewSessionStore(time.Hour)
	const goroutines = 50
	const opsPerGoroutine = 200

	var wg sync.WaitGroup
	var issued, validHits, invalidHits atomic.Int64

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				token := store.Issue()
				issued.Add(1)
				if store.Valid(token) {
					validHits.Add(1)
				} else {
					invalidHits.Add(1)
				}
				// Also probe a known-bogus token to ensure Valid handles
				// the "not present" path under contention.
				if store.Valid("not-a-real-token") {
					invalidHits.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	if got := issued.Load(); int(got) != goroutines*opsPerGoroutine {
		t.Fatalf("issued = %d, want %d", got, goroutines*opsPerGoroutine)
	}
	// Every freshly-issued token must be valid; otherwise the cap
	// eviction is racing with the read in a way that drops our own
	// token before we can validate it.
	if validHits.Load() != issued.Load() {
		t.Fatalf("valid hits (%d) != issued (%d): %d misses",
			validHits.Load(), issued.Load(), invalidHits.Load())
	}
}

// TestSessionStoreCapHoldsUnderConcurrentIssue drives the store well
// past its cap and confirms the bound is preserved. Run with -race.
func TestSessionStoreCapHoldsUnderConcurrentIssue(t *testing.T) {
	store := NewSessionStore(time.Hour)
	const goroutines = 32
	const issuesPerGoroutine = 200

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < issuesPerGoroutine; j++ {
				store.Issue()
			}
		}()
	}
	wg.Wait()

	store.mu.Lock()
	size := len(store.tokens)
	store.mu.Unlock()
	if size > maxSessionTokens {
		t.Fatalf("size = %d, want <= %d", size, maxSessionTokens)
	}
}
