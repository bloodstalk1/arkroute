import StatusBadge from "./shared/StatusBadge";

export default function RoutePresetPanel({ presets, status, onApply }) {
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
