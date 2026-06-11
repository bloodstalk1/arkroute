export default function EmptyState({ icon, title, children }) {
  return (
    <div className="empty-state">
      <i className={`ph-light ${icon}`}></i>
      <strong>{title}</strong>
      <p>{children}</p>
    </div>
  );
}
