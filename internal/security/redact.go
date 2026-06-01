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

func LooksSecret(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "authorization") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "api-key") ||
		strings.Contains(lower, "apikey") ||
		strings.Contains(lower, "key")
}
