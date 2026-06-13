package runtime

import (
	"testing"
)

// FuzzFormatUpstreamError exercises formatUpstreamError with arbitrary
// upstream response bodies. The contract is: never panic, return a
// string that always mentions the upstream status.
func FuzzFormatUpstreamError(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("not json"))
	f.Add([]byte(`{"error":{"message":"bad"}}`))
	f.Add([]byte(`{"message":"top level"}`))
	f.Add([]byte(`[{"message":"a"}]`))
	f.Add(make([]byte, 8192))
	f.Add([]byte{0xff, 0xfe, 0xfd})

	f.Fuzz(func(t *testing.T, body []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic on body %q: %v", body, r)
			}
		}()
		got := formatUpstreamError(500, body)
		if got == "" {
			t.Fatalf("formatUpstreamError returned empty for body %q", body)
		}
	})
}
