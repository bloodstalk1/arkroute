package runtime

import (
	"github.com/bloodstalk1/arkroute/internal/config"
	providercatalog "github.com/bloodstalk1/arkroute/internal/provider"
)

var defaultProviderResolver = providercatalog.DefaultResolver()

func resolveProviderType(provider config.ProviderConfig, model config.ModelConfig) string {
	return defaultProviderResolver.Resolve(providercatalog.ProviderRef{
		ID:      provider.ID,
		Name:    provider.Name,
		Type:    provider.Type,
		BaseURL: provider.BaseURL,
	}, providercatalog.ModelRef{
		Protocol:      model.Protocol,
		UpstreamModel: model.UpstreamModel,
	})
}
