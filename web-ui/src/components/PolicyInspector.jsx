import { useEffect, useState } from "react";
import StatusBadge from "./shared/StatusBadge";
import DataRow from "./shared/DataRow";
import EmptyState from "./shared/EmptyState";
import PolicyValue from "./PolicyValue";

export default function PolicyInspector({ inspection, loading, status, apiHeaders, onOverrideChanged }) {
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
    Promise.resolve().then(() => {
      setOverrideDraft({
        auto_enable: override.auto_enable === true ? "true" : override.auto_enable === false ? "false" : "unset",
        auto_effort: override.auto_effort || "unset",
        replay: override.replay === true ? "true" : override.replay === false ? "false" : "unset",
        omit_tool_choice: override.omit_tool_choice === true ? "true" : override.omit_tool_choice === false ? "false" : "unset"
      });
      setOverrideStatus({ text: "", type: "" });
    });
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
            className="primary-button"
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
              className="secondary-button"
              type="button"
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
