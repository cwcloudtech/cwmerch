package woo

import (
	"testing"
	"time"
)

func TestBucketStart(t *testing.T) {
	paris, _ := time.LoadLocation("Europe/Paris")
	at := time.Date(2026, 11, 26, 15, 4, 0, 0, paris) // a Thursday
	cases := map[string]string{"day": "2026-11-26", "week": "2026-11-23", "month": "2026-11-01", "year": "2026-01-01"}
	for interval, want := range cases {
		if got := repDay(bucketStart(at, interval)); got != want {
			t.Errorf("%s: got %s, want %s", interval, got, want)
		}
	}
	// A Sunday belongs to the week started the Monday before.
	if got := repDay(bucketStart(time.Date(2026, 11, 29, 23, 0, 0, 0, paris), "week")); got != "2026-11-23" {
		t.Errorf("sunday week: got %s", got)
	}
}

func TestBucketStartsZeroFilled(t *testing.T) {
	from := time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	got := bucketStarts(from, to, "month")
	want := []string{"2026-01-01", "2026-02-01", "2026-03-01", "2026-04-01"}
	if len(got) != len(want) {
		t.Fatalf("got %d buckets, want %d", len(got), len(want))
	}
	for i := range want {
		if repDay(got[i]) != want[i] {
			t.Errorf("bucket %d: got %s, want %s", i, repDay(got[i]), want[i])
		}
	}
	days := bucketStarts(from, from.AddDate(0, 0, 3), "day")
	if len(days) != 3 {
		t.Errorf("3 days: got %d buckets", len(days))
	}
	if len(bucketStarts(to, from, "day")) != 0 {
		t.Error("an empty range has no bucket")
	}
}

func TestBuildIntervals(t *testing.T) {
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 3)
	orders := []repOrder{
		{ID: 1, Created: from.Add(10 * time.Hour), Total: 126, CartTax: 20, ShippingTax: 1, Shipping: 5,
			Lines: []repLine{{Type: "line_item", Qty: 2, Subtotal: 110, Total: 100}}, Discount: 10},
		{ID: 2, Refund: true, ParentID: 1, Created: from.AddDate(0, 0, 2), Total: -12, CartTax: -2,
			Lines: []repLine{{Type: "line_item", Qty: -1, Subtotal: -10, Total: -10}}},
	}
	got := buildIntervals(orders, from, to, "day", 2)
	if len(got) != 3 {
		t.Fatalf("got %d intervals", len(got))
	}
	d0 := got[0]
	if d0.Orders != 1 || d0.GrossSales != 110 || d0.NetSales != 100 || d0.TotalSales != 126 || d0.ItemsSold != 2 {
		t.Errorf("day 0: %+v", d0)
	}
	if got[1].Orders != 0 || got[1].NetSales != 0 {
		t.Errorf("day 1 should be empty: %+v", got[1])
	}
	if d2 := got[2]; d2.Refunds != 10 || d2.NetSales != -10 || d2.TotalSales != -12 || d2.ItemsSold != -1 {
		t.Errorf("day 2: %+v", d2)
	}
}
