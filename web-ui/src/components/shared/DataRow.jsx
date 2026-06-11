export default function DataRow({ label, children }) {
  return (
    <div className="data-row">
      <span>{label}</span>
      <strong>{children}</strong>
    </div>
  );
}
