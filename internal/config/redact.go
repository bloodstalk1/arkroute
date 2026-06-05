package config

func Redacted(cfg Config) Config {
	out := cfg
	out.Server.ClientKey = "[redacted]"
	out.Providers = append([]ProviderConfig(nil), cfg.Providers...)
	for i := range out.Providers {
		if out.Providers[i].APIKey != "" {
			out.Providers[i].APIKey = "[redacted]"
		}
		if out.Providers[i].Headers != nil {
			redacted := make(map[string]string, len(out.Providers[i].Headers))
			for k := range out.Providers[i].Headers {
				redacted[k] = "[redacted]"
			}
			out.Providers[i].Headers = redacted
		}
	}
	return out
}
