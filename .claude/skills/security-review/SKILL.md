---
name: security-review
description: Security audit checklist for arkrouter — validates API key handling, redaction, auth pathways, and injection vectors
disable-model-invocation: true
---

# Security Review for Arkrouter

Use this skill before merging any PR that touches security-sensitive code paths.

## Checklist

### 1. API Key & Secret Handling
- [ ] No hardcoded keys or secrets in source (use `env:NAME` references or `os.Getenv`)
- [ ] Generated keys use `crypto/rand` (not `math/rand`)
- [ ] Keys are redacted in logs via `security.Redact()` or `security.RedactMap()`
- [ ] `security.LooksSecret()` covers new header/field names if added

### 2. Injection Vectors
- [ ] `exec.Command` calls use explicit args (not shell strings)
- [ ] Config file paths are not user-controllable from network requests
- [ ] YAML parsing does not use `yaml.Unmarshal` on untrusted input without validation

### 3. TLS & Networking
- [ ] Server binds to `127.0.0.1` by default (not `0.0.0.0`)
- [ ] TLS config uses minimum TLS 1.2
- [ ] No sensitive data in URL query strings

### 4. Adapter Security
- [ ] Provider API keys are passed via headers only, never in URLs
- [ ] Request/response bodies are not logged (arkrouter policy: no prompt/response body logging)
- [ ] Stream connections have read/write timeouts

### 5. Config Validation
- [ ] `config.Validate()` rejects non-loopback hosts
- [ ] `config.Validate()` rejects broken provider/model references
- [ ] Snapshot integrity is maintained (no mutation after build)

## Review Command

Run before review:
```sh
go test ./internal/security/... ./internal/config/...
go vet ./internal/security/... ./internal/config/...
```
