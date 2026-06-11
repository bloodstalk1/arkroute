export default function ModelItem({ model, active, onSelect }) {
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
