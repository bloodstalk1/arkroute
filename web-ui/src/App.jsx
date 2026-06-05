import { useCallback, useEffect, useMemo, useRef, useState } from "react";

const setupToken = new URLSearchParams(window.location.hash.slice(1)).get("setup_token") || "";
const assetPath = (path) => `${import.meta.env.BASE_URL}${path}`;

const PROVIDER_MODELS = {
  anthropic: [
    { value: "claude-3-5-sonnet-latest", label: "Claude 3.5 Sonnet (Latest / Recommended)" },
    { value: "claude-3-5-haiku-latest", label: "Claude 3.5 Haiku (Latest)" },
    { value: "claude-3-opus-latest", label: "Claude 3 Opus (Latest)" },
    { value: "claude-3-5-sonnet-20241022", label: "Claude 3.5 Sonnet (v2)" },
    { value: "claude-3-5-haiku-20241022", label: "Claude 3.5 Haiku" },
    { value: "claude-3-opus-20240229", label: "Claude 3 Opus" }
  ],
  gemini: [
    { value: "gemini-2.5-pro", label: "Gemini 2.5 Pro (Recommended)" },
    { value: "gemini-2.5-flash", label: "Gemini 2.5 Flash" },
    { value: "gemini-1.5-pro", label: "Gemini 1.5 Pro" },
    { value: "gemini-1.5-flash", label: "Gemini 1.5 Flash" }
  ],
  openrouter: [
    { value: "anthropic/claude-3.5-sonnet", label: "Claude 3.5 Sonnet (via OR / Recommended)" },
    { value: "anthropic/claude-3.5-sonnet:beta", label: "Claude 3.5 Sonnet Beta" },
    { value: "google/gemini-2.5-pro", label: "Gemini 2.5 Pro" },
    { value: "deepseek/deepseek-chat", label: "DeepSeek V3" },
    { value: "deepseek/deepseek-reasoner", label: "DeepSeek R1" },
    { value: "meta-llama/llama-3.3-70b-instruct", label: "Llama 3.3 70B" }
  ],
  "openai-compatible": [
    { value: "gpt-4o", label: "GPT-4o (Recommended)" },
    { value: "gpt-4o-mini", label: "GPT-4o Mini" },
    { value: "gpt-4-turbo", label: "GPT-4 Turbo" },
    { value: "o1-mini", label: "o1-mini" },
    { value: "o3-mini", label: "o3-mini" },
    { value: "deepseek-chat", label: "DeepSeek V3" },
    { value: "deepseek-reasoner", label: "DeepSeek R1" }
  ],
  "opencode-go": [
    { value: "qwen3.7-max", label: "Qwen 3.7 Max (Recommended)" },
    { value: "deepseek-v3", label: "DeepSeek V3" },
    { value: "deepseek-r1", label: "DeepSeek R1" }
  ]
};

const PROTOCOL_TYPES = [
  { value: "auto", label: "Auto-detect Protocol (Recommended)" },
  { value: "anthropic", label: "Anthropic Native Protocol" },
  { value: "gemini", label: "Gemini Native Protocol" },
  { value: "openai_compatible", label: "OpenAI-compatible Compatibility Layer" }
];

const ROUTE_ALIASES = [
  { value: "sonnet", label: "sonnet (Standard Route / Recommended)" },
  { value: "haiku", label: "haiku" },
  { value: "opus", label: "opus" }
];

const NAV_ITEMS = [
  { id: "setup", icon: "ph-sliders-horizontal", label: "Setup" },
  { id: "providers", icon: "ph-hard-drive", label: "Providers" },
  { id: "models", icon: "ph-git-fork", label: "Routes" },
  { id: "logs", icon: "ph-scroll", label: "Traces" },
  { id: "system", icon: "ph-cpu", label: "System" }
];

function envNameForProvider(id) {
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
      return "OPENCODE_API_KEY";
    default:
      return "";
  }
}

function formatLogTime(timeStr) {
  try {
    const d = new Date(timeStr);
    return `${d.toLocaleTimeString()}.${String(d.getMilliseconds()).padStart(3, "0")}`;
  } catch {
    return "";
  }
}

