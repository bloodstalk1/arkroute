package tokens

import "testing"

func TestEstimateEmpty(t *testing.T) {
	if Estimate("") != 0 {
		t.Errorf("Estimate(\"\") = %d, want 0", Estimate(""))
	}
}

func TestEstimateShort(t *testing.T) {
	got := Estimate("hello")
	if got == 0 {
		t.Error("Estimate(\"hello\") = 0, want > 0")
	}
	if got > 3 {
		t.Errorf("Estimate(\"hello\") = %d, want <= 3", got)
	}
}

func TestEstimateGrowsWithText(t *testing.T) {
	short := Estimate("hello")
	long := Estimate("hello world this is a longer string with more text")
	if long <= short {
		t.Errorf("long estimate %d <= short estimate %d", long, short)
	}
}

func TestRequestEstimates(t *testing.T) {
	req := TokenRequest{
		SystemTexts: []string{"You are a helpful assistant."},
		Messages: []Message{
			{Content: "What is the capital of France?"},
		},
	}
	got := RequestEstimate(req)
	if got == 0 {
		t.Error("Request() = 0, want > 0")
	}
	t.Logf("estimated %d tokens for simple request", got)
}
