package woo

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"goocommerce-api/internal/php"
)

// French accounting exports for the shop's accountant: the turnover
// (chiffre d'affaires), the trial balance (balance des comptes) and the
// general ledger (grand livre) of a calendar year, in the French chart of
// accounts (PCG).
//
// They are ESTIMATES built from the shop's own data (its WooCommerce orders
// and refunds), not a certified bookkeeping: only the orders created during
// the year are booked (a sale on its creation date in the VT journal, its
// payment on its paid date in the BQ journal when that date falls in the
// year), purchases and expenses are not known to the shop at all.

// AccountingNote is the disclaimer every export carries.
const AccountingNote = "Estimation établie à partir des commandes de la boutique (ventes, remboursements, encaissements). " +
	"Ne constitue pas une comptabilité certifiée : à rapprocher des relevés bancaires et à valider par votre expert-comptable."

// AccountingAccount is a PCG account.
type AccountingAccount struct {
	Number string `json:"number"`
	Label  string `json:"label"`
}

// AccountPlan is the accounts the journal books to. Each can be overridden
// with the option goocommerce_accounting_accounts: a JSON object or a
// PHP-serialized array keyed by the field's json name, whose values are
// "706", "706|Prestations de services" or {"number":"706","label":"..."}.
type AccountPlan struct {
	Customers AccountingAccount `json:"customers"`
	Sales     AccountingAccount `json:"sales"`
	Shipping  AccountingAccount `json:"shipping"`
	Fees      AccountingAccount `json:"fees"`
	Discounts AccountingAccount `json:"discounts"`
	VAT       AccountingAccount `json:"vat"`
	Bank      AccountingAccount `json:"bank"`
	Cheques   AccountingAccount `json:"cheques"`
	Cash      AccountingAccount `json:"cash"`
}

// DefaultAccountPlan is the PCG accounts of a shop selling goods.
func DefaultAccountPlan() AccountPlan {
	return AccountPlan{
		Customers: AccountingAccount{"411", "Clients"},
		Sales:     AccountingAccount{"7071", "Ventes de marchandises"},
		Shipping:  AccountingAccount{"7085", "Ports et frais accessoires facturés"},
		Fees:      AccountingAccount{"7088", "Autres produits d'activités annexes"},
		Discounts: AccountingAccount{"709", "Rabais, remises et ristournes accordés"},
		VAT:       AccountingAccount{"44571", "TVA collectée"},
		Bank:      AccountingAccount{"512", "Banque"},
		Cheques:   AccountingAccount{"5112", "Chèques à encaisser"},
		Cash:      AccountingAccount{"531", "Caisse"},
	}
}

// AccountingOptionName is the option overriding the account plan.
const AccountingOptionName = "goocommerce_accounting_accounts"

// AccountPlan is the default plan with the shop's overrides.
func (s *Store) AccountPlan(ctx context.Context) AccountPlan {
	return parseAccountPlan(s.Option(ctx, AccountingOptionName))
}

func parseAccountPlan(raw string) AccountPlan {
	plan := DefaultAccountPlan()
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return plan
	}
	values := map[string]any{}
	if strings.HasPrefix(raw, "{") {
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return plan
		}
	} else {
		a := php.UnmarshalArray(raw)
		for _, k := range a.Keys {
			v := a.Get(k)
			if sub, ok := v.(*php.Array); ok {
				values[php.ToString(k)] = map[string]any{"number": sub.String("number"), "label": sub.String("label")}
			} else {
				values[php.ToString(k)] = php.ToString(v)
			}
		}
	}
	set := func(key string, acc *AccountingAccount) {
		v, ok := values[key]
		if !ok {
			return
		}
		number, label := "", ""
		switch x := v.(type) {
		case string:
			number, label, _ = strings.Cut(x, "|")
		case float64:
			number = strconv.FormatFloat(x, 'f', -1, 64)
		case map[string]any:
			number, _ = x["number"].(string)
			label, _ = x["label"].(string)
		}
		if number = strings.TrimSpace(number); number != "" {
			acc.Number = number
		}
		if label = strings.TrimSpace(label); label != "" {
			acc.Label = label
		}
	}
	set("customers", &plan.Customers)
	set("sales", &plan.Sales)
	set("shipping", &plan.Shipping)
	set("fees", &plan.Fees)
	set("discounts", &plan.Discounts)
	set("vat", &plan.VAT)
	set("bank", &plan.Bank)
	set("cheques", &plan.Cheques)
	set("cash", &plan.Cash)
	return plan
}

