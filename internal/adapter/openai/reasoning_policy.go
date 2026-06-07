package openai

import (
	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/protocol"
	"github.com/bloodstalk1/arkroute/internal/reasoning"
)

type reasoningBehavior = reasoning.Behavior

func resolveReasoning(provider config.ProviderConfig, model config.ModelConfig, req protocol.Request) reasoningBehavior {
	return reasoning.Resolve(provider, model, req)
}
