export default function RouteItem({ route, selectedModelId, onSelectModel }) {
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