// treasury is the account a payment method lands in.
func (p AccountPlan) treasury(method string) AccountingAccount {
	switch method {
	case "cod":
		return p.Cash
	case "cheque":
		return p.Cheques
	}
	return p.Bank
}

// AccountingShop is who the exports are for.
type AccountingShop struct {
	Name           string   `json:"name"`
	Address        []string `json:"address"`
	Email          string   `json:"email"`
	Currency       string   `json:"currency"`
	CurrencySymbol string   `json:"currencySymbol"`
	Decimals       int      `json:"decimals"`
}

// AccountingShop reads the shop's identity (blogname, woocommerce_store_*).
func (s *Store) AccountingShop(ctx context.Context) AccountingShop {
	cur := s.Currency(ctx)
	shop := AccountingShop{Name: s.Option(ctx, "blogname"), Email: s.Option(ctx, "woocommerce_email_from_address"),
		Currency: cur, CurrencySymbol: CurrencySymbol(cur), Decimals: s.Decimals(ctx)}
	if shop.Email == "" {
		shop.Email = s.Option(ctx, "admin_email")
	}
	for _, k := range []string{"woocommerce_store_address", "woocommerce_store_address_2"} {
		if v := strings.TrimSpace(s.Option(ctx, k)); v != "" {
			shop.Address = append(shop.Address, v)
		}
	}
	if city := strings.TrimSpace(strings.TrimSpace(s.Option(ctx, "woocommerce_store_postcode")) + " " +
		strings.TrimSpace(s.Option(ctx, "woocommerce_store_city"))); city != "" {
		shop.Address = append(shop.Address, city)
	}
	if country, _ := splitCountry(s.Option(ctx, "woocommerce_default_country")); country != "" {
		shop.Address = append(shop.Address, countryName(country, "fr"))
	}
	return shop
}

// accYearBounds is [Jan 1st, next Jan 1st) of year in loc.
func accYearBounds(year int, loc *time.Location) (time.Time, time.Time) {
	return time.Date(year, 1, 1, 0, 0, 0, 0, loc), time.Date(year+1, 1, 1, 0, 0, 0, 0, loc)
}

var frenchMonths = []string{"Janvier", "Février", "Mars", "Avril", "Mai", "Juin", "Juillet", "Août",
	"Septembre", "Octobre", "Novembre", "Décembre"}

// FrenchMonth is a month's French name (1-12).
func FrenchMonth(m int) string {
	if m < 1 || m > 12 {
		return ""
	}
	return frenchMonths[m-1]
}

// ------------------------------------------------------------------ turnover

// TurnoverMonth is a month of the turnover (all amounts excl. tax unless TTC).
type TurnoverMonth struct {
	Month      int     `json:"month"` // 1-12, 0 for the totals
	Label      string  `json:"label"`
	Orders     int64   `json:"orders"`
	SalesHT    float64 `json:"salesHT"`    // goods, coupons deducted
	Discounts  float64 `json:"discounts"`  // coupons granted (information)
	ShippingHT float64 `json:"shippingHT"` // shipping charged
	FeesHT     float64 `json:"feesHT"`
	Tax        float64 `json:"tax"`      // VAT collected on the sales
	GrossTTC   float64 `json:"grossTTC"` // orders' totals
	Refunds    float64 `json:"refunds"`  // refunded, tax included
	RefundsHT  float64 `json:"refundsHT"`
	RefundsTax float64 `json:"refundsTax"`
	NetHT      float64 `json:"netHT"`  // turnover: sales + shipping + fees − refunds, excl. tax
	NetTax     float64 `json:"netTax"` // VAT due: collected − refunded
	NetTTC     float64 `json:"netTTC"`
}

// Turnover is a year's chiffre d'affaires by month.
type Turnover struct {
	Year     int             `json:"year"`
	Currency string          `json:"currency"`
	Months   []TurnoverMonth `json:"months"`
	Totals   TurnoverMonth   `json:"totals"`
	Note     string          `json:"note"`
}

