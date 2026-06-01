package builtin

import (
	"testing"

	"bat.dev/arkrouter/internal/adapter"
	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/failure"
	"bat.dev/arkrouter/internal/protocol"
)

func TestDefaultRegistryHasBuiltIns(t *testing.T) {
	registry := DefaultRegistry()
	for _, providerType := range []string{"openai_compatible", "gemini", "anthropic"} {
		if _, ok := registry.Get(providerType); !ok {
			t.Fatalf("DefaultRegistry missing %s", providerType)
		}
	}
}

func TestMapRegistry(t *testing.T) {
	registry := adapter.MapRegistry{"fake": fakeAdapterForTest{}}
	if _, ok := registry.Get("fake"); !ok {
		t.Fatal("fake adapter missing")
	}
	if _, ok := registry.Get("missing"); ok {
		t.Fatal("missing adapter unexpectedly present")
	}
}

type fakeAdapterForTest struct{}

func (fakeAdapterForTest) BuildRequest(protocol.Request, config.ProviderConfig, config.ModelConfig) (adapter.UpstreamRequest, error) {
	return adapter.UpstreamRequest{}, nil
}
func (fakeAdapterForTest) MapResponse([]byte) (protocol.Response, error) {
	return protocol.Response{}, nil
}
func (fakeAdapterForTest) NewStreamMapper() (adapter.StreamMapper, bool) { return nil, false }
func (fakeAdapterForTest) ClassifyError(status int, body []byte) failure.ErrorClass {
	return failure.ErrorUpstreamFatal
}
