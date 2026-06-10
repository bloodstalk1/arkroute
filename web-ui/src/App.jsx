import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  buildFetchModelsPayload,
  fetchModelsFailureStatus,
  modelFetchCacheKey,
  shouldAutoFetchModels,
} from "./modelFetch.js";
import {
  buildProviderSetupPayload,
  formFromPreset,
  formFromProvider,
  initialProviderForm,
  providerKeySummary,
  validateProviderForm,
} from "./providerSetup.js";


const setupToken = new URLSearchParams(window.location.hash.slice(1)).get("setup_token") || "";
const assetPath = (path) => `${import.meta.env.BASE_URL}${path}`;

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
  { id: "providers", icon: "ph-hard-drive", label: "Providers" },
  { id: "models", icon: "ph-git-fork", label: "Routes" },
  { id: "logs", icon: "ph-scroll", label: "Traces" },
  { id: "cli-tools", icon: "ph-terminal-window", label: "CLI Tools" },
  { id: "system", icon: "ph-cpu", label: "System" }
];

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

function PageHeader({ icon, eyebrow, title, description, stats = [], action = null }) {
  return (
    <header className="page-header">
      <div className="title-stack">
        <span className="eyebrow"><i className={`ph-fill ${icon}`}></i>{eyebrow}</span>
        <h1>{title}</h1>
        <p className="muted">{description}</p>
      </div>
      <div className="header-actions">
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
        {action}
      </div>
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
        <DataRow label="Key">{providerKeySummary(provider)}</DataRow>
      </div>
    </article>
  );
}