func (m *TurnoverMonth) add(o repOrder) {
	if o.Refund {
		m.Refunds += -o.Total
		m.RefundsTax += -o.totalTax()
		m.RefundsHT += -(o.Total - o.totalTax())
		return
	}
	m.Orders++
	for _, l := range o.Lines {
		switch l.Type {
		case "line_item":
			m.SalesHT += l.Total
			m.Discounts += l.Subtotal - l.Total
		case "fee":
			m.FeesHT += l.Total
		}
	}
	m.ShippingHT += o.Shipping
	m.Tax += o.totalTax()
	m.GrossTTC += o.Total
}

func (m *TurnoverMonth) finish(dec int) {
	m.NetHT = m.SalesHT + m.ShippingHT + m.FeesHT - m.RefundsHT
	m.NetTax = m.Tax - m.RefundsTax
	m.NetTTC = m.GrossTTC - m.Refunds
	for _, f := range []*float64{&m.SalesHT, &m.Discounts, &m.ShippingHT, &m.FeesHT, &m.Tax, &m.GrossTTC,
		&m.Refunds, &m.RefundsHT, &m.RefundsTax, &m.NetHT, &m.NetTax, &m.NetTTC} {
		*f = round(*f, dec)
	}
}

func buildTurnover(orders []repOrder, year int, loc *time.Location, dec int) Turnover {
	t := Turnover{Year: year, Note: AccountingNote, Months: make([]TurnoverMonth, 12)}
	for i := range t.Months {
		t.Months[i] = TurnoverMonth{Month: i + 1, Label: frenchMonths[i]}
	}
	t.Totals = TurnoverMonth{Label: "Total"}
	for _, o := range orders {
		local := o.Created.In(loc)
		if local.Year() != year {
			continue
		}
		t.Months[local.Month()-1].add(o)
		t.Totals.add(o)
	}
	for i := range t.Months {
		t.Months[i].finish(dec)
	}
	t.Totals.finish(dec)
	return t
}

// Turnover is the chiffre d'affaires of year, by month (shop timezone).
func (s *Store) Turnover(ctx context.Context, year int) (Turnover, error) {
	loc := s.ReportLocation(ctx)
	from, to := accYearBounds(year, loc)
	orders, err := s.loadReportOrders(ctx, from, to)
	if err != nil {
		return Turnover{}, err
	}
	t := buildTurnover(orders, year, loc, s.Decimals(ctx))
	t.Currency = s.Currency(ctx)
	return t, nil
}

// ------------------------------------------------------------------ journal

// Journal codes.
const (
	JournalSales = "VT"
	JournalBank  = "BQ"
)

// JournalMove is one line of an entry: a debit or a credit on an account.
type JournalMove struct {
	Account AccountingAccount `json:"account"`
	Debit   float64           `json:"debit"`
	Credit  float64           `json:"credit"`
}

// JournalEntry is a balanced accounting entry (écriture).
type JournalEntry struct {
	Date    string        `json:"date"` // YYYY-MM-DD, shop timezone
	Journal string        `json:"journal"`
	Piece   string        `json:"piece"`
	OrderID uint64        `json:"orderId"`
	Label   string        `json:"label"`
	Moves   []JournalMove `json:"moves"`
}

// residualTolerance is how far an order's recorded tax may be off its
// total before the gap is booked on the sales account instead (manual edits).
const residualTolerance = 0.05

// signed books amount on acc: positive is a debit, negative a credit.
func (e *JournalEntry) signed(acc AccountingAccount, amount float64, dec int) {
	amount = round(amount, dec)
	if amount == 0 {
		return
	}
	if amount > 0 {
		e.Moves = append(e.Moves, JournalMove{Account: acc, Debit: amount})
	} else {
		e.Moves = append(e.Moves, JournalMove{Account: acc, Credit: -amount})
	}
}

// orderSplit is a sale's (or a refund's, negative) amounts, rounded, whose
// parts add up to the total exactly.
type orderSplit struct {
	ttc, sales, discount, shipping, fees, tax float64
}

