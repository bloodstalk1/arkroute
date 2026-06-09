package compatpolicy

import (
	"testing"
)

func TestStableModelPolicyIDSanitizesModelID(t *testing.T) {
	got := StableModelPolicyID("DeepSeek/V4 Pro++")
	want := "model-deepseek-v4-pro-compat"
	if got != want {
		t.Fatalf("StableModelPolicyID() = %q, want %q", got, want)
	}
}
