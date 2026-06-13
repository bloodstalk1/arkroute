package config

// Redacted returns a copy of cfg with every secret field replaced by
// "[redacted]". The admin panel returns it to the browser so the user
// can see their config without leaking the client key or upstream
// API keys.
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
