const cards = [0, 1, 2, 3, 4, 5];
const rows = [0, 1, 2, 3, 4];

/** Mirrors the loaded dashboard (figures, two charts, endpoint table) so the layout does not jump when data arrives. */
export function PerformanceSkeleton({ tab = "overview" }: { tab?: string } = {}) {
  return (
    <div className="performance-skeleton flex flex-1 min-h-0 flex-col overflow-hidden pointer-events-none motion-safe:animate-appear" role="status" aria-busy="true">
      <span className="sr-only">Loading performance data…</span>
      <div className="stat-grid performance-stats shrink-0 m-0 grid grid-cols-2 border-y border-glass-divider bg-surface/25 min-[621px]:grid-cols-3 min-[1281px]:grid-cols-6 [&>.stat-card]:border-r [&>.stat-card]:border-glass-divider [&>.stat-card]:px-3.5 [&>.stat-card]:py-2.5 [&>.stat-card:last-child]:border-r-0 max-[1280px]:[&>.stat-card:nth-child(n+4)]:border-t max-[620px]:[&>.stat-card:nth-child(2n)]:border-r-0 min-[621px]:max-[1280px]:[&>.stat-card:nth-child(3n)]:border-r-0" aria-hidden="true">
        {cards.map((card) => (
          <div className="stat-card flex min-w-0 flex-col hover:bg-glass-hover" key={card}>
            <i className="sk block h-3 rounded-md bg-linear-to-r from-text/7 via-text/14 to-text/7 bg-size-[240%_100%] motion-safe:animate-skeleton" style={{ width: "58%", height: 10 }} />
            <i className="sk block h-3 rounded-md bg-linear-to-r from-text/7 via-text/14 to-text/7 bg-size-[240%_100%] motion-safe:animate-skeleton" style={{ width: "42%", height: 22, marginTop: 10 }} />
            <i className="sk block h-3 rounded-md bg-linear-to-r from-text/7 via-text/14 to-text/7 bg-size-[240%_100%] motion-safe:animate-skeleton" style={{ width: "78%", height: 10, marginTop: 10 }} />
          </div>
        ))}
      </div>
      <div className="performance-trends shrink-0 grid grid-cols-1 gap-3.5 px-4.5 py-3 min-[1001px]:grid-cols-2 max-[1000px]:p-2" aria-hidden="true">
        {[0, 1].map((chart) => (
          <div className="performance-trend m-0 min-w-0 rounded-xl border border-glass-divider bg-surface/35 px-4 pt-3 pb-2.5 [&>figcaption]:flex [&>figcaption]:flex-wrap [&>figcaption]:items-start [&>figcaption]:justify-between [&>figcaption]:gap-2 [&>figcaption]:text-[12.5px] [&>figcaption]:font-semibold [&>svg]:mt-1.5 [&>svg]:block [&>svg]:h-auto [&>svg]:w-full [&>svg]:cursor-crosshair [&>svg]:overflow-visible [&_svg_text]:fill-faint [&_svg_text]:font-mono [&_svg_text]:text-[10.5px]" key={chart}>
            <i className="sk block h-3 rounded-md bg-linear-to-r from-text/7 via-text/14 to-text/7 bg-size-[240%_100%] motion-safe:animate-skeleton" style={{ width: "34%", height: 12 }} />
            <i className="sk block h-3 rounded-md bg-linear-to-r from-text/7 via-text/14 to-text/7 bg-size-[240%_100%] motion-safe:animate-skeleton performance-sk-chart mt-2.5 h-[130px] rounded-lg" />
          </div>
        ))}
      </div>
      {tab === "endpoints" && (
        <div className="performance-breakdown flex flex-1 min-h-0 flex-col overflow-hidden border-t border-glass-divider" aria-hidden="true">
          <div className="performance-sk-rows flex-1 min-h-0 overflow-hidden grid gap-2.5 px-4 pt-3 pb-4 [&_.sk]:h-3.5">
            {rows.map((row) => (
              <i className="sk block h-3 rounded-md bg-linear-to-r from-text/7 via-text/14 to-text/7 bg-size-[240%_100%] motion-safe:animate-skeleton" key={row} style={{ width: `${100 - row * 4}%` }} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
