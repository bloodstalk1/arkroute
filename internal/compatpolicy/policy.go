package compatpolicy

import (
	"strings"
)

func StableModelPolicyID(modelID string) string {
	clean := strings.ToLower(strings.TrimSpace(modelID))
	var b strings.Builder
	lastDash := false
	for _, r := range clean {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	value := strings.Trim(b.String(), "-")
	if value == "" {
		value = "model"
	}
	return "model-" + value + "-compat"
}
