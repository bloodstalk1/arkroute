# Arkroute Panel UX Refresh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the Arkroute panel into a setup-friendly provider dashboard with Add/Edit drawer flows, safer provider mutation, and a quiet control-plane visual system.

**Architecture:** Make backend provider setup safe first, because the current setup path replaces the entire provider/model/route config. Then add frontend helper functions with unit tests so the large React panel can use predictable form state, validation, and payload shaping. Finally, replace the always-visible setup form with dashboard/drawer layouts and refresh the remaining tabs through shared CSS tokens and components.

**Tech Stack:** Go `net/http` and table-driven tests, Arkroute YAML config structs, React 19, Vite, vanilla CSS, Node `node:test`, existing Phosphor icon web package.

---

## File Structure

- Modify: `internal/setup/planner.go`
  - Convert provider setup from whole-config replacement to provider/model/route upsert.
  - Add provider removal helpers used by panel endpoints.
- Modify: `internal/setup/planner_test.go`
  - Cover preserving unrelated providers, editing a provider without duplicate route targets, and removing provider-owned models/route targets.
- Modify: `internal/panel/server.go`
  - Extend `/internal/setup/provider` to accept `DELETE ?id=<provider_id>`.
  - Keep `POST /internal/setup/provider` compatible with the existing frontend payload.
- Modify: `internal/panel/server_test.go`
  - Cover multi-provider POST behavior, DELETE auth/method behavior, redaction, and reload callback.
- Create: `web-ui/src/providerSetup.js`
  - Pure helpers for provider form defaults, preset fill, edit fill, validation, key summaries, and save payloads.
- Create: `web-ui/src/providerSetup.test.js`
  - Node tests for add mode, edit mode, validation, and secret-safe summaries.
- Modify: `web-ui/src/App.jsx`
  - Replace the always-visible Providers setup form with Providers dashboard and Add/Edit drawer.
  - Reuse existing fetch/status/CLI/policy data loading.
  - Remove layout/color inline styles that belong in CSS.
- Modify: `web-ui/src/index.css`
  - Define the refreshed visual tokens and component system.
  - Add drawer/sheet, dashboard, provider row, readiness console, route topology, trace terminal, and responsive styles.
  - Define `--border` and `--surface-soft` or replace their uses with existing tokens.
- Build output: `internal/panel/assets/*`
  - Regenerated only after frontend implementation passes `npm run build --prefix web-ui`.

---

## Task 1: Safe Provider Setup Mutation

**Files:**
- Modify: `internal/setup/planner.go`
- Modify: `internal/setup/planner_test.go`

- [ ] **Step 1: Add failing tests for preserving existing config**

Append these tests to `internal/setup/planner_test.go`:

```go
func TestApplyProviderSetupPreservesExistingProvidersAndAddsRouteTarget(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	cfg, err := ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "anthropic",
		APIKeyMode:    APIKeyModeEnv,
		EnvName:       "ANTHROPIC_API_KEY",
		UpstreamModel: "claude-sonnet-4-5",
		ExposedAlias:  "sonnet-anthropic",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("first ApplyProviderSetup() error = %v", err)
	}

	cfg, err = ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "openrouter",
		APIKeyMode:    APIKeyModeEnv,
		EnvName:       "OPENROUTER_API_KEY",
		UpstreamModel: "anthropic/claude-sonnet-4.5",
		ExposedAlias:  "sonnet-or",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("second ApplyProviderSetup() error = %v", err)
	}

	if got, want := len(cfg.Providers), 2; got != want {
		t.Fatalf("providers = %d, want %d: %+v", got, want, cfg.Providers)
	}
	if got, want := len(cfg.Models), 2; got != want {
		t.Fatalf("models = %d, want %d: %+v", got, want, cfg.Models)
	}
	if got, want := len(cfg.Routes), 1; got != want {
		t.Fatalf("routes = %d, want %d: %+v", got, want, cfg.Routes)
	}
	targets := cfg.Routes[0].Targets
	if got, want := len(targets), 2; got != want {
		t.Fatalf("route targets = %d, want %d: %+v", got, want, targets)
	}
	if targets[0].ModelID != "anthropic-sonnet-anthropic" || targets[1].ModelID != "openrouter-sonnet-or" {
		t.Fatalf("route target order = %+v, want anthropic then openrouter", targets)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestApplyProviderSetupUpdatesExistingProviderWithoutDuplicateTarget(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	cfg, err := ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "openrouter",
		APIKeyMode:    APIKeyModeEnv,
		EnvName:       "OPENROUTER_API_KEY",
		UpstreamModel: "anthropic/claude-sonnet-4.5",
		ExposedAlias:  "sonnet-or",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("first ApplyProviderSetup() error = %v", err)
	}

	cfg, err = ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "openrouter",
		APIKeyMode:    APIKeyModeEnv,
		EnvName:       "OPENROUTER_ALT_KEY",
		UpstreamModel: "openai/gpt-4o",
		ExposedAlias:  "openrouter-gpt4o",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("second ApplyProviderSetup() error = %v", err)
	}

	if got, want := len(cfg.Providers), 1; got != want {
		t.Fatalf("providers = %d, want %d: %+v", got, want, cfg.Providers)
	}
	if cfg.Providers[0].APIKey != "env:OPENROUTER_ALT_KEY" {
		t.Fatalf("provider key = %q, want updated env reference", cfg.Providers[0].APIKey)
	}
	if got, want := len(cfg.Models), 1; got != want {
		t.Fatalf("models = %d, want %d: %+v", got, want, cfg.Models)
	}
	if cfg.Models[0].ID != "openrouter-openrouter-gpt4o" || cfg.Models[0].UpstreamModel != "openai/gpt-4o" {
		t.Fatalf("model = %+v, want edited OpenRouter model", cfg.Models[0])
	}
	if got, want := len(cfg.Routes[0].Targets), 1; got != want {
		t.Fatalf("route targets = %d, want %d: %+v", got, want, cfg.Routes[0].Targets)
	}
	if cfg.Routes[0].Targets[0].ModelID != "openrouter-openrouter-gpt4o" {
		t.Fatalf("target = %+v, want edited model target", cfg.Routes[0].Targets[0])
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRemoveProviderSetupRemovesOwnedModelsAndRouteTargets(t *testing.T) {
	cfg := config.BootstrapLocalConfig("local-key")
	var err error
	cfg, err = ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "anthropic",
		APIKeyMode:    APIKeyModeEnv,
		EnvName:       "ANTHROPIC_API_KEY",
		UpstreamModel: "claude-sonnet-4-5",
		ExposedAlias:  "sonnet-anthropic",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("anthropic ApplyProviderSetup() error = %v", err)
	}
	cfg, err = ApplyProviderSetup(cfg, ProviderSetup{
		PresetID:      "openrouter",
		APIKeyMode:    APIKeyModeEnv,
		EnvName:       "OPENROUTER_API_KEY",
		UpstreamModel: "anthropic/claude-sonnet-4.5",
		ExposedAlias:  "sonnet-or",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatalf("openrouter ApplyProviderSetup() error = %v", err)
	}

	cfg, err = RemoveProviderSetup(cfg, "openrouter")
	if err != nil {
		t.Fatalf("RemoveProviderSetup() error = %v", err)
	}

	if got, want := len(cfg.Providers), 1; got != want {
		t.Fatalf("providers = %d, want %d: %+v", got, want, cfg.Providers)
	}
	if cfg.Providers[0].ID != "anthropic" {
		t.Fatalf("remaining provider = %+v, want anthropic", cfg.Providers[0])
	}
	if got, want := len(cfg.Models), 1; got != want {
		t.Fatalf("models = %d, want %d: %+v", got, want, cfg.Models)
	}
	if cfg.Models[0].ProviderID != "anthropic" {
		t.Fatalf("remaining model = %+v, want anthropic-owned model", cfg.Models[0])
	}
	if got, want := len(cfg.Routes), 1; got != want {
		t.Fatalf("routes = %d, want %d: %+v", got, want, cfg.Routes)
	}
	if got, want := len(cfg.Routes[0].Targets), 1; got != want {
		t.Fatalf("targets = %d, want %d: %+v", got, want, cfg.Routes[0].Targets)
	}
	if cfg.Routes[0].Targets[0].ModelID != "anthropic-sonnet-anthropic" {
		t.Fatalf("target = %+v, want anthropic target", cfg.Routes[0].Targets[0])
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRemoveProviderSetupRejectsMissingProvider(t *testing.T) {
	_, err := RemoveProviderSetup(config.BootstrapLocalConfig("local-key"), "missing")
	if err == nil || !strings.Contains(err.Error(), "provider not found") {
		t.Fatalf("error = %v, want provider not found", err)
	}
}
```

