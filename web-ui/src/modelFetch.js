export function buildFetchModelsPayload(form) {
  return {
    preset_id: form.preset_id,
    base_url: form.base_url,
    api_key: form.api_key_mode === "env" ? `env:${form.env_name || ""}` : form.api_key || "",
    protocol: form.type === "auto" ? "" : form.type || "",
  };
}

export function shouldAutoFetchModels(form) {
  return Boolean(form?.preset_id && form?.base_url);
}

export function modelFetchCacheKey(form) {
  return [
    form.preset_id || "",
    form.base_url || "",
    form.type || "",
    form.api_key_mode || "",
    form.env_name || "",
    form.api_key || "",
  ].join("|");
}

export function envNameForProvider(id) {
  switch (id) {
    case "openrouter":
      return "OPENROUTER_API_KEY";
    case "anthropic":
      return "ANTHROPIC_API_KEY";
    case "gemini":
      return "GEMINI_API_KEY";
    case "openai-compatible":
      return "OPENAI_API_KEY";
    case "opencode-go":
    case "opencode-zen":
      return "OPENCODE_API_KEY";
    default: {
      const normalized = String(id || "")
        .replace(/[^A-Za-z0-9]+/g, "_")
        .replace(/^_+|_+$/g, "")
        .toUpperCase();
      return `${normalized || "PROVIDER"}_API_KEY`;
    }
  }
}

export function fetchModelsFailureStatus(data, status, { automatic = false } = {}) {
  if (data?.auth_error) {
    return {
      text: "Upstream rejected the API key. Check the selected key source.",
      type: "error",
    };
  }

  const detail = data?.error || `fetch failed with HTTP ${status}`;
  const fallback = data?.catalog ? " Showing curated list." : "";
  return {
    text: `Live model discovery unavailable: ${detail}.${fallback}`.replace("..", "."),
    type: automatic ? "info" : "error",
  };
}
