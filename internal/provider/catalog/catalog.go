// Package catalog exposes the static provider/model catalog bundled in models.json.
//
// The catalog is curated to cover the providers most users connect on first
// run. It is intentionally not exhaustive; users can always type a custom
// model name, and the live-fetch endpoint (/internal/setup/provider/fetch-models)
// pulls the authoritative list from each upstream API.
package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed models.json
var raw []byte

// Model is one model entry in the catalog.
type Model struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Context   int    `json:"context,omitempty"`
	MaxOutput int    `json:"max_output,omitempty"`
	Default   bool   `json:"default,omitempty"`
}

// Provider is the catalog entry for one provider preset.
type Provider struct {
	Name          string  `json:"name"`
	DefaultBaseURL string `json:"default_base_url"`
	DefaultEnv    string  `json:"default_env"`
	DefaultModel  string  `json:"default_model"`
	Protocol      string  `json:"protocol"`
	Models        []Model `json:"models"`
	Passthrough   bool    `json:"passthrough,omitempty"`
}

// Catalog is the root of models.json.
type Catalog struct {
	SchemaVersion int                `json:"schema_version"`
	Providers     map[string]Provider `json:"providers"`
}

// Load parses the embedded catalog. The file is small and static, so we
// decode once at init and cache the result.
func Load() (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("catalog: parse models.json: %w", err)
	}
	if c.SchemaVersion != 1 {
		return nil, fmt.Errorf("catalog: unsupported schema_version %d", c.SchemaVersion)
	}
	return &c, nil
}

// Get returns the catalog entry for a provider preset ID (lowercase, trimmed).
// Returns nil if the preset is not in the catalog.
func (c *Catalog) Get(presetID string) *Provider {
	presetID = strings.ToLower(strings.TrimSpace(presetID))
	if presetID == "" {
		return nil
	}
	p, ok := c.Providers[presetID]
	if !ok {
		return nil
	}
	return &p
}

// IDs returns all known provider preset IDs, sorted alphabetically.
func (c *Catalog) IDs() []string {
	ids := make([]string, 0, len(c.Providers))
	for id := range c.Providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
