import StatusBadge from "./shared/StatusBadge";
import DataRow from "./shared/DataRow";
import { providerKeySummary } from "../providerSetup.js";

export default function ProviderCard({ provider, onEdit, onInspect }) {
  return (
    <article className="operator-card provider-dashboard-card">
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
      <div className="provider-row-actions">
        <button type="button" className="secondary-button" onClick={onEdit}>
          <i className="ph-light ph-pencil-simple-line"></i>Edit
        </button>
        <button type="button" className="secondary-button" onClick={onInspect}>
          <i className="ph-light ph-crosshair"></i>Inspect
        </button>
      </div>
    </article>
  );
}
