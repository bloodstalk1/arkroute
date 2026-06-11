// Package ratelimit provides a simple token-bucket rate limiter
// for the local gateway API key. It is designed to prevent a single
// misbehaving client from overwhelming the upstream providers.
//
// Default limit: 60 requests per minute per API key.
// Burst: 5 concurrent requests.
package ratelimit

import (
	"sync"
	"time"
)

// Store tracks per-key rate limit state. A nil Store means no rate limiting.
type Store struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	interval time.Duration
	limit    int
	burst    int
}

type bucket struct {
	tokens   float64
	lastTime time.Time
}

// New creates a Store with the given interval and limit.
// limit is the maximum number of tokens refilled per interval.
// burst is the initial token count and maximum accumulated tokens.
func New(interval time.Duration, limit int, burst int) *Store {
	return &Store{
		buckets:  map[string]*bucket{},
		interval: interval,
		limit:    limit,
		burst:    burst,
	}
}

// DefaultStore returns a Store with sensible defaults: 60 req/min, burst 5.
func DefaultStore() *Store {
	return New(time.Minute, 60, 5)
}

// Allow returns true if a request with the given key is allowed at this time.
// An empty key always passes (no rate limiting).
func (s *Store) Allow(key string) bool {
	if s == nil || key == "" {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	b, ok := s.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(s.burst), lastTime: now}
		s.buckets[key] = b
	}
	elapsed := now.Sub(b.lastTime).Seconds()
	refill := elapsed * (float64(s.limit) / s.interval.Seconds())
	b.tokens += refill
	if b.tokens > float64(s.burst) {
		b.tokens = float64(s.burst)
	}
	b.lastTime = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// Cleanup removes stale buckets to prevent unbounded memory growth.
// Call this periodically (e.g., every 5 minutes) from a background goroutine.
func (s *Store) Cleanup(maxAge time.Duration) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for key, b := range s.buckets {
		if b.lastTime.Before(cutoff) {
			delete(s.buckets, key)
		}
	}
}
