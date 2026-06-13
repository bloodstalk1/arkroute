// Package adapter defines the [ProviderAdapter] interface that every
// upstream provider implements, the [UpstreamRequest] / [StreamMapper]
// types shared between them, and a tiny [Registry] for looking one up
// by provider type. The concrete implementations live in
// adapter/openai, adapter/anthropic, and adapter/gemini; adapter/builtin
// wires them into a [Registry].
package adapter

import (
	"net/http"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/failure"
	"github.com/bloodstalk1/arkroute/internal/protocol"
)

// UpstreamRequest is what an adapter hands to the executor's HTTP
// client: a fully-formed method, URL, headers, and body, ready to be
// sent to the provider.
type UpstreamRequest struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
}

// StreamMapper translates one chunk of provider-specific SSE output
// into a sequence of [protocol.StreamEvent] values. The executor owns
// the lifecycle and calls MapLine for every line read from the wire.
type StreamMapper interface {
	MapLine(line []byte) ([]protocol.StreamEvent, error)
}

// ProviderAdapter is the contract every upstream must satisfy. The
// methods are called in this order by the executor:
//
//  1. BuildRequest — translate the normalised request into a wire call
//  2. Either MapResponse (non-streaming) or NewStreamMapper + MapLine
//     (streaming), depending on req.Stream
//  3. ClassifyError — on non-2xx, decide whether to retry/fallback
type ProviderAdapter interface {
	BuildRequest(req protocol.Request, provider config.ProviderConfig, model config.ModelConfig) (UpstreamRequest, error)
	MapResponse(body []byte) (protocol.Response, error)
	NewStreamMapper() (StreamMapper, bool)
	ClassifyError(status int, body []byte) failure.ErrorClass
}

// Registry maps a provider type string (e.g. "openai_compatible") to
// its [ProviderAdapter]. The runtime keeps a single Registry per
// process.
type Registry interface {
	Get(providerType string) (ProviderAdapter, bool)
}

// MapRegistry is a [Registry] backed by a Go map. Use it with a
// composite literal at startup:
//
//	adapter.MapRegistry{"openai_compatible": OpenAIAdapter{}, ...}
type MapRegistry map[string]ProviderAdapter

// Get looks up the adapter registered under providerType. It returns
// the zero value and false when nothing is registered.
func (r MapRegistry) Get(providerType string) (ProviderAdapter, bool) {
	providerAdapter, ok := r[providerType]
	return providerAdapter, ok
}
