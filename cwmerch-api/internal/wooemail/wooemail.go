// Package wooemail sends WooCommerce's transactional emails for orders
// cwmerch changes (WordPress never runs its hooks for those, so it would
// never email the customer). Each email honours the shop's own settings
// (option woocommerce_<id>_settings: enabled, subject, heading, recipient,
// additional content) and the email styling options, and is sent through
// CWCloud's email API. Sending is best-effort: it never undoes the change.
package wooemail

import (
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"cwmerch-api/internal/email"
	"cwmerch-api/internal/woo"
)

// Mailer implements woo.OrderEvents.
type Mailer struct {
	woo    *woo.Store
	sender *email.Sender
	// Notify is called after each email (Discord), optional.
	Notify func(ctx context.Context, emailID, to string, orderID uint64, sent bool)
}

func New(w *woo.Store, sender *email.Sender) *Mailer { return &Mailer{woo: w, sender: sender} }

var _ woo.OrderEvents = (*Mailer)(nil)

// OrderStatusChanged sends what WooCommerce's status-transition hooks send.
func (m *Mailer) OrderStatusChanged(ctx context.Context, o woo.Order, from, to string) {
	ctx = context.WithoutCancel(ctx)
	switch {
	case (from == "pending" || from == "failed" || from == "cancelled" || from == "checkout-draft") && (to == "processing" || to == "on-hold" || to == "completed"):
		m.Send(ctx, "new_order", o, "")
	case (from == "processing" || from == "on-hold") && to == "cancelled":
		m.Send(ctx, "cancelled_order", o, "")
	case (from == "pending" || from == "on-hold") && to == "failed":
		m.Send(ctx, "failed_order", o, "")
	}
	switch to {
	case "on-hold":
		if from == "pending" || from == "failed" || from == "cancelled" || from == "checkout-draft" {
			m.Send(ctx, "customer_on_hold_order", o, "")
		}
	case "processing":
		if from == "pending" || from == "failed" || from == "on-hold" || from == "cancelled" || from == "checkout-draft" {
			m.Send(ctx, "customer_processing_order", o, "")
		}
	case "completed":
		m.Send(ctx, "customer_completed_order", o, "")
	}
}

// OrderCustomerNote sends the customer_note email.
func (m *Mailer) OrderCustomerNote(ctx context.Context, o woo.Order, note string) {
	m.Send(context.WithoutCancel(ctx), "customer_note", o, note)
}

// OrderRefunded sends the (partially) refunded email.
func (m *Mailer) OrderRefunded(ctx context.Context, o woo.Order, r woo.Refund, full bool) {
	id := "customer_partially_refunded_order"
	if full {
		id = "customer_refunded_order"
	}
	m.Send(context.WithoutCancel(ctx), id, o, r.Reason)
}

// Send renders and sends one email for an order; whether it went out.
func (m *Mailer) Send(ctx context.Context, emailID string, o woo.Order, note string) bool {
	def, ok := woo.EmailDefByID(emailID)
	if !ok {
		return false
	}
	settingsID := emailID
	if emailID == "customer_partially_refunded_order" {
		settingsID = "customer_refunded_order"
	}
	es := m.woo.EmailSetting(ctx, settingsID)
	if !es.Enabled && emailID != "customer_invoice" {
		slog.InfoContext(ctx, "woocommerce email disabled in the shop", "email", emailID, "order", o.ID)
		return false
	}
	to := es.Recipient
	if def.Customer {
		to = o.Billing.Email
	}
	if strings.TrimSpace(to) == "" {
		return false
	}
	subject, body, err := m.Render(ctx, def, es, o, note)
	if err != nil {
		slog.ErrorContext(ctx, "woocommerce email could not be rendered", "email", emailID, "order", o.ID, "error", err)
		return false
	}
	replyTo := ""
	if !def.Customer {
		replyTo = o.Billing.Email
	}
	sent := m.sender.Send(ctx, m.from(ctx), to, "", replyTo, subject, body, nil)
	if sent && emailID == "new_order" {
		_ = m.woo.MarkNewOrderEmailSent(ctx, o.ID)
	}
	if m.Notify != nil {
		m.Notify(ctx, emailID, to, o.ID, sent)
	}
	return sent
}