- [ ] **Step 2: Run setup tests and verify the new tests fail**

Run:

```bash
go test -count=1 ./internal/setup
```

Expected: failure because `RemoveProviderSetup` is undefined and current `ApplyProviderSetup` replaces the config.

- [ ] **Step 3: Implement provider upsert and removal**

Modify `internal/setup/planner.go`:

```go
func ApplyProviderSetup(cfg config.Config, input ProviderSetup) (config.Config, error) {
	preset, ok := findPreset(input.PresetID)
	if !ok {
		return config.Config{}, fmt.Errorf("unknown preset %q", input.PresetID)
	}
	providerID := preset.ID
	providerName := firstNonEmpty(input.ProviderName, preset.Name)
	baseURL := firstNonEmpty(input.BaseURL, preset.BaseURL)
	providerType := firstNonEmpty(input.Type, preset.Type)
	upstreamModel := firstNonEmpty(input.UpstreamModel, preset.DefaultModel)
	exposedAlias := firstNonEmpty(input.ExposedAlias, preset.DefaultAlias)
	routeAlias := firstNonEmpty(input.RouteAlias, preset.DefaultRoute)
	envName := firstNonEmpty(input.EnvName, EnvNameForProvider(providerID))
	apiKey := providerAPIKey(input.APIKeyMode, input.APIKey, envName)

	modelID := providerID + "-" + normalizeID(exposedAlias)
	if providerID == "opencode-zen" {
		modelID = normalizeID(exposedAlias)
	}

	oldModelIDs := modelIDsForProvider(cfg.Models, providerID)
	cfg.Providers = upsertProvider(cfg.Providers, config.ProviderConfig{
		ID: providerID, Name: providerName, Type: providerType, BaseURL: baseURL,
		APIKey: apiKey, Headers: cloneStringMap(preset.Headers), Enabled: true,
	})
	cfg.Models = removeModelsForProvider(cfg.Models, providerID)
	cfg.Routes = removeRouteTargets(cfg.Routes, oldModelIDs)
	cfg.Models = append(cfg.Models, config.ModelConfig{
		ID: modelID, ProviderID: providerID, UpstreamModel: upstreamModel, ExposedAlias: exposedAlias,
		ClaudeDiscoveryAlias: preset.DiscoveryAlias, DisplayName: providerName + " " + upstreamModel,
		Capabilities: preset.Capabilities, Enabled: true,
	})
	cfg.Routes = upsertRouteTarget(cfg.Routes, config.RouteConfig{
		Alias: routeAlias, ClaudeDiscoveryAlias: preset.DiscoveryAlias, Strategy: "fallback",
		Targets: []config.RouteTarget{{ModelID: modelID, Enabled: true}}, Enabled: true,
	})
	ensureProfiles(&cfg, routeAlias)
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func RemoveProviderSetup(cfg config.Config, providerID string) (config.Config, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return config.Config{}, fmt.Errorf("provider id is required")
	}
	if !hasProvider(cfg.Providers, providerID) {
		return config.Config{}, fmt.Errorf("provider not found: %s", providerID)
	}
	modelIDs := modelIDsForProvider(cfg.Models, providerID)
	cfg.Providers = removeProviderByID(cfg.Providers, providerID)
	cfg.Models = removeModelsForProvider(cfg.Models, providerID)
	cfg.Routes = removeRouteTargets(cfg.Routes, modelIDs)
	cfg.Routes = removeEmptyRoutes(cfg.Routes)
	pruneProfiles(&cfg)
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}
```

Add helper functions below `ApplyProviderSetup` in the same file:

```go
func upsertProvider(providers []config.ProviderConfig, provider config.ProviderConfig) []config.ProviderConfig {
	out := make([]config.ProviderConfig, 0, len(providers)+1)
	replaced := false
	for _, existing := range providers {
		if existing.ID == provider.ID {
			out = append(out, provider)
			replaced = true
			continue
		}
		out = append(out, existing)
	}
	if !replaced {
		out = append(out, provider)
	}
	return out
}

func hasProvider(providers []config.ProviderConfig, providerID string) bool {
	for _, provider := range providers {
		if provider.ID == providerID {
			return true
		}
	}
	return false
}

func removeProviderByID(providers []config.ProviderConfig, providerID string) []config.ProviderConfig {
	out := make([]config.ProviderConfig, 0, len(providers))
	for _, provider := range providers {
		if provider.ID != providerID {
			out = append(out, provider)
		}
	}
	return out
}

func modelIDsForProvider(models []config.ModelConfig, providerID string) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, model := range models {
		if model.ProviderID == providerID {
			ids[model.ID] = struct{}{}
		}
	}
	return ids
}

func removeModelsForProvider(models []config.ModelConfig, providerID string) []config.ModelConfig {
	out := make([]config.ModelConfig, 0, len(models))
	for _, model := range models {
		if model.ProviderID != providerID {
			out = append(out, model)
		}
	}
	return out
}

func removeRouteTargets(routes []config.RouteConfig, removed map[string]struct{}) []config.RouteConfig {
	if len(removed) == 0 {
		return routes
	}
	out := make([]config.RouteConfig, 0, len(routes))
	for _, route := range routes {
		targets := make([]config.RouteTarget, 0, len(route.Targets))
		for _, target := range route.Targets {
			if _, drop := removed[target.ModelID]; !drop {
				targets = append(targets, target)
			}
		}
		route.Targets = targets
		out = append(out, route)
	}
	return out
}

func upsertRouteTarget(routes []config.RouteConfig, route config.RouteConfig) []config.RouteConfig {
	modelID := route.Targets[0].ModelID
	out := make([]config.RouteConfig, 0, len(routes)+1)
	replacedRoute := false
	for _, existing := range routes {
		if existing.Alias != route.Alias {
			out = append(out, existing)
			continue
		}
		existing.ClaudeDiscoveryAlias = firstNonEmpty(existing.ClaudeDiscoveryAlias, route.ClaudeDiscoveryAlias)
		existing.Strategy = firstNonEmpty(existing.Strategy, route.Strategy)
		existing.Enabled = true
		targets := make([]config.RouteTarget, 0, len(existing.Targets)+1)
		replacedTarget := false
		for _, target := range existing.Targets {
			if target.ModelID == modelID {
				targets = append(targets, config.RouteTarget{ModelID: modelID, Enabled: true})
				replacedTarget = true
				continue
			}
			targets = append(targets, target)
		}
		if !replacedTarget {
			targets = append(targets, config.RouteTarget{ModelID: modelID, Enabled: true})
		}
		existing.Targets = targets
		out = append(out, existing)
		replacedRoute = true
	}
	if !replacedRoute {
		out = append(out, route)
	}
	return out
}

func removeEmptyRoutes(routes []config.RouteConfig) []config.RouteConfig {
	out := make([]config.RouteConfig, 0, len(routes))
	for _, route := range routes {
		if len(route.Targets) > 0 {
			out = append(out, route)
		}
	}
	return out
}

func ensureProfiles(cfg *config.Config, routeAlias string) {
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]string{}
	}
	if strings.TrimSpace(cfg.Profiles["default"]) == "" {
		cfg.Profiles["default"] = routeAlias
	}
	if strings.TrimSpace(cfg.Profiles["best"]) == "" {
		cfg.Profiles["best"] = routeAlias
	}
}

func pruneProfiles(cfg *config.Config) {
	if len(cfg.Profiles) == 0 {
		return
	}
	valid := map[string]struct{}{}
	for _, route := range cfg.Routes {
		valid[route.Alias] = struct{}{}
	}
	for _, model := range cfg.Models {
		valid[model.ExposedAlias] = struct{}{}
	}
	for name, alias := range cfg.Profiles {
		if _, ok := valid[alias]; !ok {
			delete(cfg.Profiles, name)
		}
	}
	if len(cfg.Profiles) == 0 && len(cfg.Routes) > 0 {
		cfg.Profiles["default"] = cfg.Routes[0].Alias
		cfg.Profiles["best"] = cfg.Routes[0].Alias
	}
}
```

