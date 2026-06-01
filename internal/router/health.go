package router

import (
	"strings"
	"sync"
	"time"
)

type Health struct {
	Status           string        `json:"status"`
	UpstreamModel    string        `json:"upstream_model,omitempty"`
	LastStatusCode   int           `json:"last_status_code,omitempty"`
	LastErrorClass   string        `json:"last_error_class,omitempty"`
	LastErrorMessage string        `json:"last_error_message,omitempty"`
	LastLatency      time.Duration `json:"last_latency,omitempty"`
	LastUpdated      time.Time     `json:"last_updated,omitempty"`
}

type Update struct {
	ProviderID    string
	UpstreamModel string
	Status        string
	StatusCode    int
	ErrorClass    string
	ErrorMessage  string
	Latency       time.Duration
}

type HealthStore struct {
	mu        sync.RWMutex
	upstreams map[string]Health
}

func NewHealthStore() *HealthStore {
	return &HealthStore{upstreams: map[string]Health{}}
}

func (s *HealthStore) Set(id string, status string) {
	s.Update(Update{ProviderID: id, Status: status})
}

func (s *HealthStore) Update(update Update) {
	if update.ProviderID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	message := sanitizeMessage(update.ErrorMessage)
	s.upstreams[update.ProviderID] = Health{
		Status:           update.Status,
		UpstreamModel:    update.UpstreamModel,
		LastStatusCode:   update.StatusCode,
		LastErrorClass:   update.ErrorClass,
		LastErrorMessage: message,
		LastLatency:      update.Latency,
		LastUpdated:      time.Now().UTC(),
	}
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

func sanitizeMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 160 {
		return message[:160]
	}
	return message
}
