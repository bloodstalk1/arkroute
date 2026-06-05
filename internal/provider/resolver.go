package provider

import (
	"net/url"
	"path"
	"strings"
)

const (
	TypeAuto             = "auto"
	TypeOpenAICompatible = "openai_compatible"
	TypeAnthropic        = "anthropic"
	TypeGemini           = "gemini"
)

type ProviderRef struct {
	ID      string
	Name    string
	Type    string
	BaseURL string
}

type ModelRef struct {
	Protocol      string
	UpstreamModel string
}

type Catalog struct {
	Entries []CatalogEntry
}

type CatalogEntry struct {
	Name            string
	Match           Match
	DefaultProtocol string
	ModelRules      []ModelRule
}

type Match struct {
	IDContains      []string
	NameContains    []string
	BaseURLContains []string
	HostContains    []string
}

type ModelRule struct {
	Patterns []string
	Protocol string
}

type Resolver struct {
	Catalog Catalog
}

func NewResolver(catalog Catalog) Resolver {
	if len(catalog.Entries) == 0 {
		catalog = BuiltinCatalog()
	}
	return Resolver{Catalog: catalog}
}

func DefaultResolver() Resolver {
	return NewResolver(BuiltinCatalog())
}

func BuiltinCatalog() Catalog {
	return Catalog{Entries: []CatalogEntry{
		{
			Name: "opencode_go",
			Match: Match{
				IDContains:      []string{"opencode-go", "opencode_go"},
				NameContains:    []string{"opencode go", "opencode-go"},
				BaseURLContains: []string{"opencode.ai/zen/go"},
			},
			DefaultProtocol: TypeOpenAICompatible,
			ModelRules: []ModelRule{
				{
					Patterns: []string{"qwen3*", "minimax*", "mimax*"},
					Protocol: TypeAnthropic,
				},
			},
		},
		{
			Name:            "anthropic",
			Match:           Match{HostContains: []string{"api.anthropic.com"}, NameContains: []string{"anthropic"}},
			DefaultProtocol: TypeAnthropic,
		},
		{
			Name:            "gemini",
			Match:           Match{HostContains: []string{"generativelanguage.googleapis.com"}, NameContains: []string{"gemini"}},
			DefaultProtocol: TypeGemini,
		},
	}}
}

func (r Resolver) Resolve(provider ProviderRef, model ModelRef) string {
	if protocol := normalizeProtocol(model.Protocol); IsKnownProtocol(protocol) {
		return protocol
	}
	if protocol := normalizeProtocol(provider.Type); protocol != "" && protocol != TypeAuto {
		return protocol
	}
	if entry, ok := r.Catalog.match(provider); ok {
		if protocol := entry.matchModel(model.UpstreamModel); protocol != "" {
			return protocol
		}
		if IsKnownProtocol(entry.DefaultProtocol) {
			return entry.DefaultProtocol
		}
	}
	if protocol := inferFromEndpoint(provider); protocol != "" {
		return protocol
	}
	return TypeOpenAICompatible
}

func IsKnownProtocol(protocol string) bool {
	switch normalizeProtocol(protocol) {
	case TypeOpenAICompatible, TypeAnthropic, TypeGemini:
		return true
	default:
		return false
	}
}

func IsAutoProtocol(protocol string) bool {
	protocol = normalizeProtocol(protocol)
	return protocol == "" || protocol == TypeAuto
}

func normalizeProtocol(protocol string) string {
	return strings.ToLower(strings.TrimSpace(protocol))
}

func (c Catalog) match(provider ProviderRef) (CatalogEntry, bool) {
	for _, entry := range c.Entries {
		if entry.Match.matches(provider) {
			return entry, true
		}
	}
	return CatalogEntry{}, false
}

func (m Match) matches(provider ProviderRef) bool {
	if containsAny(provider.ID, m.IDContains) {
		return true
	}
	if containsAny(provider.Name, m.NameContains) {
		return true
	}
	if containsAny(provider.BaseURL, m.BaseURLContains) {
		return true
	}
	if len(m.HostContains) > 0 {
		if parsed, err := url.Parse(provider.BaseURL); err == nil && containsAny(parsed.Host, m.HostContains) {
			return true
		}
	}
	return false
}

func containsAny(value string, needles []string) bool {
	value = strings.ToLower(value)
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func (e CatalogEntry) matchModel(upstreamModel string) string {
	model := normalizeModelName(upstreamModel)
	for _, rule := range e.ModelRules {
		if !IsKnownProtocol(rule.Protocol) {
			continue
		}
		for _, pattern := range rule.Patterns {
			if ok, _ := path.Match(strings.ToLower(pattern), model); ok {
				return rule.Protocol
			}
		}
	}
	return ""
}

func normalizeModelName(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if slash := strings.LastIndex(model, "/"); slash >= 0 {
		model = model[slash+1:]
	}
	return model
}

func inferFromEndpoint(provider ProviderRef) string {
	identity := strings.ToLower(provider.ID + " " + provider.Name + " " + provider.BaseURL)
	if strings.Contains(identity, "openai-compatible") {
		return TypeOpenAICompatible
	}
	if strings.Contains(identity, "anthropic-compatible") {
		return TypeAnthropic
	}
	parsed, err := url.Parse(provider.BaseURL)
	if err != nil {
		return ""
	}
	path := strings.ToLower(strings.TrimRight(parsed.Path, "/"))
	switch {
	case strings.HasSuffix(path, "/v1/messages") || strings.HasSuffix(path, "/messages"):
		return TypeAnthropic
	case strings.HasSuffix(path, "/v1/chat/completions") || strings.HasSuffix(path, "/chat/completions"):
		return TypeOpenAICompatible
	default:
		return ""
	}
}