- [ ] **Step 4: Run setup tests and verify they pass**

Run:

```bash
go test -count=1 ./internal/setup
```

Expected: PASS.

- [ ] **Step 5: Commit backend setup mutation**

```bash
git add internal/setup/planner.go internal/setup/planner_test.go
git commit -m "feat: make provider setup editable"
```

---

## Task 2: Panel Provider Delete Endpoint

**Files:**
- Modify: `internal/panel/server.go`
- Modify: `internal/panel/server_test.go`

- [ ] **Step 1: Add failing panel tests for multi-provider save and delete**

Append these tests to `internal/panel/server_test.go`:

```go
func TestSetupProviderPreservesExistingProviders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{Sessions: store, ConfigPath: path})

	first := strings.NewReader(`{"preset_id":"anthropic","api_key_mode":"env","env_name":"ANTHROPIC_API_KEY","upstream_model":"claude-sonnet-4-5","exposed_alias":"sonnet-anthropic","route_alias":"sonnet"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/setup/provider", first)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", rec.Code, rec.Body.String())
	}

	second := strings.NewReader(`{"preset_id":"openrouter","api_key_mode":"env","env_name":"OPENROUTER_API_KEY","upstream_model":"anthropic/claude-sonnet-4.5","exposed_alias":"sonnet-or","route_alias":"sonnet"}`)
	req = httptest.NewRequest(http.MethodPost, "/internal/setup/provider", second)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("second status = %d, body = %s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(cfg.Providers), 2; got != want {
		t.Fatalf("providers = %d, want %d: %+v", got, want, cfg.Providers)
	}
	if got, want := len(cfg.Routes[0].Targets), 2; got != want {
		t.Fatalf("route targets = %d, want %d: %+v", got, want, cfg.Routes[0].Targets)
	}
	if strings.Contains(rec.Body.String(), "OPENROUTER_API_KEY") {
		t.Fatalf("response leaked env name through redacted config: %s", rec.Body.String())
	}
}

func TestSetupProviderDeleteRemovesProviderAndReloads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.BootstrapLocalConfig("local-key")
	var err error
	cfg, err = setupcore.ApplyProviderSetup(cfg, setupcore.ProviderSetup{
		PresetID:      "anthropic",
		APIKeyMode:    setupcore.APIKeyModeEnv,
		EnvName:       "ANTHROPIC_API_KEY",
		UpstreamModel: "claude-sonnet-4-5",
		ExposedAlias:  "sonnet-anthropic",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = setupcore.ApplyProviderSetup(cfg, setupcore.ProviderSetup{
		PresetID:      "openrouter",
		APIKeyMode:    setupcore.APIKeyModeEnv,
		EnvName:       "OPENROUTER_API_KEY",
		UpstreamModel: "anthropic/claude-sonnet-4.5",
		ExposedAlias:  "sonnet-or",
		RouteAlias:    "sonnet",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := savePanelConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	reloads := 0
	store := NewSessionStore(time.Minute)
	token := store.Issue()
	handler := Routes(Deps{
		Sessions:   store,
		ConfigPath: path,
		OnSave: func() error {
			reloads++
			return nil
		},
	})

	req := httptest.NewRequest(http.MethodDelete, "/internal/setup/provider?id=openrouter", nil)
	req.Header.Set("X-Arkroute-Setup-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want 1", reloads)
	}
	current, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(current.Providers), 1; got != want {
		t.Fatalf("providers = %d, want %d: %+v", got, want, current.Providers)
	}
	if current.Providers[0].ID != "anthropic" {
		t.Fatalf("remaining provider = %+v, want anthropic", current.Providers[0])
	}
}
```

Add this import to `internal/panel/server_test.go`:

```go
setupcore "github.com/bloodstalk1/arkroute/internal/setup"
```

- [ ] **Step 2: Run panel tests and verify the new tests fail**

Run:

```bash
go test -count=1 ./internal/panel
```

Expected: failure because `DELETE /internal/setup/provider` is not handled.

- [ ] **Step 3: Add DELETE handling to the provider endpoint**

Replace `handleProvider` in `internal/panel/server.go` with method dispatch:

```go
func handleProvider(path string, claudeWriter func(config.Config) error, onSave func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handleProviderSave(path, claudeWriter, onSave)(w, r)
		case http.MethodDelete:
			handleProviderDelete(path, onSave)(w, r)
		default:
			w.Header().Set("Allow", strings.Join([]string{http.MethodPost, http.MethodDelete}, ", "))
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"schema_version": 1, "error": "method not allowed"})
		}
	}
}
```

Move the current POST body into a new helper:

```go
func handleProviderSave(path string, claudeWriter func(config.Config) error, onSave func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input setupcore.ProviderSetup
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": "invalid setup payload"})
			return
		}
		cfg, err := loadOrBootstrapConfig(path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		cfg, err = setupcore.ApplyProviderSetup(cfg, input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		result, err := NewConfigStore(path).SaveAndReload(cfg, onSave)
		if err != nil {
			writeJSON(w, httpStatusForSaveError(err), map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		claudeActivated := false
		var claudeErr string
		if input.ActivateClaude && claudeWriter != nil {
			if err := claudeWriter(cfg); err == nil {
				claudeActivated = true
			} else {
				claudeErr = err.Error()
			}
		}
		response := map[string]any{
			"schema_version":   1,
			"status":           "saved",
			"backup_path":      result.BackupPath,
			"claude_activated": claudeActivated,
			"config":           config.Redacted(cfg),
		}
		if claudeErr != "" {
			response["claude_error"] = claudeErr
		}
		writeJSON(w, http.StatusOK, response)
	}
}
```

Add the delete helper:

```go
func handleProviderDelete(path string, onSave func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providerID := strings.TrimSpace(r.URL.Query().Get("id"))
		if providerID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"schema_version": 1, "error": "provider id is required"})
			return
		}
		store := NewConfigStore(path)
		cfg, err := store.LoadOrBootstrap()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		cfg, err = setupcore.RemoveProviderSetup(cfg, providerID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		result, err := store.SaveAndReload(cfg, onSave)
		if err != nil {
			writeJSON(w, httpStatusForSaveError(err), map[string]any{"schema_version": 1, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schema_version": 1,
			"status":         "deleted",
			"backup_path":    result.BackupPath,
			"config":         config.Redacted(cfg),
		})
	}
}
```

- [ ] **Step 4: Run panel tests and verify they pass**

Run:

```bash
go test -count=1 ./internal/panel
```

Expected: PASS.

- [ ] **Step 5: Commit panel provider delete endpoint**

```bash
git add internal/panel/server.go internal/panel/server_test.go
git commit -m "feat: add provider edit delete endpoint"
```

---

## Task 3: Provider Setup Form Helpers

**Files:**
- Create: `web-ui/src/providerSetup.js`
- Create: `web-ui/src/providerSetup.test.js`
- Modify: `web-ui/src/App.jsx`

- [ ] **Step 1: Write failing helper tests**

Create `web-ui/src/providerSetup.test.js`:

```js
import assert from "node:assert/strict";
import test from "node:test";

import {
  buildProviderSetupPayload,
  formFromPreset,
  formFromProvider,
  initialProviderForm,
  providerKeySummary,
  validateProviderForm,
} from "./providerSetup.js";

