package woo

import (
	"math"
	"testing"
	"time"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestPostcodeMatches(t *testing.T) {
	cases := []struct {
		pattern, postcode string
		want              bool
	}{
		{"75001", "75001", true}, {"75*", "75011", true}, {"75*", "69001", false},
		{"10000...19999", "13008", true}, {"10000...19999", "20000", false},
		{"SW1A 1AA", "sw1a1aa", true}, {"SW1*", "SW1A 2BB", true},
	}
	for _, c := range cases {
		if got := PostcodeMatches(c.pattern, c.postcode); got != c.want {
			t.Errorf("PostcodeMatches(%q, %q) = %v", c.pattern, c.postcode, got)
		}
	}
}

func TestEvalCostFormulas(t *testing.T) {
	cases := []struct {
		formula string
		qty     int64
		cost    float64
		want    float64
	}{
		{"10", 1, 50, 10}, {"5 + 2 * [qty]", 3, 0, 11}, {"[cost] * 0.1", 1, 80, 8},
		{`[fee percent="10" min_fee="5"]`, 1, 20, 5}, {`2 + [fee percent="10" max_fee="3"]`, 1, 100, 5},
		{"(1 + 1) * 3", 1, 0, 6}, {"", 1, 0, 0}, {"oops", 1, 0, 0},
	}
	for _, c := range cases {
		if got := evalCost(c.formula, c.qty, c.cost, "."); !near(got, c.want) {
			t.Errorf("evalCost(%q) = %v, want %v", c.formula, got, c.want)
		}
	}
	if got := evalCost("2,5 * [qty]", 2, 0, ","); !near(got, 5) {
		t.Errorf("decimal comma: %v", got)
	}
}

func TestTaxCalculations(t *testing.T) {
	vat := []TaxRate{{ID: 1, Rate: 20}}
	if got := CalcExclusiveTax(100, vat)[1]; !near(got, 20) {
		t.Errorf("exclusive: %v", got)
	}
	if got := CalcInclusiveTax(120, vat)[1]; !near(got, 20) {
		t.Errorf("inclusive: %v", got)
	}
	// Canada-style: GST 5% + compound QST 9.975% on price+GST
	rates := []TaxRate{{ID: 1, Rate: 5}, {ID: 2, Rate: 9.975, Compound: true, Priority: 2}}
	ex := CalcExclusiveTax(100, rates)
	if !near(ex[1], 5) || !near(ex[2], 10.47375) {
		t.Errorf("compound exclusive: %v", ex)
	}
	in := CalcInclusiveTax(100+ex[1]+ex[2], rates)
	if !near(in[1], 5) || !near(in[2], 10.47375) {
		t.Errorf("compound inclusive: %v", in)
	}
}

func TestActivePriceHonoursSchedule(t *testing.T) {
	reg, sale := 50.0, 40.0
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	from, to := now.Add(-time.Hour), now.Add(time.Hour)
	if p, on := activePrice(&reg, &sale, &from, &to, now); p != 40 || !on {
		t.Errorf("active sale: %v %v", p, on)
	}
	past := now.Add(-2 * time.Hour)
	if p, on := activePrice(&reg, &sale, nil, &past, now); p != 50 || on {
		t.Errorf("ended sale: %v %v", p, on)
	}
	if p, on := activePrice(&reg, nil, nil, nil, now); p != 50 || on {
		t.Errorf("no sale: %v %v", p, on)
	}
}

func TestStockStatusFor(t *testing.T) {
	if stockStatusFor(true, 0, "no", "", 0) != "outofstock" || stockStatusFor(true, 0, "notify", "", 0) != "onbackorder" ||
		stockStatusFor(true, 3, "no", "", 0) != "instock" || stockStatusFor(false, 0, "no", "onbackorder", 0) != "onbackorder" {
		t.Error("unexpected stock status")
	}
}

func TestSlugify(t *testing.T) {
	if got := Slugify("  Bouquet de Roses Été ! "); got != "bouquet-de-roses-ete" {
		t.Errorf("Slugify = %q", got)
	}
}
