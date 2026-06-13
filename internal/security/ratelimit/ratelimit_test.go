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
	// After Cleanup, the bucket is gone; the next Allow must allocate a
	// fresh bucket and immediately allow (within burst). With burst=2
	// this is the first call, so it should succeed.
	if !s.Allow("old-key") {
		t.Fatal("Allow after cleanup should succeed (fresh burst)")
	}
	// Second call is also within the new burst.
	if !s.Allow("old-key") {
		t.Fatal("second Allow after cleanup should succeed (within burst)")
	}
	// Third call exceeds the burst.
	if s.Allow("old-key") {
		t.Fatal("third Allow after cleanup should be denied (burst exhausted)")
	}
}
