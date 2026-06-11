import StatusBadge from "./shared/StatusBadge";
import EmptyState from "./shared/EmptyState";

export default function CLIContextPanel({ context, status, onCopy }) {
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
            <button type="button" className="secondary-button" onClick={() => onCopy(profile.command)}>
              <i className="ph-bold ph-copy"></i>Copy
            </button>
          </article>
        ))}
      </div>
    </section>
  );
}
