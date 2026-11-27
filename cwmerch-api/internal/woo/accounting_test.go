package woo

import (
	"math"
	"testing"
	"time"
)

func accountingSample() ([]repOrder, map[uint64]repOrder) {
	at := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 10, 0, 0, 0, time.UTC) }
	paid := repOrder{ID: 10, Status: "completed", Created: at(2, 3), Paid: at(2, 4), Method: "bacs", MethodTitle: "Virement",
		Total: 81.6, CartTax: 12.6, ShippingTax: 1, Shipping: 5, Discount: 7,
		Lines: []repLine{{Type: "line_item", Qty: 2, Subtotal: 70, Total: 63}}}
	// Recorded tax off by a cent: absorbed by the VAT line.
	cod := repOrder{ID: 11, Status: "processing", Created: at(3, 9), Method: "cod",
		Total: 24.01, CartTax: 4,
		Lines: []repLine{{Type: "line_item", Qty: 1, Subtotal: 20, Total: 20}}}
	// Manually edited total: the gap goes to sales.
	edited := repOrder{ID: 12, Status: "on-hold", Created: at(5, 1), Method: "cheque",
		Total: 130, CartTax: 20, Lines: []repLine{{Type: "line_item", Qty: 1, Subtotal: 100, Total: 100},
			{Type: "fee", Total: -3}}}
	// Line refund on order 10, and an amount-only one split like its order.
	refund := repOrder{ID: 20, Refund: true, ParentID: 10, Created: at(2, 20), Total: -42, CartTax: -7,
		Lines: []repLine{{Type: "line_item", Qty: -1, Subtotal: -35, Total: -35}}}
	amountOnly := repOrder{ID: 21, Refund: true, ParentID: 10, Created: at(3, 1), Total: -10}
	// Last year's order: not booked.
	old := repOrder{ID: 5, Status: "completed", Created: time.Date(2025, 12, 31, 10, 0, 0, 0, time.UTC), Total: 50, CartTax: 10,
		Lines: []repLine{{Type: "line_item", Qty: 1, Subtotal: 40, Total: 40}}}
	orders := []repOrder{old, paid, cod, edited, refund, amountOnly}
	parents := map[uint64]repOrder{10: paid, 11: cod, 12: edited}
	return orders, parents
}

func TestJournalEntriesBalance(t *testing.T) {
	orders, parents := accountingSample()
	entries := buildJournal(orders, parents, DefaultAccountPlan(), 2026, time.UTC, 2)
	if len(entries) == 0 {
		t.Fatal("no entries")
	}
	for _, e := range entries {
		var d, c float64
		for _, m := range e.Moves {
			if m.Debit < 0 || m.Credit < 0 || (m.Debit != 0 && m.Credit != 0) {
				t.Errorf("%s %s: bad move %+v", e.Journal, e.Piece, m)
			}
			d += m.Debit
			c += m.Credit
		}
		if math.Abs(d-c) > 1e-9 {
			t.Errorf("%s %s unbalanced: debit %.2f credit %.2f", e.Journal, e.Piece, d, c)
		}
		if e.Date[:4] != "2026" {
			t.Errorf("%s dated out of the year: %s", e.Piece, e.Date)
		}
	}
	l := buildLedger(entries, 2026, 2)
	if !l.Balanced || l.TotalDebit != l.TotalCredit {
		t.Fatalf("ledger unbalanced: %.2f / %.2f", l.TotalDebit, l.TotalCredit)
	}
	b := buildBalance(l, 2)
	if !b.Balanced || b.TotalBalanceDebit != b.TotalBalanceCredit {
		t.Fatalf("balance unbalanced: %+v", b)
	}
	acc := map[string]LedgerAccount{}
	for _, a := range l.Accounts {
		acc[a.Number] = a
	}
	// 411: invoiced 81.6 + 24.01 + 130, paid 81.6 (bacs) and 24.01 (cash on
	// delivery, processing), refunds 42 + 10 paid back; the on-hold cheque is due.
	if got := acc["411"].Balance; math.Abs(got-130) > 1e-9 {
		t.Errorf("411 balance %.2f", got)
	}
	if got := acc["531"].Balance; got != 24.01 {
		t.Errorf("531 balance %.2f", got)
	}
	if _, ok := acc["5112"]; ok {
		t.Error("an unpaid cheque order must not reach 5112")
	}
	if got := acc["512"].Balance; math.Abs(got-(81.6-42-10)) > 1e-9 {
		t.Errorf("512 balance %.2f", got)
	}
	if got := acc["709"].TotalDebit; got != 7 {
		t.Errorf("709 debit %.2f", got)
	}
	// VAT: 13.6 + 4.01 + 20 collected, 7 + 10*13.6/81.6 (=1.67) refunded.
	if got := acc["44571"].Balance; math.Abs(got-(-(13.6+4.01+20)+7+1.67)) > 1e-9 {
		t.Errorf("44571 balance %.2f", got)
	}
	// The running balance of the last line is the account's balance.
	for _, a := range l.Accounts {
		if last := a.Lines[len(a.Lines)-1].Balance; last != a.Balance {
			t.Errorf("%s: last running balance %.2f != %.2f", a.Number, last, a.Balance)
		}
	}
}

func TestTurnoverTotals(t *testing.T) {
	orders, _ := accountingSample()
	tu := buildTurnover(orders, 2026, time.UTC, 2)
	if tu.Totals.Orders != 3 {
		t.Errorf("orders %d", tu.Totals.Orders)
	}
	feb := tu.Months[1]
	if feb.SalesHT != 63 || feb.Discounts != 7 || feb.ShippingHT != 5 || feb.Tax != 13.6 || feb.Refunds != 42 || feb.RefundsHT != 35 {
		t.Errorf("february: %+v", feb)
	}
	if feb.NetHT != 63+5-35 || feb.NetTTC != 81.6-42 {
		t.Errorf("february net: %+v", feb)
	}
}

func TestParseAccountPlan(t *testing.T) {
	p := parseAccountPlan(`{"sales":"707|Ventes","bank":{"number":"5121","label":"Banque Pop"}}`)
	if p.Sales.Number != "707" || p.Sales.Label != "Ventes" || p.Bank.Number != "5121" || p.Bank.Label != "Banque Pop" {
		t.Errorf("json: %+v", p)
	}
	if p.Customers.Number != "411" {
		t.Errorf("defaults lost: %+v", p.Customers)
	}
	p = parseAccountPlan(`a:1:{s:3:"vat";s:6:"445710";}`)
	if p.VAT.Number != "445710" || p.VAT.Label != "TVA collectée" {
		t.Errorf("php: %+v", p.VAT)
	}
}
