export default function PageHeader({ icon, eyebrow, title, description, stats = [], action = null }) {
  return (
    <header className="page-header">
      <div className="title-stack">
        <span className="eyebrow"><i className={`ph-fill ${icon}`}></i>// {eyebrow.toUpperCase()}</span>
        <h1>{title.toUpperCase()}</h1>
        <p className="muted">{description}</p>
      </div>
      <div className="header-actions">
        {stats.length > 0 && (
          <div className="header-metrics">
            {stats.map((stat) => (
              <div className="metric" key={stat.label}>
                <span>{stat.label}</span>
                <strong>{stat.value}</strong>
              </div>
            ))}
          </div>
        )}
        {action}
      </div>
    </header>
  );
}