// SendNewAccount welcomes a customer who created an account.
func (m *Mailer) SendNewAccount(ctx context.Context, to, firstName, login, accountURL string) bool {
	def, _ := woo.EmailDefByID("customer_new_account")
	es := m.woo.EmailSetting(ctx, def.ID)
	if !es.Enabled || to == "" {
		return false
	}
	lang := m.woo.Lang(ctx)
	values := m.values(ctx, woo.Order{Billing: woo.Address{FirstName: firstName}})
	subject := fill(orDefault(es.Subject, def.Subject[lang]), values)
	heading := fill(orDefault(es.Heading, def.Heading[lang]), values)
	greeting := map[string]string{"en": "Hi %s,", "fr": "Bonjour %s,"}[lang]
	userLine := map[string]string{"en": "Your username is <strong>%s</strong>.", "fr": "Votre identifiant est <strong>%s</strong>."}[lang]
	button := map[string]string{"en": "My account", "fr": "Mon compte"}[lang]
	content := fmt.Sprintf(`<p>%s</p><p>%s</p><p>%s</p><p><a href="%s" style="color:%s;font-weight:600">%s</a></p>`,
		fmt.Sprintf(greeting, template.HTMLEscapeString(orDefault(firstName, login))), template.HTMLEscapeString(fill(def.Intro[lang], values)),
		fmt.Sprintf(userLine, template.HTMLEscapeString(login)), template.HTMLEscapeString(accountURL), m.color(ctx, "base"), button)
	if es.AdditionalContent != "" {
		content += "<p>" + nl2br(fill(es.AdditionalContent, values)) + "</p>"
	}
	return m.sender.Send(ctx, m.from(ctx), to, "", "", subject, m.wrap(ctx, heading, template.HTML(content)), nil)
}

func (m *Mailer) from(ctx context.Context) string {
	addr := m.woo.Option(ctx, "woocommerce_email_from_address")
	if addr == "" {
		return m.sender.From()
	}
	return addr
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func fill(text string, values map[string]string) string {
	for k, v := range values {
		text = strings.ReplaceAll(text, "{"+k+"}", v)
	}
	return text
}

func nl2br(s string) string {
	return strings.ReplaceAll(template.HTMLEscapeString(s), "\n", "<br>")
}

func (m *Mailer) values(ctx context.Context, o woo.Order) map[string]string {
	date := ""
	if !o.CreatedAt.IsZero() {
		layout := "January 2, 2006"
		if m.woo.Lang(ctx) == "fr" {
			layout = "02/01/2006"
		}
		date = o.CreatedAt.In(time.Local).Format(layout)
	}
	site := m.woo.Option(ctx, "blogname")
	return map[string]string{
		"site_title": site, "site_address": m.woo.SiteURL, "site_url": m.woo.SiteURL, "store_email": m.from(ctx),
		"order_number": o.Number, "order_date": date, "customer_first_name": o.Billing.FirstName,
		"customer_last_name": o.Billing.LastName, "customer_full_name": strings.TrimSpace(o.Billing.FirstName + " " + o.Billing.LastName),
	}
}

func (m *Mailer) color(ctx context.Context, which string) string {
	defaults := map[string]string{"base": "#7f54b3", "bg": "#f7f7f7", "body_bg": "#ffffff", "text": "#3c3c3c"}
	keys := map[string]string{"base": "woocommerce_email_base_color", "bg": "woocommerce_email_background_color",
		"body_bg": "woocommerce_email_body_background_color", "text": "woocommerce_email_text_color"}
	return orDefault(m.woo.Option(ctx, keys[which]), defaults[which])
}

// money formats an amount with the shop's currency settings.
func (m *Mailer) money(ctx context.Context, amount float64, currency string) string {
	set := m.woo.ShopSettings(ctx)
	neg := amount < 0
	if neg {
		amount = -amount
	}
	s := strconv.FormatFloat(amount, 'f', set.Decimals, 64)
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i+1:]
	}
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteString(set.ThousandSeparator)
		}
		b.WriteRune(r)
	}
	num := b.String()
	if frac != "" {
		num += set.DecimalSeparator + frac
	}
	sym := woo.CurrencySymbol(orDefault(currency, set.Currency))
	var out string
	switch set.CurrencyPosition {
	case "right":
		out = num + sym
	case "right_space":
		out = num + " " + sym
	case "left_space":
		out = sym + " " + num
	default:
		out = sym + num
	}
	if neg {
		out = "-" + out
	}
	return out
}

