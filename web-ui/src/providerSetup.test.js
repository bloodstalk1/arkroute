import assert from "node:assert/strict";
import test from "node:test";

import {
  buildProviderSetupPayload,
  formFromPreset,
  formFromProvider,
  initialProviderForm,
  providerKeySummary,
  validateProviderForm,
} from "./providerSetup.js";

const presets = [
  {
    id: "openrouter",
    name: "OpenRouter",
    type: "openai_compatible",
    base_url: "https://openrouter.ai/api/v1",
    default_model: "anthropic/claude-sonnet-4.5",
    default_alias: "sonnet-or",
    default_route: "sonnet",
  },
  {
    id: "custom",
    name: "Custom",
    type: "auto",
    base_url: "https://example.com/v1",
    default_model: "provider/model",
    default_alias: "custom-model",
    default_route: "sonnet",
  },
];

test("initialProviderForm creates add-mode defaults", () => {
  assert.deepEqual(initialProviderForm(), {
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
  });
});

test("formFromPreset fills the happy path fields", () => {
  assert.deepEqual(formFromPreset(presets[0]), {
    mode: "add",
    preset_id: "openrouter",
    provider_name: "OpenRouter",
    base_url: "https://openrouter.ai/api/v1",
    type: "openai_compatible",
    api_key_mode: "env",
    api_key: "",
    env_name: "OPENROUTER_API_KEY",
    upstream_model: "anthropic/claude-sonnet-4.5",
    exposed_alias: "sonnet-or",
    route_alias: "sonnet",
    activate_claude: true,
  });
});

test("formFromProvider reconstructs edit mode without leaking config secrets", () => {
  const provider = {
    id: "openrouter",
    name: "OpenRouter",
    type: "openai_compatible",
    base_url: "https://openrouter.ai/api/v1",
    api_key: "sk-secret",
  };
  const models = [
    {
      id: "openrouter-sonnet-or",
      provider_id: "openrouter",
      upstream_model: "anthropic/claude-sonnet-4.5",
      exposed_alias: "sonnet-or",
    },
  ];
  const routes = [{ alias: "sonnet", targets: [{ model_id: "openrouter-sonnet-or", enabled: true }] }];

  const form = formFromProvider(provider, models, routes, presets);
  assert.equal(form.mode, "edit");
  assert.equal(form.preset_id, "openrouter");
  assert.equal(form.api_key_mode, "config");
  assert.equal(form.api_key, "");
  assert.equal(form.env_name, "OPENROUTER_API_KEY");
  assert.equal(form.upstream_model, "anthropic/claude-sonnet-4.5");
  assert.equal(form.exposed_alias, "sonnet-or");
  assert.equal(form.route_alias, "sonnet");
});

test("validateProviderForm reports actionable field errors", () => {
  assert.deepEqual(validateProviderForm(initialProviderForm()), {
    preset_id: "Choose a provider preset.",
    base_url: "Enter a provider base URL.",
    env_name: "Enter the environment variable name.",
    upstream_model: "Choose or enter an upstream model.",
    exposed_alias: "Enter the model name shown to clients.",
    route_alias: "Choose a route alias.",
  });
});

test("buildProviderSetupPayload keeps edit payload compatible with setup endpoint", () => {
  const form = {
    ...formFromPreset(presets[0]),
    mode: "edit",
    api_key_mode: "config",
    api_key: "sk-updated",
  };
  assert.deepEqual(buildProviderSetupPayload(form), {
    preset_id: "openrouter",
    provider_name: "OpenRouter",
    base_url: "https://openrouter.ai/api/v1",
    type: "openai_compatible",
    api_key_mode: "config",
    api_key: "sk-updated",
    env_name: "",
    upstream_model: "anthropic/claude-sonnet-4.5",
    exposed_alias: "sonnet-or",
    route_alias: "sonnet",
    activate_claude: true,
  });
});

test("providerKeySummary never returns raw config keys", () => {
  assert.equal(providerKeySummary({ api_key: "env:OPENROUTER_API_KEY" }), "env:OPENROUTER_API_KEY");
  assert.equal(providerKeySummary({ api_key: "sk-secret" }), "stored in config");
  assert.equal(providerKeySummary({ api_key: "" }), "not configured");
});