const presets = [
  {
    id: "openrouter",
    name: "OpenRouter",
    type: "openai_compatible",
    base_url: "https://openrouter.ai/api/v1",
    default_model: "anthropic/claude-sonnet-4.5",
    default_alias: "sonnet-or",
    default_route: "sonnet",
  },
  {
    id: "custom",
    name: "Custom",
    type: "auto",
    base_url: "https://example.com/v1",
    default_model: "provider/model",
    default_alias: "custom-model",
    default_route: "sonnet",
  },
];

test("initialProviderForm creates add-mode defaults", () => {
  assert.deepEqual(initialProviderForm(), {
    mode: "add",
    preset_id: "",
    provider_name: "",
    base_url: "",
    type: "",
    api_key_mode: "env",
    api_key: "",
    env_name: "",
    upstream_model: "",
    exposed_alias: "",
    route_alias: "",
    activate_claude: true,
  });
});

test("formFromPreset fills the happy path fields", () => {
  assert.deepEqual(formFromPreset(presets[0]), {
    mode: "add",
    preset_id: "openrouter",
    provider_name: "OpenRouter",
    base_url: "https://openrouter.ai/api/v1",
    type: "openai_compatible",
    api_key_mode: "env",
    api_key: "",
    env_name: "OPENROUTER_API_KEY",
    upstream_model: "anthropic/claude-sonnet-4.5",
    exposed_alias: "sonnet-or",
    route_alias: "sonnet",
    activate_claude: true,
  });
});

test("formFromProvider reconstructs edit mode without leaking config secrets", () => {
  const provider = {
    id: "openrouter",
    name: "OpenRouter",
    type: "openai_compatible",
    base_url: "https://openrouter.ai/api/v1",
    api_key: "sk-secret",
  };
  const models = [
    {
      id: "openrouter-sonnet-or",
      provider_id: "openrouter",
      upstream_model: "anthropic/claude-sonnet-4.5",
      exposed_alias: "sonnet-or",
    },
  ];
  const routes = [{ alias: "sonnet", targets: [{ model_id: "openrouter-sonnet-or", enabled: true }] }];

  const form = formFromProvider(provider, models, routes, presets);
  assert.equal(form.mode, "edit");
  assert.equal(form.preset_id, "openrouter");
  assert.equal(form.api_key_mode, "config");
  assert.equal(form.api_key, "");
  assert.equal(form.env_name, "OPENROUTER_API_KEY");
  assert.equal(form.upstream_model, "anthropic/claude-sonnet-4.5");
  assert.equal(form.exposed_alias, "sonnet-or");
  assert.equal(form.route_alias, "sonnet");
});

test("validateProviderForm reports actionable field errors", () => {
  assert.deepEqual(validateProviderForm(initialProviderForm()), {
    preset_id: "Choose a provider preset.",
    base_url: "Enter a provider base URL.",
    env_name: "Enter the environment variable name.",
    upstream_model: "Choose or enter an upstream model.",
    exposed_alias: "Enter the model name shown to clients.",
    route_alias: "Choose a route alias.",
  });
});

test("buildProviderSetupPayload keeps edit payload compatible with setup endpoint", () => {
  const form = {
    ...formFromPreset(presets[0]),
    mode: "edit",
    api_key_mode: "config",
    api_key: "sk-updated",
  };
  assert.deepEqual(buildProviderSetupPayload(form), {
    preset_id: "openrouter",
    provider_name: "OpenRouter",
    base_url: "https://openrouter.ai/api/v1",
    type: "openai_compatible",
    api_key_mode: "config",
    api_key: "sk-updated",
    env_name: "",
    upstream_model: "anthropic/claude-sonnet-4.5",
    exposed_alias: "sonnet-or",
    route_alias: "sonnet",
    activate_claude: true,
  });
});

test("providerKeySummary never returns raw config keys", () => {
  assert.equal(providerKeySummary({ api_key: "env:OPENROUTER_API_KEY" }), "env:OPENROUTER_API_KEY");
  assert.equal(providerKeySummary({ api_key: "sk-secret" }), "stored in config");
  assert.equal(providerKeySummary({ api_key: "" }), "not configured");
});
```

- [ ] **Step 2: Run helper tests and verify they fail**

Run:

```bash
node --test web-ui/src/providerSetup.test.js
```

Expected: failure because `web-ui/src/providerSetup.js` does not exist.

- [ ] **Step 3: Implement provider setup helpers**

Create `web-ui/src/providerSetup.js`:

```js
import { envNameForProvider } from "./modelFetch.js";

export function initialProviderForm() {
  return {
    mode: "add",
    preset_id: "",
    provider_name: "",
    base_url: "",
    type: "",
    api_key_mode: "env",
    api_key: "",
    env_name: "",
    upstream_model: "",
    exposed_alias: "",
    route_alias: "",
    activate_claude: true,
  };
}

export function formFromPreset(preset, previous = initialProviderForm()) {
  if (!preset) {
    return { ...previous, preset_id: "" };
  }
  return {
    ...previous,
    mode: previous.mode || "add",
    preset_id: preset.id || "",
    provider_name: preset.name || "",
    base_url: preset.base_url || "",
    type: preset.type || "",
    upstream_model: preset.default_model || "",
    exposed_alias: preset.default_alias || "",
    route_alias: preset.default_route || "",
    env_name: preset.id ? envNameForProvider(preset.id) : "",
  };
}

export function formFromProvider(provider, models = [], routes = [], presets = []) {
  const preset = presets.find((item) => item.id === provider?.id);
  const providerModel = models.find((model) => model.provider_id === provider?.id);
  const route = routes.find((item) =>
    (item.targets || []).some((target) => target.model_id === providerModel?.id),
  );
  const apiKey = provider?.api_key || "";
  const envMode = apiKey.startsWith("env:");
  return {
    ...initialProviderForm(),
    mode: "edit",
    preset_id: preset?.id || provider?.id || "",
    provider_name: provider?.name || preset?.name || provider?.id || "",
    base_url: provider?.base_url || preset?.base_url || "",
    type: provider?.type || preset?.type || "",
    api_key_mode: envMode ? "env" : "config",
    api_key: "",
    env_name: envMode ? apiKey.slice(4) : envNameForProvider(provider?.id || preset?.id || ""),
    upstream_model: providerModel?.upstream_model || preset?.default_model || "",
    exposed_alias: providerModel?.exposed_alias || preset?.default_alias || "",
    route_alias: route?.alias || preset?.default_route || "sonnet",
    activate_claude: true,
  };
}

export function validateProviderForm(form) {
  const errors = {};
  if (!form.preset_id?.trim()) errors.preset_id = "Choose a provider preset.";
  if (!form.base_url?.trim()) errors.base_url = "Enter a provider base URL.";
  if (form.api_key_mode === "env" && !form.env_name?.trim()) {
    errors.env_name = "Enter the environment variable name.";
  }
  if (form.api_key_mode === "config" && form.mode === "add" && !form.api_key?.trim()) {
    errors.api_key = "Enter an API key or use an environment variable.";
  }
  if (!form.upstream_model?.trim()) errors.upstream_model = "Choose or enter an upstream model.";
  if (!form.exposed_alias?.trim()) errors.exposed_alias = "Enter the model name shown to clients.";
  if (!form.route_alias?.trim()) errors.route_alias = "Choose a route alias.";
  return errors;
}

export function buildProviderSetupPayload(form) {
  return {
    preset_id: form.preset_id,
    provider_name: form.provider_name,
    base_url: form.base_url,
    type: form.type,
    api_key_mode: form.api_key_mode,
    api_key: form.api_key_mode === "config" ? form.api_key : "",
    env_name: form.api_key_mode === "env" ? form.env_name : "",
    upstream_model: form.upstream_model,
    exposed_alias: form.exposed_alias,
    route_alias: form.route_alias,
    activate_claude: form.activate_claude,
  };
}

