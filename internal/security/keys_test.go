package security

import "testing"

func TestGenerateClientKey(t *testing.T) {
	key, err := GenerateClientKey()
	if err != nil {
		t.Fatalf("GenerateClientKey() error = %v", err)
	}
	if len(key) < 32 {
		t.Fatalf("key length = %d, want at least 32", len(key))
	}
}

