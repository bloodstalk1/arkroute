package builtin

import (
	"bat.dev/arkroute/internal/adapter"
	anthropicadapter "bat.dev/arkroute/internal/adapter/anthropic"
	geminiadapter "bat.dev/arkroute/internal/adapter/gemini"
	openaiadapter "bat.dev/arkroute/internal/adapter/openai"
)

func DefaultRegistry() adapter.Registry {
	return adapter.MapRegistry{
		"openai_compatible": openaiadapter.Adapter{},
		"gemini":            geminiadapter.Adapter{},
		"anthropic":         anthropicadapter.Adapter{},
	}
}
