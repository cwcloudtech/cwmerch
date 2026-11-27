package invoicing

import (
	"strings"
	"text/template"
)

// DefaultTemplate is the invoice when the shop has not written its own.
// It is a Go text/template producing Markdown (see Variables).
const DefaultTemplate = `# {{.T.Invoice}} {{.Invoice.Number}}

**{{.Seller.Name}}**
{{- with .Seller.Address}}
{{.}}{{end}}
{{- if or .Seller.Postcode .Seller.City}}
{{.Seller.Postcode}} {{.Seller.City}}{{end}}
{{- with .Seller.CountryName}}
{{.}}{{end}}
{{- with .Seller.Email}}
{{.}}{{end}}
{{- with .Seller.Phone}}
{{.}}{{end}}
{{- if .Seller.RegistrationNumber}}
{{.Seller.RegistrationLabel}} : {{.Seller.RegistrationNumber}}{{end}}
{{- if .Seller.VatNumber}}
{{.Seller.VatNumberLabel}} : {{.Seller.VatNumber}}{{end}}

---

| {{.T.InvoiceDate}} | {{.T.Order}} | {{.T.OrderDate}} | {{.T.PaymentMethod}} |
|---|---|---|---|
| {{.Invoice.IssuedAt}} | #{{.Invoice.OrderNumber}} | {{.Invoice.OrderDate}} | {{.Invoice.PaymentMethod}} |

## {{.T.BilledTo}}

**{{.Buyer.Name}}**
{{- with .Buyer.Company}}
{{.}}{{end}}
{{- range .Buyer.AddressLines}}
{{.}}{{end}}
{{- with .Buyer.Email}}
{{.}}{{end}}
{{- with .Buyer.Phone}}
{{.}}{{end}}

## {{.T.Details}}

| {{.T.Item}} | {{.T.Quantity}} | {{.T.UnitPrice}} |{{if .HasVAT}} {{.Settings.VatLabel}} |{{end}} {{.T.TotalHT}} |
|---|---:|---:|{{if .HasVAT}}---:|{{end}}---:|
{{- range .Lines}}
| {{.Name}}{{with .SKU}} ({{.}}){{end}} | {{.Quantity}} | {{.UnitHT}} |{{if $.HasVAT}} {{.VatRate}} |{{end}} {{.TotalHT}} |
{{- end}}

| | |
|---|---:|
| {{.T.TotalHT}} | {{.Totals.HT}} |
{{- if .HasVAT}}{{range .Taxes}}
| {{.Label}} {{.Rate}} ({{$.T.Base}} {{.Base}}) | {{.Amount}} |{{end}}{{end}}
{{- if .HasStamp}}
| {{.Settings.StampLabel}} | {{.Totals.Stamp}} |{{end}}
| **{{.T.TotalTTC}}** | **{{.Totals.TTC}}** |
{{- if .Totals.HasRefund}}
| {{.T.Refunded}} | -{{.Totals.Refunded}} |{{end}}

{{- if not .HasVAT}}{{with .Settings.VatExemptionMention}}

{{.}}{{end}}{{end}}
{{- with .Settings.PaymentTerms}}

**{{$.T.PaymentTerms}}** : {{.}}{{end}}
{{- if .Seller.IBAN}}

IBAN : {{.Seller.IBAN}}{{with .Seller.BIC}} - BIC : {{.}}{{end}}{{end}}
{{- with .Settings.LegalMentions}}

{{.}}{{end}}
`

// Variable documents one template field for the editor.
type Variable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Variables lists what a template can use.
var Variables = []Variable{
	{".Invoice.Number", "Invoice number (e.g. FAC-2026-00001)"},
	{".Invoice.IssuedAt", "Issue date"},
	{".Invoice.OrderNumber / .Invoice.OrderDate", "Order number and date"},
	{".Invoice.PaymentMethod", "Payment method title"},
	{".Invoice.Currency", "Currency code"},
	{".Seller.Name, .Address, .Postcode, .City, .CountryName, .Email, .Phone, .Website", "Issuer"},
	{".Seller.RegistrationLabel / .RegistrationNumber", "e.g. SIRET / Matricule fiscal"},
	{".Seller.VatNumberLabel / .VatNumber, .IBAN, .BIC", "VAT number and bank details"},
	{".Buyer.Name, .Company, .AddressLines (list), .Email, .Phone", "Customer (billing address)"},
	{".Lines (list): .Name, .SKU, .Quantity, .UnitHT, .VatRate, .TotalHT, .TotalTTC", "Products, shipping and fees, excl. tax"},
	{".Taxes (list): .Label, .Rate, .Base, .Amount", "Taxes per rate"},
	{".Totals.HT, .Tax, .Stamp, .TTC, .Discount, .Shipping, .Refunded, .HasRefund", "Totals (formatted with the shop currency)"},
	{".HasVAT / .HasStamp", "Whether VAT / the fiscal stamp apply (tax mode)"},
	{".Settings.VatLabel, .StampLabel, .VatExemptionMention, .LegalMentions, .PaymentTerms, .Footer", "Invoicing settings"},
	{".T.Invoice, .T.BilledTo, .T.TotalHT ...", "Labels in the invoice locale (en/fr)"},
}

var labels = map[string]map[string]string{
	"en": {"Invoice": "Invoice", "InvoiceDate": "Invoice date", "Order": "Order", "OrderDate": "Order date", "PaymentMethod": "Payment",
		"BilledTo": "Billed to", "Details": "Details", "Item": "Item", "Quantity": "Qty", "UnitPrice": "Unit price excl. tax",
		"TotalHT": "Total excl. tax", "TotalTTC": "Total incl. tax", "Base": "on", "Refunded": "Refunded", "PaymentTerms": "Payment terms",
		"Shipping": "Shipping", "Discount": "Discount"},
	"fr": {"Invoice": "Facture", "InvoiceDate": "Date de facture", "Order": "Commande", "OrderDate": "Date de commande", "PaymentMethod": "Paiement",
		"BilledTo": "Facturé à", "Details": "Détail", "Item": "Désignation", "Quantity": "Qté", "UnitPrice": "P.U. HT",
		"TotalHT": "Total HT", "TotalTTC": "Total TTC", "Base": "sur", "Refunded": "Remboursé", "PaymentTerms": "Conditions de paiement",
		"Shipping": "Livraison", "Discount": "Remise"},
}

func parseTemplate(text string) (*template.Template, error) {
	return template.New("invoice").Option("missingkey=zero").Funcs(template.FuncMap{
		"upper": strings.ToUpper, "lower": strings.ToLower, "join": strings.Join,
	}).Parse(text)
}
