import StatusBadge from "./shared/StatusBadge";
import DataRow from "./shared/DataRow";
import EmptyState from "./shared/EmptyState";

export default function ProviderDetail({ provider, models, routes, onSelectModel, onSelectRoute }) {
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