export function providerKeySummary(provider) {
  const apiKey = provider?.api_key || "";
  if (!apiKey) return "not configured";
  if (apiKey.startsWith("env:")) return `env:${apiKey.slice(4)}`;
  return "stored in config";
}
```

- [ ] **Step 4: Run helper tests and existing model fetch tests**

Run:

```bash
node --test web-ui/src/modelFetch.test.js web-ui/src/providerSetup.test.js
```

Expected: PASS.

- [ ] **Step 5: Commit provider setup helpers**

```bash
git add web-ui/src/providerSetup.js web-ui/src/providerSetup.test.js
git commit -m "test: add provider setup form helpers"
```

---

## Task 4: Providers Dashboard And Add/Edit Drawer

**Files:**
- Modify: `web-ui/src/App.jsx`
- Modify: `web-ui/src/index.css`

- [ ] **Step 1: Import provider setup helpers**

Modify the imports at the top of `web-ui/src/App.jsx`:

```js
import {
  buildProviderSetupPayload,
  formFromPreset,
  formFromProvider,
  initialProviderForm,
  providerKeySummary,
  validateProviderForm,
} from "./providerSetup.js";
```

- [ ] **Step 2: Replace provider key display with helper**

In `ProviderCard`, replace local key-summary logic:

```jsx
function ProviderCard({ provider }) {
  return (
    <article className="operator-card span-2">
      <div className="card-heading">
        <div>
          <StatusBadge>Enabled</StatusBadge>
          <h3><i className="ph-light ph-puzzle-piece"></i>{provider.name || provider.id}</h3>
        </div>
        <code>{provider.id}</code>
      </div>
      <div className="data-grid">
        <DataRow label="Protocol">{provider.type || "auto"}</DataRow>
        <DataRow label="Base URL">{provider.base_url}</DataRow>
        <DataRow label="Key">{providerKeySummary(provider)}</DataRow>
      </div>
    </article>
  );
}
```

- [ ] **Step 3: Add drawer state in `App`**

Inside `App()`, replace the current form initializer with:

```js
const [form, setForm] = useState(initialProviderForm);
const [drawerOpen, setDrawerOpen] = useState(false);
const [formErrors, setFormErrors] = useState({});
const [saveResult, setSaveResult] = useState(null);
const [deleteConfirmProviderId, setDeleteConfirmProviderId] = useState("");
```

Keep the existing `showAdvanced`, `loading`, `status`, `fetchingModels`, and `fetchModelsStatus` state.

- [ ] **Step 4: Update preset fill and edit handlers**

Replace `fillPreset`, `selectPresetById`, and `handlePresetChange` with:

```js
const fillPreset = useCallback((preset) => {
  setForm((prev) => formFromPreset(preset, prev));
  setFormErrors({});
  setSaveResult(null);
}, []);

const selectPresetById = useCallback((selectedId) => {
  const preset = presets.find((item) => item.id === selectedId);
  if (preset) {
    fillPreset(preset);
  } else {
    setForm((prev) => ({ ...prev, preset_id: selectedId }));
  }
}, [fillPreset, presets]);

const handlePresetChange = (event) => {
  selectPresetById(event.target.value);
};

const openAddProvider = () => {
  const firstPreset = presets[0];
  setForm(firstPreset ? formFromPreset(firstPreset, initialProviderForm()) : initialProviderForm());
  setFormErrors({});
  setSaveResult(null);
  setShowAdvanced(false);
  setDrawerOpen(true);
};

const openEditProvider = (provider) => {
  setForm(formFromProvider(provider, config?.models || [], config?.routes || [], presets));
  setFormErrors({});
  setSaveResult(null);
  setShowAdvanced(false);
  setDrawerOpen(true);
};

const closeProviderDrawer = () => {
  setDrawerOpen(false);
  setFormErrors({});
  setSaveResult(null);
  setDeleteConfirmProviderId("");
};
```

- [ ] **Step 5: Update save and delete handlers**

Replace `handleSaveSetup` with:

```js
const handleSaveSetup = async () => {
  const errors = validateProviderForm(form);
  setFormErrors(errors);
  setSaveResult(null);
  if (Object.keys(errors).length > 0) {
    setStatus({ text: "Fix the highlighted fields before saving.", type: "error" });
    return;
  }

  setLoading(true);
  setStatus({ text: form.mode === "edit" ? "Saving provider changes..." : "Saving provider configuration...", type: "" });
  try {
    const resp = await fetch("/internal/setup/provider", {
      method: "POST",
      headers: apiHeaders,
      body: JSON.stringify(buildProviderSetupPayload(form)),
    });
    const data = await resp.json();
    if (!resp.ok) {
      setStatus({ text: data.error || resp.statusText, type: "error" });
      return;
    }

    let msg = form.mode === "edit" ? "Provider changes saved." : "Provider saved.";
    let isErr = false;
    if (data.claude_activated) {
      msg += " Claude Code activated.";
    } else if (form.activate_claude) {
      msg += ` Claude activation failed: ${data.claude_error || "unknown error"}.`;
      isErr = true;
    }
    setConfig(data.config);
    setSaveResult({ text: msg, type: isErr ? "error" : "ok" });
    setStatus({ text: msg, type: isErr ? "error" : "ok" });
    loadStatus();
  } catch (err) {
    setStatus({ text: `Request failed: ${err.message}`, type: "error" });
  } finally {
    setLoading(false);
  }
};