func splitOrder(o repOrder, parent *repOrder, dec int) orderSplit {
	sp := orderSplit{ttc: round(o.Total, dec), shipping: round(o.Shipping, dec), tax: round(o.totalTax(), dec)}
	hasLines := false
	for _, l := range o.Lines {
		switch l.Type {
		case "line_item":
			hasLines = true
			if o.Refund {
				sp.sales += l.Total
			} else {
				sp.sales += l.Subtotal
				sp.discount += l.Subtotal - l.Total
			}
		case "fee":
			hasLines = true
			sp.fees += l.Total
		}
	}
	sp.sales, sp.discount, sp.fees = round(sp.sales, dec), round(sp.discount, dec), round(sp.fees, dec)
	// An amount-only refund carries no breakdown: split it like its order.
	if o.Refund && !hasLines && sp.shipping == 0 && sp.tax == 0 && parent != nil && parent.Total != 0 {
		sp.tax = round(sp.ttc*parent.totalTax()/parent.Total, dec)
		sp.sales = round(sp.ttc-sp.tax, dec)
	}
	residual := round(sp.ttc-(sp.sales-sp.discount+sp.shipping+sp.fees+sp.tax), dec)
	if math.Abs(residual) <= residualTolerance {
		sp.tax = round(sp.tax+residual, dec)
	} else {
		sp.sales = round(sp.sales+residual, dec)
	}
	return sp
}

// paidAt is when an order was paid (zero when it was not): its paid date,
// else its completion, else its creation for a status that means paid.
func (o repOrder) paidAt() time.Time {
	switch {
	case !o.Paid.IsZero():
		return o.Paid
	case !o.Completed.IsZero():
		return o.Completed
	case o.Status == "processing" || o.Status == "completed" || o.Status == "refunded":
		return o.Created
	}
	return time.Time{}
}

func accNumber(id uint64) string { return strconv.FormatUint(id, 10) }

// buildJournal books the orders and refunds created in year: VT entries
// (debit 411 TTC / credit sales, shipping, fees, VAT; coupons debited on
// 709; refunds the other way round) and BQ entries for the payments (debit
// treasury / credit 411) and the refunds paid back, dated in the year.
// parents holds refunds' parent orders (possibly from an earlier year).
func buildJournal(orders []repOrder, parents map[uint64]repOrder, plan AccountPlan, year int, loc *time.Location, dec int) []JournalEntry {
	inYear := func(t time.Time) bool { return !t.IsZero() && t.In(loc).Year() == year }
	day := func(t time.Time) string { return t.In(loc).Format("2006-01-02") }
	entries := []JournalEntry{}
	for _, o := range orders {
		if !inYear(o.Created) {
			continue
		}
		var parent *repOrder
		if o.Refund {
			if p, ok := parents[o.ParentID]; ok {
				parent = &p
			}
		}
		sp := splitOrder(o, parent, dec)
		who := strings.TrimSpace(o.FirstName + " " + o.LastName)
		if who == "" {
			who = o.Email
		}
		if !o.Refund {
			e := JournalEntry{Date: day(o.Created), Journal: JournalSales, Piece: "CMD-" + accNumber(o.ID), OrderID: o.ID,
				Label: strings.TrimSpace("Commande n°" + accNumber(o.ID) + " " + who)}
			e.signed(plan.Customers, sp.ttc, dec)
			e.signed(plan.Discounts, sp.discount, dec)
			e.signed(plan.Sales, -sp.sales, dec)
			e.signed(plan.Shipping, -sp.shipping, dec)
			e.signed(plan.Fees, -sp.fees, dec)
			e.signed(plan.VAT, -sp.tax, dec)
			if len(e.Moves) > 0 {
				entries = append(entries, e)
			}
			if paid := o.paidAt(); inYear(paid) && sp.ttc != 0 {
				title := o.MethodTitle
				if title == "" {
					title = o.Method
				}
				b := JournalEntry{Date: day(paid), Journal: JournalBank, Piece: "CMD-" + accNumber(o.ID), OrderID: o.ID,
					Label: strings.TrimSpace("Encaissement commande n°" + accNumber(o.ID) + " " + title)}
				b.signed(plan.treasury(o.Method), sp.ttc, dec)
				b.signed(plan.Customers, -sp.ttc, dec)
				entries = append(entries, b)
			}
			continue
		}
		// A refund: an avoir, the amounts being negative the sides flip.
		e := JournalEntry{Date: day(o.Created), Journal: JournalSales, Piece: "AV-" + accNumber(o.ID), OrderID: o.ParentID,
			Label: strings.TrimSpace("Remboursement commande n°" + accNumber(o.ParentID) + " " + who)}
		e.signed(plan.Customers, sp.ttc, dec)
		e.signed(plan.Sales, -sp.sales, dec)
		e.signed(plan.Shipping, -sp.shipping, dec)
		e.signed(plan.Fees, -sp.fees, dec)
		e.signed(plan.VAT, -sp.tax, dec)
		if len(e.Moves) > 0 {
			entries = append(entries, e)
		}
		if parent != nil && !parent.paidAt().IsZero() && sp.ttc != 0 {
			b := JournalEntry{Date: day(o.Created), Journal: JournalBank, Piece: "AV-" + accNumber(o.ID), OrderID: o.ParentID,
				Label: "Remboursement client commande n°" + accNumber(o.ParentID)}
			b.signed(plan.Customers, -sp.ttc, dec) // sp.ttc < 0: debit 411
			b.signed(plan.treasury(parent.Method), sp.ttc, dec)
			entries = append(entries, b)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Date != entries[j].Date {
			return entries[i].Date < entries[j].Date
		}
		return entries[i].Journal > entries[j].Journal // VT before BQ on the same day
	})
	return entries
}

