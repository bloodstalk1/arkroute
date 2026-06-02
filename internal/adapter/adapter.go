package adapter

import (
	"net/http"

	"bat.dev/arkroute/internal/config"
	"bat.dev/arkroute/internal/failure"
	"bat.dev/arkroute/internal/protocol"
)

type UpstreamRequest struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
}

type StreamMapper interface {
	MapLine(line []byte) ([]protocol.StreamEvent, error)
}

type ProviderAdapter interface {
	BuildRequest(req protocol.Request, provider config.ProviderConfig, model config.ModelConfig) (UpstreamRequest, error)
	MapResponse(body []byte) (protocol.Response, error)
	NewStreamMapper() (StreamMapper, bool)
	ClassifyError(status int, body []byte) failure.ErrorClass
}

type Registry interface {
	Get(providerType string) (ProviderAdapter, bool)
}

type MapRegistry map[string]ProviderAdapter

func (r MapRegistry) Get(providerType string) (ProviderAdapter, bool) {
	providerAdapter, ok := r[providerType]
	return providerAdapter, ok
}