function ProviderSetupDrawer({
  open,
  form,
  errors,
  presets,
  loading,
  status,
  saveResult,
  showAdvanced,
  providerNameOptions,
  baseUrlOptions,
  envNameOptions,
  upstreamModelOptions,
  exposedAliasOptions,
  fetchingModels,
  fetchModelsStatus,
  deleteConfirmProviderId,
  onPresetChange,
  onInputChange,
  onSaveSetup,
  onClose,
  onToggleAdvanced,
  onFetchModels,
  onDeleteProvider,
  onDeleteConfirmChange,
}) {
  if (!open) return null;
  const isEdit = form.mode === "edit";
  return (
    <aside className="provider-drawer" aria-label={isEdit ? "Edit provider" : "Add provider"}>
      <div className="drawer-header">
        <div>
          <span className="eyebrow"><i className="ph-fill ph-plugs-connected"></i>{isEdit ? "edit upstream" : "new upstream"}</span>
          <h2>{isEdit ? "Edit provider" : "Add provider"}</h2>
          <p className="muted">Choose a preset, key source, model, route, and Claude activation behavior.</p>
        </div>
        <button className="icon-button" type="button" onClick={onClose} aria-label="Close provider drawer">
          <i className="ph-bold ph-x"></i>
        </button>
      </div>

      <div className="drawer-steps" aria-label="Setup steps">
        <span className="done">Provider</span>
        <span className={form.api_key_mode ? "done" : ""}>Key</span>
        <span className={form.upstream_model && form.route_alias ? "done" : ""}>Model</span>
        <span className={form.activate_claude ? "done" : ""}>CLI</span>
      </div>

      <form className="drawer-body" onSubmit={(event) => event.preventDefault()}>
        <section className="drawer-section">
          <div className="section-heading">
            <span>01</span>
            <div>
              <h3>Provider preset</h3>
              <p>Pick the upstream service Arkroute should route through.</p>
            </div>
          </div>
          <div className="field-grid">
            <div className={`field ${errors.preset_id ? "field-error" : ""}`}>
              <label htmlFor="preset">Provider</label>
              <select id="preset" value={form.preset_id} onChange={onPresetChange}>
                {presets.map((preset) => <option key={preset.id} value={preset.id}>{preset.name}</option>)}
              </select>
              {errors.preset_id && <small>{errors.preset_id}</small>}
            </div>
            <div className={`field ${errors.base_url ? "field-error" : ""}`}>
              <label htmlFor="base-url">Base URL</label>
              <input id="base-url" type="text" list="base-url-options" value={form.base_url} onChange={(event) => onInputChange("base_url", event.target.value)} />
              <datalist id="base-url-options">{baseUrlOptions.map((option) => <option key={option} value={option} />)}</datalist>
              {errors.base_url && <small>{errors.base_url}</small>}
            </div>
          </div>
        </section>

        <section className="drawer-section">
          <div className="section-heading">
            <span>02</span>
            <div>
              <h3>Key source</h3>
              <p>Use an environment variable for safer local setup, or store a key in config.</p>
            </div>
          </div>
          <div className="segmented-control">
            <button type="button" className={form.api_key_mode === "env" ? "active" : ""} onClick={() => onInputChange("api_key_mode", "env")}>Environment</button>
            <button type="button" className={form.api_key_mode === "config" ? "active" : ""} onClick={() => onInputChange("api_key_mode", "config")}>Config</button>
          </div>
          {form.api_key_mode === "env" ? (
            <div className={`field ${errors.env_name ? "field-error" : ""}`}>
              <label htmlFor="env-name">Environment variable</label>
              <input id="env-name" type="text" list="env-name-options" value={form.env_name} onChange={(event) => onInputChange("env_name", event.target.value)} />
              <datalist id="env-name-options">{envNameOptions.map((option) => <option key={option} value={option} />)}</datalist>
              <div className="terminal-note"><i className="ph-light ph-terminal-window"></i><span>export {form.env_name || "API_KEY"}=...</span></div>
              {errors.env_name && <small>{errors.env_name}</small>}
            </div>
          ) : (
            <div className={`field ${errors.api_key ? "field-error" : ""}`}>
              <label htmlFor="api-key">API key</label>
              <input id="api-key" type="password" aria-describedby="api-key-help" value={form.api_key} onChange={(event) => onInputChange("api_key", event.target.value)} />
              <small id="api-key-help">{isEdit ? "Leave blank to keep the existing config key." : "Enter the provider API key."}</small>
              {errors.api_key && <small>{errors.api_key}</small>}
            </div>
          )}
        </section>

        <section className="drawer-section">
          <div className="section-heading">
            <span>03</span>
            <div>
              <h3>Model and route</h3>
              <p>Choose the upstream model and the local name clients will request.</p>
            </div>
          </div>
          <div className="field-grid">
            <div className={`field ${errors.upstream_model ? "field-error" : ""}`}>
              <div className="field-label-row">
                <label htmlFor="upstream-model">Upstream model</label>
                <button type="button" className="btn-tertiary compact-action" onClick={onFetchModels} disabled={fetchingModels || !form.preset_id || !form.base_url}>
                  <i className="ph-bold ph-arrows-clockwise"></i>{fetchingModels ? "Fetching" : "Fetch live"}
                </button>
              </div>
              <select id="upstream-model" value={form.upstream_model || ""} onChange={(event) => onInputChange("upstream_model", event.target.value)}>
                <option value="" disabled>Pick a model</option>
                {upstreamModelOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
              <input className="custom-model-input" type="text" aria-label="Custom upstream model ID" value={form.upstream_model} onChange={(event) => onInputChange("upstream_model", event.target.value)} />
              {fetchModelsStatus?.text && <small className={`status-inline status-${fetchModelsStatus.type}`}>{fetchModelsStatus.text}</small>}
              {errors.upstream_model && <small>{errors.upstream_model}</small>}
            </div>
            <div className={`field ${errors.exposed_alias ? "field-error" : ""}`}>
              <label htmlFor="exposed-alias">Client model name</label>
              <input id="exposed-alias" type="text" list="exposed-alias-options" value={form.exposed_alias} onChange={(event) => onInputChange("exposed_alias", event.target.value)} />
              <datalist id="exposed-alias-options">{exposedAliasOptions.map((option) => <option key={option} value={option} />)}</datalist>
              {errors.exposed_alias && <small>{errors.exposed_alias}</small>}
            </div>
            <div className={`field ${errors.route_alias ? "field-error" : ""}`}>
              <label htmlFor="route-alias">Route alias</label>
              <select id="route-alias" value={form.route_alias} onChange={(event) => onInputChange("route_alias", event.target.value)}>
                {ROUTE_ALIASES.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
              {errors.route_alias && <small>{errors.route_alias}</small>}
            </div>
          </div>
        </section>

        <section className="drawer-section compact">
          <label className="checkbox-label">
            <input id="activate-claude" type="checkbox" checked={form.activate_claude} onChange={(event) => onInputChange("activate_claude", event.target.checked)} />
            <span>Activate Claude Code after save</span>
          </label>
        </section>

        <button className="advanced-toggle" type="button" onClick={onToggleAdvanced}>
          <i className={`ph-bold ph-caret-${showAdvanced ? "up" : "down"}`}></i>
          <span>Advanced mapping</span>
        </button>

        {showAdvanced && (
          <section className="drawer-section advanced-fields">
            <div className="field-grid">
              <div className="field">
                <label htmlFor="provider-name">Provider name</label>
                <input id="provider-name" type="text" list="provider-name-options" value={form.provider_name} onChange={(event) => onInputChange("provider_name", event.target.value)} />
                <datalist id="provider-name-options">{providerNameOptions.map((option) => <option key={option} value={option} />)}</datalist>
              </div>
              <div className="field">
                <label htmlFor="provider-type">Protocol</label>
                <select id="provider-type" value={form.type} onChange={(event) => onInputChange("type", event.target.value)}>
                  {PROTOCOL_TYPES.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                </select>
              </div>
            </div>
          </section>
        )}

        {saveResult?.text && <div className={`status-box ${saveResult.type}`}>{saveResult.text}</div>}
        {status.text && !saveResult?.text && <div className={`status-box ${status.type}`}>{status.text}</div>}
      </form>

      <div className="drawer-actions">
        {isEdit && (
          deleteConfirmProviderId === form.preset_id ? (
            <button className="btn-danger" type="button" onClick={() => onDeleteProvider(form.preset_id)} disabled={loading}>Confirm remove</button>
          ) : (
            <button className="btn-danger subtle" type="button" onClick={() => onDeleteConfirmChange(form.preset_id)} disabled={loading}>Remove provider</button>
          )
        )}
        <div className="drawer-action-spacer"></div>
        <button type="button" className="btn-secondary" onClick={onClose} disabled={loading}>Cancel</button>
        <button id="save-setup" type="button" onClick={onSaveSetup} disabled={loading}>
          <i className="ph-bold ph-floppy-disk"></i>{isEdit ? "Save changes" : "Save provider"}
        </button>
      </div>
    </aside>
  );
}

function ModelItem({ model, active, onSelect }) {
  return (
    <button className={`list-item selectable-list-item ${active ? "active" : ""}`} type="button" onClick={() => onSelect(model.id)}>
      <div>
        <strong>{model.display_name || model.id}</strong>
        <span>{model.provider_id}</span>
      </div>
      <code>{model.upstream_model}</code>
    </button>
  );
}

function RouteItem({ route, selectedModelId, onSelectModel }) {
  return (
    <div className="list-item route-item">
      <div>
        <strong>{route.alias}</strong>
        <span>{route.strategy}</span>
      </div>
      <div className="target-list">
        {(route.targets || []).map((target, index) => (
          <button
            className={`${target.enabled ? "target enabled" : "target"} ${selectedModelId === target.model_id ? "selected" : ""}`}
            key={`${target.model_id}-${index}`}
            type="button"
            onClick={() => onSelectModel(target.model_id)}
          >
            {index + 1}. {target.model_id}
          </button>
        ))}
      </div>
    </div>
  );
}

function PolicyValue({ label, value, source }) {
  const renderedValue = typeof value === "boolean" ? (value ? "true" : "false") : (value || "unset");
  return (
    <div className="policy-value">
      <span>{label}</span>
      <strong>{renderedValue}</strong>
      {source && <small>{source.policy_id || source.source}</small>}
    </div>
  );
}

function PolicyInspector({ inspection, loading, status, apiHeaders, onOverrideChanged }) {
  const [overrideDraft, setOverrideDraft] = useState({
    auto_enable: "unset",
    auto_effort: "unset",
    replay: "unset",
    omit_tool_choice: "unset"
  });
  const [overrideSaving, setOverrideSaving] = useState(false);
  const [overrideStatus, setOverrideStatus] = useState({ text: "", type: "" });

  useEffect(() => {
    if (!inspection) return;
    const override = inspection.user_override || {};
    setOverrideDraft({
      auto_enable: override.auto_enable === true ? "true" : override.auto_enable === false ? "false" : "unset",
      auto_effort: override.auto_effort || "unset",
      replay: override.replay === true ? "true" : override.replay === false ? "false" : "unset",
      omit_tool_choice: override.omit_tool_choice === true ? "true" : override.omit_tool_choice === false ? "false" : "unset"
    });
    setOverrideStatus({ text: "", type: "" });
  }, [inspection]);

  const handleSaveOverride = async () => {
    setOverrideSaving(true);
    setOverrideStatus({ text: "Saving override...", type: "" });
    try {
      const payload = {
        model_id: inspection.model_id,
        auto_enable: overrideDraft.auto_enable === "true" ? true : overrideDraft.auto_enable === "false" ? false : null,
        auto_effort: overrideDraft.auto_effort === "unset" ? "" : overrideDraft.auto_effort,
        replay: overrideDraft.replay === "true" ? true : overrideDraft.replay === "false" ? false : null,
        omit_tool_choice: overrideDraft.omit_tool_choice === "true" ? true : overrideDraft.omit_tool_choice === "false" ? false : null
      };
      const response = await fetch("/internal/policy/override", {
        method: "PUT",
        headers: apiHeaders,
        body: JSON.stringify(payload)
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        setOverrideStatus({ text: data.error || "Save override failed", type: "error" });
        return;
      }
      setOverrideStatus({ text: "Override saved successfully.", type: "ok" });
      if (onOverrideChanged) {
        onOverrideChanged(inspection.model_id);
      }
    } catch (err) {
      setOverrideStatus({ text: err.message, type: "error" });
    } finally {
      setOverrideSaving(false);
    }
  };

  const handleResetToBuiltin = async () => {
    setOverrideSaving(true);
    setOverrideStatus({ text: "Resetting override...", type: "" });
    try {
      const response = await fetch(`/internal/policy/override?model_id=${encodeURIComponent(inspection.model_id)}`, {
        method: "DELETE",
        headers: apiHeaders
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        setOverrideStatus({ text: data.error || "Reset failed", type: "error" });
        return;
      }
      setOverrideStatus({ text: "Override reset to builtin successfully.", type: "ok" });
      if (onOverrideChanged) {
        onOverrideChanged(inspection.model_id);
      }
    } catch (err) {
      setOverrideStatus({ text: err.message, type: "error" });
    } finally {
      setOverrideSaving(false);
    }
  };

  if (loading) {
    return <EmptyState icon="ph-shield-checkered" title="Inspecting policy">Reading local config and policy matches.</EmptyState>;
  }
  if (status.text) {
    return <div className={`status-box ${status.type}`}>{status.text}</div>;
  }
  if (!inspection) {
    return <EmptyState icon="ph-shield-checkered" title="No model selected">Select a registered model or route target.</EmptyState>;
  }
  const reasoning = inspection.resolved_reasoning || {};
  const sources = inspection.reasoning_sources || {};
  return (
    <section className="operator-card policy-inspector-card">
      <div className="card-heading">
        <div>
          <StatusBadge tone={inspection.matched_policies?.length > 0 ? "ok" : "pending"}>
            {inspection.matched_policies?.length || 0} policies
          </StatusBadge>
          <h3><i className="ph-light ph-shield-checkered"></i>Policy Inspector</h3>
        </div>
      </div>

      <div className="policy-summary-grid">
        <DataRow label="Model">{inspection.model_id}</DataRow>
        <DataRow label="Provider">{inspection.provider_id}</DataRow>
        <DataRow label="Provider type">{inspection.provider_type || "auto"}</DataRow>
        <DataRow label="Protocol">{inspection.protocol}</DataRow>
        <DataRow label="Upstream">{inspection.upstream_model}</DataRow>
      </div>

      <div className="policy-chip-row">
        {(inspection.matched_policies || []).length > 0 ? (
          inspection.matched_policies.map((policy) => (
            <span className={`policy-chip ${policy.source} ${policy.source === 'user' ? 'user' : 'builtin'}`} key={`${policy.source}-${policy.id}`}>{policy.source}: {policy.id}</span>
          ))
        ) : (
          <span className="policy-chip muted">no compatibility policy matched</span>
        )}
      </div>

      <div className="policy-value-grid">
        <PolicyValue label="enabled" value={reasoning.enabled} source={sources.enabled} />
        <PolicyValue label="effort" value={reasoning.effort} source={sources.effort} />
        <PolicyValue label="auto_enable" value={reasoning.auto_enable} source={sources.auto_enable} />
        <PolicyValue label="auto_effort" value={reasoning.auto_effort} source={sources.auto_effort} />
        <PolicyValue label="replay" value={reasoning.replay} source={sources.replay} />
        <PolicyValue label="omit_tool_choice" value={reasoning.omit_tool_choice} source={sources.omit_tool_choice} />
        <PolicyValue label="follow_claude_effort" value={reasoning.follow_claude_effort} source={sources.follow_claude_effort} />
      </div>

      {(inspection.explain || []).length > 0 && (
        <div className="policy-explain-list">
          {inspection.explain.map((line, index) => <span key={`${line}-${index}`}>{line}</span>)}
        </div>
      )}

      <div className="policy-override-editor">
        <h4>
          <i className="ph-bold ph-pencil-simple-line"></i>
          Compatibility Policy Override
        </h4>

        <div className="field-grid policy-override-grid">
          <div className="field">
            <label htmlFor="override-auto-enable">auto_enable</label>
            <select
              id="override-auto-enable"
              value={overrideDraft.auto_enable}
              onChange={(e) => setOverrideDraft(prev => ({ ...prev, auto_enable: e.target.value }))}
            >
              <option value="unset">unset (default)</option>
              <option value="true">true (force enable)</option>
              <option value="false">false (force disable)</option>
            </select>
          </div>

          <div className="field">
            <label htmlFor="override-auto-effort">auto_effort</label>
            <select
              id="override-auto-effort"
              value={overrideDraft.auto_effort}
              onChange={(e) => setOverrideDraft(prev => ({ ...prev, auto_effort: e.target.value }))}
            >
              <option value="unset">unset (default)</option>
              <option value="low">low</option>
              <option value="medium">medium</option>
              <option value="high">high</option>
              <option value="max">max</option>
            </select>
          </div>

          <div className="field">
            <label htmlFor="override-replay">replay</label>
            <select
              id="override-replay"
              value={overrideDraft.replay}
              onChange={(e) => setOverrideDraft(prev => ({ ...prev, replay: e.target.value }))}
            >
              <option value="unset">unset (default)</option>
              <option value="true">true (force enable)</option>
              <option value="false">false (force disable)</option>
            </select>
          </div>

          <div className="field">
            <label htmlFor="override-omit-tool-choice">omit_tool_choice</label>
            <select
              id="override-omit-tool-choice"
              value={overrideDraft.omit_tool_choice}
              onChange={(e) => setOverrideDraft(prev => ({ ...prev, omit_tool_choice: e.target.value }))}
            >
              <option value="unset">unset (default)</option>
              <option value="true">true (force enable)</option>
              <option value="false">false (force disable)</option>
            </select>
          </div>
        </div>

        <div className="actions policy-override-actions">
          <button
            id="save-policy-override"
            type="button"
            onClick={handleSaveOverride}
            disabled={overrideSaving}
          >
            <i className="ph-bold ph-floppy-disk"></i>
            Save override
          </button>

          {inspection.user_override?.exists && (
            <button
              id="reset-policy-override"
              type="button"
              className="btn-secondary"
              onClick={handleResetToBuiltin}
              disabled={overrideSaving}
            >
              Reset to builtin
            </button>
          )}
        </div>

        {overrideStatus.text && (
          <div className={`status-box ${overrideStatus.type} policy-override-status`}>
            {overrideStatus.text}
          </div>
        )}
      </div>
    </section>
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

function ProviderDetail({ provider, models, routes, onSelectModel, onSelectRoute }) {
  if (!provider) {
    return <EmptyState icon="ph-hard-drive" title="No provider selected">Choose a configured provider.</EmptyState>;
  }
  const providerModels = models.filter((model) => model.provider_id === provider.id);
  return (
    <section className="operator-card detail-panel">
      <div className="card-heading">
        <div>
          <StatusBadge tone={provider.enabled ? "ok" : "pending"}>{provider.type || "auto"}</StatusBadge>
          <h3><i className="ph-light ph-hard-drive"></i>{provider.name || provider.id}</h3>
        </div>
      </div>
      <div className="policy-summary-grid provider-detail-summary">
        <DataRow label="Provider ID">{provider.id}</DataRow>
        <DataRow label="Base URL">{provider.base_url}</DataRow>
        <DataRow label="Models">{providerModels.length}</DataRow>
      </div>
      <div className="context-list provider-detail-context">
        <strong className="eyebrow context-list-title">Exposed Models</strong>
        {providerModels.map((model) => (
          <button type="button" key={model.id} onClick={() => onSelectModel(model.id)}>
            <span>{model.exposed_alias || model.id}</span>
            <code>{model.upstream_model}</code>
          </button>
        ))}
      </div>
      <div className="context-list provider-detail-context">
        <strong className="eyebrow context-list-title">Associated Routes</strong>
        {routes.map((route) => (
          <button type="button" key={route.alias} onClick={() => onSelectRoute(route.alias)}>
            <span>{route.alias}</span>
            <code>{route.strategy}</code>
          </button>
        ))}
      </div>
    </section>
  );
}

function CLIContextPanel({ context, status, onCopy }) {
  if (status.text) {
    return <div className={`status-box ${status.type}`}>{status.text}</div>;
  }
  if (!context) {
    return <EmptyState icon="ph-terminal-window" title="No CLI context">Select a model or route.</EmptyState>;
  }
  return (
    <section className="operator-card cli-context-card">
      <div className="card-heading">
        <div>
          <StatusBadge tone="ok">{context.selected_alias}</StatusBadge>
          <h3><i className="ph-light ph-terminal-window"></i>CLI Setup</h3>
        </div>
      </div>
      <div className="cli-context-grid">
        {(context.profiles || []).map((profile) => (
          <article className="cli-context-profile" key={profile.id}>
            <div className="cli-context-title">
              <strong>{profile.name}</strong>
              <code>{profile.protocol}</code>
            </div>
            <pre>{profile.command}</pre>
            <button type="button" className="btn-secondary" onClick={() => onCopy(profile.command)}>
              <i className="ph-bold ph-copy"></i>Copy
            </button>
          </article>
        ))}
      </div>
    </section>
  );
}

function RoutePresetPanel({ presets, status, onApply }) {
  return (
    <section className="operator-card route-presets-card">
      <div className="card-heading">
        <div>
          <StatusBadge tone={presets.length > 0 ? "ok" : "pending"}>{presets.length || "loading"}</StatusBadge>
          <h3><i className="ph-light ph-stack-plus"></i>Route Presets</h3>
        </div>
      </div>
      <div className="preset-grid">
        {presets.map((preset) => (
          <button type="button" className="route-preset-card" key={preset.id} onClick={() => onApply(preset)}>
            <span>{preset.name}</span>
            <code>{preset.default_alias} {"->"} {preset.upstream_model}</code>
          </button>
        ))}
      </div>
      {status.text && <div className={`status-box ${status.type}`}>{status.text}</div>}
    </section>
  );
}

function App() {
  const [activeTab, setActiveTab] = useState("providers");
  const [presets, setPresets] = useState([]);
  const [loading, setLoading] = useState(false);
  const [status, setStatus] = useState({ text: "Loading provider presets...", type: "" });
  const [config, setConfig] = useState(null);
  const [logs, setLogs] = useState([]);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const logsEndRef = useRef(null);

  const [catalogProviders, setCatalogProviders] = useState({});
  const [fetchedModels, setFetchedModels] = useState({});
  const [fetchingModels, setFetchingModels] = useState(false);
  const [fetchModelsStatus, setFetchModelsStatus] = useState({ text: "", type: "" });
  const modelFetchCacheRef = useRef(new Set());
  const modelFetchInFlightRef = useRef("");

  const [cliTools, setCliTools] = useState([]);
  const [cliToolsStatus, setCliToolsStatus] = useState({ text: "", type: "" });
  const [launchingTool, setLaunchingTool] = useState("");
  const [selectedModelId, setSelectedModelId] = useState("");
  const [selectedProviderId, setSelectedProviderId] = useState("");
  const [selectedRouteAlias, setSelectedRouteAlias] = useState("");
  const [cliContext, setCliContext] = useState(null);
  const [cliContextStatus, setCliContextStatus] = useState({ text: "", type: "" });
  const [routePresets, setRoutePresets] = useState([]);
  const [routePresetStatus, setRoutePresetStatus] = useState({ text: "", type: "" });
  const [policyInspect, setPolicyInspect] = useState(null);
  const [policyInspectLoading, setPolicyInspectLoading] = useState(false);
  const [policyInspectStatus, setPolicyInspectStatus] = useState({ text: "", type: "" });

  const [configDraft, setConfigDraft] = useState("");
  const [configTransferStatus, setConfigTransferStatus] = useState({ text: "", type: "" });
  const [configImportSummary, setConfigImportSummary] = useState(null);

  const [form, setForm] = useState(initialProviderForm);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [formErrors, setFormErrors] = useState({});
  const [saveResult, setSaveResult] = useState(null);
  const [deleteConfirmProviderId, setDeleteConfirmProviderId] = useState("");

  const apiHeaders = useMemo(() => ({
    "Content-Type": "application/json",
    "X-Arkroute-Setup-Token": setupToken
  }), []);

  const providerCount = config?.providers?.length || 0;
  const modelCount = config?.models?.length || 0;
  const routeCount = config?.routes?.length || 0;
  const configState = providerCount > 0 ? "Configured" : "Bootstrap";

  const fillPreset = useCallback((preset) => {
    setForm((prev) => formFromPreset(preset, prev));
    setFormErrors({});
    setSaveResult(null);
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

  const loadCliTools = useCallback((cancelled = () => false) => {
    return fetch("/internal/cli-tools", { headers: apiHeaders })
      .then((resp) => (resp.ok ? resp.json() : resp.json().then((data) => Promise.reject(new Error(data.error || resp.statusText)))))
      .then((data) => {
        if (!cancelled()) {
          setCliTools(data.tools || []);
        }
      })
      .catch((err) => {
        if (!cancelled()) {
          setCliToolsStatus({ text: err.message, type: "error" });
        }
      });
  }, [apiHeaders]);

  const loadRoutePresets = useCallback((cancelled = () => false) => {
    return fetch("/internal/route-presets", { headers: apiHeaders })
      .then((resp) => resp.ok ? resp.json() : resp.json().then((payload) => Promise.reject(new Error(payload.error || resp.statusText))))
      .then((payload) => {
        if (!cancelled()) {
          setRoutePresets(payload.presets || []);
        }
      })
      .catch((err) => {
        if (!cancelled()) {
          setRoutePresetStatus({ text: err.message, type: "error" });
        }
      });
  }, [apiHeaders]);

  const loadCLIContext = useCallback((selection, cancelled = () => false) => {
    const params = new URLSearchParams();
    if (selection.route_alias) params.set("route_alias", selection.route_alias);
    if (selection.model_id) params.set("model_id", selection.model_id);
    if (!params.toString()) return Promise.resolve();
    setCliContextStatus({ text: "", type: "" });
    return fetch(`/internal/cli-context?${params.toString()}`, { headers: apiHeaders })
      .then((resp) => resp.ok ? resp.json() : resp.json().then((payload) => Promise.reject(new Error(payload.error || resp.statusText))))
      .then((payload) => {
        if (!cancelled()) {
          setCliContext(payload);
        }
      })
      .catch((err) => {
        if (!cancelled()) {
          setCliContext(null);
          setCliContextStatus({ text: err.message, type: "error" });
        }
      });
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

    fetch("/internal/setup/catalog", { headers: apiHeaders })
      .then((resp) => (resp.ok ? resp.json() : null))
      .then((data) => {
        if (cancelled || !data) return;
        setCatalogProviders(data.providers || {});
      })
      .catch(() => {
        // catalog fetch failure is non-fatal: dropdown falls back to "Other"
      });

    loadStatus(isCancelled);
    return () => {
      cancelled = true;
    };
  }, [apiHeaders, fillPreset, loadStatus]);

  const fetchLiveModels = useCallback(async ({ automatic = false, force = false } = {}) => {
    if (!shouldAutoFetchModels(form)) {
      if (!automatic) {
        setFetchModelsStatus({
          text: form.preset_id ? "Set a base URL first." : "Select a provider first.",
          type: "error",
        });
      }
      return;
    }

    const cacheKey = modelFetchCacheKey(form);
    if (!force && modelFetchCacheRef.current.has(cacheKey)) {
      return;
    }
    if (modelFetchInFlightRef.current === cacheKey) {
      return;
    }

    modelFetchInFlightRef.current = cacheKey;
    setFetchingModels(true);
    setFetchModelsStatus({ text: automatic ? "Loading live model list…" : "Fetching live model list…", type: "info" });

    const isCurrentRequest = () => modelFetchInFlightRef.current === cacheKey;
    try {
      const resp = await fetch("/internal/setup/fetch-models", {
        method: "POST",
        headers: { ...apiHeaders, "Content-Type": "application/json" },
        body: JSON.stringify(buildFetchModelsPayload(form)),
      });
      const data = await resp.json().catch(() => ({}));
      if (!resp.ok) {
        if (isCurrentRequest()) {
          setFetchModelsStatus(fetchModelsFailureStatus(data, resp.status, { automatic }));
        }
        return;
      }
      const liveModels = (data.fetched && data.fetched.models) || [];
      modelFetchCacheRef.current.add(cacheKey);
      setFetchedModels((prev) => ({ ...prev, [form.preset_id]: liveModels }));
      if (isCurrentRequest()) {
        setFetchModelsStatus({
          text: `Loaded ${liveModels.length} live model${liveModels.length === 1 ? "" : "s"} from upstream.`,
          type: "ok",
        });
      }
    } catch (err) {
      if (isCurrentRequest()) {
        setFetchModelsStatus({ text: `Fetch error: ${err.message}`, type: "error" });
      }
    } finally {
      if (isCurrentRequest()) {
        modelFetchInFlightRef.current = "";
        setFetchingModels(false);
      }
    }
  }, [apiHeaders, form.preset_id, form.base_url, form.api_key, form.api_key_mode, form.env_name, form.type]);

  useEffect(() => {
    if (!shouldAutoFetchModels(form)) {
      return undefined;
    }

    const timer = setTimeout(() => {
      fetchLiveModels({ automatic: true });
    }, 500);
    return () => clearTimeout(timer);
  }, [
    fetchLiveModels,
    form.preset_id,
    form.base_url,
    form.api_key,
    form.api_key_mode,
    form.env_name,
    form.type,
  ]);

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
    if (activeTab === "cli-tools") {
      loadCliTools(isCancelled);
    }
    if (activeTab === "models") {
      loadRoutePresets(isCancelled);
      if (selectedRouteAlias) {
        loadCLIContext({ route_alias: selectedRouteAlias }, isCancelled);
      } else if (selectedModelId) {
        loadCLIContext({ model_id: selectedModelId }, isCancelled);
      }
    }
    return () => {
      cancelled = true;
    };
  }, [activeTab, loadLogs, loadStatus, loadCliTools, loadRoutePresets, loadCLIContext, selectedRouteAlias, selectedModelId]);

  useEffect(() => {
    if (activeTab === "logs" && logsEndRef.current) {
      logsEndRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [logs, activeTab]);

  useEffect(() => {
    const models = config?.models || [];
    if (models.length === 0) {
      setSelectedModelId("");
      setPolicyInspect(null);
      return;
    }
    if (!selectedModelId || !models.some((model) => model.id === selectedModelId)) {
      setSelectedModelId(models[0].id);
    }
  }, [config, selectedModelId]);

  useEffect(() => {
    const providers = config?.providers || [];
    if (providers.length === 0) {
      setSelectedProviderId("");
      return;
    }
    if (!selectedProviderId || !providers.some((provider) => provider.id === selectedProviderId)) {
      setSelectedProviderId(providers[0].id);
    }
  }, [config, selectedProviderId]);

  useEffect(() => {
    const routes = config?.routes || [];
    if (routes.length === 0) {
      setSelectedRouteAlias("");
      return;
    }
    if (!selectedRouteAlias || !routes.some((route) => route.alias === selectedRouteAlias)) {
      setSelectedRouteAlias(routes[0].alias);
    }
  }, [config, selectedRouteAlias]);

  const loadPolicyInspect = useCallback((modelId, cancelled = () => false) => {
    if (!modelId) return;
    setPolicyInspectLoading(true);
    setPolicyInspectStatus({ text: "", type: "" });
    fetch(`/internal/policy/inspect?model_id=${encodeURIComponent(modelId)}`, { headers: apiHeaders })
      .then((resp) => resp.ok ? resp.json() : resp.json().then((payload) => Promise.reject(new Error(payload.error || resp.statusText))))
      .then((payload) => {
        if (!cancelled()) {
          setPolicyInspect(payload);
        }
      })
      .catch((err) => {
        if (!cancelled()) {
          setPolicyInspect(null);
          setPolicyInspectStatus({ text: err.message, type: "error" });
        }
      })
      .finally(() => {
        if (!cancelled()) {
          setPolicyInspectLoading(false);
        }
      });
  }, [apiHeaders]);

  useEffect(() => {
    if (activeTab !== "models" || !selectedModelId) {
      return;
    }
    let cancelled = false;
    loadPolicyInspect(selectedModelId, () => cancelled);
    return () => {
      cancelled = true;
    };
  }, [activeTab, selectedModelId, loadPolicyInspect]);

  const handleOverrideChanged = useCallback((modelId) => {
    loadStatus();
    loadPolicyInspect(modelId);
  }, [loadStatus, loadPolicyInspect]);

  const providerNameOptions = useMemo(() => {
    const list = new Set();
    const activePreset = presets.find((p) => p.id === form.preset_id);
    if (activePreset) list.add(activePreset.name);
    presets.forEach((p) => list.add(p.name));
    ["OpenRouter", "Anthropic", "Gemini", "OpenAI-compatible", "OpenCode Go", "OpenCode Zen", "Custom"].forEach((name) => list.add(name));
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
      "https://opencode.ai/zen/go",
      "https://opencode.ai/zen/v1"
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
    const seen = new Set();
    const push = (value, label) => {
      if (!value || seen.has(value)) return;
      seen.add(value);
      list.push({ value, label: label || value });
    };
    const activePreset = presets.find((p) => p.id === form.preset_id);
    if (activePreset?.default_model) {
      push(activePreset.default_model, `${activePreset.default_model} (Preset Default)`);
    }
    // Live-fetched models take priority (most up-to-date).
    (fetchedModels[form.preset_id] || []).forEach((m) => push(m.id, m.label));
    // Curated catalog from /internal/setup/catalog.
    (catalogProviders[form.preset_id]?.models || []).forEach((m) => {
      const label = m.default ? `${m.id} (Recommended)` : m.id;
      push(m.id, label);
    });
    // Final fallback: curated catalog across all known providers.
    if (list.length === 0) {
      Object.values(catalogProviders).forEach((provider) => {
        (provider.models || []).forEach((m) => push(m.id, m.label));
      });
    }
    return list;
  }, [form.preset_id, presets, catalogProviders, fetchedModels]);

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

  const selectPresetById = useCallback((selectedId) => {
    const preset = presets.find((item) => item.id === selectedId);
    if (preset) {
      fillPreset(preset);
    } else {
      setForm((prev) => ({ ...prev, preset_id: selectedId }));
    }
  }, [fillPreset, presets]);

  const handlePresetChange = (event) => {
    selectPresetById(event.target.value);
  };

  const openAddProvider = () => {
    const firstPreset = presets[0];
    setForm(firstPreset ? formFromPreset(firstPreset, initialProviderForm()) : initialProviderForm());
    setFormErrors({});
    setSaveResult(null);
    setShowAdvanced(false);
    setDrawerOpen(true);
  };

  const openEditProvider = (provider) => {
    setForm(formFromProvider(provider, config?.models || [], config?.routes || [], presets));
    setFormErrors({});
    setSaveResult(null);
    setShowAdvanced(false);
    setDrawerOpen(true);
  };

  const closeProviderDrawer = () => {
    setDrawerOpen(false);
    setFormErrors({});
    setSaveResult(null);
    setDeleteConfirmProviderId("");
  };

  const handleInputChange = (field, value) => {
    setForm((prev) => ({ ...prev, [field]: value }));
  };

  const downloadConfig = async (redacted) => {
    setConfigTransferStatus({ text: "", type: "" });
    const response = await fetch(`/internal/config/export?redacted=${redacted ? "1" : "0"}`, {
      headers: apiHeaders
    });
    const text = await response.text();
    if (!response.ok) {
      setConfigTransferStatus({ text: text || "Export failed", type: "error" });
      return;
    }
    const blob = new Blob([text], { type: "application/x-yaml;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = redacted ? "arkroute-config-redacted.yaml" : "arkroute-config.yaml";
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    URL.revokeObjectURL(url);
    setConfigTransferStatus({ text: redacted ? "Redacted config exported" : "Config exported", type: "ok" });
  };

  const copyRedactedConfig = async () => {
    setConfigTransferStatus({ text: "", type: "" });
    const response = await fetch("/internal/config/export?redacted=1", {
      headers: apiHeaders
    });
    const text = await response.text();
    if (!response.ok) {
      setConfigTransferStatus({ text: text || "Copy failed", type: "error" });
      return;
    }
    await navigator.clipboard.writeText(text);
    setConfigTransferStatus({ text: "Redacted config copied", type: "ok" });
  };

  const validateConfigDraft = async () => {
    setConfigTransferStatus({ text: "", type: "" });
    setConfigImportSummary(null);
    const response = await fetch("/internal/config/import/validate", {
      method: "POST",
      headers: { ...apiHeaders, "Content-Type": "application/json" },
      body: JSON.stringify({ yaml: configDraft })
    });
    const result = await response.json().catch(() => ({}));
    if (!response.ok) {
      setConfigTransferStatus({ text: result.error || "Config validation failed", type: "error" });
      return;
    }
    setConfigImportSummary(result.summary || null);
    setConfigTransferStatus({ text: "Config is valid", type: "ok" });
  };

  const applyConfigDraft = async () => {
    setConfigTransferStatus({ text: "", type: "" });
    const response = await fetch("/internal/config/import/apply", {
      method: "POST",
      headers: { ...apiHeaders, "Content-Type": "application/json" },
      body: JSON.stringify({ yaml: configDraft })
    });
    const result = await response.json().catch(() => ({}));
    if (!response.ok) {
      setConfigTransferStatus({ text: result.error || "Import failed", type: "error" });
      return;
    }
    setConfig(result.config);
    setConfigImportSummary(result.summary || null);
    setConfigTransferStatus({
      text: result.backup_path ? `Config imported, backup: ${result.backup_path}` : "Config imported",
      type: "ok"
    });
    loadStatus();
  };

  const handleSaveSetup = async () => {
    const errors = validateProviderForm(form);
    setFormErrors(errors);
    setSaveResult(null);
    if (Object.keys(errors).length > 0) {
      setStatus({ text: "Fix the highlighted fields before saving.", type: "error" });
      return;
    }

    setLoading(true);
    setStatus({ text: form.mode === "edit" ? "Saving provider changes..." : "Saving provider configuration...", type: "" });
    try {
      const resp = await fetch("/internal/setup/provider", {
        method: "POST",
        headers: apiHeaders,
        body: JSON.stringify(buildProviderSetupPayload(form)),
      });
      const data = await resp.json();
      if (!resp.ok) {
        setStatus({ text: data.error || resp.statusText, type: "error" });
        return;
      }

      let msg = form.mode === "edit" ? "Provider changes saved." : "Provider saved.";
      let isErr = false;
      if (data.claude_activated) {
        msg += " Claude Code activated.";
      } else if (form.activate_claude) {
        msg += ` Claude activation failed: ${data.claude_error || "unknown error"}.`;
        isErr = true;
      }
      setConfig(data.config);
      setSaveResult({ text: msg, type: isErr ? "error" : "ok" });
      setStatus({ text: msg, type: isErr ? "error" : "ok" });
      loadStatus();
    } catch (err) {
      setStatus({ text: `Request failed: ${err.message}`, type: "error" });
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteProvider = async (providerId) => {
    setLoading(true);
    setStatus({ text: "Removing provider...", type: "" });
    try {
      const resp = await fetch(`/internal/setup/provider?id=${encodeURIComponent(providerId)}`, {
        method: "DELETE",
        headers: apiHeaders,
      });
      const data = await resp.json().catch(() => ({}));
      if (!resp.ok) {
        setStatus({ text: data.error || "Remove failed", type: "error" });
        return;
      }
      setConfig(data.config);
      setStatus({ text: "Provider removed.", type: "ok" });
      closeProviderDrawer();
      loadStatus();
    } catch (err) {
      setStatus({ text: `Remove failed: ${err.message}`, type: "error" });
    } finally {
      setLoading(false);
    }
  };

  const copyCLICommand = async (command) => {
    try {
      await navigator.clipboard.writeText(command);
      setCliContextStatus({ text: "Command copied.", type: "ok" });
    } catch {
      setCliContextStatus({ text: command, type: "info" });
    }
  };

  const applyRoutePreset = async (preset) => {
    setRoutePresetStatus({ text: "Applying route preset...", type: "" });
    fetch("/internal/route-presets/apply", {
      method: "POST",
      headers: apiHeaders,
      body: JSON.stringify({
        preset_id: preset.id,
        provider_id: preset.default_provider_id || preset.id,
        env_name: preset.default_env_name || "",
        api_key_mode: "env",
        route_alias: preset.default_route,
        profile_name: preset.default_provider_id || preset.id,
        append_to_route: true
      })
    })
      .then((resp) => resp.ok ? resp.json() : resp.json().then((payload) => Promise.reject(new Error(payload.error || resp.statusText))))
      .then((result) => {
        setConfig(result.config);
        setRoutePresetStatus({ text: `Preset applied: ${result.summary?.model_id || preset.id}`, type: "ok" });
        loadStatus();
      })
      .catch((err) => {
        setRoutePresetStatus({ text: err.message, type: "error" });
      });
  };

  const handleCopyActivation = async (tool) => {
    try {
      await navigator.clipboard.writeText(tool.activation_command || "arkroute activate claude");
      setCliToolsStatus({ text: "Activation command copied.", type: "ok" });
    } catch {
      setCliToolsStatus({ text: tool.activation_command || "arkroute activate claude", type: "info" });
    }
  };

  const handleLaunchClaude = async () => {
    setLaunchingTool("claude");
    setCliToolsStatus({ text: "Launching Claude Code...", type: "" });
    try {
      const resp = await fetch("/internal/cli-tools/claude/launch", { method: "POST", headers: apiHeaders });
      const data = await resp.json();
      if (!resp.ok) {
        const remediation = data.remediation ? ` ${data.remediation}` : "";
        setCliToolsStatus({ text: `${data.error || resp.statusText}.${remediation}`, type: "error" });
        return;
      }
      setCliToolsStatus({ text: `Claude Code launched with pid ${data.pid}.`, type: "ok" });
      loadCliTools();
    } catch (err) {
      setCliToolsStatus({ text: `Launch failed: ${err.message}`, type: "error" });
    } finally {
      setLaunchingTool("");
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
        <div className={`tab-content ${activeTab === "providers" ? "active" : ""}`}>
          <PageHeader
            icon="ph-hard-drive"
            eyebrow="gateway agents"
            title="Providers"
            description={providerCount > 0 ? "Manage upstream providers and keep the setup path close." : "Add your first upstream provider to start routing local CLI traffic."}
            stats={providerCount > 0 ? [
              { label: "enabled", value: providerCount },
              { label: "routes", value: routeCount },
              { label: "models", value: modelCount }
            ] : []}
            action={<button type="button" className="primary-button" onClick={openAddProvider}><i className="ph-bold ph-plus"></i>Add provider</button>}
          />

          {providerCount === 0 ? (
            <section className="first-run-panel">
              <EmptyState icon="ph-plugs-connected" title="Add your first provider">
                Choose a provider preset, key source, upstream model, and route alias. Arkroute will keep the local gateway config in sync.
              </EmptyState>
              <button type="button" className="primary-button first-run-action" onClick={openAddProvider}>
                <i className="ph-bold ph-plus"></i>Add first provider
              </button>
            </section>
          ) : (
            <>
              <section className="provider-dashboard">
                <div className="operator-grid configured-provider-grid">
                  {config.providers.map((provider) => (
                    <article className="provider-row-card" key={provider.id}>
                      <ProviderCard provider={provider} />
                      <div className="provider-row-actions">
                        <button type="button" className="secondary-button" onClick={() => openEditProvider(provider)}>
                          <i className="ph-light ph-pencil-simple-line"></i>Edit
                        </button>
                        <button type="button" className="secondary-button" onClick={() => setSelectedProviderId(provider.id)}>
                          <i className="ph-light ph-crosshair"></i>Inspect
                        </button>
                      </div>
                    </article>
                  ))}
                </div>
              </section>

              <div className="detail-workbench">
                <ProviderDetail
                  provider={(config?.providers || []).find((provider) => provider.id === selectedProviderId)}
                  models={config?.models || []}
                  routes={config?.routes || []}
                  onSelectModel={(modelId) => {
                    setSelectedModelId(modelId);
                    loadCLIContext({ model_id: modelId });
                    setActiveTab("models");
                  }}
                  onSelectRoute={(routeAlias) => {
                    setSelectedRouteAlias(routeAlias);
                    loadCLIContext({ route_alias: routeAlias });
                    setActiveTab("models");
                  }}
                />
                <CLIContextPanel context={cliContext} status={cliContextStatus} onCopy={copyCLICommand} />
              </div>
            </>
          )}

          <ProviderSetupDrawer
            open={drawerOpen}
            baseUrlOptions={baseUrlOptions}
            envNameOptions={envNameOptions}
            exposedAliasOptions={exposedAliasOptions}
            form={form}
            errors={formErrors}
            loading={loading}
            presets={presets}
            providerNameOptions={providerNameOptions}
            showAdvanced={showAdvanced}
            status={status}
            saveResult={saveResult}
            upstreamModelOptions={upstreamModelOptions}
            fetchingModels={fetchingModels}
            fetchModelsStatus={fetchModelsStatus}
            deleteConfirmProviderId={deleteConfirmProviderId}
            onInputChange={handleInputChange}
            onPresetChange={handlePresetChange}
            onSaveSetup={handleSaveSetup}
            onClose={closeProviderDrawer}
            onToggleAdvanced={() => setShowAdvanced(!showAdvanced)}
            onFetchModels={() => fetchLiveModels({ force: true })}
            onDeleteProvider={handleDeleteProvider}
            onDeleteConfirmChange={setDeleteConfirmProviderId}
          />
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

          <div className="operator-grid topology-grid">
            <section className="operator-card">
              <div className="card-heading">
                <h3><i className="ph-light ph-cube"></i>Registered Models</h3>
              </div>
              <div className="stack-list">
                {modelCount > 0 ? (
                  config.models.map((model) => (
                    <ModelItem key={model.id} model={model} active={selectedModelId === model.id} onSelect={setSelectedModelId} />
                  ))
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
                  config.routes.map((route) => (
                    <RouteItem
                      key={route.alias}
                      route={route}
                      selectedModelId={selectedModelId}
                      onSelectModel={(modelId) => {
                        setSelectedModelId(modelId);
                        loadCLIContext({ model_id: modelId });
                      }}
                    />
                  ))
                ) : (
                  <EmptyState icon="ph-git-branch" title="No routes">Create a route alias during provider setup.</EmptyState>
                )}
              </div>
            </section>

            <RoutePresetPanel presets={routePresets} status={routePresetStatus} onApply={applyRoutePreset} />
            <CLIContextPanel context={cliContext} status={cliContextStatus} onCopy={copyCLICommand} />

            <PolicyInspector
              inspection={policyInspect}
              loading={policyInspectLoading}
              status={policyInspectStatus}
              apiHeaders={apiHeaders}
              onOverrideChanged={handleOverrideChanged}
            />
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

        <div className={`tab-content ${activeTab === "cli-tools" ? "active" : ""}`}>
          <PageHeader
            icon="ph-terminal-window"
            eyebrow="local clients"
            title="CLI Tools"
            description="Inspect local client readiness and launch supported tools through the Arkroute gateway."
            stats={[{ label: "tools", value: cliTools.length }]}
          />
          <section className="operator-panel cli-tools-panel">
            {cliTools.length > 0 ? cliTools.map((tool) => {
              const ready = tool.installed && tool.gateway_reachable;
              const canLaunch = ready && tool.launch_supported && launchingTool !== tool.id;
              return (
                <article className="cli-launcher-console" key={tool.id}>
                  <div className="cli-console-bar">
                    <div className="cli-console-title">
                      <span className="console-dots" aria-hidden="true"><span></span><span></span><span></span></span>
                      <strong>{tool.name}</strong>
                      <code>{tool.command}</code>
                    </div>
                    <StatusBadge tone={canLaunch ? "ok" : ready ? "pending" : "error"}>
                      {canLaunch ? "Ready" : ready ? "Launch blocked" : "Needs attention"}
                    </StatusBadge>
                  </div>

                  <div className="cli-console-body">
                    <section className="cli-launch-main">
                      <div className="cli-launch-icon"><i className="ph-light ph-terminal-window"></i></div>
                      <div>
                        <h3>Claude Code launcher</h3>
                        <p>Starts Claude Code with Arkroute's Anthropic-compatible gateway environment.</p>
                      </div>
                      <div className="cli-tool-actions cli-launch-actions">
                        <button className="launch-primary" type="button" disabled={!canLaunch} onClick={handleLaunchClaude}>
                          <i className="ph-bold ph-play"></i>
                          {launchingTool === tool.id ? "Launching" : "Launch"}
                        </button>
                        <button type="button" className="btn-secondary" onClick={() => handleCopyActivation(tool)}>
                          <i className="ph-bold ph-copy"></i>
                          Copy Env
                        </button>
                      </div>
                    </section>

                    <aside className="cli-readiness">
                      <div className={`readiness-check ${tool.installed ? "ok" : "error"}`}>
                        <span>Binary</span>
                        <strong>{tool.installed ? "found" : "not found"}</strong>
                      </div>
                      <div className={`readiness-check ${tool.gateway_reachable ? "ok" : "error"}`}>
                        <span>Gateway</span>
                        <strong>{tool.gateway_reachable ? "reachable" : "offline"}</strong>
                      </div>
                      <div className={`readiness-check ${tool.model_discovery ? "ok" : "pending"}`}>
                        <span>Models</span>
                        <strong>{tool.model_discovery ? "discovery on" : "static only"}</strong>
                      </div>
                      <div className="cli-route-strip">
                        <span>ANTHROPIC_BASE_URL</span>
                        <code>{tool.base_url || "not configured"}</code>
                      </div>
                    </aside>
                  </div>

                  {tool.launch_blocked_reason && (
                    <div className="terminal-note cli-tool-note">
                      <i className="ph-light ph-info"></i>
                      <span>{tool.launch_blocked_reason}</span>
                    </div>
                  )}
                </article>
              );
            }) : (
              <EmptyState icon="ph-terminal-window" title="No CLI tools detected">
                Refresh the panel after the gateway session is ready.
              </EmptyState>
            )}
            {cliToolsStatus.text && (
              <div className={`status-box ${cliToolsStatus.type}`}>{cliToolsStatus.text}</div>
            )}
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

            <section className="operator-card config-safety-card">
              <div className="card-heading">
                <div>
                  <StatusBadge tone={configImportSummary ? "ok" : "pending"}>{configImportSummary ? "validated" : "config"}</StatusBadge>
                  <h3><i className="ph-light ph-floppy-disk-back"></i>Config Safety</h3>
                </div>
              </div>
              <div className="config-action-row">
                <button className="secondary-button" type="button" onClick={() => downloadConfig(false)}>
                  <i className="ph-light ph-download-simple"></i>Export full
                </button>
                <button className="secondary-button" type="button" onClick={() => downloadConfig(true)}>
                  <i className="ph-light ph-shield-check"></i>Export redacted
                </button>
                <button className="secondary-button" type="button" onClick={copyRedactedConfig}>
                  <i className="ph-light ph-copy"></i>Copy redacted
                </button>
              </div>
              <textarea
                className="config-import-textarea"
                value={configDraft}
                onChange={(event) => setConfigDraft(event.target.value)}
                spellCheck="false"
                placeholder="version: 1"
              />
              <div className="config-action-row">
                <button className="secondary-button" type="button" onClick={validateConfigDraft} disabled={!configDraft.trim()}>
                  <i className="ph-light ph-check-circle"></i>Validate import
                </button>
                <button className="primary-button" type="button" onClick={applyConfigDraft} disabled={!configDraft.trim()}>
                  <i className="ph-light ph-upload-simple"></i>Apply import
                </button>
              </div>
              {configImportSummary && (
                <div className="config-summary-row">
                  <DataRow label="Providers">{configImportSummary.providers}</DataRow>
                  <DataRow label="Models">{configImportSummary.models}</DataRow>
                  <DataRow label="Routes">{configImportSummary.routes}</DataRow>
                  <DataRow label="Policies">{configImportSummary.compatibility_policies}</DataRow>
                </div>
              )}
              {configTransferStatus.text && <div className={`status-box ${configTransferStatus.type}`}>{configTransferStatus.text}</div>}
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
