package router

import (
	"strings"
	"sync"
	"time"
)

const (
	circuitFailThreshold = 3
	circuitOpenDuration  = 2 * time.Minute
)

// Health is the public, JSON-serialisable view of an upstream's
// current health. The router and the admin endpoints both consume it.
type Health struct {
	Status           string        `json:"status"`
	UpstreamModel    string        `json:"upstream_model,omitempty"`
	LastStatusCode   int           `json:"last_status_code,omitempty"`
	LastErrorClass   string        `json:"last_error_class,omitempty"`
	LastErrorMessage string        `json:"last_error_message,omitempty"`
	LastLatency      time.Duration `json:"last_latency,omitempty"`
	LastUpdated      time.Time     `json:"last_updated,omitempty"`
}

type circuitState struct {
	consecutiveFailures int
	openedAt            time.Time
}

// Update is the input to HealthStore.Update. Only ProviderID is
// required; the rest are surfaced verbatim into [Health].
type Update struct {
	ProviderID    string
	UpstreamModel string
	Status        string
	StatusCode    int
	ErrorClass    string
	ErrorMessage  string
	Latency       time.Duration
}

// HealthStore tracks per-upstream health and a per-(provider, model)
// circuit breaker. All methods are safe for concurrent use.
type HealthStore struct {
	mu        sync.RWMutex
	upstreams map[string]Health
	circuits  map[string]*circuitState
}

// NewHealthStore returns an empty store ready for [HealthStore.Update]
// calls.
func NewHealthStore() *HealthStore {
	return &HealthStore{
		upstreams: map[string]Health{},
		circuits:  map[string]*circuitState{},
	}
}

// Set records a minimal status update for a provider. Equivalent to
// Update with only ProviderID and Status set.
func (s *HealthStore) Set(id string, status string) {
	s.Update(Update{ProviderID: id, Status: status})
}

// Update records a fresh observation for a provider+model pair. The
// provider's circuit-breaker counter is reset on status=="ok" and
// incremented otherwise; once the threshold is reached the circuit
// stays open for circuitOpenDuration.
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
	// Track circuit breaker state.
	circuitKey := update.ProviderID + "::" + update.UpstreamModel
	cs := s.circuits[circuitKey]
	if cs == nil {
		cs = &circuitState{}
		s.circuits[circuitKey] = cs
	}
	if update.Status == "ok" {
		cs.consecutiveFailures = 0
		cs.openedAt = time.Time{}
	} else {
		cs.consecutiveFailures++
		if cs.consecutiveFailures >= circuitFailThreshold {
			cs.openedAt = time.Now()
		}
	}
}

// IsCircuited returns true when consecutive failures on the provider+model
// have exceeded the threshold and the cooldown period has not expired.
// A circuit stays open for circuitOpenDuration, then allows one probe.
func (s *HealthStore) IsCircuited(providerID, upstreamModel string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	circuitKey := providerID + "::" + upstreamModel
	cs := s.circuits[circuitKey]
	if cs == nil {
		return false
	}
	if cs.consecutiveFailures < circuitFailThreshold {
		return false
	}
	if time.Since(cs.openedAt) > circuitOpenDuration {
		return false
	}
	return true
}

// Snapshot returns a copy of the per-provider health map. Callers can
// mutate the returned map without affecting the store.
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

