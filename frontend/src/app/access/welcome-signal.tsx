/** Decorative telemetry paths animate with CSS; no timers, canvas loop or network calls. */
export function WelcomeSignal() {
  const routes = [
    "M0 180H150Q180 180 180 210V310Q180 340 210 340H390",
    "M1200 200H1040Q1010 200 1010 230V390Q1010 420 980 420H810",
    "M40 600H240Q270 600 270 570V500Q270 470 300 470H390",
    "M1160 650H970Q940 650 940 620V530Q940 500 910 500H810",
  ];
  return <div className="welcome-signal pointer-events-none absolute inset-0 flex items-center justify-center overflow-hidden" aria-hidden="true">
    <svg viewBox="0 0 1200 800" className="h-auto w-full min-w-[700px] max-w-[1400px] fill-none text-accent opacity-30 dark:opacity-40">
      {routes.map((route, index) => <g key={route}>
        <path d={route} stroke="currentColor" strokeOpacity=".2" />
        <path d={route} stroke="currentColor" strokeWidth="1.5" strokeDasharray="12 68" className="signal-flow motion-safe:animate-telemetry-flow" style={{ animationDelay: `${index * -2}s` }} />
      </g>)}
      {[[150, 180], [1040, 200], [240, 600], [970, 650]].map(([x, y]) => <g key={x}><circle cx={x} cy={y} r="14" className="fill-accent/5 stroke-accent/20" /><circle cx={x} cy={y} r="3" className="fill-accent" /></g>)}
    </svg>
  </div>;
}
