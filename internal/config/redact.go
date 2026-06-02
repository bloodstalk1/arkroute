package config

func Redacted(cfg Config) Config {
	out := cfg
	out.Server.ClientKey = "[redacted]"
	out.Providers = append([]ProviderConfig(nil), cfg.Providers...)
	for i := range out.Providers {
		if out.Providers[i].APIKey != "" {
			out.Providers[i].APIKey = "[redacted]"
		}
	}
	return out
}
