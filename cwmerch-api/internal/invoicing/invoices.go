package invoicing

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"cwmerch-api/internal/assets"
	"cwmerch-api/internal/mdpdf"
	"cwmerch-api/internal/woo"
)

// Line is an invoice line (excl. tax).
type Line struct {
	Name     string  `json:"name"`
	SKU      string  `json:"sku"`
	Quantity string  `json:"quantity"`
	UnitHT   string  `json:"unitHT"`
	VatRate  string  `json:"vatRate"`
	TotalHT  string  `json:"totalHT"`
	TotalTTC string  `json:"totalTTC"`
	RawHT    float64 `json:"rawHT"`
	RawTax   float64 `json:"rawTax"`
}

// Tax is a tax total per rate.
type Tax struct {
	Label     string  `json:"label"`
	Rate      string  `json:"rate"`
	Base      string  `json:"base"`
	Amount    string  `json:"amount"`
	RawRate   float64 `json:"rawRate"`
	RawBase   float64 `json:"rawBase"`
	RawAmount float64 `json:"rawAmount"`
}

// Totals are the invoice totals (formatted) with their raw values.
type Totals struct {
	HT        string  `json:"ht"`
	Tax       string  `json:"tax"`
	Stamp     string  `json:"stamp"`
	TTC       string  `json:"ttc"`
	Discount  string  `json:"discount"`
	Shipping  string  `json:"shipping"`
	Refunded  string  `json:"refunded"`
	HasRefund bool    `json:"hasRefund"`
	RawHT     float64 `json:"rawHT"`
	RawTax    float64 `json:"rawTax"`
	RawStamp  float64 `json:"rawStamp"`
	RawTTC    float64 `json:"rawTTC"`
}

// Buyer is the billed customer.
type Buyer struct {
	Name         string   `json:"name"`
	Company      string   `json:"company"`
	AddressLines []string `json:"addressLines"`
	Email        string   `json:"email"`
	Phone        string   `json:"phone"`
}

// SellerView is the seller with its country name.
type SellerView struct {
	Seller
	CountryName string `json:"countryName"`
}

// Header is the invoice identity.
type Header struct {
	Number        string `json:"number"`
	IssuedAt      string `json:"issuedAt"`
	OrderNumber   string `json:"orderNumber"`
	OrderDate     string `json:"orderDate"`
	PaymentMethod string `json:"paymentMethod"`
	Currency      string `json:"currency"`
}

// Model is everything a template sees (and the frozen snapshot).
type Model struct {
	Invoice  Header            `json:"invoice"`
	Seller   SellerView        `json:"seller"`
	Buyer    Buyer             `json:"buyer"`
	Lines    []Line            `json:"lines"`
	Taxes    []Tax             `json:"taxes"`
	Totals   Totals            `json:"totals"`
	HasVAT   bool              `json:"hasVat"`
	HasStamp bool              `json:"hasStamp"`
	Settings Settings          `json:"settings"`
	T        map[string]string `json:"t"`
}