// Render builds the subject and the HTML of an order email.
func (m *Mailer) Render(ctx context.Context, def woo.EmailDef, es woo.EmailSettings, o woo.Order, note string) (string, string, error) {
	lang := m.woo.Lang(ctx)
	values := m.values(ctx, o)
	subject := fill(orDefault(es.Subject, def.Subject[lang]), values)
	heading := fill(orDefault(es.Heading, def.Heading[lang]), values)
	t := func(en, fr string) string {
		if lang == "fr" {
			return fr
		}
		return en
	}
	esc := template.HTMLEscapeString
	base := m.color(ctx, "base")
	var b strings.Builder
	if def.Customer {
		b.WriteString("<p>" + esc(fmt.Sprintf(t("Hi %s,", "Bonjour %s,"), orDefault(o.Billing.FirstName, t("there", "")))) + "</p>")
	}
	b.WriteString("<p>" + esc(fill(def.Intro[lang], values)) + "</p>")
	if note != "" && (def.ID == "customer_note" || strings.Contains(def.ID, "refunded")) {
		b.WriteString(`<blockquote style="margin:0 0 16px;padding:12px 16px;border-left:4px solid ` + base + `;background:#f7f7f7">` + nl2br(note) + `</blockquote>`)
	}
	if (def.ID == "customer_on_hold_order" || def.ID == "customer_invoice") && (o.Status == "on-hold" || o.Status == "pending") {
		if instr := m.woo.PaymentInstructions(ctx, o.PaymentMethod); instr != "" {
			b.WriteString(`<div style="margin:0 0 16px;padding:12px 16px;border:1px solid #e5e5e5;border-radius:4px">` + nl2br(instr) + `</div>`)
		}
	}
	// order table
	b.WriteString(fmt.Sprintf(`<h2 style="color:%s;font-size:18px;margin:16px 0 8px">%s</h2>`, base,
		esc(fmt.Sprintf(t("[Order #%s] (%s)", "[Commande n°%s] (%s)"), o.Number, values["order_date"]))))
	cell := `style="text-align:left;border:1px solid #e5e5e5;padding:10px;vertical-align:middle"`
	b.WriteString(`<table cellspacing="0" cellpadding="6" border="1" style="width:100%;border-collapse:collapse;border:1px solid #e5e5e5;margin-bottom:16px">`)
	b.WriteString(fmt.Sprintf(`<thead><tr><th %s>%s</th><th %s>%s</th><th %s>%s</th></tr></thead><tbody>`,
		cell, t("Product", "Produit"), cell, t("Quantity", "Quantité"), cell, t("Price", "Prix")))
	for _, li := range o.LineItems {
		amount := li.Total
		if o.PricesIncludeTax || m.woo.Option(ctx, "woocommerce_tax_display_cart") == "incl" {
			amount += li.TotalTax
		}
		meta := ""
		for _, e := range li.Meta {
			if strings.HasPrefix(e.Key, "_") {
				continue
			}
			key := e.Key
			if strings.HasPrefix(key, "pa_") {
				key = strings.TrimPrefix(key, "pa_")
			}
			meta += `<br><small>` + esc(key) + `: ` + esc(e.Value) + `</small>`
		}
		b.WriteString(fmt.Sprintf(`<tr><td %s>%s%s</td><td %s>%d</td><td %s>%s</td></tr>`, cell, esc(li.Name), meta, cell, li.Quantity, cell, esc(m.money(ctx, amount, o.Currency))))
	}
	b.WriteString(`</tbody><tfoot>`)
	row := func(label, value string) {
		b.WriteString(fmt.Sprintf(`<tr><th scope="row" colspan="2" %s>%s</th><td %s>%s</td></tr>`, cell, esc(label), cell, esc(value)))
	}
	subtotal := 0.0
	for _, li := range o.LineItems {
		subtotal += li.Subtotal
		if o.PricesIncludeTax {
			subtotal += li.SubtotalTax
		}
	}
	row(t("Subtotal:", "Sous-total :"), m.money(ctx, subtotal, o.Currency))
	if o.DiscountTotal > 0 {
		row(t("Discount:", "Remise :"), "-"+m.money(ctx, o.DiscountTotal, o.Currency))
	}
	for _, sl := range o.ShippingLines {
		row(t("Shipping:", "Expédition :"), m.money(ctx, sl.Total, o.Currency)+" — "+sl.MethodTitle)
	}
	for _, tl := range o.TaxLines {
		row(tl.Label+":", m.money(ctx, tl.TaxTotal+tl.ShippingTaxTotal, o.Currency))
	}
	if o.PaymentMethodTitle != "" {
		row(t("Payment method:", "Moyen de paiement :"), o.PaymentMethodTitle)
	}
	for _, r := range o.Refunds {
		row(t("Refund:", "Remboursement :"), "-"+m.money(ctx, r.Amount, o.Currency))
	}
	row(t("Total:", "Total :"), m.money(ctx, o.Total-o.TotalRefunded, o.Currency))
	b.WriteString(`</tfoot></table>`)
	if o.CustomerNote != "" {
		b.WriteString("<p><strong>" + t("Note:", "Note :") + "</strong> " + nl2br(o.CustomerNote) + "</p>")
	}
	// addresses
	addr := func(a woo.Address) string {
		parts := []string{strings.TrimSpace(a.FirstName + " " + a.LastName), a.Company, a.Address1, a.Address2,
			strings.TrimSpace(a.Postcode + " " + a.City), woo.CountryStateName(a.Country, a.State), countryName(a.Country), a.Phone, a.Email}
		out := []string{}
		for _, p := range parts {
			if strings.TrimSpace(p) != "" {
				out = append(out, esc(p))
			}
		}
		return strings.Join(out, "<br>")
	}
	b.WriteString(`<table cellspacing="0" cellpadding="0" border="0" style="width:100%;margin-bottom:16px"><tr>`)
	b.WriteString(fmt.Sprintf(`<td valign="top" width="50%%" style="padding:0 8px 0 0"><h2 style="color:%s;font-size:16px">%s</h2><address style="padding:12px;border:1px solid #e5e5e5;font-style:normal">%s</address></td>`,
		base, t("Billing address", "Adresse de facturation"), addr(o.Billing)))
	if o.Shipping.Address1 != "" {
		b.WriteString(fmt.Sprintf(`<td valign="top" width="50%%" style="padding:0 0 0 8px"><h2 style="color:%s;font-size:16px">%s</h2><address style="padding:12px;border:1px solid #e5e5e5;font-style:normal">%s</address></td>`,
			base, t("Shipping address", "Adresse de livraison"), addr(o.Shipping)))
	}
	b.WriteString(`</tr></table>`)
	if es.AdditionalContent != "" {
		b.WriteString("<p>" + nl2br(fill(es.AdditionalContent, values)) + "</p>")
	} else if def.Customer {
		b.WriteString("<p>" + esc(t("Thanks for using "+values["site_title"]+"!", "Merci d'avoir choisi "+values["site_title"]+" !")) + "</p>")
	}
	return subject, m.wrap(ctx, heading, template.HTML(b.String())), nil
}