// journal loads year's orders (plus refunds' parents) and books them.
func (s *Store) journal(ctx context.Context, year int) ([]JournalEntry, []repOrder, error) {
	loc := s.ReportLocation(ctx)
	from, to := accYearBounds(year, loc)
	dec := s.Decimals(ctx)
	orders, err := s.loadReportOrders(ctx, from, to)
	if err != nil {
		return nil, nil, err
	}
	parents := map[uint64]repOrder{}
	missing := []uint64{}
	for _, o := range orders {
		if !o.Refund {
			parents[o.ID] = o
		}
	}
	for _, o := range orders {
		if _, ok := parents[o.ParentID]; o.Refund && !ok && o.ParentID != 0 {
			missing = append(missing, o.ParentID)
		}
	}
	if len(missing) > 0 {
		more, err := s.loadReportOrdersByID(ctx, missing)
		if err != nil {
			return nil, nil, err
		}
		for _, o := range more {
			parents[o.ID] = o
		}
	}
	return buildJournal(orders, parents, s.AccountPlan(ctx), year, loc, dec), orders, nil
}

// ------------------------------------------------------------------ ledger

// LedgerLine is a move on an account with the account's running balance.
type LedgerLine struct {
	Date    string  `json:"date"`
	Journal string  `json:"journal"`
	Piece   string  `json:"piece"`
	Label   string  `json:"label"`
	Debit   float64 `json:"debit"`
	Credit  float64 `json:"credit"`
	Balance float64 `json:"balance"` // debit − credit so far
}

// LedgerAccount is an account of the general ledger.
type LedgerAccount struct {
	Number      string       `json:"number"`
	Label       string       `json:"label"`
	Lines       []LedgerLine `json:"lines"`
	TotalDebit  float64      `json:"totalDebit"`
	TotalCredit float64      `json:"totalCredit"`
	Balance     float64      `json:"balance"`
}

// Ledger is a year's grand livre.
type Ledger struct {
	Year        int             `json:"year"`
	Currency    string          `json:"currency"`
	Entries     int             `json:"entries"`
	Accounts    []LedgerAccount `json:"accounts"`
	TotalDebit  float64         `json:"totalDebit"`
	TotalCredit float64         `json:"totalCredit"`
	Balanced    bool            `json:"balanced"`
	Note        string          `json:"note"`
}

func buildLedger(entries []JournalEntry, year, dec int) Ledger {
	l := Ledger{Year: year, Entries: len(entries), Note: AccountingNote, Accounts: []LedgerAccount{}}
	byNumber := map[string]*LedgerAccount{}
	order := []string{}
	for _, e := range entries {
		for _, m := range e.Moves {
			a := byNumber[m.Account.Number]
			if a == nil {
				a = &LedgerAccount{Number: m.Account.Number, Label: m.Account.Label}
				byNumber[m.Account.Number] = a
				order = append(order, m.Account.Number)
			}
			a.TotalDebit = round(a.TotalDebit+m.Debit, dec)
			a.TotalCredit = round(a.TotalCredit+m.Credit, dec)
			a.Balance = round(a.TotalDebit-a.TotalCredit, dec)
			a.Lines = append(a.Lines, LedgerLine{Date: e.Date, Journal: e.Journal, Piece: e.Piece, Label: e.Label,
				Debit: m.Debit, Credit: m.Credit, Balance: a.Balance})
			l.TotalDebit = round(l.TotalDebit+m.Debit, dec)
			l.TotalCredit = round(l.TotalCredit+m.Credit, dec)
		}
	}
	sort.Strings(order)
	for _, n := range order {
		l.Accounts = append(l.Accounts, *byNumber[n])
	}
	l.Balanced = round(l.TotalDebit-l.TotalCredit, dec) == 0
	return l
}

