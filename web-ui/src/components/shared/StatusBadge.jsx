export default function StatusBadge({ tone = "ok", children }) {
  return (
    <span className={`status-indicator ${tone}`}>
      [{String(children).toUpperCase()}]
    </span>
  );
}
