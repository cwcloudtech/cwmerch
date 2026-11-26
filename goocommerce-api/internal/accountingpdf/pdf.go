// Package accountingpdf renders goocommerce's French accounting exports
// (chiffre d'affaires, balance des comptes, grand livre) as A4 PDFs for the
// shop's accountant.
package accountingpdf

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

	"goocommerce-api/internal/assets"
	"goocommerce-api/internal/woo"
)

const (
	marginMM   = 12.0
	logoHeight = 14.0
	lineHeight = 5.2
	fontSize   = 8.0
)

// brand colours (table header, totals).
var (
	headerFill = [3]int{28, 185, 247}
	totalFill  = [3]int{232, 244, 252}
	muted      = [3]int{120, 120, 120}
)

// FormatNumber is v in French notation: "1 234,56" (thousands separated by
// a space, decimal comma), "-1 234,56" when negative.
func FormatNumber(v float64, decimals int) string {
	if decimals < 0 {
		decimals = 0
	}
	p := math.Pow(10, float64(decimals))
	v = math.Round(v*p) / p
	if v == 0 {
		v = 0 // no "-0,00"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	out := b.String()
	if frac != "" {
		out += "," + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

// FormatAmount is FormatNumber followed by the currency symbol: "1 234,56 €".
func FormatAmount(v float64, decimals int, symbol string) string {
	if symbol == "" {
		return FormatNumber(v, decimals)
	}
	return FormatNumber(v, decimals) + " " + symbol
}

// doc is a PDF being written.
type doc struct {
	pdf    *fpdf.Fpdf
	tr     func(string) string
	shop   woo.AccountingShop
	symbol string
	title  string
	sub    string
}

func newDoc(shop woo.AccountingShop, orientation, title, subtitle string) *doc {
	pdf := fpdf.New(orientation, "mm", "A4", "")
	pdf.SetMargins(marginMM, marginMM, marginMM)
	pdf.SetAutoPageBreak(true, 16)
	pdf.AliasNbPages("{nb}")
	pdf.SetTitle(title, true)
	pdf.SetAuthor(shop.Name, true)
	pdf.SetCreator("goocommerce", true)
	d := &doc{pdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor(""), shop: shop, title: title, sub: subtitle}
	// A symbol outside cp1252 (most non-Latin currencies) prints as its ISO code.
	d.symbol = shop.CurrencySymbol
	if strings.Contains(d.tr(d.symbol), "?") && !strings.Contains(d.symbol, "?") {
		d.symbol = shop.Currency
	}
	generated := time.Now().Format("02/01/2006 à 15:04")
	pdf.SetHeaderFunc(d.header)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("Helvetica", "I", 7)
		pdf.SetTextColor(muted[0], muted[1], muted[2])
		w, _ := pdf.GetPageSize()
		pdf.CellFormat((w-2*marginMM)/2, 5, d.tr("Document généré par goocommerce le "+generated), "", 0, "L", false, 0, "")
		pdf.CellFormat((w-2*marginMM)/2, 5, d.tr(fmt.Sprintf("Page %d/{nb}", pdf.PageNo())), "", 0, "R", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})
	pdf.AddPage()
	return d
}

func (d *doc) header() {
	pdf := d.pdf
	w, _ := pdf.GetPageSize()
	top := 8.0
	textX := marginMM
	if len(assets.LogoPNG) > 0 {
		opts := fpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}
		if info := pdf.RegisterImageOptionsReader("shop-logo", opts, bytes.NewReader(assets.LogoPNG)); info != nil && info.Height() > 0 {
			lw := info.Width() * logoHeight / info.Height()
			pdf.ImageOptions("shop-logo", marginMM, top, lw, logoHeight, false, opts, 0, "")
			textX = marginMM + lw + 4
		}
	}
	pdf.SetXY(textX, top)
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(90, 5, d.tr(d.shop.Name), "", 2, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 7.5)
	pdf.SetTextColor(muted[0], muted[1], muted[2])
	for _, l := range d.shop.Address {
		pdf.CellFormat(90, 3.6, d.tr(l), "", 2, "L", false, 0, "")
	}
	pdf.SetTextColor(0, 0, 0)
	pdf.SetXY(w/2, top)
	pdf.SetFont("Helvetica", "B", 13)
	pdf.CellFormat(w/2-marginMM, 6, d.tr(d.title), "", 2, "R", false, 0, "")
	pdf.SetFont("Helvetica", "", 8.5)
	pdf.CellFormat(w/2-marginMM, 4.5, d.tr(d.sub), "", 2, "R", false, 0, "")
	pdf.SetY(math.Max(pdf.GetY(), top+logoHeight) + 4)
	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(marginMM, pdf.GetY()-2, w-marginMM, pdf.GetY()-2)
}

func (d *doc) money(v float64) string { return FormatAmount(v, d.shop.Decimals, d.symbol) }

// blankZero is "" for 0 (debit/credit columns), else the amount.
func (d *doc) blankZero(v float64) string {
	if math.Abs(v) < 1e-9 {
		return ""
	}
	return d.money(v)
}

func (d *doc) note(text string) {
	pdf := d.pdf
	pdf.Ln(3)
	pdf.SetFont("Helvetica", "I", 7)
	pdf.SetTextColor(muted[0], muted[1], muted[2])
	pdf.MultiCell(0, 3.6, d.tr(text), "", "L", false)
	pdf.SetTextColor(0, 0, 0)
}

func (d *doc) heading(text string) {
	pdf := d.pdf
	pdf.Ln(2)
	pdf.SetFont("Helvetica", "B", 9.5)
	pdf.CellFormat(0, 6, d.tr(text), "", 1, "L", false, 0, "")
}

func (d *doc) output() ([]byte, error) {
	var buf bytes.Buffer
	if err := d.pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// column of a table.
type column struct {
	header string
	weight float64
	align  string // L, C, R
}

// row of a table; bold rows are totals.
type row struct {
	cells []string
	bold  bool
}

// table draws a bordered table, repeating its header on every page and
// truncating cells that do not fit their column.
func (d *doc) table(cols []column, rows []row) {
	pdf := d.pdf
	w, h := pdf.GetPageSize()
	usable := w - 2*marginMM
	total := 0.0
	for _, c := range cols {
		total += c.weight
	}
	widths := make([]float64, len(cols))
	for i, c := range cols {
		widths[i] = usable * c.weight / total
	}
	drawHeader := func() {
		pdf.SetFont("Helvetica", "B", fontSize)
		pdf.SetFillColor(headerFill[0], headerFill[1], headerFill[2])
		pdf.SetTextColor(255, 255, 255)
		pdf.SetDrawColor(210, 210, 210)
		for i, c := range cols {
			pdf.CellFormat(widths[i], lineHeight+0.8, fit(pdf, d.tr(c.header), widths[i]-1), "1", 0, "C", true, 0, "")
		}
		pdf.Ln(-1)
		pdf.SetTextColor(0, 0, 0)
	}
	pdf.SetAutoPageBreak(false, 0)
	defer pdf.SetAutoPageBreak(true, 16)
	drawHeader()
	for i, r := range rows {
		if pdf.GetY()+lineHeight > h-16 {
			pdf.AddPage()
			drawHeader()
		}
		style := ""
		fill := i%2 == 1
		pdf.SetFillColor(248, 248, 248)
		if r.bold {
			style = "B"
			fill = true
			pdf.SetFillColor(totalFill[0], totalFill[1], totalFill[2])
		}
		pdf.SetFont("Helvetica", style, fontSize)
		for j := range cols {
			text := ""
			if j < len(r.cells) {
				text = d.tr(r.cells[j])
			}
			text = fit(pdf, text, widths[j]-1.5)
			pdf.CellFormat(widths[j], lineHeight, text, "1", 0, cols[j].align, fill, 0, "")
		}
		pdf.Ln(-1)
	}
}

// fit shortens s with "..." until it fits width at the current font.
func fit(pdf *fpdf.Fpdf, s string, width float64) string {
	if pdf.GetStringWidth(s) <= width {
		return s
	}
	b := []byte(s) // already cp1252: one byte per character
	for len(b) > 1 {
		b = b[:len(b)-1]
		if c := string(b) + "..."; pdf.GetStringWidth(c) <= width {
			return c
		}
	}
	return string(b)
}

func yearSubtitle(year int, currency string) string {
	return fmt.Sprintf("Exercice du 01/01/%d au 31/12/%d - %s", year, year, currency)
}

// Turnover renders the chiffre d'affaires by month.
func Turnover(shop woo.AccountingShop, t woo.Turnover) ([]byte, error) {
	d := newDoc(shop, "L", "Chiffre d'affaires "+strconv.Itoa(t.Year), yearSubtitle(t.Year, shop.Currency))
	cols := []column{
		{"Mois", 1.2, "L"}, {"Commandes", 1.0, "R"}, {"Ventes HT", 1.3, "R"}, {"Remises HT", 1.1, "R"},
		{"Port HT", 1.1, "R"}, {"Frais HT", 1, "R"}, {"TVA collectée", 1.2, "R"}, {"Total TTC", 1.3, "R"},
		{"Rembours. TTC", 1.4, "R"}, {"CA net HT", 1.3, "R"}, {"TVA nette", 1.2, "R"}, {"Net TTC", 1.3, "R"},
	}
	line := func(m woo.TurnoverMonth, bold bool) row {
		return row{bold: bold, cells: []string{m.Label, strconv.FormatInt(m.Orders, 10), d.money(m.SalesHT), d.money(m.Discounts),
			d.money(m.ShippingHT), d.money(m.FeesHT), d.money(m.Tax), d.money(m.GrossTTC), d.money(m.Refunds),
			d.money(m.NetHT), d.money(m.NetTax), d.money(m.NetTTC)}}
	}
	rows := make([]row, 0, len(t.Months)+1)
	for _, m := range t.Months {
		rows = append(rows, line(m, false))
	}
	rows = append(rows, line(t.Totals, true))
	d.table(cols, rows)
	d.note("Ventes HT : produits après remises. CA net HT = ventes HT + port HT + frais HT - remboursements HT. " +
		"Dates : création des commandes et des remboursements (fuseau de la boutique). " + t.Note)
	return d.output()
}

// Balance renders the balance des comptes.
func Balance(shop woo.AccountingShop, b woo.Balance) ([]byte, error) {
	d := newDoc(shop, "P", "Balance des comptes "+strconv.Itoa(b.Year), yearSubtitle(b.Year, shop.Currency))
	cols := []column{{"Compte", 0.9, "L"}, {"Intitulé", 2.6, "L"}, {"Débit", 1.3, "R"}, {"Crédit", 1.3, "R"},
		{"Solde débiteur", 1.3, "R"}, {"Solde créditeur", 1.3, "R"}}
	rows := []row{}
	for _, r := range b.Rows {
		rows = append(rows, row{cells: []string{r.Number, r.Label, d.blankZero(r.Debit), d.blankZero(r.Credit),
			d.blankZero(r.BalanceDebit), d.blankZero(r.BalanceCredit)}})
	}
	rows = append(rows, row{bold: true, cells: []string{"", "Total", d.money(b.TotalDebit), d.money(b.TotalCredit),
		d.money(b.TotalBalanceDebit), d.money(b.TotalBalanceCredit)}})
	d.table(cols, rows)
	if !b.Balanced {
		d.note("Attention : la balance n'est pas équilibrée.")
	}
	if len(b.Payments) > 0 {
		d.heading("Encaissements par moyen de paiement")
		prows := []row{}
		total := 0.0
		var count int64
		for _, p := range b.Payments {
			prows = append(prows, row{cells: []string{p.Title, p.Account.Number + " " + p.Account.Label,
				strconv.FormatInt(p.Orders, 10), d.money(p.Amount)}})
			total += p.Amount
			count += p.Orders
		}
		prows = append(prows, row{bold: true, cells: []string{"Total", "", strconv.FormatInt(count, 10), d.money(total)}})
		d.table([]column{{"Moyen de paiement", 2.2, "L"}, {"Compte", 2.2, "L"}, {"Commandes", 1, "R"}, {"Montant TTC", 1.4, "R"}}, prows)
	}
	d.note(b.Note)
	return d.output()
}

// Ledger renders the grand livre: one table section per account with its
// running balance, then the grand totals.
func Ledger(shop woo.AccountingShop, l woo.Ledger) ([]byte, error) {
	d := newDoc(shop, "P", "Grand livre "+strconv.Itoa(l.Year), yearSubtitle(l.Year, shop.Currency))
	cols := []column{{"Date", 0.95, "L"}, {"Jnl", 0.45, "C"}, {"Pièce", 1.05, "L"}, {"Libellé", 3.1, "L"},
		{"Débit", 1.2, "R"}, {"Crédit", 1.2, "R"}, {"Solde", 1.25, "R"}}
	if len(l.Accounts) == 0 {
		d.note("Aucune écriture sur l'exercice.")
	}
	for _, a := range l.Accounts {
		d.heading(a.Number + " - " + a.Label)
		rows := make([]row, 0, len(a.Lines)+1)
		for _, ln := range a.Lines {
			rows = append(rows, row{cells: []string{frenchDate(ln.Date), ln.Journal, ln.Piece, ln.Label,
				d.blankZero(ln.Debit), d.blankZero(ln.Credit), d.money(ln.Balance)}})
		}
		rows = append(rows, row{bold: true, cells: []string{"", "", "", "Total compte " + a.Number,
			d.money(a.TotalDebit), d.money(a.TotalCredit), d.money(a.Balance)}})
		d.table(cols, rows)
	}
	d.heading("Totaux généraux")
	d.table([]column{{"Écritures", 1, "R"}, {"Total débit", 1.5, "R"}, {"Total crédit", 1.5, "R"}, {"Équilibre", 1, "C"}},
		[]row{{bold: true, cells: []string{strconv.Itoa(l.Entries), d.money(l.TotalDebit), d.money(l.TotalCredit),
			map[bool]string{true: "Oui", false: "Non"}[l.Balanced]}}})
	d.note("Journaux : VT ventes (commandes et avoirs à leur date de création), BQ trésorerie (encaissements à la date de paiement, remboursements). " + l.Note)
	return d.output()
}

// frenchDate is YYYY-MM-DD as DD/MM/YYYY.
func frenchDate(iso string) string {
	if t, err := time.Parse("2006-01-02", iso); err == nil {
		return t.Format("02/01/2006")
	}
	return iso
}
