// formatMoney renders an amount with the shop's WooCommerce currency options
// (symbol, position, separators, decimals) from GET /v1/shop/settings.
const NBSP = "\u00a0";

export const formatMoney = (amount, settings = {}) => {
  const decimals = Number.isFinite(Number(settings.decimals)) ? Number(settings.decimals) : 2;
  const thousand = settings.thousandSeparator ?? ",";
  const decimal = settings.decimalSeparator ?? ".";
  const symbol = settings.currencySymbol ?? settings.currency ?? "";
  const n = Number(amount) || 0;
  const negative = n < 0;
  const fixed = Math.abs(n).toFixed(decimals);
  const [int, frac] = fixed.split(".");
  const grouped = int.replace(/\B(?=(\d{3})+(?!\d))/g, thousand);
  const number = frac ? `${grouped}${decimal}${frac}` : grouped;
  let out;
  switch (settings.currencyPosition) {
    case "right":
      out = `${number}${symbol}`;
      break;
    case "left_space":
      out = `${symbol}${NBSP}${number}`;
      break;
    case "right_space":
      out = `${number}${NBSP}${symbol}`;
      break;
    default:
      out = `${symbol}${number}`;
  }
  return negative ? `-${out}` : out;
};
