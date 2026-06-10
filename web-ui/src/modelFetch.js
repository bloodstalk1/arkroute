export function buildFetchModelsPayload(form) {
  return {
    preset_id: form.preset_id,
    base_url: form.base_url,
    api_key: form.api_key || "",
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
    form.api_key || "",
  ].join("|");
}

export function fetchModelsFailureStatus(data, status, { automatic = false } = {}) {
  if (data?.auth_error) {
    return {
      text: "Upstream rejected the API key. Check the selected key.",
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
