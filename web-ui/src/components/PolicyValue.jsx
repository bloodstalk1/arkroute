export default function PolicyValue({ label, value, source }) {
  const renderedValue = typeof value === "boolean" ? (value ? "true" : "false") : (value || "unset");
  return (
    <div className="policy-value">
      <span>{label}</span>
      <strong>{renderedValue}</strong>
      {source && <small>{source.policy_id || source.source}</small>}
    </div>
  );
}
