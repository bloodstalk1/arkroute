# Spec: Arkroute Remaining Features

**Status:** spec  
**Author:** review session  
**Date:** 2026-06-11  

## Scope

3 features còn thiếu sau review toàn diện:

1. **Rate limiter wiring** — gắn token bucket vào gateway auth middleware  
2. **Frontend component split** — tách App.jsx 1700 dòng thành components  
3. **Advanced compression** — RTK-style cho tool outputs (git diff, grep, JSON logs)

## 1. Rate Limiter Wiring

### Problem
Library `internal/security/ratelimit` đã có (token bucket, 60 req/min, burst 5). Chưa được gọi ở đâu. Gateway không giới hạn request rate.

### Solution
- Thêm `RateLimiter *ratelimit.Store` vào `client/claude/server.go` struct
- Trong auth middleware: sau khi validate `Authorization: Bearer <key>`, gọi `rateLimiter.Allow(key)`. Nếu false → trả 429.
- Config: thêm `server.rate_limit_rpm` (default 0 = disabled)
- Test: gửi burst+1 request, expect 429 ở request thứ burst+1

### Acceptance
- `TestRateLimiterRejectsExcess` pass
- Server config có field `RateLimitRPM`
- `arkroute serve` chạy không crash
- Không thay đổi behavior khi rate_limit_rpm = 0

---

## 2. Frontend Component Split

### Problem
`web-ui/src/App.jsx` dài 1700 dòng, 1 file duy nhất. Không test được, khó maintain, khó review.

### Solution
Tách thành cấu trúc:

```
web-ui/src/
├── App.jsx               (~100 dòng: layout + state management)
├── main.jsx
├── hooks/
│   └── useCatalog.js     (fetch catalog + live fetch logic)
├── components/
│   ├── ProviderSetupPanel.jsx
│   ├── ProviderDetail.jsx
│   ├── ModelItem.jsx
│   ├── RouteItem.jsx
│   ├── PolicyInspector.jsx
│   ├── CLIContextPanel.jsx
│   ├── RoutePresetPanel.jsx
│   ├── ConfigTransferPanel.jsx
│   ├── LogsPanel.jsx
│   ├── CliToolsPanel.jsx
│   └── shared/
│       ├── StatusBadge.jsx
│       ├── DataRow.jsx
│       ├── EmptyState.jsx
│       └── PageHeader.jsx
└── constants.js           (PROTOCOL_TYPES, ROUTE_ALIASES)
```

Mỗi component nhận props, không import trực tiếp state từ App.

### Acceptance
- `npm run build` pass
- Panel load không crash, tất cả tabs hoạt động
- Mỗi component < 200 dòng
- Không thay đổi UX/UI

---

## 3. Advanced Compression

### Problem
`internal/observability/compress` chỉ có Lite/Aggressive cơ bản (whitespace, truncation). Tool outputs chiếm phần lớn token usage nhưng không được nén thông minh.

### Solution
Thêm `compressor` interface + 3 engine:

```
type Compressor interface {
    Compress(text string, context CompressContext) string
}

type CompressContext struct {
    ContentType string // "git_diff", "grep_output", "json_log", "shell_output", "code", "text"
}
```

| Engine | Target | Strategy | Savings |
|--------|--------|----------|---------|
| `GitDiff` | `git diff` output | Chỉ giữ + and - lines, bỏ context lines trùng lặp | ~40% |
| `GrepOutput` | `grep -r` results | Group by file, dedup matching lines | ~50% |
| `JSONLog` | JSON log lines | Strip common keys, keep message + level | ~60% |

### Acceptance
- 3 engine + test với input thật (git diff, grep output, JSON logs)
- `compress.Tidy()` auto-detect content type
- Không thay đổi semantic của code blocks (skip nếu phát hiện code fence)

---

## Non-goals (sẽ không làm)
- A2A protocol — không cần cho coding tool gateway
- Multi-user / SSO — arkroute là single-user local tool
- Desktop app / PWA — không phải use case
- MCP stream transport (HTTP/SSE) — STDIO đủ cho Claude Code

---

## Self-Review

### Rate Limiter Risks
- Thiếu cleanup goroutine → memory leak. Cần background goroutine 5 phút gọi `Cleanup()`.
- Cần thêm `rate_limit_rpm` vào `ServerConfig` + update `ApplyDefaults()`, `MinimalValidConfig()`, `cloneServer()` trong state.go.
- Auth middleware dùng `subtle.ConstantTimeCompare`, cần inject `rateLimiter.Allow()` **trước** khi compare để không leak timing.
- Token bucket test dùng `time.Sleep` → flaky trên CI. Chấp nhận rủi ro thấp.

### Frontend Split Risks
- `setupToken`, `apiHeaders` đang là global → chuyển sang App scope, pass qua props.
- Build fail nếu import path sai → tách từng component, build sau mỗi lần.
- `fetchLiveModels` dùng `apiHeaders` → extract ra `useLiveFetch(apiHeaders)` hook riêng.
- CSS global trong App.css → không cần CSS modules, vẫn dùng chung sau split.

### Advanced Compression Risks
- False positive: code `+ added line` giống git diff → cần detect `diff --git` header.
- JSON detection: parse thử, nếu fail → skip compression.
- Large input (>100KB) → early return, không regex.
- Interface cần `CanCompress(contentType string) bool` để dispatcher chọn engine.
