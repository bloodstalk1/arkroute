package adapter

import (
	"encoding/json"
	"testing"
)

// FuzzFormatStreamError exercises FormatStreamError with arbitrary
// inputs. The contract is: never panic, never return empty (the
// function substitutes "upstream stream error" for null/empty input).
func FuzzFormatStreamError(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("null"))
	f.Add([]byte("plain text body"))
	f.Add([]byte(`{"message":"hi"}`))
	f.Add([]byte(`{"error":{"message":"nested","code":"x"}}`))
	f.Add([]byte(`{"type":"error","error":{"type":"x","message":"deep"}}`))
	f.Add([]byte(`[{"message":"a"},{"message":"b"}]`))
	f.Add([]byte("not json at all"))
	f.Add([]byte{0x00, 0x01, 0x02})
	f.Add(make([]byte, 4096))

	f.Fuzz(func(t *testing.T, raw []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic on input %q: %v", raw, r)
			}
		}()
		got := FormatStreamError(json.RawMessage(raw))
		if got == "" {
			t.Fatalf("FormatStreamError returned empty for input %q", raw)
		}
	})
}
