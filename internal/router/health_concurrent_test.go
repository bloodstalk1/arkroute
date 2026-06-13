package router

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestHealthStoreConcurrentUpdateAndSnapshot runs many writers and
// many readers concurrently. With -race this catches any lock-scope
// mistakes around the snapshot/circuit maps.
func TestHealthStoreConcurrentUpdateAndSnapshot(t *testing.T) {
	const writers = 16
	const readers = 16
	const opsPerWriter = 500
	const opsPerReader = 500

	store := NewHealthStore()
	var wg sync.WaitGroup
	var updates, snapshots, circuitChecks atomic.Int64

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerWriter; j++ {
				store.Update(Update{
					ProviderID:    "p",
					UpstreamModel: "m",
					Status:        "ok",
				})
				updates.Add(1)
			}
		}()
	}
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerReader; j++ {
				_ = store.Snapshot()
				_ = store.IsCircuited("p", "m")
				snapshots.Add(1)
				circuitChecks.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := updates.Load(); int(got) != writers*opsPerWriter {
		t.Fatalf("updates = %d, want %d", got, writers*opsPerWriter)
	}
	snap := store.Snapshot()
	if _, ok := snap["p"]; !ok {
		t.Fatal("provider 'p' missing from final snapshot")
	}
}