const handleDeleteProvider = async (providerId) => {
  setLoading(true);
  setStatus({ text: "Removing provider...", type: "" });
  try {
    const resp = await fetch(`/internal/setup/provider?id=${encodeURIComponent(providerId)}`, {
      method: "DELETE",
      headers: apiHeaders,
    });
    const data = await resp.json().catch(() => ({}));
    if (!resp.ok) {
      setStatus({ text: data.error || "Remove failed", type: "error" });
      return;
    }
    setConfig(data.config);
    setStatus({ text: "Provider removed.", type: "ok" });
    closeProviderDrawer();
    loadStatus();
  } catch (err) {
    setStatus({ text: `Remove failed: ${err.message}`, type: "error" });
  } finally {
    setLoading(false);
  }
};
```

- [ ] **Step 6: Replace `ProviderSetupPanel` with drawer-oriented component**

Rename `ProviderSetupPanel` to `ProviderSetupDrawer` and use this signature:

```jsx
function ProviderSetupDrawer({
  open,
  form,
  errors,
  presets,
  loading,
  status,
  saveResult,
  showAdvanced,
  providerNameOptions,
  baseUrlOptions,
  envNameOptions,
  upstreamModelOptions,
  exposedAliasOptions,
  fetchingModels,
  fetchModelsStatus,
  deleteConfirmProviderId,
  onPresetChange,
  onInputChange,
  onSaveSetup,
  onClose,
  onToggleAdvanced,
  onFetchModels,
  onDeleteProvider,
  onDeleteConfirmChange,
}) {
  if (!open) return null;
  const isEdit = form.mode === "edit";
  return (
    <aside className="provider-drawer" aria-label={isEdit ? "Edit provider" : "Add provider"}>
      <div className="drawer-header">
        <div>
          <span className="eyebrow"><i className="ph-fill ph-plugs-connected"></i>{isEdit ? "edit upstream" : "new upstream"}</span>
          <h2>{isEdit ? "Edit provider" : "Add provider"}</h2>
          <p className="muted">Choose a preset, key source, model, route, and Claude activation behavior.</p>
        </div>
        <button className="icon-button" type="button" onClick={onClose} aria-label="Close provider drawer">
          <i className="ph-bold ph-x"></i>
        </button>
      </div>

      <div className="drawer-steps" aria-label="Setup steps">
        <span className="done">Provider</span>
        <span className={form.api_key_mode ? "done" : ""}>Key</span>
        <span className={form.upstream_model && form.route_alias ? "done" : ""}>Model</span>
        <span className={form.activate_claude ? "done" : ""}>CLI</span>
      </div>

      <form className="drawer-body" onSubmit={(event) => event.preventDefault()}>
        <section className="drawer-section">
          <div className="section-heading">
            <span>01</span>
            <div>
              <h3>Provider preset</h3>
              <p>Pick the upstream service Arkroute should route through.</p>
            </div>
          </div>
          <div className="field-grid">
            <div className={`field ${errors.preset_id ? "field-error" : ""}`}>
              <label htmlFor="preset">Provider</label>
              <select id="preset" value={form.preset_id} onChange={onPresetChange}>
                {presets.map((preset) => <option key={preset.id} value={preset.id}>{preset.name}</option>)}
              </select>
              {errors.preset_id && <small>{errors.preset_id}</small>}
            </div>
            <div className={`field ${errors.base_url ? "field-error" : ""}`}>
              <label htmlFor="base-url">Base URL</label>
              <input id="base-url" type="text" list="base-url-options" value={form.base_url} onChange={(event) => onInputChange("base_url", event.target.value)} />
              <datalist id="base-url-options">{baseUrlOptions.map((option) => <option key={option} value={option} />)}</datalist>
              {errors.base_url && <small>{errors.base_url}</small>}
            </div>
          </div>
        </section>

        <section className="drawer-section">
          <div className="section-heading">
            <span>02</span>
            <div>
              <h3>Key source</h3>
              <p>Use an environment variable for safer local setup, or store a key in config.</p>
            </div>
          </div>
          <div className="segmented-control">
            <button type="button" className={form.api_key_mode === "env" ? "active" : ""} onClick={() => onInputChange("api_key_mode", "env")}>Environment</button>
            <button type="button" className={form.api_key_mode === "config" ? "active" : ""} onClick={() => onInputChange("api_key_mode", "config")}>Config</button>
          </div>
          {form.api_key_mode === "env" ? (
            <div className={`field ${errors.env_name ? "field-error" : ""}`}>
              <label htmlFor="env-name">Environment variable</label>
              <input id="env-name" type="text" list="env-name-options" value={form.env_name} onChange={(event) => onInputChange("env_name", event.target.value)} />
              <datalist id="env-name-options">{envNameOptions.map((option) => <option key={option} value={option} />)}</datalist>
              <div className="terminal-note"><i className="ph-light ph-terminal-window"></i><span>export {form.env_name || "API_KEY"}=...</span></div>
              {errors.env_name && <small>{errors.env_name}</small>}
            </div>
          ) : (
            <div className={`field ${errors.api_key ? "field-error" : ""}`}>
              <label htmlFor="api-key">API key</label>
              <input id="api-key" type="password" aria-describedby="api-key-help" value={form.api_key} onChange={(event) => onInputChange("api_key", event.target.value)} />
              <small id="api-key-help">{isEdit ? "Leave blank to keep the existing config key." : "Enter the provider API key."}</small>
              {errors.api_key && <small>{errors.api_key}</small>}
            </div>
          )}
        </section>

        <section className="drawer-section">
          <div className="section-heading">
            <span>03</span>
            <div>
              <h3>Model and route</h3>
              <p>Choose the upstream model and the local name clients will request.</p>
            </div>
          </div>
          <div className="field-grid">
            <div className={`field ${errors.upstream_model ? "field-error" : ""}`}>
              <div className="field-label-row">
                <label htmlFor="upstream-model">Upstream model</label>
                <button type="button" className="btn-tertiary compact-action" onClick={onFetchModels} disabled={fetchingModels || !form.preset_id || !form.base_url}>
                  <i className="ph-bold ph-arrows-clockwise"></i>{fetchingModels ? "Fetching" : "Fetch live"}
                </button>
              </div>
              <select id="upstream-model" value={form.upstream_model || ""} onChange={(event) => onInputChange("upstream_model", event.target.value)}>
                <option value="" disabled>Pick a model</option>
                {upstreamModelOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
              <input className="custom-model-input" type="text" aria-label="Custom upstream model ID" value={form.upstream_model} onChange={(event) => onInputChange("upstream_model", event.target.value)} />
              {fetchModelsStatus?.text && <small className={`status-inline status-${fetchModelsStatus.type}`}>{fetchModelsStatus.text}</small>}
              {errors.upstream_model && <small>{errors.upstream_model}</small>}
            </div>
            <div className={`field ${errors.exposed_alias ? "field-error" : ""}`}>
              <label htmlFor="exposed-alias">Client model name</label>
              <input id="exposed-alias" type="text" list="exposed-alias-options" value={form.exposed_alias} onChange={(event) => onInputChange("exposed_alias", event.target.value)} />
              <datalist id="exposed-alias-options">{exposedAliasOptions.map((option) => <option key={option} value={option} />)}</datalist>
              {errors.exposed_alias && <small>{errors.exposed_alias}</small>}
            </div>
            <div className={`field ${errors.route_alias ? "field-error" : ""}`}>
              <label htmlFor="route-alias">Route alias</label>
              <select id="route-alias" value={form.route_alias} onChange={(event) => onInputChange("route_alias", event.target.value)}>
                {ROUTE_ALIASES.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
              {errors.route_alias && <small>{errors.route_alias}</small>}
            </div>
          </div>
        </section>

        <section className="drawer-section compact">
          <label className="checkbox-label">
            <input id="activate-claude" type="checkbox" checked={form.activate_claude} onChange={(event) => onInputChange("activate_claude", event.target.checked)} />
            <span>Activate Claude Code after save</span>
          </label>
        </section>

        <button className="advanced-toggle" type="button" onClick={onToggleAdvanced}>
          <i className={`ph-bold ph-caret-${showAdvanced ? "up" : "down"}`}></i>
          <span>Advanced mapping</span>
        </button>

        {showAdvanced && (
          <section className="drawer-section advanced-fields">
            <div className="field-grid">
              <div className="field">
                <label htmlFor="provider-name">Provider name</label>
                <input id="provider-name" type="text" list="provider-name-options" value={form.provider_name} onChange={(event) => onInputChange("provider_name", event.target.value)} />
                <datalist id="provider-name-options">{providerNameOptions.map((option) => <option key={option} value={option} />)}</datalist>
              </div>
              <div className="field">
                <label htmlFor="provider-type">Protocol</label>
                <select id="provider-type" value={form.type} onChange={(event) => onInputChange("type", event.target.value)}>
                  {PROTOCOL_TYPES.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                </select>
              </div>
            </div>
          </section>
        )}

        {saveResult?.text && <div className={`status-box ${saveResult.type}`}>{saveResult.text}</div>}
        {status.text && !saveResult?.text && <div className={`status-box ${status.type}`}>{status.text}</div>}
      </form>

      <div className="drawer-actions">
        {isEdit && (
          deleteConfirmProviderId === form.preset_id ? (
            <button className="btn-danger" type="button" onClick={() => onDeleteProvider(form.preset_id)} disabled={loading}>Confirm remove</button>
          ) : (
            <button className="btn-danger subtle" type="button" onClick={() => onDeleteConfirmChange(form.preset_id)} disabled={loading}>Remove provider</button>
          )
        )}
        <div className="drawer-action-spacer"></div>
        <button type="button" className="btn-secondary" onClick={onClose} disabled={loading}>Cancel</button>
        <button id="save-setup" type="button" onClick={onSaveSetup} disabled={loading}>
          <i className="ph-bold ph-floppy-disk"></i>{isEdit ? "Save changes" : "Save provider"}
        </button>
      </div>
    </aside>
  );
}
```

- [ ] **Step 7: Replace Providers tab markup**

Replace the current Providers tab body with:

```jsx
<div className={`tab-content ${activeTab === "providers" ? "active" : ""}`}>
  <PageHeader
    icon="ph-hard-drive"
    eyebrow="gateway agents"
    title="Providers"
    description={providerCount > 0 ? "Manage upstream providers and keep the setup path close." : "Add your first upstream provider to start routing local CLI traffic."}
    stats={providerCount > 0 ? [
      { label: "enabled", value: providerCount },
      { label: "routes", value: routeCount },
      { label: "models", value: modelCount }
    ] : []}
    action={<button type="button" className="primary-button" onClick={openAddProvider}><i className="ph-bold ph-plus"></i>Add provider</button>}
  />

  {providerCount === 0 ? (
    <section className="first-run-panel">
      <EmptyState icon="ph-plugs-connected" title="Add your first provider">
        Choose a provider preset, key source, upstream model, and route alias. Arkroute will keep the local gateway config in sync.
      </EmptyState>
      <button type="button" className="primary-button first-run-action" onClick={openAddProvider}>
        <i className="ph-bold ph-plus"></i>Add first provider
      </button>
    </section>
  ) : (
    <>
      <section className="provider-dashboard">
        <div className="operator-grid configured-provider-grid">
          {config.providers.map((provider) => (
            <article className="provider-row-card" key={provider.id}>
              <ProviderCard provider={provider} />
              <div className="provider-row-actions">
                <button type="button" className="secondary-button" onClick={() => openEditProvider(provider)}>
                  <i className="ph-light ph-pencil-simple-line"></i>Edit
                </button>
                <button type="button" className="secondary-button" onClick={() => setSelectedProviderId(provider.id)}>
                  <i className="ph-light ph-crosshair"></i>Inspect
                </button>
              </div>
            </article>
          ))}
        </div>
      </section>

      <div className="detail-workbench">
        <ProviderDetail
          provider={(config?.providers || []).find((provider) => provider.id === selectedProviderId)}
          models={config?.models || []}
          routes={config?.routes || []}
          onSelectModel={(modelId) => {
            setSelectedModelId(modelId);
            loadCLIContext({ model_id: modelId });
            setActiveTab("models");
          }}
          onSelectRoute={(routeAlias) => {
            setSelectedRouteAlias(routeAlias);
            loadCLIContext({ route_alias: routeAlias });
            setActiveTab("models");
          }}
        />
        <CLIContextPanel context={cliContext} status={cliContextStatus} onCopy={copyCLICommand} />
      </div>
    </>
  )}

  <ProviderSetupDrawer
    open={drawerOpen}
    baseUrlOptions={baseUrlOptions}
    envNameOptions={envNameOptions}
    exposedAliasOptions={exposedAliasOptions}
    form={form}
    errors={formErrors}
    loading={loading}
    presets={presets}
    providerNameOptions={providerNameOptions}
    showAdvanced={showAdvanced}
    status={status}
    saveResult={saveResult}
    upstreamModelOptions={upstreamModelOptions}
    fetchingModels={fetchingModels}
    fetchModelsStatus={fetchModelsStatus}
    deleteConfirmProviderId={deleteConfirmProviderId}
    onInputChange={handleInputChange}
    onPresetChange={handlePresetChange}
    onSaveSetup={handleSaveSetup}
    onClose={closeProviderDrawer}
    onToggleAdvanced={() => setShowAdvanced(!showAdvanced)}
    onFetchModels={() => fetchLiveModels({ force: true })}
    onDeleteProvider={handleDeleteProvider}
    onDeleteConfirmChange={setDeleteConfirmProviderId}
  />
</div>
```

Update `PageHeader` to accept an optional `action` prop:

```jsx
function PageHeader({ icon, eyebrow, title, description, stats = [], action = null }) {
  return (
    <header className="page-header">
      <div className="title-stack">
        <span className="eyebrow"><i className={`ph-fill ${icon}`}></i>{eyebrow}</span>
        <h1>{title}</h1>
        <p className="muted">{description}</p>
      </div>
      <div className="header-actions">
        {stats.length > 0 && (
          <div className="header-metrics">
            {stats.map((stat) => (
              <div className="metric" key={stat.label}>
                <span>{stat.label}</span>
                <strong>{stat.value}</strong>
              </div>
            ))}
          </div>
        )}
        {action}
      </div>
    </header>
  );
}
```

- [ ] **Step 8: Add drawer and provider dashboard CSS**

Add these class groups to `web-ui/src/index.css` near the provider styles:

```css
.header-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: end;
  justify-content: flex-end;
}

.first-run-panel {
  display: grid;
  gap: 18px;
  border: 1px solid rgba(127, 244, 94, 0.18);
  border-radius: 8px;
  background: rgba(127, 244, 94, 0.035);
  padding: 28px;
}

.first-run-action {
  width: max-content;
  justify-self: center;
}

.provider-dashboard {
  display: grid;
  gap: 16px;
}

.provider-row-card {
  display: grid;
  gap: 10px;
}

.provider-row-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.provider-drawer {
  position: fixed;
  top: 0;
  right: 0;
  z-index: 30;
  display: grid;
  grid-template-rows: auto auto minmax(0, 1fr) auto;
  width: min(520px, 100vw);
  height: 100dvh;
  border-left: 1px solid rgba(127, 244, 94, 0.24);
  background: rgba(8, 12, 10, 0.98);
  box-shadow: -32px 0 90px rgba(0, 0, 0, 0.44);
}

.drawer-header,
.drawer-actions {
  border-bottom: 1px solid var(--line);
  padding: 22px;
}

.drawer-header {
  display: flex;
  gap: 16px;
  align-items: flex-start;
  justify-content: space-between;
}

.icon-button {
  display: inline-grid;
  width: 36px;
  height: 36px;
  place-items: center;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.018);
  color: var(--muted);
  cursor: pointer;
}

.drawer-steps {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
  border-bottom: 1px solid var(--line);
  padding: 14px 22px;
}

.drawer-steps span {
  border-top: 4px solid var(--line-strong);
  color: var(--faint);
  font-family: var(--font-mono);
  font-size: 11px;
  padding-top: 8px;
}

.drawer-steps span.done {
  border-color: var(--accent);
  color: var(--accent);
}

.drawer-body {
  display: grid;
  gap: 0;
  overflow-y: auto;
}

.drawer-section {
  display: grid;
  gap: 16px;
  border-bottom: 1px solid var(--line);
  padding: 22px;
}

.drawer-section.compact {
  padding: 16px 22px;
}

.segmented-control {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.segmented-control button,
.btn-tertiary,
.btn-danger {
  min-height: 40px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.018);
  color: var(--muted);
  cursor: pointer;
  font-weight: 700;
  padding: 0 12px;
}

.segmented-control button.active {
  border-color: rgba(127, 244, 94, 0.34);
  background: rgba(127, 244, 94, 0.075);
  color: var(--text);
}

.compact-action {
  min-height: 30px;
  margin-left: auto;
}

.custom-model-input,
.status-inline {
  margin-top: 6px;
}

.field-error input,
.field-error select {
  border-color: rgba(255, 107, 107, 0.6);
  box-shadow: 0 0 0 3px rgba(255, 107, 107, 0.1);
}

.field small {
  color: var(--muted);
  font-size: 12px;
}

.field-error small {
  color: var(--error);
}

.drawer-actions {
  display: flex;
  gap: 10px;
  align-items: center;
  border-top: 1px solid var(--line);
  border-bottom: 0;
}

.drawer-action-spacer {
  flex: 1 1 auto;
}

.btn-danger {
  border-color: rgba(255, 107, 107, 0.32);
  background: rgba(255, 107, 107, 0.06);
  color: var(--error);
}

.btn-danger.subtle {
  color: var(--muted);
}
```

- [ ] **Step 9: Run frontend helper tests and build**

Run:

```bash
node --test web-ui/src/modelFetch.test.js web-ui/src/providerSetup.test.js
npm run build --prefix web-ui
```

Expected: both commands PASS.

- [ ] **Step 10: Commit Providers dashboard and drawer**

```bash
git add web-ui/src/App.jsx web-ui/src/index.css
git commit -m "feat: redesign providers setup drawer"
```

---

## Task 5: Refresh Remaining Tabs And Remove Inline Styling

**Files:**
- Modify: `web-ui/src/App.jsx`
- Modify: `web-ui/src/index.css`

- [ ] **Step 1: Move policy inspector inline styles into classes**

In `PolicyInspector`, replace:

```jsx
<div className="policy-override-editor" style={{ marginTop: "24px", paddingTop: "20px", borderTop: "1px solid rgba(148, 163, 184, 0.15)" }}>
  <h4 style={{ margin: "0 0 16px 0", color: "#f8fafc", fontSize: "14px" }}>
    <i className="ph-bold ph-pencil-simple-line" style={{ marginRight: "8px" }}></i>
    Compatibility Policy Override
  </h4>
```

with:

```jsx
<div className="policy-override-editor">
  <h4>
    <i className="ph-bold ph-pencil-simple-line"></i>
    Compatibility Policy Override
  </h4>
```

Replace inline-styled policy form wrappers with:

```jsx
<div className="field-grid policy-override-grid">
```

```jsx
<div className="actions policy-override-actions">
```

```jsx
<div className={`status-box ${overrideStatus.type} policy-override-status`}>
```

Add CSS:

```css
.policy-override-editor {
  border-top: 1px solid var(--line);
  margin-top: 24px;
  padding-top: 20px;
}

.policy-override-editor h4 {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 16px;
  color: var(--text);
  font-size: 14px;
}

.policy-override-grid {
  gap: 12px;
  margin-bottom: 16px;
}

.policy-override-actions {
  gap: 10px;
  margin-top: 16px;
  padding: 0;
}

.policy-override-status {
  margin-top: 12px;
}
```

- [ ] **Step 2: Move provider detail inline styles into classes**

In `ProviderDetail`, replace inline style wrappers with:

```jsx
<div className="policy-summary-grid provider-detail-summary">
```

```jsx
<div className="context-list provider-detail-context">
  <strong className="eyebrow context-list-title">Exposed Models</strong>
```

```jsx
<div className="context-list provider-detail-context">
  <strong className="eyebrow context-list-title">Associated Routes</strong>
```

Add CSS:

```css
.provider-detail-summary {
  gap: 8px;
  margin: 12px 0;
}

.provider-detail-context {
  margin-top: 12px;
}

.context-list-title {
  display: block;
  margin-bottom: 8px;
}
```

- [ ] **Step 3: Define missing CSS tokens**

In `:root`, add aliases so existing route preset/context blocks render consistently while the broader CSS refresh is applied:

```css
--border: var(--line);
--surface-soft: rgba(255, 255, 255, 0.018);
--shadow-panel: 0 20px 70px rgba(0, 0, 0, 0.32);
```

- [ ] **Step 4: Refresh Models & Routes layout**

Wrap the Models tab grid in a topology class:

```jsx
<div className="operator-grid topology-grid">
```

Add CSS:

```css
.topology-grid {
  grid-template-columns: minmax(260px, 0.9fr) minmax(0, 1.1fr);
}

.topology-grid .policy-inspector-card,
.topology-grid .cli-context-card {
  grid-column: 1 / -1;
}

.route-presets-card {
  border-color: rgba(105, 215, 255, 0.18);
  background: rgba(105, 215, 255, 0.035);
}
```

- [ ] **Step 5: Refresh Traces terminal styling**

Add CSS:

```css
.terminal-window {
  border-color: rgba(244, 201, 93, 0.14);
}

.log-stream {
  background:
    linear-gradient(rgba(244, 201, 93, 0.018) 1px, transparent 1px),
    #050606;
  background-size: 100% 28px;
}

.log-line {
  min-height: 34px;
  align-items: center;
}

.log-line.selected .log-label {
  color: var(--accent);
}
```

- [ ] **Step 6: Refresh CLI Tools and System density**

Add CSS:

```css
.cli-readiness {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.cli-route-strip {
  grid-column: 1 / -1;
}

.config-safety-card {
  grid-column: span 2;
}

.config-import-textarea {
  min-height: 140px;
}
```

- [ ] **Step 7: Remove all layout inline styles from `App.jsx`**

Run:

```bash
rg -n "style=\\{" web-ui/src/App.jsx
```

Expected before final cleanup: no matches. If matches remain, move each style object into a class in `web-ui/src/index.css` and rerun the command.

- [ ] **Step 8: Build after tab refresh**

Run:

```bash
npm run build --prefix web-ui
```

Expected: PASS.

- [ ] **Step 9: Commit remaining tab refresh**

```bash
git add web-ui/src/App.jsx web-ui/src/index.css
git commit -m "style: refresh panel tabs"
```

---

## Task 6: Responsive Polish And Asset Build

**Files:**
- Modify: `web-ui/src/index.css`
- Modify: `internal/panel/assets/*`
- Modify: `internal/panel/assets/panel.html`

- [ ] **Step 1: Add responsive drawer and dashboard rules**

Add or update media rules in `web-ui/src/index.css`:

```css
@media (max-width: 1080px) {
  .topology-grid,
  .detail-workbench,
  .provider-workbench,
  .cli-console-body {
    grid-template-columns: 1fr;
  }

  .provider-drawer {
    width: min(560px, 100vw);
  }
}

@media (max-width: 760px) {
  .shell {
    grid-template-columns: 1fr;
  }

  .sidebar {
    position: relative;
    height: auto;
    padding: 16px;
  }

  nav {
    grid-template-columns: repeat(5, minmax(0, 1fr));
  }

  .nav-item {
    height: 44px;
    justify-content: center;
    padding: 0 6px;
  }

  .nav-item span {
    display: block;
    max-width: 54px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 11px;
  }

  .content {
    padding: 24px 14px 40px;
  }

  .page-header {
    grid-template-columns: 1fr;
  }

  .header-actions,
  .header-metrics {
    width: 100%;
    justify-content: stretch;
  }

  .provider-drawer {
    left: 0;
    width: 100vw;
    border-left: 0;
  }

  .drawer-actions {
    align-items: stretch;
    flex-direction: column;
  }

  .drawer-action-spacer {
    display: none;
  }

  .drawer-actions button,
  .primary-button,
  .secondary-button {
    width: 100%;
  }

  .field-grid,
  .policy-summary-grid,
  .policy-value-grid,
  .cli-readiness {
    grid-template-columns: 1fr;
  }
}
```

- [ ] **Step 2: Run focused tests**

Run:

```bash
go test -count=1 ./internal/setup ./internal/panel
node --test web-ui/src/modelFetch.test.js web-ui/src/providerSetup.test.js
npm run build --prefix web-ui
```

Expected: all commands PASS.

- [ ] **Step 3: Run full Go test suite**

Run:

```bash
go test -count=1 ./...
```

Expected: PASS.

- [ ] **Step 4: Regenerate embedded panel assets**

Run:

```bash
make build-frontend
```

Expected: Vite build passes, `internal/panel/assets/panel.html` points to the new hashed JS and CSS asset names, and stale hashed assets are removed by the Makefile.

- [ ] **Step 5: Inspect asset diff**

Run:

```bash
git status --short internal/panel/assets web-ui/src/App.jsx web-ui/src/index.css
```

Expected: modified `internal/panel/assets/panel.html`, new hashed `index-*.js` and possibly `index-*.css`, deleted stale hashed assets, and modified frontend source files.

- [ ] **Step 6: Document lint status**

Run:

```bash
npm run lint --prefix web-ui
```

Expected for this plan: either PASS after the refactor, or the same React hook rule failures that existed before implementation. If lint still fails, record the exact failing rule names in the implementation summary and keep build/tests as the completion gate for this slice.

- [ ] **Step 7: Commit final asset build**

```bash
git add web-ui/src/App.jsx web-ui/src/index.css web-ui/src/providerSetup.js web-ui/src/providerSetup.test.js internal/panel/assets
git commit -m "build: refresh embedded panel assets"
```

---

## Final Verification Checklist

- [ ] `go test -count=1 ./...` passes.
- [ ] `node --test web-ui/src/modelFetch.test.js web-ui/src/providerSetup.test.js` passes.
- [ ] `npm run build --prefix web-ui` passes.
- [ ] `make build-frontend` passes and embedded assets are updated.
- [ ] `rg -n "style=\\{" web-ui/src/App.jsx` returns no matches.
- [ ] `rg -n -- "--border|--surface-soft" web-ui/src/index.css` shows defined tokens and intentional uses only.
- [ ] First-run Providers tab shows a focused `Add first provider` path.
- [ ] Configured Providers tab shows dashboard cards/rows and `Add provider`.
- [ ] `Edit` opens a prefilled drawer without exposing stored API keys.
- [ ] Add/Edit save updates config and keeps unrelated providers/routes.
- [ ] Remove provider uses inline confirmation and removes only that provider's owned model/route targets.
- [ ] Mobile width check shows full-screen drawer sheet and usable navigation.
- [ ] Traces view retains terminal-like density while matching the refreshed token system.
