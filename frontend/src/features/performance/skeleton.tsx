const cards = [0, 1, 2, 3, 4, 5];
const rows = [0, 1, 2, 3, 4];

/** Mirrors the loaded dashboard (figures, two charts, endpoint table) so the layout does not jump when data arrives. */
export function PerformanceSkeleton() {
  return <div className="performance-skeleton" role="status" aria-busy="true">
    <span className="sr-only">Loading performance data…</span>
    <div className="stat-grid performance-stats" aria-hidden="true">{cards.map((card) => <div className="stat-card" key={card}>
      <i className="sk" style={{ width: "58%", height: 10 }} />
      <i className="sk" style={{ width: "42%", height: 22, marginTop: 10 }} />
      <i className="sk" style={{ width: "78%", height: 10, marginTop: 10 }} />
    </div>)}</div>
    <div className="performance-trends" aria-hidden="true">{[0, 1].map((chart) => <div className="performance-trend" key={chart}>
      <i className="sk" style={{ width: "34%", height: 12 }} />
      <i className="sk performance-sk-chart" />
    </div>)}</div>
    <div className="performance-breakdown" aria-hidden="true">
      <h3><i className="sk" style={{ width: 120, height: 12 }} /></h3>
      <div className="performance-sk-rows">{rows.map((row) => <i className="sk" key={row} style={{ width: `${100 - row * 4}%` }} />)}</div>
    </div>
  </div>;
}
