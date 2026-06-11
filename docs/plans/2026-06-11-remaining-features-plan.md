# Plan: Arkroute Remaining Features

**Status:** plan  
**Spec:** `docs/specs/2026-06-11-remaining-features.md`  
**Date:** 2026-06-11  
**Estimated:** 6-8 hours

---

## Phase 1: Rate Limiter Wiring (1 hour)

### 1.1 Config
- `internal/config/types.go`: thêm `RateLimitRPM int yaml:"rate_limit_rpm"` vào `ServerConfig`
- `internal/config/load.go`: ApplyDefaults → default 0 (disabled)
- `internal/config/load.go`: MinimalValidConfig → set 0
- `internal/runtime/state.go`: cloneServer → copy new field

### 1.2 Gateway integration
- `internal/client/claude/server.go`: thêm `rateLimiter *ratelimit.Store` vào struct
- `internal/client/claude/server.go`: NewServer → tạo `ratelimit.DefaultStore()` nếu config enabled
- `internal/client/claude/auth.go`: middleware gọi `rateLimiter.Allow(key)` trước compare
- 429 response: `{"error": "rate limit exceeded"}`

### 1.3 Cleanup goroutine
- `internal/client/claude/server.go`: background goroutine gọi `rateLimiter.Cleanup(10*time.Minute)` mỗi 5 phút

### 1.4 Tests
- `internal/client/claude/server_test.go`: TestRateLimiterRejectsExcess — gửi burst+1 request, expect 429

---

## Phase 2: Advanced Compression (2 hours)

### 2.1 Interface + registry
- `internal/observability/compress/compress.go`: thêm `Compressor` interface + `Registry`
- `internal/observability/compress/compress.go`: `Compress(text, ctx) string` dispatcher

### 2.2 GitDiff engine
- `internal/observability/compress/gitdiff.go`: detect `diff --git`, keep +/-/@@ lines, skip unchanged context
- Test: real `git diff` output → verify < original size

### 2.3 GrepOutput engine  
- `internal/observability/compress/grep.go`: detect `filename:line:match` pattern, group by file, dedup
- Test: real `grep -r` output → verify dedup

### 2.4 JSONLog engine
- `internal/observability/compress/jsonlog.go`: parse JSON per line, strip `timestamp`/`level`/`trace_id`, keep `message`
- Test: JSON log lines → verify keys stripped

### 2.5 Integration
- `compress.Tidy()` → auto-detect content type → dispatch to engine

---

## Phase 3: Frontend Split (3-4 hours)

### 3.1 Setup
- Cài vitest: `npm install -D vitest @testing-library/react jsdom`
- Tạo `vitest.config.js`
- Extract `constants.js` (PROTOCOL_TYPES, ROUTE_ALIASES)

### 3.2 Extract custom hooks
- `web-ui/src/hooks/useCatalog.js` — fetch + fetchLiveModels + state
- Test: mock fetch, verify state transitions

### 3.3 Split shared components (dễ nhất, ít dependency)
- `StatusBadge.jsx`, `DataRow.jsx`, `EmptyState.jsx`, `PageHeader.jsx`
- Build + verify panel load

### 3.4 Split tab components (mỗi lần 1 tab)
- `LogsPanel.jsx` → build → verify
- `CliToolsPanel.jsx` → build → verify
- `ConfigTransferPanel.jsx` → build → verify
- `RoutePresetPanel.jsx` → build → verify
- `CLIContextPanel.jsx` → build → verify
- `PolicyInspector.jsx` → build → verify
- `ProviderDetail.jsx` + `ModelItem.jsx` + `RouteItem.jsx` → build → verify
- `ProviderSetupPanel.jsx` → build → verify

### 3.5 Final App.jsx
- Chỉ còn state management + layout + tab switching (~150 dòng)
- Build + verify toàn bộ panel

### 3.6 Tests (optional, time permitting)
- Render test cho mỗi component với mock props
- Integration test: tab switching hiển thị đúng component

---

## Test Plan

| Phase | Test type | Count |
|-------|-----------|-------|
| Rate limiter | Go unit test | 2 (allow, reject) |
| GitDiff | Go unit test | 1 |
| Grep | Go unit test | 1 |
| JSONLog | Go unit test | 1 |
| Frontend hooks | Vitest unit | 2 |
| Frontend components | Vitest render | 5 |
| E2E | Manual (panel load) | 1 |
| **Total** | | **13** |

---

## Rollback Plan
- Mỗi commit riêng biệt → revert từng commit nếu fail
- Frontend: giữ bản copy App.jsx gốc, revert nếu build fail
- Rate limiter: default disabled → không ảnh hưởng production nếu bug

---

## Plan Self-Review

### Risks Identified
1. **Vitest + jsdom compatibility**: Vite 8 + React 19 cần vitest config phù hợp. jsdom có thể thiếu `fetch` → cần polyfill hoặc dùng `happy-dom`. Nếu không setup được → skip tests, chỉ tách components + verify manual.
2. **cloneServer missing**: `internal/runtime/state.go` có thể không có `cloneServer` helper → cần kiểm tra trước khi thêm field mới.
3. **useCatalog dependency on apiHeaders**: hook cần `apiHeaders` từ App scope → pass làm parameter hoặc dùng React context.
4. **Phase ordering risk**: Compression engines có thể conflict với frontend build nếu import path sai → làm compression trước (Go-only, không ảnh hưởng frontend).
5. **CSS specificity**: sau khi tách component, CSS có thể break do class name scope → verify visual trước/sau mỗi split.

### Mitigations
- Skip vitest nếu không setup được trong 30 phút
- Kiểm tra `cloneServer` trước Phase 1
- Compression + Rate limiter làm trước (Go-only, low risk)
- Frontend split làm cuối (high risk, cần manual verify)
