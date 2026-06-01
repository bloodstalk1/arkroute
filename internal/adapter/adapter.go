package adapter

import (
	"net/http"

	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/protocol"
)

type UpstreamRequest struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
}

type ProviderAdapter interface {
	BuildRequest(req protocol.Request, provider config.ProviderConfig, model config.ModelConfig) (UpstreamRequest, error)
	MapResponse(body []byte) (protocol.Response, error)
}