function logMessage(item) {
  switch (item.event) {
    case "config_reload_started":
      return {
        tone: "pending",
        label: "RELOAD",
        text: `Config reload started, generation ${item.previous_config_generation} -> ${item.config_generation || "?"}`
      };
    case "config_reload_succeeded":
      return { tone: "ok", label: "RELOAD", text: `Config reloaded, generation ${item.config_generation}` };
    case "config_reload_failed":
      return { tone: "error", label: "RELOAD", text: `Config reload failed: ${item.reason || item.error_class}` };
    case "request_started":
      return { tone: "info", label: "INBOUND", text: `${item.client || "client"} -> ${item.route || "route"}` };
    case "route_planned":
      return { tone: "info", label: "PLAN", text: `Routing strategy: ${item.strategy}` };
    case "target_selected":
      return { tone: "selected", label: "TARGET", text: `${item.model} on ${item.provider}` };
    case "upstream_request_started":
      return { tone: "info", label: "UPSTREAM", text: `Dispatching to ${item.upstream_model}` };
    case "upstream_response":
      return { tone: "ok", label: "RESPONSE", text: `Status ${item.status}, latency ${item.latency_ms}ms` };
    case "request_finished":
      return { tone: "muted", label: "DONE", text: `Status ${item.status}, total ${item.latency_ms}ms` };
    case "request_failed":
      return { tone: "error", label: "FAILED", text: `${item.reason || item.error_class}, latency ${item.latency_ms}ms` };
    default:
      return { tone: "muted", label: item.event || "LOG", text: item.msg || JSON.stringify(item) };
  }
}

function PageHeader({ icon, eyebrow, title, description, stats = [] }) {
  return (
    <header className="page-header">
      <div className="title-stack">
        <span className="eyebrow"><i className={`ph-fill ${icon}`}></i>{eyebrow}</span>
        <h1>{title}</h1>
        <p className="muted">{description}</p>
      </div>
      {stats.length > 0 && (
        <div className="header-metrics">
          {stats.map((stat) => (
            <div className="metric" key={stat.label}>
              <span>{stat.label}</span>
              <strong>{stat.value}</strong>
            </div>
          ))}
        </div>
      )}
    </header>
  );
}

function StatusBadge({ tone = "ok", children }) {
  return (
    <span className={`status-indicator ${tone}`}>
      <span className="status-dot"></span>
      {children}
    </span>
  );
}

function DataRow({ label, children }) {
  return (
    <div className="data-row">
      <span>{label}</span>
      <strong>{children}</strong>
    </div>
  );
}

function EmptyState({ icon, title, children }) {
  return (
    <div className="empty-state">
      <i className={`ph-light ${icon}`}></i>
      <strong>{title}</strong>
      <p>{children}</p>
    </div>
  );
}

function ProviderCard({ provider }) {
  const apiKey = provider.api_key || "";
  return (
    <article className="operator-card span-2">
      <div className="card-heading">
        <div>
          <StatusBadge>Enabled</StatusBadge>
          <h3><i className="ph-light ph-puzzle-piece"></i>{provider.name || provider.id}</h3>
        </div>
        <code>{provider.id}</code>
      </div>
      <div className="data-grid">
        <DataRow label="Protocol">{provider.type || "auto"}</DataRow>
        <DataRow label="Base URL">{provider.base_url}</DataRow>
        <DataRow label="Key">{apiKey.startsWith("env:") ? `env:${apiKey.slice(4)}` : "stored in config"}</DataRow>
      </div>
    </article>
  );
}

function ModelItem({ model }) {
  return (
    <div className="list-item">
      <div>
        <strong>{model.display_name || model.id}</strong>
        <span>{model.provider_id}</span>
      </div>
      <code>{model.upstream_model}</code>
    </div>
  );
}

