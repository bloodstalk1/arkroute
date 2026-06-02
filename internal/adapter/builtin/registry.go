package builtin

import (
	"github.com/bloodstalk1/arkroute/internal/adapter"
	anthropicadapter "github.com/bloodstalk1/arkroute/internal/adapter/anthropic"
	geminiadapter "github.com/bloodstalk1/arkroute/internal/adapter/gemini"
	openaiadapter "github.com/bloodstalk1/arkroute/internal/adapter/openai"
)

func DefaultRegistry() adapter.Registry {
	return adapter.MapRegistry{
		"openai_compatible": openaiadapter.Adapter{},
		"gemini":            geminiadapter.Adapter{},
		"anthropic":         anthropicadapter.Adapter{},
	}
}