// Invoice is an issued invoice.
type Invoice struct {
	ID       uint64    `json:"id"`
	Number   string    `json:"number"`
	OrderID  uint64    `json:"orderId"`
	IssuedAt time.Time `json:"issuedAt"`
	Customer struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"customer"`
	Currency string  `json:"currency"`
	TotalHT  float64 `json:"totalHT"`
	TotalTax float64 `json:"totalTax"`
	Stamp    float64 `json:"stamp"`
	TotalTTC float64 `json:"totalTTC"`
	Taxes    []Tax   `json:"taxes"`
	Lines    []Line  `json:"lines"`
	Status   string  `json:"status"`
	Markdown string  `json:"-"`
	Model    Model   `json:"-"`
}

type snapshot struct {
	Model    Model  `json:"model"`
	Markdown string `json:"markdown"`
	Footer   string `json:"footer"`
}

// money formats an amount like the shop does.
func (s *Service) money(ctx context.Context, amount float64, currency string) string {
	set := s.woo.ShopSettings(ctx)
	neg := amount < 0
	if neg {
		amount = -amount
	}
	str := strconv.FormatFloat(amount, 'f', set.Decimals, 64)
	intPart, frac := str, ""
	if i := strings.IndexByte(str, '.'); i >= 0 {
		intPart, frac = str[:i], str[i+1:]
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
	out := num + " " + sym
	switch set.CurrencyPosition {
	case "left":
		out = sym + num
	case "left_space":
		out = sym + " " + num
	case "right":
		out = num + sym
	}
	if neg {
		out = "-" + out
	}
	return out
}

func pct(r float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(r, 'f', 2, 64), "0"), ".") + " %"
}

func round(v float64, d int) float64 {
	p := math.Pow(10, float64(d))
	return math.Round(v*p) / p
}

// BuildModel computes an invoice's content from an order and the settings.
func (s *Service) BuildModel(ctx context.Context, o woo.Order, set Settings, number string, issued time.Time) Model {
	dec := s.woo.Decimals(ctx)
	cur := o.Currency
	m := func(v float64) string { return s.money(ctx, v, cur) }
	lang := set.Locale
	if labels[lang] == nil {
		lang = "en"
	}
	dateFmt := "2006-01-02"
	if lang == "fr" {
		dateFmt = "02/01/2006"
	}
	model := Model{Settings: set, HasVAT: HasVAT(set.TaxMode), HasStamp: HasStamp(set.TaxMode), T: labels[lang], Lines: []Line{}, Taxes: []Tax{}}
	model.Settings.Template = ""
	model.Invoice = Header{Number: number, IssuedAt: issued.Format(dateFmt), OrderNumber: o.Number, OrderDate: o.CreatedAt.Format(dateFmt),
		PaymentMethod: o.PaymentMethodTitle, Currency: cur}
	sellerCountry := set.Seller.Country
	if c, ok := woo.CountryByCode(sellerCountry); ok {
		sellerCountry = c.Name
	}
	model.Seller = SellerView{Seller: set.Seller, CountryName: sellerCountry}
	b := o.Billing
	lines := []string{}
	for _, l := range []string{b.Address1, b.Address2, strings.TrimSpace(b.Postcode + " " + b.City), woo.CountryStateName(b.Country, b.State)} {
		if strings.TrimSpace(l) != "" && l != b.State {
			lines = append(lines, l)
		}
	}
	if c, ok := woo.CountryByCode(b.Country); ok {
		lines = append(lines, c.Name)
	}
	model.Buyer = Buyer{Name: strings.TrimSpace(b.FirstName + " " + b.LastName), Company: b.Company, AddressLines: lines, Email: b.Email, Phone: b.Phone}

	rateOf := func(taxes map[uint64]float64) float64 {
		r := 0.0
		for id := range taxes {
			for _, tl := range o.TaxLines {
				if tl.RateID == id {
					r += tl.RatePercent
				}
			}
		}
		return r
	}
	ht, tax := 0.0, 0.0
	for _, li := range o.LineItems {
		unit := 0.0
		if li.Quantity != 0 {
			unit = li.Total / float64(li.Quantity)
		}
		rate := rateOf(li.Taxes)
		model.Lines = append(model.Lines, Line{Name: li.Name, SKU: li.SKU, Quantity: strconv.FormatInt(li.Quantity, 10), UnitHT: m(unit),
			VatRate: pct(rate), TotalHT: m(li.Total), TotalTTC: m(li.Total + li.TotalTax), RawHT: li.Total, RawTax: li.TotalTax})
		ht += li.Total
		tax += li.TotalTax
	}
	for _, sl := range o.ShippingLines {
		rate := rateOf(sl.Taxes)
		model.Lines = append(model.Lines, Line{Name: model.T["Shipping"] + " – " + sl.MethodTitle, Quantity: "1", UnitHT: m(sl.Total),
			VatRate: pct(rate), TotalHT: m(sl.Total), TotalTTC: m(sl.Total + sl.TotalTax), RawHT: sl.Total, RawTax: sl.TotalTax})
		ht += sl.Total
		tax += sl.TotalTax
	}
	stamp := 0.0
	for _, f := range o.FeeLines {
		if model.HasStamp && strings.EqualFold(f.Name, orDefault(set.StampLabel, "Timbre fiscal")) {
			stamp += f.Total
			continue
		}
		model.Lines = append(model.Lines, Line{Name: f.Name, Quantity: "1", UnitHT: m(f.Total), VatRate: pct(0), TotalHT: m(f.Total),
			TotalTTC: m(f.Total + f.TotalTax), RawHT: f.Total, RawTax: f.TotalTax})
		ht += f.Total
		tax += f.TotalTax
	}
	// stamp not charged at checkout: still due on the invoice
	if model.HasStamp && stamp == 0 && set.StampAmount > 0 {
		stamp = set.StampAmount
	}
	for _, tl := range o.TaxLines {
		amount := tl.TaxTotal + tl.ShippingTaxTotal
		base := 0.0
		if tl.RatePercent > 0 {
			base = amount * 100 / tl.RatePercent
		}
		model.Taxes = append(model.Taxes, Tax{Label: orDefault(tl.Label, set.VatLabel), Rate: pct(tl.RatePercent), Base: m(base), Amount: m(amount),
			RawRate: tl.RatePercent, RawBase: round(base, dec), RawAmount: amount})
	}
	sort.SliceStable(model.Taxes, func(i, j int) bool { return model.Taxes[i].RawRate > model.Taxes[j].RawRate })
	if !model.HasVAT {
		// no VAT on the invoice: tax amounts (if any were charged) stay in the lines' totals
		ht += tax
		tax = 0
		model.Taxes = []Tax{}
	}
	ht, tax = round(ht, dec), round(tax, dec)
	ttc := round(ht+tax+stamp, dec)
	model.Totals = Totals{HT: m(ht), Tax: m(tax), Stamp: m(stamp), TTC: m(ttc), Discount: m(o.DiscountTotal), Shipping: m(o.ShippingTotal),
		Refunded: m(o.TotalRefunded), HasRefund: o.TotalRefunded > 0, RawHT: ht, RawTax: tax, RawStamp: stamp, RawTTC: ttc}
	return model
}

// RenderMarkdown runs the template (the shop's, else the default).
func RenderMarkdown(tpl string, model Model) (string, error) {
	if strings.TrimSpace(tpl) == "" {
		tpl = DefaultTemplate
	}
	t, err := parseTemplate(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, model); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderPDF renders Markdown to the invoice PDF.
func (s *Service) RenderPDF(ctx context.Context, markdown, footer, title string) ([]byte, error) {
	logo := assets.LogoPNG
	if app, err := s.woo.Appearance(ctx, false); err == nil && app.Logo != nil && app.Logo.ID != 0 {
		if raw, ok := s.woo.MediaBytes(app.Logo.ID); ok {
			logo = raw
		}
	}
	return mdpdf.Render(markdown, mdpdf.Options{LogoPNG: logo, Footer: footer, Title: title})
}

func footerOf(set Settings) string {
	parts := []string{}
	for _, p := range []string{set.Footer, set.Seller.Name} {
		if strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " – ")
}

// nextNumber takes the next number in a transaction (no gaps, no duplicates).
func (s *Service) nextNumber(ctx context.Context, tx *sql.Tx, set Settings, at time.Time) (string, error) {
	key := "invoice_counter"
	if set.ResetYearly {
		key += "_" + strconv.Itoa(at.Year())
	}
	if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO `+s.woo.T("cwmerch_settings")+` (name, data) VALUES (?, ?)`,
		key, fmt.Sprintf(`{"next": %d}`, max(set.NextNumber, 1))); err != nil {
		return "", err
	}
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT data FROM `+s.woo.T("cwmerch_settings")+` WHERE name = ? FOR UPDATE`, key).Scan(&raw); err != nil {
		return "", err
	}
	var c struct {
		Next int `json:"next"`
	}
	_ = json.Unmarshal(raw, &c)
	if c.Next < 1 {
		c.Next = 1
	}
	if _, err := tx.ExecContext(ctx, `UPDATE `+s.woo.T("cwmerch_settings")+` SET data = ? WHERE name = ?`, fmt.Sprintf(`{"next": %d}`, c.Next+1), key); err != nil {
		return "", err
	}
	seq := fmt.Sprintf("%0*d", set.NumberPadding, c.Next)
	if set.ResetYearly {
		return set.NumberPrefix + strconv.Itoa(at.Year()) + "-" + seq, nil
	}
	return set.NumberPrefix + seq, nil
}

// Issue creates the order's invoice (or answers the existing one, created=false).
func (s *Service) Issue(ctx context.Context, orderID uint64) (Invoice, bool, error) {
	if inv, err := s.ByOrder(ctx, orderID); err == nil {
		return inv, false, nil
	}
	set := s.Settings(ctx)
	if !set.Enabled {
		return Invoice{}, false, &woo.Error{Status: 400, Code: "errors.invoicingDisabled", Message: "Invoicing is disabled in the settings"}
	}
	o, err := s.woo.Order(ctx, orderID)
	if err != nil {
		return Invoice{}, false, err
	}
	if o.Status == "pending" || o.Status == "failed" || o.Status == "cancelled" || o.Status == "checkout-draft" {
		return Invoice{}, false, &woo.Error{Status: 409, Code: "errors.orderNotInvoiceable", Message: "An unpaid or cancelled order cannot be invoiced"}
	}
	issued := time.Now().UTC()
	var id uint64
	err = s.woo.DB.InTx(ctx, func(tx *sql.Tx) error {
		number, err := s.nextNumber(ctx, tx, set, issued)
		if err != nil {
			return err
		}
		model := s.BuildModel(ctx, o, set, number, issued)
		md, err := RenderMarkdown(set.Template, model)
		if err != nil {
			return &woo.Error{Status: 400, Code: "errors.invalidInvoiceTemplate", Message: "Invalid template: " + err.Error()}
		}
		raw, _ := json.Marshal(snapshot{Model: model, Markdown: md, Footer: footerOf(set)})
		res, err := tx.ExecContext(ctx, `INSERT INTO `+s.woo.T("cwmerch_invoices")+` (order_id, number, data, issued_at) VALUES (?, ?, ?, ?)`,
			orderID, number, string(raw), issued)
		if err != nil {
			return err
		}
		n, _ := res.LastInsertId()
		id = uint64(n)
		return nil
	})
	if err != nil {
		return Invoice{}, false, err
	}
	inv, err := s.Get(ctx, id)
	if err == nil {
		_, _ = s.woo.AddNote(ctx, orderID, "Invoice "+inv.Number+" issued.", false, "", "", 0)
	}
	return inv, true, err
}

func (s *Service) scan(ctx context.Context, where string, args ...any) ([]Invoice, error) {
	rows, err := s.woo.DB.QueryContext(ctx, `SELECT id, order_id, number, data, issued_at FROM `+s.woo.T("cwmerch_invoices")+` WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Invoice{}
	for rows.Next() {
		var inv Invoice
		var raw []byte
		if err := rows.Scan(&inv.ID, &inv.OrderID, &inv.Number, &raw, &inv.IssuedAt); err != nil {
			return nil, err
		}
		var snap snapshot
		_ = json.Unmarshal(raw, &snap)
		inv.Model, inv.Markdown = snap.Model, snap.Markdown
		inv.Customer.Name, inv.Customer.Email = snap.Model.Buyer.Name, snap.Model.Buyer.Email
		inv.Currency = snap.Model.Invoice.Currency
		t := snap.Model.Totals
		inv.TotalHT, inv.TotalTax, inv.Stamp, inv.TotalTTC = t.RawHT, t.RawTax, t.RawStamp, t.RawTTC
		inv.Taxes, inv.Lines, inv.Status = snap.Model.Taxes, snap.Model.Lines, "issued"
		if inv.Taxes == nil {
			inv.Taxes = []Tax{}
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// Get is one invoice.
func (s *Service) Get(ctx context.Context, id uint64) (Invoice, error) {
	list, err := s.scan(ctx, "id = ?", id)
	if err != nil {
		return Invoice{}, err
	}
	if len(list) == 0 {
		return Invoice{}, woo.ErrNotFound
	}
	return list[0], nil
}

// ByOrder is an order's invoice.
func (s *Service) ByOrder(ctx context.Context, orderID uint64) (Invoice, error) {
	list, err := s.scan(ctx, "order_id = ?", orderID)
	if err != nil {
		return Invoice{}, err
	}
	if len(list) == 0 {
		return Invoice{}, woo.ErrNotFound
	}
	return list[0], nil
}

// List pages through invoices, newest first.
func (s *Service) List(ctx context.Context, search string, page, perPage int) (woo.Paged[Invoice], error) {
	page, perPage, offset := woo.Page(page, perPage)
	where, args := "1=1", []any{}
	if search = strings.TrimSpace(search); search != "" {
		where = "(number LIKE ? OR CAST(order_id AS CHAR) = ? OR JSON_UNQUOTE(JSON_EXTRACT(data, '$.model.buyer.name')) LIKE ? OR JSON_UNQUOTE(JSON_EXTRACT(data, '$.model.buyer.email')) LIKE ?)"
		like := "%" + search + "%"
		args = []any{like, search, like, like}
	}
	var total int64
	if err := s.woo.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.woo.T("cwmerch_invoices")+` WHERE `+where, args...).Scan(&total); err != nil {
		return woo.Paged[Invoice]{}, err
	}
	items, err := s.scan(ctx, where+` ORDER BY issued_at DESC, id DESC LIMIT ? OFFSET ?`, append(args, perPage, offset)...)
	if err != nil {
		return woo.Paged[Invoice]{}, err
	}
	return woo.NewPaged(items, total, page, perPage), nil
}

// PDF renders an issued invoice from its frozen Markdown.
func (s *Service) PDF(ctx context.Context, inv Invoice) ([]byte, error) {
	var snap snapshot
	list, err := s.woo.DB.QueryContext(ctx, `SELECT data FROM `+s.woo.T("cwmerch_invoices")+` WHERE id = ?`, inv.ID)
	if err != nil {
		return nil, err
	}
	defer list.Close()
	if list.Next() {
		var raw []byte
		if err := list.Scan(&raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &snap)
	}
	return s.RenderPDF(ctx, snap.Markdown, snap.Footer, inv.Model.T["Invoice"]+" "+inv.Number)
}

// Preview renders a template against an order (or the latest one) without issuing.
func (s *Service) Preview(ctx context.Context, tpl string, orderID uint64) ([]byte, error) {
	set := s.Settings(ctx)
	if strings.TrimSpace(tpl) == "" {
		tpl = set.Template
	}
	var o woo.Order
	var err error
	if orderID != 0 {
		o, err = s.woo.Order(ctx, orderID)
	} else {
		var page woo.Paged[woo.Order]
		page, err = s.woo.Orders(ctx, woo.OrderQuery{Page: 1, PerPage: 1})
		if err == nil && len(page.Items) > 0 {
			o = page.Items[0]
		} else if err == nil {
			o = sampleOrder(s.woo.Currency(ctx))
		}
	}
	if err != nil {
		return nil, err
	}
	model := s.BuildModel(ctx, o, set, set.NumberPrefix+"PREVIEW", time.Now().UTC())
	md, err := RenderMarkdown(tpl, model)
	if err != nil {
		return nil, &woo.Error{Status: 400, Code: "errors.invalidInvoiceTemplate", Message: "Invalid template: " + err.Error()}
	}
	return s.RenderPDF(ctx, md, footerOf(set), "Preview")
}

func sampleOrder(currency string) woo.Order {
	return woo.Order{Number: "1001", Currency: currency, CreatedAt: time.Now().UTC(), PaymentMethodTitle: "Card",
		Billing:   woo.Address{FirstName: "Jane", LastName: "Doe", Address1: "1 Sample Street", City: "Paris", Postcode: "75001", Country: "FR", Email: "jane@example.com"},
		LineItems: []woo.LineItem{{Name: "Sample product", SKU: "SKU-1", Quantity: 2, Total: 50, TotalTax: 10, Taxes: map[uint64]float64{1: 10}}},
		TaxLines:  []woo.TaxLine{{RateID: 1, Label: "VAT", RatePercent: 20, TaxTotal: 10}}, Total: 60}
}

// Markdown is an issued invoice's Markdown.
func (s *Service) Markdown(inv Invoice) string { return inv.Markdown }

// Filename is the PDF file name.
func Filename(inv Invoice) string {
	label := "invoice"
	if inv.Model.T["Invoice"] == "Facture" {
		label = "facture"
	}
	return label + "-" + strings.Map(func(r rune) rune {
		if r == '/' || r == ' ' {
			return '-'
		}
		return r
	}, inv.Number) + ".pdf"
}

// AutoIssue is called on status changes: issues the invoice when configured.
func (s *Service) AutoIssue(ctx context.Context, o woo.Order, to string) (Invoice, bool) {
	set := s.Settings(ctx)
	if !set.Enabled || set.AutoGenerate == "none" {
		return Invoice{}, false
	}
	if set.AutoGenerate == to || (set.AutoGenerate == "processing" && to == "completed") {
		inv, created, err := s.Issue(ctx, o.ID)
		if err == nil && created {
			return inv, true
		}
	}
	return Invoice{}, false
}
