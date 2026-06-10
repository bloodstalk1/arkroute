import assert from "node:assert/strict";
import test from "node:test";

import {
  buildFetchModelsPayload,
  envNameForProvider,
  fetchModelsFailureStatus,
  modelFetchCacheKey,
  shouldAutoFetchModels,
} from "./modelFetch.js";

test("buildFetchModelsPayload sends env key references for env-mode fetches", () => {
  const form = {
    preset_id: "openai",
    base_url: "https://api.openai.com/v1",
    api_key_mode: "env",
    env_name: "OPENAI_API_KEY",
    api_key: "",
    type: "auto",
  };

  assert.deepEqual(buildFetchModelsPayload(form), {
    preset_id: "openai",
    base_url: "https://api.openai.com/v1",
    api_key: "env:OPENAI_API_KEY",
    protocol: "",
  });
});

test("buildFetchModelsPayload sends raw config keys and explicit protocol", () => {
  const form = {
    preset_id: "anthropic",
    base_url: "https://api.anthropic.com",
    api_key_mode: "config",
    env_name: "ANTHROPIC_API_KEY",
    api_key: "sk-ant-test",
    type: "anthropic",
  };

  assert.deepEqual(buildFetchModelsPayload(form), {
    preset_id: "anthropic",
    base_url: "https://api.anthropic.com",
    api_key: "sk-ant-test",
    protocol: "anthropic",
  });
});

test("shouldAutoFetchModels requires preset and base URL", () => {
  assert.equal(shouldAutoFetchModels({ preset_id: "", base_url: "https://api.openai.com/v1" }), false);
  assert.equal(shouldAutoFetchModels({ preset_id: "openai", base_url: "" }), false);
  assert.equal(shouldAutoFetchModels({ preset_id: "openai", base_url: "https://api.openai.com/v1" }), true);
});

test("modelFetchCacheKey changes when provider connection inputs change", () => {
  const base = {
    preset_id: "openai",
    base_url: "https://api.openai.com/v1",
    api_key_mode: "env",
    env_name: "OPENAI_API_KEY",
    api_key: "",
    type: "auto",
  };

  assert.equal(
    modelFetchCacheKey(base),
    "openai|https://api.openai.com/v1|auto|env|OPENAI_API_KEY|",
  );
  assert.notEqual(modelFetchCacheKey(base), modelFetchCacheKey({ ...base, env_name: "OPENAI_ALT_KEY" }));
  assert.notEqual(modelFetchCacheKey(base), modelFetchCacheKey({ ...base, api_key_mode: "config", api_key: "sk-test" }));
});

test("envNameForProvider generates env names for catalog presets", () => {
  assert.equal(envNameForProvider("deepseek"), "DEEPSEEK_API_KEY");
  assert.equal(envNameForProvider("lm-studio"), "LM_STUDIO_API_KEY");
  assert.equal(envNameForProvider("openai-compatible"), "OPENAI_API_KEY");
  assert.equal(envNameForProvider(""), "PROVIDER_API_KEY");
});

test("fetchModelsFailureStatus uses upstream detail for auto fallback", () => {
  assert.deepEqual(
    fetchModelsFailureStatus({ error: "upstream returned HTTP 404", catalog: { name: "OpenCode Go" } }, 502, {
      automatic: true,
    }),
    {
      text: "Live model discovery unavailable: upstream returned HTTP 404. Showing curated list.",
      type: "info",
    },
  );
});

test("fetchModelsFailureStatus keeps auth failures actionable", () => {
  assert.deepEqual(
    fetchModelsFailureStatus({ auth_error: true, error: "upstream rejected the API key" }, 401, {
      automatic: true,
    }),
    {
      text: "Upstream rejected the API key. Check the selected key source.",
      type: "error",
    },
  );
});
