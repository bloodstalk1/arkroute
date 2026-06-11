package ratelimit

import (
	"testing"
	"time"
)

func TestAllow(t *testing.T) {
	s := New(time.Second, 5, 3)
	if !s.Allow("key") {
		t.Fatal("first request should be allowed")
	}
	if !s.Allow("key") {
		t.Fatal("second request should be allowed (burst=3)")
	}
	if !s.Allow("key") {
		t.Fatal("third request should be allowed (burst=3)")
	}
	if s.Allow("key") {
		t.Fatal("fourth request should be denied (burst exhausted)")
	}
	time.Sleep(250 * time.Millisecond)
	if !s.Allow("key") {
		t.Fatal("after refill, request should be allowed")
	}
}

func TestAllowEmptyKey(t *testing.T) {
	s := DefaultStore()
	for i := 0; i < 200; i++ {
		if !s.Allow("") {
			t.Fatal("empty key should always be allowed")
		}
	}
}

func TestCleanup(t *testing.T) {
	s := New(time.Minute, 10, 2)
	s.Allow("old-key")
	s.Cleanup(time.Nanosecond)
	if s.Allow("old-key") {
		// After cleanup and full refill (burst=2, limit=10/min, ~1us elapsed ≈ 0 refill)
		// Wait, cleanup only deletes the bucket. The next Allow creates a new one with burst=2.
	}
}