function RouteItem({ route }) {
  return (
    <div className="list-item route-item">
      <div>
        <strong>{route.alias}</strong>
        <span>{route.strategy}</span>
      </div>
      <div className="target-list">
        {(route.targets || []).map((target, index) => (
          <span className={target.enabled ? "target enabled" : "target"} key={`${target.model_id}-${index}`}>
            {index + 1}. {target.model_id}
          </span>
        ))}
      </div>
    </div>
  );
}

function LogLine({ item }) {
  const log = logMessage(item);
  return (
    <div className={`log-line ${log.tone}`}>
      <time>{formatLogTime(item.time)}</time>
      <span className="log-label">{log.label}</span>
      <p>{log.text}</p>
    </div>
  );
}

function App() {
  const [activeTab, setActiveTab] = useState("setup");
  const [presets, setPresets] = useState([]);
  const [loading, setLoading] = useState(false);
  const [status, setStatus] = useState({ text: "Loading provider presets...", type: "" });
  const [config, setConfig] = useState(null);
  const [logs, setLogs] = useState([]);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const logsEndRef = useRef(null);

  const [form, setForm] = useState({
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
    activate_claude: true
  });

  const apiHeaders = useMemo(() => ({
    "Content-Type": "application/json",
    "X-Arkroute-Setup-Token": setupToken
  }), []);

  const providerCount = config?.providers?.length || 0;
  const modelCount = config?.models?.length || 0;
  const routeCount = config?.routes?.length || 0;
  const configState = providerCount > 0 ? "Configured" : "Bootstrap";

  const fillPreset = useCallback((preset) => {
    setForm((prev) => ({
      ...prev,
      preset_id: preset.id || "",
      provider_name: preset.name || "",
      base_url: preset.base_url || "",
      type: preset.type || "",
      upstream_model: preset.default_model || "",
      exposed_alias: preset.default_alias || "",
      route_alias: preset.default_route || "",
      env_name: preset.id ? envNameForProvider(preset.id) : ""
    }));
  }, []);

  const loadStatus = useCallback((cancelled = () => false) => {
    return fetch("/internal/setup/status", { headers: apiHeaders })
      .then((resp) => (resp.ok ? resp.json() : null))
      .then((data) => {
        if (!cancelled() && data?.config) {
          setConfig(data.config);
        }
      })
      .catch((err) => console.error("Failed to fetch status:", err));
  }, [apiHeaders]);

  const loadLogs = useCallback((cancelled = () => false) => {
    return fetch("/internal/setup/logs", { headers: apiHeaders })
      .then((resp) => (resp.ok ? resp.json() : null))
      .then((data) => {
        if (!cancelled() && data) {
          setLogs(data.logs || []);
        }
      })
      .catch((err) => console.error("Failed to fetch logs:", err));
  }, [apiHeaders]);

  useEffect(() => {
    let cancelled = false;
    const isCancelled = () => cancelled;

    fetch("/internal/setup/options", { headers: apiHeaders })
      .then((resp) => {
        if (!resp.ok) {
          throw new Error("Setup session expired. Run arkroute setup again.");
        }
        return resp.json();
      })
      .then((data) => {
        if (cancelled) return;
        const loadedPresets = data.presets || [];
        setPresets(loadedPresets);
        setStatus({ text: "Provider presets loaded.", type: "ok" });
        if (loadedPresets.length > 0) {
          fillPreset(loadedPresets[0]);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setStatus({ text: err.message, type: "error" });
        }
      });

    loadStatus(isCancelled);
    return () => {
      cancelled = true;
    };
  }, [apiHeaders, fillPreset, loadStatus]);

  useEffect(() => {
    let cancelled = false;
    const isCancelled = () => cancelled;

    loadStatus(isCancelled);
    if (activeTab === "logs") {
      loadLogs(isCancelled);
      const interval = setInterval(() => loadLogs(isCancelled), 3000);
      return () => {
        cancelled = true;
        clearInterval(interval);
      };
    }
    return () => {
      cancelled = true;
    };
  }, [activeTab, loadLogs, loadStatus]);

  useEffect(() => {
    if (activeTab === "logs" && logsEndRef.current) {
      logsEndRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [logs, activeTab]);

  const providerNameOptions = useMemo(() => {
    const list = new Set();
    const activePreset = presets.find((p) => p.id === form.preset_id);
    if (activePreset) list.add(activePreset.name);
    presets.forEach((p) => list.add(p.name));
    ["OpenRouter", "Anthropic", "Gemini", "OpenAI-compatible", "OpenCode Go", "Custom"].forEach((name) => list.add(name));
    return Array.from(list);
  }, [form.preset_id, presets]);

  const baseUrlOptions = useMemo(() => {
    const list = new Set();
    const activePreset = presets.find((p) => p.id === form.preset_id);
    if (activePreset) list.add(activePreset.base_url);
    presets.forEach((p) => list.add(p.base_url));
    [
      "https://openrouter.ai/api/v1",
      "https://api.anthropic.com",
      "https://generativelanguage.googleapis.com/v1beta",
      "https://api.openai.com/v1",
      "https://opencode.ai/zen/go"
    ].forEach((url) => list.add(url));
    return Array.from(list);
  }, [form.preset_id, presets]);

  const envNameOptions = useMemo(() => {
    const list = new Set([form.env_name, "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "OPENCODE_API_KEY"]);
    list.delete("");
    return Array.from(list);
  }, [form.env_name]);

  const upstreamModelOptions = useMemo(() => {
    const list = [];
    const activePreset = presets.find((p) => p.id === form.preset_id);
    if (activePreset?.default_model) {
      list.push({ value: activePreset.default_model, label: `${activePreset.default_model} (Preset Default)` });
    }
    (PROVIDER_MODELS[form.preset_id] || []).forEach((model) => {
      if (!list.some((item) => item.value === model.value)) list.push(model);
    });
    if (list.length === 0) {
      Object.values(PROVIDER_MODELS).flat().forEach((model) => {
        if (!list.some((item) => item.value === model.value)) list.push(model);
      });
    }
    return list;
  }, [form.preset_id, presets]);

  const exposedAliasOptions = useMemo(() => {
    const list = new Set();
    const activePreset = presets.find((p) => p.id === form.preset_id);
    if (activePreset?.default_alias) list.add(activePreset.default_alias);
    presets.forEach((p) => {
      if (p.default_alias) list.add(p.default_alias);
    });
    ["claude-3-5-sonnet-latest", "sonnet-anthropic", "sonnet-or", "gemini-pro", "openai-model", "qwen37"].forEach((alias) => list.add(alias));
    return Array.from(list);
  }, [form.preset_id, presets]);

  const handlePresetChange = (event) => {
    const selectedId = event.target.value;
    const preset = presets.find((item) => item.id === selectedId);
    if (preset) {
      fillPreset(preset);
    } else {
      setForm((prev) => ({ ...prev, preset_id: selectedId }));
    }
  };

  const handleInputChange = (field, value) => {
    setForm((prev) => ({ ...prev, [field]: value }));
  };

  const handleSaveSetup = async () => {
    setLoading(true);
    setStatus({ text: "Saving provider configuration...", type: "" });
    try {
      const payload = {
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
        activate_claude: form.activate_claude
      };

      const resp = await fetch("/internal/setup/provider", {
        method: "POST",
        headers: apiHeaders,
        body: JSON.stringify(payload)
      });
      const data = await resp.json();
      if (!resp.ok) {
        setStatus({ text: `Error: ${data.error || resp.statusText}`, type: "error" });
        return;
      }

      let msg = "Configuration saved.";
      let isErr = false;
      if (data.claude_activated) {
        msg += " Claude Code activated.";
      } else if (payload.activate_claude) {
        msg += ` Claude activation failed: ${data.claude_error || "unknown error"}.`;
        isErr = true;
      }
      setStatus({ text: msg, type: isErr ? "error" : "ok" });
      loadStatus();
    } catch (err) {
      setStatus({ text: `Request failed: ${err.message}`, type: "error" });
    } finally {
      setLoading(false);
    }
  };

  const handleSetupLater = async () => {
    setLoading(true);
    setStatus({ text: "Saving bootstrap config...", type: "" });
    try {
      const resp = await fetch("/internal/setup/later", { method: "POST", headers: apiHeaders });
      const data = await resp.json();
      if (!resp.ok) {
        setStatus({ text: `Error: ${data.error || resp.statusText}`, type: "error" });
        return;
      }
      setStatus({ text: "Bootstrap config saved.", type: "ok" });
      loadStatus();
    } catch (err) {
      setStatus({ text: `Request failed: ${err.message}`, type: "error" });
    } finally {
      setLoading(false);
    }
  };

  return (
    <main className="shell">
      <aside className="sidebar">
        <div className="sidebar-top">
          <div className="brand">
            <img className="brand-mark" src={assetPath("arkroute-mark.svg")} alt="" aria-hidden="true" />
            <div>
              <span>Arkroute</span>
              <code>terminal portal</code>
            </div>
          </div>

          <nav>
            {NAV_ITEMS.map((item) => (
              <button
                className={`nav-item ${activeTab === item.id ? "active" : ""}`}
                key={item.id}
                type="button"
                onClick={() => setActiveTab(item.id)}
              >
                <i className={`ph-light ${item.icon}`}></i>
                <span>{item.label}</span>
              </button>
            ))}
          </nav>
        </div>

        <div className="sidebar-footer">
          <StatusBadge tone={providerCount > 0 ? "ok" : "pending"}>{configState}</StatusBadge>
          <span className="version-tag">v0.0.0-dev</span>
        </div>
      </aside>

      <section className="content">
        <div className={`tab-content ${activeTab === "setup" ? "active" : ""}`}>
          <PageHeader
            icon="ph-circles-three-plus"
            eyebrow="local gateway"
            title="Configure Provider"
            description="Select an upstream, map the exposed route, and write the local gateway config."
            stats={[
              { label: "providers", value: providerCount },
              { label: "routes", value: routeCount },
              { label: "models", value: modelCount }
            ]}
          />

          <form className="operator-panel setup-panel" onSubmit={(event) => event.preventDefault()}>
            <section className="panel-section">
              <div className="section-heading">
                <span>01</span>
                <div>
                  <h2>Provider</h2>
                  <p>Gateway upstream and protocol.</p>
                </div>
              </div>

              <div className="field-grid">
                <div className="field">
                  <label htmlFor="preset">Preset</label>
                  <select id="preset" value={form.preset_id} onChange={handlePresetChange}>
                    {presets.length === 0 ? (
                      <option value="">Loading presets...</option>
                    ) : (
                      presets.map((preset) => (
                        <option key={preset.id} value={preset.id}>{preset.name}</option>
                      ))
                    )}
                  </select>
                </div>

                <div className="field">
                  <label>API key mode</label>
                  <div className="radio-group">
                    <label className="radio-label">
                      <input type="radio" name="api-key-mode" value="env" checked={form.api_key_mode === "env"} onChange={() => handleInputChange("api_key_mode", "env")} />
                      <span>Environment</span>
                    </label>
                    <label className="radio-label">
                      <input type="radio" name="api-key-mode" value="config" checked={form.api_key_mode === "config"} onChange={() => handleInputChange("api_key_mode", "config")} />
                      <span>Config</span>
                    </label>
                  </div>
                </div>
              </div>

              {form.api_key_mode === "env" ? (
                <div className="terminal-note">
                  <i className="ph-light ph-terminal-window"></i>
                  <span>export {form.env_name || "API_KEY"}=...</span>
                </div>
              ) : (
                <div className="field">
                  <label htmlFor="api-key">API key</label>
                  <input id="api-key" type="password" placeholder="sk-..." value={form.api_key} onChange={(event) => handleInputChange("api_key", event.target.value)} />
                </div>
              )}
            </section>

            <section className="panel-section compact">
              <div className="checkbox-field">
                <label className="checkbox-label">
                  <input id="activate-claude" type="checkbox" checked={form.activate_claude} onChange={(event) => handleInputChange("activate_claude", event.target.checked)} />
                  <span>Activate Claude Code after save</span>
                </label>
              </div>
            </section>

            <button className="advanced-toggle" type="button" onClick={() => setShowAdvanced(!showAdvanced)}>
              <i className={`ph-bold ph-caret-${showAdvanced ? "up" : "down"}`}></i>
              <span>Advanced mapping</span>
            </button>

            {showAdvanced && (
              <section className="panel-section advanced-fields">
                <div className="field-grid">
                  <div className="field">
                    <label htmlFor="provider-name">Provider name</label>
                    <input id="provider-name" type="text" list="provider-name-options" value={form.provider_name} onChange={(event) => handleInputChange("provider_name", event.target.value)} />
                    <datalist id="provider-name-options">
                      {providerNameOptions.map((option) => <option key={option} value={option} />)}
                    </datalist>
                  </div>

                  <div className="field">
                    <label htmlFor="provider-type">Protocol</label>
                    <select id="provider-type" value={form.type} onChange={(event) => handleInputChange("type", event.target.value)}>
                      {PROTOCOL_TYPES.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                    </select>
                  </div>

                  <div className="field span-2">
                    <label htmlFor="base-url">Base URL</label>
                    <input id="base-url" type="text" list="base-url-options" value={form.base_url} onChange={(event) => handleInputChange("base_url", event.target.value)} />
                    <datalist id="base-url-options">
                      {baseUrlOptions.map((option) => <option key={option} value={option} />)}
                    </datalist>
                  </div>

                  {form.api_key_mode === "env" && (
                    <div className="field">
                      <label htmlFor="env-name">Env name</label>
                      <input id="env-name" type="text" list="env-name-options" value={form.env_name} onChange={(event) => handleInputChange("env_name", event.target.value)} />
                      <datalist id="env-name-options">
                        {envNameOptions.map((option) => <option key={option} value={option} />)}
                      </datalist>
                    </div>
                  )}

                  <div className="field">
                    <label htmlFor="upstream-model">Upstream model</label>
                    <input id="upstream-model" type="text" list="upstream-model-options" value={form.upstream_model} onChange={(event) => handleInputChange("upstream_model", event.target.value)} />
                    <datalist id="upstream-model-options">
                      {upstreamModelOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                    </datalist>
                  </div>

                  <div className="field">
                    <label htmlFor="exposed-alias">Exposed alias</label>
                    <input id="exposed-alias" type="text" list="exposed-alias-options" value={form.exposed_alias} onChange={(event) => handleInputChange("exposed_alias", event.target.value)} />
                    <datalist id="exposed-alias-options">
                      {exposedAliasOptions.map((option) => <option key={option} value={option} />)}
                    </datalist>
                  </div>

                  <div className="field">
                    <label htmlFor="route-alias">Route alias</label>
                    <select id="route-alias" value={form.route_alias} onChange={(event) => handleInputChange("route_alias", event.target.value)}>
                      {ROUTE_ALIASES.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                    </select>
                  </div>
                </div>
              </section>
            )}

            <div className="actions">
              <button id="save-setup" type="button" onClick={handleSaveSetup} disabled={loading}>
                <i className="ph-bold ph-floppy-disk"></i>
                Save & Setup
              </button>
              <button id="setup-later" type="button" className="btn-secondary" onClick={handleSetupLater} disabled={loading}>
                Setup Later
              </button>
            </div>

            {status.text && <div className={`status-box ${status.type}`} id="status">{status.text}</div>}
          </form>
        </div>

        <div className={`tab-content ${activeTab === "providers" ? "active" : ""}`}>
          <PageHeader
            icon="ph-hard-drive"
            eyebrow="configuration"
            title="Configured Providers"
            description="Upstream services currently available to the router."
            stats={[{ label: "enabled", value: providerCount }]}
          />

          <div className="operator-grid">
            {providerCount > 0 ? (
              config.providers.map((provider) => <ProviderCard key={provider.id} provider={provider} />)
            ) : (
              <EmptyState icon="ph-database" title="No providers">Save a provider from Setup to enable routing.</EmptyState>
            )}
          </div>
        </div>

        <div className={`tab-content ${activeTab === "models" ? "active" : ""}`}>
          <PageHeader
            icon="ph-git-fork"
            eyebrow="topology"
            title="Models & Routes"
            description="Exposed model names, upstream targets, and route strategy."
            stats={[
              { label: "models", value: modelCount },
              { label: "routes", value: routeCount }
            ]}
          />

          <div className="operator-grid">
            <section className="operator-card">
              <div className="card-heading">
                <h3><i className="ph-light ph-cube"></i>Registered Models</h3>
              </div>
              <div className="stack-list">
                {modelCount > 0 ? (
                  config.models.map((model) => <ModelItem key={model.id} model={model} />)
                ) : (
                  <EmptyState icon="ph-cube" title="No models">Provider setup creates the first exposed model.</EmptyState>
                )}
              </div>
            </section>

            <section className="operator-card">
              <div className="card-heading">
                <h3><i className="ph-light ph-network"></i>Router Definitions</h3>
              </div>
              <div className="stack-list">
                {routeCount > 0 ? (
                  config.routes.map((route) => <RouteItem key={route.alias} route={route} />)
                ) : (
                  <EmptyState icon="ph-git-branch" title="No routes">Create a route alias during provider setup.</EmptyState>
                )}
              </div>
            </section>
          </div>
        </div>

        <div className={`tab-content ${activeTab === "logs" ? "active" : ""}`}>
          <PageHeader
            icon="ph-scroll"
            eyebrow="system log"
            title="Live Traces"
            description="Recent routing events from the local trace stream."
            stats={[{ label: "events", value: logs.length }]}
          />

          <section className="terminal-window">
            <div className="terminal-bar">
              <span></span>
              <span></span>
              <span></span>
              <strong>arkroute traces</strong>
            </div>
            <div className="log-stream">
              {logs.length > 0 ? (
                logs.map((item, index) => <LogLine item={item} key={`${item.time || "log"}-${index}`} />)
              ) : (
                <EmptyState icon="ph-pulse" title="Waiting for trace events">Start a client request to populate the stream.</EmptyState>
              )}
              <div ref={logsEndRef} />
            </div>
          </section>
        </div>

        <div className={`tab-content ${activeTab === "system" ? "active" : ""}`}>
          <PageHeader
            icon="ph-info"
            eyebrow="local gateway"
            title="System Overview"
            description="Runtime state and client activation commands."
            stats={[
              { label: "state", value: configState },
              { label: "routes", value: routeCount }
            ]}
          />

          <div className="operator-grid">
            <section className="operator-card span-2">
              <div className="card-heading">
                <div>
                  <StatusBadge tone={providerCount > 0 ? "ok" : "pending"}>{providerCount > 0 ? "Ready" : "Bootstrap"}</StatusBadge>
                  <h3><i className="ph-light ph-activity"></i>Local Gateway</h3>
                </div>
              </div>
              <div className="data-grid">
                <DataRow label="Providers">{providerCount}</DataRow>
                <DataRow label="Models">{modelCount}</DataRow>
                <DataRow label="Routes">{routeCount}</DataRow>
              </div>
            </section>

            <section className="operator-card">
              <div className="card-heading">
                <h3><i className="ph-light ph-terminal"></i>Claude Code</h3>
              </div>
              <div className="cmd-block">eval "$(arkroute activate claude)"</div>
            </section>

            <section className="operator-card">
              <div className="card-heading">
                <h3><i className="ph-light ph-book-open"></i>Resources</h3>
              </div>
              <ul className="doc-list">
                <li>
                  <a href="https://github.com/bloodstalk1/arkroute" target="_blank" rel="noreferrer">
                    <i className="ph-light ph-github-logo"></i>
                    GitHub Repository
                    <i className="ph-light ph-arrow-up-right"></i>
                  </a>
                </li>
              </ul>
            </section>
          </div>
        </div>
      </section>
    </main>
  );
}

export default App;
