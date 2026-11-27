// Simple drawn marks for the online payment providers (not the providers'
// official artwork): the name in white on the brand's colour, as an inline SVG
// (no download, sharp at any size, same in light and dark themes).
const MARKS = {
  stripe: { label: "stripe", bg: "#635bff", weight: 800, spacing: -0.5 },
  konnect: { label: "konnect", bg: "#1e3a8a", weight: 700, spacing: 0 },
  flouci: { label: "flouci", bg: "#0f9d58", weight: 800, spacing: 0 },
};

const PaymentLogo = ({ id, size = 28, className = "" }) => {
  const mark = MARKS[String(id || "").toLowerCase()];
  if (!mark) return null;
  const width = Math.round(size * 3.2);
  return (
    <svg
      className={`payment-logo ${className}`.trim()}
      viewBox="0 0 160 50"
      width={width}
      height={size}
      role="img"
      aria-label={mark.label.charAt(0).toUpperCase() + mark.label.slice(1)}
    >
      <rect width="160" height="50" rx="12" fill={mark.bg} />
      <text
        x="80"
        y="33"
        textAnchor="middle"
        fill="#ffffff"
        fontFamily="system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif"
        fontSize="26"
        fontWeight={mark.weight}
        letterSpacing={mark.spacing}
      >
        {mark.label}
      </text>
    </svg>
  );
};

export default PaymentLogo;
