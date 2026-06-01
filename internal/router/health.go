package router

import "sync"

type Health struct {
	Status string `json:"status"`
}

type HealthStore struct {
	mu        sync.RWMutex
	upstreams map[string]Health
}

func NewHealthStore() *HealthStore {
	return &HealthStore{upstreams: map[string]Health{}}
}

func (s *HealthStore) Set(id string, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upstreams[id] = Health{Status: status}
}

func (s *HealthStore) Snapshot() map[string]Health {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Health, len(s.upstreams))
	for id, health := range s.upstreams {
		out[id] = health
	}
	return out
}