// Ledger is the grand livre of year: the sales and payments booked per
// account, with running balances.
func (s *Store) Ledger(ctx context.Context, year int) (Ledger, error) {
	entries, _, err := s.journal(ctx, year)
	if err != nil {
		return Ledger{}, err
	}
	l := buildLedger(entries, year, s.Decimals(ctx))
	l.Currency = s.Currency(ctx)
	return l, nil
}

// ------------------------------------------------------------------ balance

// BalanceRow is an account of the trial balance.
type BalanceRow struct {
	Number        string  `json:"number"`
	Label         string  `json:"label"`
	Debit         float64 `json:"debit"`
	Credit        float64 `json:"credit"`
	BalanceDebit  float64 `json:"balanceDebit"`  // solde débiteur
	BalanceCredit float64 `json:"balanceCredit"` // solde créditeur
}

// BalancePayment is what came in through a payment method.
type BalancePayment struct {
	Method  string            `json:"method"`
	Title   string            `json:"title"`
	Account AccountingAccount `json:"account"`
	Orders  int64             `json:"orders"`
	Amount  float64           `json:"amount"`
}

// Balance is a year's balance des comptes.
type Balance struct {
	Year               int              `json:"year"`
	Currency           string           `json:"currency"`
	Rows               []BalanceRow     `json:"rows"`
	TotalDebit         float64          `json:"totalDebit"`
	TotalCredit        float64          `json:"totalCredit"`
	TotalBalanceDebit  float64          `json:"totalBalanceDebit"`
	TotalBalanceCredit float64          `json:"totalBalanceCredit"`
	Balanced           bool             `json:"balanced"`
	Payments           []BalancePayment `json:"payments"`
	Note               string           `json:"note"`
}

func buildBalance(l Ledger, dec int) Balance {
	b := Balance{Year: l.Year, Currency: l.Currency, Note: l.Note, Rows: []BalanceRow{}, Payments: []BalancePayment{}}
	for _, a := range l.Accounts {
		r := BalanceRow{Number: a.Number, Label: a.Label, Debit: a.TotalDebit, Credit: a.TotalCredit}
		if a.Balance > 0 {
			r.BalanceDebit = a.Balance
		} else {
			r.BalanceCredit = round(-a.Balance, dec)
		}
		b.Rows = append(b.Rows, r)
		b.TotalDebit = round(b.TotalDebit+r.Debit, dec)
		b.TotalCredit = round(b.TotalCredit+r.Credit, dec)
		b.TotalBalanceDebit = round(b.TotalBalanceDebit+r.BalanceDebit, dec)
		b.TotalBalanceCredit = round(b.TotalBalanceCredit+r.BalanceCredit, dec)
	}
	b.Balanced = round(b.TotalDebit-b.TotalCredit, dec) == 0 && round(b.TotalBalanceDebit-b.TotalBalanceCredit, dec) == 0
	return b
}

// Balance is the balance des comptes of year, with the payments received
// per payment method.
func (s *Store) Balance(ctx context.Context, year int) (Balance, error) {
	loc := s.ReportLocation(ctx)
	dec := s.Decimals(ctx)
	entries, orders, err := s.journal(ctx, year)
	if err != nil {
		return Balance{}, err
	}
	l := buildLedger(entries, year, dec)
	l.Currency = s.Currency(ctx)
	b := buildBalance(l, dec)
	plan := s.AccountPlan(ctx)
	byMethod := map[string]*BalancePayment{}
	keys := []string{}
	for _, o := range orders {
		if o.Refund || o.Total == 0 {
			continue
		}
		if paid := o.paidAt(); paid.IsZero() || paid.In(loc).Year() != year {
			continue
		}
		p := byMethod[o.Method]
		if p == nil {
			title := o.MethodTitle
			if title == "" {
				title = o.Method
			}
			p = &BalancePayment{Method: o.Method, Title: title, Account: plan.treasury(o.Method)}
			byMethod[o.Method] = p
			keys = append(keys, o.Method)
		}
		p.Orders++
		p.Amount = round(p.Amount+o.Total, dec)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.Payments = append(b.Payments, *byMethod[k])
	}
	return b, nil
}