func countryName(code string) string {
	if c, ok := woo.CountryByCode(code); ok {
		return c.Name
	}
	return code
}

// wrap is WooCommerce's email layout (header colour, header image, footer text).
func (m *Mailer) wrap(ctx context.Context, heading string, content template.HTML) string {
	base, bg, bodyBg, text := m.color(ctx, "base"), m.color(ctx, "bg"), m.color(ctx, "body_bg"), m.color(ctx, "text")
	header := m.woo.Option(ctx, "woocommerce_email_header_image")
	if header == "" {
		header = m.sender.LogoURL()
	}
	footer := orDefault(m.woo.Option(ctx, "woocommerce_email_footer_text"), "{site_title}")
	footer = fill(footer, m.values(ctx, woo.Order{}))
	return fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>%s</title></head>
<body style="margin:0;padding:0;background-color:%s;font-family:'Helvetica Neue',Helvetica,Roboto,Arial,sans-serif">
<table width="100%%" cellpadding="0" cellspacing="0" style="background-color:%s;padding:32px 0"><tr><td align="center">
<p style="margin:0 0 16px"><img src="%s" alt="%s" style="max-height:80px;max-width:240px"></p>
<table width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%%;background-color:%s;border:1px solid #dedede;border-radius:4px">
<tr><td style="background-color:%s;color:#ffffff;padding:32px 48px;border-radius:3px 3px 0 0"><h1 style="margin:0;font-size:26px;font-weight:300;line-height:1.2;color:#ffffff">%s</h1></td></tr>
<tr><td style="padding:40px 48px;color:%s;font-size:14px;line-height:1.5">%s</td></tr>
</table>
<p style="margin:24px 0 0;color:#8a8a8a;font-size:12px">%s</p>
</td></tr></table></body></html>`,
		template.HTMLEscapeString(heading), bg, bg, template.HTMLEscapeString(header), template.HTMLEscapeString(m.woo.Option(ctx, "blogname")),
		bodyBg, base, template.HTMLEscapeString(heading), text, content, nl2br(footer))
}

// SendInvoice emails an invoice PDF to the order's customer.
func (m *Mailer) SendInvoice(ctx context.Context, o woo.Order, number, filename string, pdf []byte) bool {
	to := o.Billing.Email
	if to == "" {
		return false
	}
	lang := m.woo.Lang(ctx)
	values := m.values(ctx, o)
	subject := fill(map[string]string{"en": "Your invoice {number} from {site_title}", "fr": "Votre facture {number} – {site_title}"}[lang], values)
	subject = strings.ReplaceAll(subject, "{number}", number)
	greeting := map[string]string{"en": "Hi %s,", "fr": "Bonjour %s,"}[lang]
	body := map[string]string{"en": "Please find attached the invoice %s for your order #%s.", "fr": "Veuillez trouver ci-joint la facture %s de votre commande n°%s."}[lang]
	content := fmt.Sprintf("<p>%s</p><p>%s</p>", template.HTMLEscapeString(fmt.Sprintf(greeting, orDefault(o.Billing.FirstName, ""))),
		template.HTMLEscapeString(fmt.Sprintf(body, number, o.Number)))
	att := []email.Attachment{{MimeType: "application/pdf", FileName: filename, B64: base64.StdEncoding.EncodeToString(pdf)}}
	sent := m.sender.Send(ctx, m.from(ctx), to, "", "", subject, m.wrap(ctx, subject, template.HTML(content)), att)
	if m.Notify != nil {
		m.Notify(ctx, "customer_invoice_pdf", to, o.ID, sent)
	}
	return sent
}
