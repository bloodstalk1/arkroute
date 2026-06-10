import { envNameForProvider } from "./modelFetch.js";

export function initialProviderForm() {
  return {
    mode: "add",
    preset_id: "",
    provider_name: "",
    base_url: "",
    type: "",
    api_key_mode: "env",
    api_key: "",
    env_name: "",
    upstream_model: "",
    exposed_alias: "",
    route_alias: "",
    activate_claude: true,
  };
}

export function formFromPreset(preset, previous = initialProviderForm()) {
  if (!preset) {
    return { ...previous, preset_id: "" };
  }
  return {
    ...previous,
    mode: previous.mode || "add",
    preset_id: preset.id || "",
    provider_name: preset.name || "",
    base_url: preset.base_url || "",
    type: preset.type || "",
    upstream_model: preset.default_model || "",
    exposed_alias: preset.default_alias || "",
    route_alias: preset.default_route || "",
    env_name: preset.id ? envNameForProvider(preset.id) : "",
  };
}

export function formFromProvider(provider, models = [], routes = [], presets = []) {
  const preset = presets.find((item) => item.id === provider?.id);
  const providerModel = models.find((model) => model.provider_id === provider?.id);
  const route = routes.find((item) =>
    (item.targets || []).some((target) => target.model_id === providerModel?.id),
  );
  const apiKey = provider?.api_key || "";
  const envMode = apiKey.startsWith("env:");
  return {
    ...initialProviderForm(),
    mode: "edit",
    preset_id: preset?.id || provider?.id || "",
    provider_name: provider?.name || preset?.name || provider?.id || "",
    base_url: provider?.base_url || preset?.base_url || "",
    type: provider?.type || preset?.type || "",
    api_key_mode: envMode ? "env" : "config",
    api_key: "",
    env_name: envMode ? apiKey.slice(4) : envNameForProvider(provider?.id || preset?.id || ""),
    upstream_model: providerModel?.upstream_model || preset?.default_model || "",
    exposed_alias: providerModel?.exposed_alias || preset?.default_alias || "",
    route_alias: route?.alias || preset?.default_route || "sonnet",
    activate_claude: true,
  };
}

export function validateProviderForm(form) {
  const errors = {};
  if (!form.preset_id?.trim()) errors.preset_id = "Choose a provider preset.";
  if (!form.base_url?.trim()) errors.base_url = "Enter a provider base URL.";
  if (form.api_key_mode === "env" && !form.env_name?.trim()) {
    errors.env_name = "Enter the environment variable name.";
  }
  if (form.api_key_mode === "config" && form.mode === "add" && !form.api_key?.trim()) {
    errors.api_key = "Enter an API key or use an environment variable.";
  }
  if (!form.upstream_model?.trim()) errors.upstream_model = "Choose or enter an upstream model.";
  if (!form.exposed_alias?.trim()) errors.exposed_alias = "Enter the model name shown to clients.";
  if (!form.route_alias?.trim()) errors.route_alias = "Choose a route alias.";
  return errors;
}

export function buildProviderSetupPayload(form) {
  return {
    preset_id: form.preset_id,
    provider_name: form.provider_name,
    base_url: form.base_url,
    type: form.type,
    api_key_mode: form.api_key_mode,
    api_key: form.api_key_mode === "config" ? form.api_key : "",
    env_name: form.api_key_mode === "env" ? form.env_name : "",
    upstream_model: form.upstream_model,
    exposed_alias: form.exposed_alias,
    route_alias: form.route_alias,
    activate_claude: form.activate_claude,
  };
}

export function providerKeySummary(provider) {
  const apiKey = provider?.api_key || "";
  if (!apiKey) return "not configured";
  if (apiKey.startsWith("env:")) return `env:${apiKey.slice(4)}`;
  return "stored in config";
}
