import { useId } from "react";

// BarChart is a dependency-free inline SVG bar chart (with an optional line
// series overlaid) for the admin dashboard and reports.
export const BarChart = ({ data, valueKey, lineKey, labelKey = "date", format = (v) => v, formatLabel = (l) => l, height = 220, title }) => {
  const titleId = useId();
  const rows = data || [];
  if (rows.length === 0) return null;
  const values = rows.map((r) => Number(r[valueKey]) || 0);
  const lineValues = lineKey ? rows.map((r) => Number(r[lineKey]) || 0) : [];
  const max = Math.max(1, ...values, ...lineValues);
  const width = 640;
  const padL = 8;
  const padB = 28;
  const padT = 12;
  const chartH = height - padB - padT;
  const step = (width - padL * 2) / rows.length;
  const barW = Math.max(2, step * 0.62);
  const labelEvery = Math.ceil(rows.length / 10);
  const y = (v) => padT + chartH - (v / max) * chartH;
  const linePath = lineKey
    ? lineValues.map((v, i) => `${i === 0 ? "M" : "L"}${padL + step * i + step / 2},${y(v)}`).join(" ")
    : "";
  return (
    <figure className="chart">
      <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-labelledby={titleId} preserveAspectRatio="none">
        <title id={titleId}>{title}</title>
        {[0.25, 0.5, 0.75, 1].map((f) => (
          <line key={f} x1={0} x2={width} y1={y(max * f)} y2={y(max * f)} className="chart-grid" />
        ))}
        {rows.map((r, i) => {
          const v = values[i];
          const h = (v / max) * chartH;
          return (
            <g key={i}>
              <rect x={padL + step * i + (step - barW) / 2} y={padT + chartH - h} width={barW} height={Math.max(0, h)} rx={2} className="chart-bar">
                <title>{`${formatLabel(r[labelKey])}: ${format(v)}`}</title>
              </rect>
              {i % labelEvery === 0 && (
                <text x={padL + step * i + step / 2} y={height - 8} textAnchor="middle" className="chart-label">
                  {formatLabel(r[labelKey])}
                </text>
              )}
            </g>
          );
        })}
        {lineKey && <path d={linePath} className="chart-line" fill="none" />}
      </svg>
    </figure>
  );
};
