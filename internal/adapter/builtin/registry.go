package builtin

import (
	"bat.dev/arkrouter/internal/adapter"
	anthropicadapter "bat.dev/arkrouter/internal/adapter/anthropic"
	geminiadapter "bat.dev/arkrouter/internal/adapter/gemini"
	openaiadapter "bat.dev/arkrouter/internal/adapter/openai"
)

func DefaultRegistry() adapter.Registry {
	return adapter.MapRegistry{
		"openai_compatible": openaiadapter.Adapter{},
		"gemini":            geminiadapter.Adapter{},
		"anthropic":         anthropicadapter.Adapter{},
	}
}
