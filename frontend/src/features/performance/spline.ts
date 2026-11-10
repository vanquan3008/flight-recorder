export function buildSmoothSpline(pts: { x: number; y: number }[], baselineY = 175): { linePath: string; areaPath: string } {
  const n = pts.length;
  if (n === 0) return { linePath: "", areaPath: "" };
  if (n === 1) {
    const x0 = pts[0].x.toFixed(1);
    const y0 = pts[0].y.toFixed(1);
    return {
      linePath: `M ${x0} ${y0}`,
      areaPath: `M ${x0} ${baselineY} L ${x0} ${y0} L ${x0} ${baselineY} Z`,
    };
  }

  const dx: number[] = [];
  const dy: number[] = [];
  const slope: number[] = [];
  for (let i = 0; i < n - 1; i++) {
    dx[i] = pts[i + 1].x - pts[i].x;
    dy[i] = pts[i + 1].y - pts[i].y;
    slope[i] = dx[i] !== 0 ? dy[i] / dx[i] : 0;
  }

  const m: number[] = new Array(n).fill(0);
  m[0] = slope[0];
  m[n - 1] = slope[n - 2];
  for (let i = 1; i < n - 1; i++) {
    if (slope[i - 1] * slope[i] <= 0) {
      m[i] = 0;
    } else {
      m[i] = (slope[i - 1] + slope[i]) / 2;
    }
  }

  let curvePart = "";
  for (let i = 0; i < n - 1; i++) {
    const p1 = pts[i];
    const p2 = pts[i + 1];
    const segDx = dx[i] / 3;
    const cp1x = p1.x + segDx;
    const cp1y = Math.min(baselineY, Math.max(15, p1.y + m[i] * segDx));
    const cp2x = p2.x - segDx;
    const cp2y = Math.min(baselineY, Math.max(15, p2.y - m[i + 1] * segDx));

    curvePart += ` C ${cp1x.toFixed(1)} ${cp1y.toFixed(1)}, ${cp2x.toFixed(1)} ${cp2y.toFixed(1)}, ${p2.x.toFixed(1)} ${p2.y.toFixed(1)}`;
  }

  const startX = pts[0].x.toFixed(1);
  const startY = pts[0].y.toFixed(1);
  const endX = pts[n - 1].x.toFixed(1);

  const linePath = `M ${startX} ${startY}${curvePart}`;
  const areaPath = `M ${startX} ${baselineY} L ${startX} ${startY}${curvePart} L ${endX} ${baselineY} Z`;

  return { linePath, areaPath };
}

