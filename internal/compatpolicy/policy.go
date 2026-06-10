package compatpolicy

import (
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func StableModelPolicyID(modelID string) string {
	value := Slug(modelID, "model")
	return "model-" + value + "-compat"
}

func Slug(value string, emptyFallback string) string {
	clean := strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range clean {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return emptyFallback
	}
	return out
}

func RemoveByID(policies []config.CompatibilityPolicyConfig, id string) []config.CompatibilityPolicyConfig {
	out := make([]config.CompatibilityPolicyConfig, 0, len(policies))
	for _, p := range policies {
		if p.ID != id {
			out = append(out, p)
		}
	}
	return out
}
