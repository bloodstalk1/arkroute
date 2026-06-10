package security

import "strings"

func Redact(value string) string {
	if value == "" {
		return ""
	}
	return "[redacted]"
}

func RedactMap(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		if LooksSecret(key) {
			out[key] = Redact(value)
			continue
		}
		out[key] = value
	}
	return out
}

var secretHeaderNames = map[string]struct{}{
	"authorization":       {},
	"x-api-key":           {},
	"api-key":             {},
	"apikey":              {},
	"x-goog-api-key":      {},
	"anthropic-api-key":   {},
	"x-auth-token":        {},
	"x-secret":            {},
	"openai-api-key":      {},
	"x-openai-api-key":    {},
	"proxy-authorization": {},
	"cookie":              {},
	"set-cookie":          {},
	"x-csrf-token":        {},
	"x-csrftoken":         {},
}

var benignKeyHeaders = map[string]struct{}{
	"sec-websocket-key": {},
	"key-id":            {},
}

func LooksSecret(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if lower == "" {
		return false
	}
	if _, ok := secretHeaderNames[lower]; ok {
		return true
	}
	if _, ok := benignKeyHeaders[lower]; ok {
		return false
	}
	return strings.HasSuffix(lower, "-token") ||
		strings.HasSuffix(lower, "-secret") ||
		strings.HasSuffix(lower, "-key") ||
		strings.Contains(lower, "secret") ||
		strings.HasPrefix(lower, "x-") && (strings.HasSuffix(lower, "-auth") || strings.HasSuffix(lower, "-credential") || strings.HasSuffix(lower, "-password"))
}
