package woo

// currencySymbols is WooCommerce's get_woocommerce_currency_symbols(), for the
// currencies a shop is likely to use (others fall back to the ISO code).
var currencySymbols = map[string]string{
	"AED": "د.إ", "ARS": "$", "AUD": "$", "BDT": "৳", "BGN": "лв.", "BHD": ".د.ب", "BRL": "R$", "CAD": "$",
	"CHF": "CHF", "CLP": "$", "CNY": "¥", "COP": "$", "CZK": "Kč", "DKK": "kr.", "DZD": "د.ج", "EGP": "EGP",
	"EUR": "€", "GBP": "£", "HKD": "$", "HUF": "Ft", "IDR": "Rp", "ILS": "₪", "INR": "₹", "ISK": "kr.",
	"JOD": "د.ا", "JPY": "¥", "KES": "KSh", "KRW": "₩", "KWD": "د.ك", "LBP": "ل.ل", "MAD": "د.م.", "MXN": "$",
	"MYR": "RM", "NGN": "₦", "NOK": "kr", "NZD": "$", "OMR": "ر.ع.", "PEN": "S/", "PHP": "₱", "PKR": "₨",
	"PLN": "zł", "QAR": "ر.ق", "RON": "lei", "RSD": "рсд", "RUB": "₽", "SAR": "ر.س", "SEK": "kr", "SGD": "$",
	"THB": "฿", "TND": "د.ت", "TRY": "₺", "TWD": "NT$", "UAH": "₴", "USD": "$", "VND": "₫", "XAF": "CFA",
	"XOF": "CFA", "ZAR": "R",
}

// CurrencySymbol is the display symbol of an ISO 4217 code.
func CurrencySymbol(code string) string {
	if s, ok := currencySymbols[code]; ok {
		return s
	}
	return code
}

// CurrencyCodes is every code with a known symbol.
func CurrencyCodes() []string {
	out := make([]string, 0, len(currencySymbols))
	for k := range currencySymbols {
		out = append(out, k)
	}
	return out
}
