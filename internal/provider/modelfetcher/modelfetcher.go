// Package modelfetcher provides a unified "list models" call against an
// upstream provider. The OpenAI-compat surface (`GET {base}/v1/models`) is the
// common path; the Anthropic and Gemini paths use provider-specific endpoints.
//
// All HTTP calls are bounded by a context with a 5s timeout so a misbehaving
// provider cannot hang the panel.
package modelfetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Fetched is the normalized response returned to the panel regardless of
// upstream protocol. We do not expose every upstream field; the panel only
// needs id + display name.
type Fetched struct {
	Provider string `json:"provider"`
	Source   string `json:"source"` // "openai_compatible" | "anthropic" | "gemini"
	Models   []Item `json:"models"`
}

// Item is one model in the response.
type Item struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

// ErrAuthRequired is returned when the upstream says credentials are missing
// or invalid. The panel surfaces this as a hint to fix the API key, not as a
// generic failure.
var ErrAuthRequired = errors.New("upstream rejected the API key")

// Request is the input. BaseURL is required. APIKey is required for all
// providers except loopback local servers (Ollama, vLLM, LM Studio).
type Request struct {
	Provider string // preset id (anthropic, gemini, openai, openrouter, ollama, ...)
	BaseURL  string
	APIKey   string
	// Protocol is optional. If empty, we infer from Provider + BaseURL.
	Protocol string
}

// Fetch dispatches to the right fetcher. Returns ErrAuthRequired when the
// upstream says 401/403. Returns a wrapped error otherwise.
func Fetch(ctx context.Context, req Request) (*Fetched, error) {
	if strings.TrimSpace(req.BaseURL) == "" {
		return nil, errors.New("base_url is required")
	}
	proto := resolveProtocol(req)
	switch proto {
	case "anthropic":
		return fetchAnthropic(ctx, req)
	case "gemini":
		return fetchGemini(ctx, req)
	default:
		return fetchOpenAICompatible(ctx, req)
	}
}

func resolveProtocol(req Request) string {
	if p := strings.TrimSpace(req.Protocol); p != "" {
		return p
	}
	p := strings.ToLower(strings.TrimSpace(req.Provider))
	switch p {
	case "anthropic":
		return "anthropic"
	case "gemini":
		return "gemini"
	default:
		return "openai_compatible"
	}
}

// shared HTTP client
func newClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}

func doRequest(ctx context.Context, method string, fullURL string, headers map[string]string) ([]byte, int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(reqCtx, method, fullURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	resp, err := newClient().Do(r)
	if err != nil {
		return nil, 0, fmt.Errorf("call upstream: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	return body, resp.StatusCode, nil
}

func classifyStatus(status int) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return ErrAuthRequired
	}
	if status >= 400 {
		return fmt.Errorf("upstream returned HTTP %d", status)
	}
	return nil
}

// fetchOpenAICompatible calls GET {base}/models and parses the {data: [...]} shape.
func fetchOpenAICompatible(ctx context.Context, req Request) (*Fetched, error) {
	endpoint, err := joinURL(req.BaseURL, "models")
	if err != nil {
		return nil, err
	}
	headers := map[string]string{
		"Accept": "application/json",
	}
	if req.APIKey != "" {
		headers["Authorization"] = "Bearer " + req.APIKey
	}
	body, status, err := doRequest(ctx, http.MethodGet, endpoint, headers)
	if err != nil {
		return nil, err
	}
	if err := classifyStatus(status); err != nil {
		return nil, err
	}
	var raw struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
		Object string `json:"object"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse upstream response: %w", err)
	}
	out := &Fetched{
		Provider: req.Provider,
		Source:   "openai_compatible",
		Models:   make([]Item, 0, len(raw.Data)),
	}
	for _, m := range raw.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		label := id
		if m.OwnedBy != "" && !strings.EqualFold(m.OwnedBy, "system") {
			label = id + " (" + m.OwnedBy + ")"
		}
		out.Models = append(out.Models, Item{ID: id, Label: label})
	}
	return out, nil
}

// fetchAnthropic calls GET {base}/v1/models and parses the {data: [...]} shape.
func fetchAnthropic(ctx context.Context, req Request) (*Fetched, error) {
	base := strings.TrimRight(req.BaseURL, "/")
	endpoint := base + "/v1/models"
	headers := map[string]string{
		"Accept":     "application/json",
		"anthropic-version": "2023-06-01",
		"x-api-key":  req.APIKey,
	}
	body, status, err := doRequest(ctx, http.MethodGet, endpoint, headers)
	if err != nil {
		return nil, err
	}
	if err := classifyStatus(status); err != nil {
		return nil, err
	}
	var raw struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			Type        string `json:"type"`
		} `json:"data"`
		HasMore bool   `json:"has_more"`
		FirstID string `json:"first_id"`
		LastID  string `json:"last_id"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse upstream response: %w", err)
	}
	out := &Fetched{
		Provider: req.Provider,
		Source:   "anthropic",
		Models:   make([]Item, 0, len(raw.Data)),
	}
	for _, m := range raw.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		label := id
		if m.DisplayName != "" && m.DisplayName != id {
			label = m.DisplayName + " (" + id + ")"
		}
		out.Models = append(out.Models, Item{ID: id, Label: label})
	}
	return out, nil
}

// fetchGemini calls GET {base}/models and parses the {models: [...]} shape.
func fetchGemini(ctx context.Context, req Request) (*Fetched, error) {
	base := strings.TrimRight(req.BaseURL, "/")
	endpoint := base + "/models?pageSize=200"
	headers := map[string]string{
		"Accept":           "application/json",
		"x-goog-api-key":   req.APIKey,
	}
	body, status, err := doRequest(ctx, http.MethodGet, endpoint, headers)
	if err != nil {
		return nil, err
	}
	if err := classifyStatus(status); err != nil {
		return nil, err
	}
	var raw struct {
		Models []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse upstream response: %w", err)
	}
	out := &Fetched{
		Provider: req.Provider,
		Source:   "gemini",
		Models:   make([]Item, 0, len(raw.Models)),
	}
	for _, m := range raw.Models {
		id := strings.TrimSpace(m.Name)
		if id == "" {
			continue
		}
		// Gemini names start with "models/". Strip that prefix for the picker.
		id = strings.TrimPrefix(id, "models/")
		label := id
		if m.DisplayName != "" && m.DisplayName != id {
			label = m.DisplayName + " (" + id + ")"
		}
		out.Models = append(out.Models, Item{ID: id, Label: label})
	}
	return out, nil
}

func joinURL(base, suffix string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base_url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("base_url must be absolute")
	}
	// Strip a trailing /v1 (or /v1/) so the result is "{base}/models" not
	// "{base}/v1/models" twice.
	trimmed := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(trimmed, "/v1") {
		trimmed = strings.TrimSuffix(trimmed, "/v1")
	}
	u.Path = trimmed + "/" + strings.TrimLeft(suffix, "/")
	return u.String(), nil
}
