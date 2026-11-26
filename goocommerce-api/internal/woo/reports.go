package woo

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Reports follow WooCommerce Analytics' definitions:
//
//   - gross sales  = Σ product line subtotals (before coupons, excl. tax) + fees;
//   - net sales    = gross − coupons − refunds (refunds excl. tax and shipping);
//   - total sales  = net + taxes + shipping (refunded tax/shipping deducted);
//   - an order counts unless its status is pending, failed, cancelled, a draft
//     or trashed (Analytics' default excluded statuses); a refund counts on the
//     day it was made, unless its parent order is excluded.
//
// Everything is built on one reader, loadReportOrders, that reads order
// headers and lines in either storage (HPOS tables or shop_order posts).

// reportExcludedStatuses are never counted (without the wc- prefix).
var reportExcludedStatuses = []string{"pending", "failed", "cancelled", "checkout-draft", "trash", "auto-draft", "draft"}

// reportPaidStatuses are wc_get_is_paid_statuses().
var reportPaidStatuses = []string{"processing", "completed"}

// repOrder is an order or a refund as the reports read it. Amounts of a
// refund are negative, like WooCommerce stores them.
type repOrder struct {
	ID           uint64
	ParentID     uint64
	Refund       bool
	Status       string
	Currency     string
	Created      time.Time // UTC
	Paid         time.Time // zero when unknown
	Completed    time.Time
	CustomerID   uint64
	Email        string
	FirstName    string
	LastName     string
	Country      string
	Method       string
	MethodTitle  string
	Total        float64
	CartTax      float64
	ShippingTax  float64
	Shipping     float64
	Discount     float64
	DiscountTax  float64
	Lines        []repLine
	parentStatus string
}

// repLine is a woocommerce_order_items row with its useful meta.
type repLine struct {
	ID          uint64
	Type        string // line_item, shipping, fee, coupon, tax
	Name        string
	ProductID   uint64
	VariationID uint64
	Qty         float64
	Subtotal    float64
	Total       float64
	Tax         float64
}

func (o repOrder) totalTax() float64 { return o.CartTax + o.ShippingTax }

// customerKey identifies a customer across orders: the user, else the
// billing email (guests).
func (o repOrder) customerKey() string {
	if o.CustomerID != 0 {
		return "u:" + strconv.FormatUint(o.CustomerID, 10)
	}
	if e := strings.ToLower(strings.TrimSpace(o.Email)); e != "" {
		return "e:" + e
	}
	return ""
}

func repStatus(v string) string { return strings.TrimPrefix(v, "wc-") }

// repStatusArgs is the excluded statuses as stored, with and without the
// wc- prefix (drafts and trash are stored either way across versions).
func repStatusArgs() []any {
	out := make([]any, 0, 2*len(reportExcludedStatuses))
	for _, st := range reportExcludedStatuses {
		out = append(out, st, "wc-"+st)
	}
	return out
}

// idsOrNone is inList, matching nothing when ids is empty.
func idsOrNone(ids []uint64) string {
	if len(ids) == 0 {
		return "NULL"
	}
	return inList(ids)
}

// loadReportOrders reads the counted orders and refunds created in [from, to).
func (s *Store) loadReportOrders(ctx context.Context, from, to time.Time) ([]repOrder, error) {
	return s.loadReportOrdersWhere(ctx, "o.date_created_gmt >= ? AND o.date_created_gmt < ?",
		"p.post_date_gmt >= ? AND p.post_date_gmt < ?", from.UTC(), to.UTC())
}

// loadReportOrdersByID reads orders/refunds by id (status filter still applies).
func (s *Store) loadReportOrdersByID(ctx context.Context, ids []uint64) ([]repOrder, error) {
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	return s.loadReportOrdersWhere(ctx, "o.id IN ("+inList(ids)+")", "p.ID IN ("+inList(ids)+")")
}

func (s *Store) loadReportOrdersWhere(ctx context.Context, hposWhere, postsWhere string, args ...any) ([]repOrder, error) {
	hpos, _ := s.HPOS(ctx)
	var orders []repOrder
	var err error
	if hpos {
		orders, err = s.loadHPOSReportOrders(ctx, hposWhere, args)
	} else {
		orders, err = s.loadPostReportOrders(ctx, postsWhere, args)
	}
	if err != nil {
		return nil, err
	}
	ids := make([]uint64, len(orders))
	for i, o := range orders {
		ids[i] = o.ID
	}
	lines, err := s.loadReportLines(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		orders[i].Lines = lines[orders[i].ID]
	}
	return orders, nil
}

func (s *Store) loadHPOSReportOrders(ctx context.Context, where string, args []any) ([]repOrder, error) {
	ex := repStatusArgs()
	q := `SELECT o.id, o.type, COALESCE(o.status,''), COALESCE(o.currency,''), o.date_created_gmt, COALESCE(o.parent_order_id,0),
		COALESCE(NULLIF(o.customer_id,0), par.customer_id, 0), COALESCE(NULLIF(o.billing_email,''), par.billing_email, ''),
		COALESCE(o.total_amount,0), COALESCE(o.tax_amount,0),
		COALESCE(NULLIF(o.payment_method,''), par.payment_method, ''), COALESCE(NULLIF(o.payment_method_title,''), par.payment_method_title, ''),
		op.date_paid_gmt, op.date_completed_gmt,
		COALESCE(op.shipping_total_amount,0), COALESCE(op.shipping_tax_amount,0),
		COALESCE(op.discount_total_amount,0), COALESCE(op.discount_tax_amount,0),
		COALESCE(a.first_name,''), COALESCE(a.last_name,''), COALESCE(a.country,''), COALESCE(par.status,'')
		FROM ` + s.T("wc_orders") + ` o
		LEFT JOIN ` + s.T("wc_orders") + ` par ON par.id = o.parent_order_id AND o.type = 'shop_order_refund'
		LEFT JOIN ` + s.T("wc_order_operational_data") + ` op ON op.order_id = o.id
		LEFT JOIN ` + s.T("wc_order_addresses") + ` a ON a.order_id = IF(o.type = 'shop_order_refund', o.parent_order_id, o.id) AND a.address_type = 'billing'
		WHERE (` + where + `)
		AND ((o.type = 'shop_order' AND o.status NOT IN (` + placeholders(len(ex)) + `))
		  OR (o.type = 'shop_order_refund' AND COALESCE(par.status,'') NOT IN (` + placeholders(len(ex)) + `)))
		ORDER BY o.date_created_gmt, o.id`
	all := append(append(append([]any{}, args...), ex...), ex...)
	rows, err := s.DB.QueryContext(ctx, q, all...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []repOrder{}
	seen := map[uint64]bool{}
	for rows.Next() {
		var o repOrder
		var typ string
		var created, paid, completed sql.NullTime
		var total, tax, ship, shipTax, disc, discTax sql.NullFloat64
		if err := rows.Scan(&o.ID, &typ, &o.Status, &o.Currency, &created, &o.ParentID, &o.CustomerID, &o.Email,
			&total, &tax, &o.Method, &o.MethodTitle, &paid, &completed, &ship, &shipTax, &disc, &discTax,
			&o.FirstName, &o.LastName, &o.Country, &o.parentStatus); err != nil {
			return nil, err
		}
		if seen[o.ID] { // two billing address rows: keep the first
			continue
		}
		seen[o.ID] = true
		o.Refund = typ == "shop_order_refund"
		o.Status = repStatus(o.Status)
		o.parentStatus = repStatus(o.parentStatus)
		o.Created = created.Time.UTC()
		if paid.Valid {
			o.Paid = paid.Time.UTC()
		}
		if completed.Valid {
			o.Completed = completed.Time.UTC()
		}
		o.Total, o.ShippingTax, o.Shipping = total.Float64, shipTax.Float64, ship.Float64
		o.CartTax = tax.Float64 - shipTax.Float64 // tax_amount is the order's total tax
		o.Discount, o.DiscountTax = disc.Float64, discTax.Float64
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) loadPostReportOrders(ctx context.Context, where string, args []any) ([]repOrder, error) {
	ex := repStatusArgs()
	q := `SELECT p.ID, p.post_type, p.post_status, p.post_date_gmt, p.post_parent, COALESCE(par.post_status,'')
		FROM ` + s.T("posts") + ` p
		LEFT JOIN ` + s.T("posts") + ` par ON par.ID = p.post_parent AND p.post_type = 'shop_order_refund'
		WHERE (` + where + `)
		AND ((p.post_type = 'shop_order' AND p.post_status NOT IN (` + placeholders(len(ex)) + `))
		  OR (p.post_type = 'shop_order_refund' AND COALESCE(par.post_status,'') NOT IN (` + placeholders(len(ex)) + `)))
		ORDER BY p.post_date_gmt, p.ID`
	all := append(append(append([]any{}, args...), ex...), ex...)
	rows, err := s.DB.QueryContext(ctx, q, all...)
	if err != nil {
		return nil, err
	}
	out := []repOrder{}
	for rows.Next() {
		var o repOrder
		var typ string
		var created sql.NullTime
		if err := rows.Scan(&o.ID, &typ, &o.Status, &created, &o.ParentID, &o.parentStatus); err != nil {
			rows.Close()
			return nil, err
		}
		o.Refund = typ == "shop_order_refund"
		o.Status = repStatus(o.Status)
		o.parentStatus = repStatus(o.parentStatus)
		o.Created = created.Time.UTC()
		out = append(out, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ids := make([]uint64, 0, len(out)*2)
	for _, o := range out {
		ids = append(ids, o.ID)
		if o.Refund && o.ParentID != 0 {
			ids = append(ids, o.ParentID)
		}
	}
	metas, err := s.postMetas(ctx, s.DB, uniqueIDs(ids))
	if err != nil {
		return nil, err
	}
	for i := range out {
		o := &out[i]
		m := metas[o.ID]
		src := m
		if o.Refund {
			if pm, ok := metas[o.ParentID]; ok {
				src = pm // customer, billing and payment come from the parent order
			}
		}
		o.Currency = m.Get("_order_currency")
		if o.Currency == "" {
			o.Currency = src.Get("_order_currency")
		}
		o.Total = m.Float("_order_total")
		o.CartTax = m.Float("_order_tax")
		o.ShippingTax = m.Float("_order_shipping_tax")
		o.Shipping = m.Float("_order_shipping")
		o.Discount = m.Float("_cart_discount")
		o.DiscountTax = m.Float("_cart_discount_tax")
		o.CustomerID = uint64(src.Int("_customer_user"))
		o.Email = src.Get("_billing_email")
		o.FirstName = src.Get("_billing_first_name")
		o.LastName = src.Get("_billing_last_name")
		o.Country = src.Get("_billing_country")
		o.Method = src.Get("_payment_method")
		o.MethodTitle = src.Get("_payment_method_title")
		o.Paid = repMetaTimestamp(m, "_date_paid", "_paid_date")
		o.Completed = repMetaTimestamp(m, "_date_completed", "_completed_date")
	}
	return out, nil
}

// repMetaTimestamp reads a unix timestamp meta, else a "Y-m-d H:i:s" one.
func repMetaTimestamp(m Meta, tsKey, dateKey string) time.Time {
	if v := strings.TrimSpace(m.Get(tsKey)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return time.Unix(n, 0).UTC()
		}
	}
	if v := strings.TrimSpace(m.Get(dateKey)); v != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", v); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

var repItemMetaKeys = []string{"_product_id", "_variation_id", "_qty", "_line_subtotal", "_line_total", "_line_tax", "cost", "total_tax"}

// loadReportLines reads the order items of orders, by order.
func (s *Store) loadReportLines(ctx context.Context, orderIDs []uint64) (map[uint64][]repLine, error) {
	out := map[uint64][]repLine{}
	for _, chunk := range chunks(uniqueIDs(orderIDs), 500) {
		rows, err := s.DB.QueryContext(ctx, `SELECT order_item_id, order_id, order_item_type, order_item_name FROM `+
			s.T("woocommerce_order_items")+` WHERE order_id IN (`+inList(chunk)+`) AND order_item_type IN ('line_item','shipping','fee') ORDER BY order_item_id`)
		if err != nil {
			return nil, err
		}
		type ref struct {
			order uint64
			idx   int
		}
		refs := map[uint64]ref{}
		itemIDs := []uint64{}
		for rows.Next() {
			var l repLine
			var orderID uint64
			if err := rows.Scan(&l.ID, &orderID, &l.Type, &l.Name); err != nil {
				rows.Close()
				return nil, err
			}
			refs[l.ID] = ref{orderID, len(out[orderID])}
			out[orderID] = append(out[orderID], l)
			itemIDs = append(itemIDs, l.ID)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		for _, ic := range chunks(itemIDs, 1000) {
			mrows, err := s.DB.QueryContext(ctx, `SELECT order_item_id, meta_key, meta_value FROM `+s.T("woocommerce_order_itemmeta")+
				` WHERE order_item_id IN (`+inList(ic)+`) AND meta_key IN (`+placeholders(len(repItemMetaKeys))+`)`, strArgs(repItemMetaKeys)...)
			if err != nil {
				return nil, err
			}
			for mrows.Next() {
				var id uint64
				var k string
				var v sql.NullString
				if err := mrows.Scan(&id, &k, &v); err != nil {
					mrows.Close()
					return nil, err
				}
				r := refs[id]
				l := &out[r.order][r.idx]
				f := parseFloat(v.String)
				switch k {
				case "_product_id":
					l.ProductID = ParseID(v.String)
				case "_variation_id":
					l.VariationID = ParseID(v.String)
				case "_qty":
					l.Qty = f
				case "_line_subtotal":
					l.Subtotal = f
				case "_line_total", "cost":
					l.Total = f
				case "_line_tax", "total_tax":
					l.Tax = f
				}
			}
			mrows.Close()
			if err := mrows.Err(); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// ------------------------------------------------------------------ totals

// repTotals accumulates the Analytics metrics of orders and refunds.
type repTotals struct {
	Orders       int64
	ItemsSold    float64
	Gross        float64
	Discount     float64
	RefundsNet   float64 // refunded, excl. tax and shipping (positive)
	RefundsTotal float64 // refunded, all included (positive)
	Shipping     float64
	Taxes        float64
}

func (t *repTotals) add(o repOrder) {
	if o.Refund {
		t.RefundsTotal += -o.Total
		t.RefundsNet += -(o.Total - o.totalTax() - o.Shipping)
	} else {
		t.Orders++
		t.Discount += o.Discount
	}
	for _, l := range o.Lines {
		switch l.Type {
		case "line_item":
			t.ItemsSold += l.Qty
			if !o.Refund {
				t.Gross += l.Subtotal
			}
		case "fee":
			if !o.Refund {
				t.Gross += l.Total
			}
		}
	}
	t.Shipping += o.Shipping
	t.Taxes += o.totalTax()
}

func (t repTotals) net() float64 { return t.Gross - t.Discount - t.RefundsNet }

// ReportSummary is the headline numbers of a period.
type ReportSummary struct {
	From              string  `json:"from"`
	To                string  `json:"to"`
	Currency          string  `json:"currency"`
	GrossSales        float64 `json:"grossSales"`
	NetSales          float64 `json:"netSales"`
	TotalSales        float64 `json:"totalSales"`
	Orders            int64   `json:"orders"`
	ItemsSold         int64   `json:"itemsSold"`
	Refunds           float64 `json:"refunds"`
	RefundedTotal     float64 `json:"refundedTotal"`
	Discount          float64 `json:"discount"`
	Shipping          float64 `json:"shipping"`
	Taxes             float64 `json:"taxes"`
	AverageOrderValue float64 `json:"averageOrderValue"`
	NewCustomers      int64   `json:"newCustomers"`
}

func summaryOf(t repTotals, dec int) ReportSummary {
	net := t.net()
	sum := ReportSummary{
		GrossSales:    round(t.Gross, dec),
		NetSales:      round(net, dec),
		TotalSales:    round(net+t.Taxes+t.Shipping, dec),
		Orders:        t.Orders,
		ItemsSold:     int64(math.Round(t.ItemsSold)),
		Refunds:       round(t.RefundsNet, dec),
		RefundedTotal: round(t.RefundsTotal, dec),
		Discount:      round(t.Discount, dec),
		Shipping:      round(t.Shipping, dec),
		Taxes:         round(t.Taxes, dec),
	}
	if t.Orders > 0 {
		sum.AverageOrderValue = round(net/float64(t.Orders), dec)
	}
	return sum
}

// ReportLocation is the shop's timezone (timezone_string, else gmt_offset).
func (s *Store) ReportLocation(ctx context.Context) *time.Location {
	if tz := s.Option(ctx, "timezone_string"); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	if off := s.Option(ctx, "gmt_offset"); off != "" {
		secs := int(parseFloat(off) * 3600)
		if secs != 0 {
			return time.FixedZone("shop", secs)
		}
	}
	return time.UTC
}

func repDay(t time.Time) string { return t.Format("2006-01-02") }

// ReportSummary is the summary of the orders created in [from, to).
func (s *Store) ReportSummary(ctx context.Context, from, to time.Time) (ReportSummary, error) {
	orders, err := s.loadReportOrders(ctx, from, to)
	if err != nil {
		return ReportSummary{}, err
	}
	var t repTotals
	for _, o := range orders {
		t.add(o)
	}
	sum := summaryOf(t, s.Decimals(ctx))
	sum.From, sum.To, sum.Currency = repDay(from), repDay(to.Add(-time.Second)), s.Currency(ctx)
	sum.NewCustomers, err = s.newCustomers(ctx, from, orders)
	return sum, err
}

// newCustomers counts the customers of orders with no counted order before from.
func (s *Store) newCustomers(ctx context.Context, from time.Time, orders []repOrder) (int64, error) {
	keys := map[string]bool{}
	ids := []uint64{}
	emails := []string{}
	for _, o := range orders {
		if o.Refund {
			continue
		}
		k := o.customerKey()
		if k == "" || keys[k] {
			continue
		}
		keys[k] = true
		if o.CustomerID != 0 {
			ids = append(ids, o.CustomerID)
		} else {
			emails = append(emails, strings.ToLower(strings.TrimSpace(o.Email)))
		}
	}
	if len(keys) == 0 {
		return 0, nil
	}
	earlier := map[string]bool{}
	ex := repStatusArgs()
	hpos, _ := s.HPOS(ctx)
	var q string
	if hpos {
		q = `SELECT DISTINCT COALESCE(customer_id,0), LOWER(COALESCE(billing_email,'')) FROM ` + s.T("wc_orders") +
			` WHERE type = 'shop_order' AND date_created_gmt < ? AND status NOT IN (` + placeholders(len(ex)) + `)
			AND (customer_id IN (` + idsOrNone(ids) + `) OR (COALESCE(customer_id,0) = 0 AND LOWER(billing_email) IN (` + placeholders(len(emails)) + `)))`
	} else {
		q = `SELECT DISTINCT COALESCE(CAST(cu.meta_value AS UNSIGNED),0), LOWER(COALESCE(be.meta_value,'')) FROM ` + s.T("posts") + ` p
			LEFT JOIN ` + s.T("postmeta") + ` cu ON cu.post_id = p.ID AND cu.meta_key = '_customer_user'
			LEFT JOIN ` + s.T("postmeta") + ` be ON be.post_id = p.ID AND be.meta_key = '_billing_email'
			WHERE p.post_type = 'shop_order' AND p.post_date_gmt < ? AND p.post_status NOT IN (` + placeholders(len(ex)) + `)
			AND (cu.meta_value IN (` + repQuotedIDs(ids) + `) OR (COALESCE(cu.meta_value,'0') IN ('0','') AND LOWER(be.meta_value) IN (` + placeholders(len(emails)) + `)))`
	}
	args := append(append([]any{from.UTC()}, ex...), strArgs(emails)...)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uint64
		var email string
		if err := rows.Scan(&id, &email); err != nil {
			return 0, err
		}
		earlier[repOrder{CustomerID: id, Email: email}.customerKey()] = true
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var n int64
	for k := range keys {
		if !earlier[k] {
			n++
		}
	}
	return n, nil
}

// repQuotedIDs renders ids as quoted strings (meta_value is text).
func repQuotedIDs(ids []uint64) string {
	if len(ids) == 0 {
		return "'-'"
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = "'" + strconv.FormatUint(id, 10) + "'"
	}
	return strings.Join(parts, ",")
}

// ------------------------------------------------------------------ intervals

// SalesInterval is one bucket of the sales chart.
type SalesInterval struct {
	Date       string  `json:"date"`
	Orders     int64   `json:"orders"`
	GrossSales float64 `json:"grossSales"`
	NetSales   float64 `json:"netSales"`
	TotalSales float64 `json:"totalSales"`
	ItemsSold  int64   `json:"itemsSold"`
	Refunds    float64 `json:"refunds"`
	Discount   float64 `json:"discount"`
	Taxes      float64 `json:"taxes"`
	Shipping   float64 `json:"shipping"`
}

// SalesReport is the zero-filled sales chart of a period.
type SalesReport struct {
	Interval  string          `json:"interval"`
	Intervals []SalesInterval `json:"intervals"`
}

// maxIntervals bounds a chart (10+ years of days).
const maxIntervals = 4000

// NormalizeInterval is day|week|month|year (day by default).
func NormalizeInterval(interval string) string {
	switch interval {
	case "week", "month", "year":
		return interval
	}
	return "day"
}

// bucketStart is the start of t's bucket, in t's location. Weeks start on
// Monday (ISO 8601, WooCommerce's default start_of_week for most shops).
func bucketStart(t time.Time, interval string) time.Time {
	y, m, d := t.Date()
	loc := t.Location()
	switch interval {
	case "year":
		return time.Date(y, 1, 1, 0, 0, 0, 0, loc)
	case "month":
		return time.Date(y, m, 1, 0, 0, 0, 0, loc)
	case "week":
		day := time.Date(y, m, d, 0, 0, 0, 0, loc)
		back := (int(day.Weekday()) + 6) % 7 // Monday = 0
		return day.AddDate(0, 0, -back)
	}
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func nextBucket(t time.Time, interval string) time.Time {
	switch interval {
	case "year":
		return t.AddDate(1, 0, 0)
	case "month":
		return t.AddDate(0, 1, 0)
	case "week":
		return t.AddDate(0, 0, 7)
	}
	return t.AddDate(0, 0, 1)
}

// bucketStarts lists the buckets covering [from, to), in from's location.
func bucketStarts(from, to time.Time, interval string) []time.Time {
	out := []time.Time{}
	if !to.After(from) {
		return out
	}
	to = to.In(from.Location())
	for b := bucketStart(from, interval); b.Before(to) && len(out) < maxIntervals; b = nextBucket(b, interval) {
		out = append(out, b)
	}
	return out
}

// SalesIntervals is the sales chart of [from, to) by interval, zero-filled,
// bucketed in the shop's timezone.
func (s *Store) SalesIntervals(ctx context.Context, from, to time.Time, interval string) (SalesReport, error) {
	interval = NormalizeInterval(interval)
	loc := s.ReportLocation(ctx)
	orders, err := s.loadReportOrders(ctx, from, to)
	if err != nil {
		return SalesReport{}, err
	}
	return SalesReport{Interval: interval, Intervals: buildIntervals(orders, from.In(loc), to, interval, s.Decimals(ctx))}, nil
}

func buildIntervals(orders []repOrder, from, to time.Time, interval string, dec int) []SalesInterval {
	starts := bucketStarts(from, to, interval)
	totals := make([]repTotals, len(starts))
	index := map[string]int{}
	for i, b := range starts {
		index[repDay(b)] = i
	}
	for _, o := range orders {
		if i, ok := index[repDay(bucketStart(o.Created.In(from.Location()), interval))]; ok {
			totals[i].add(o)
		}
	}
	out := make([]SalesInterval, len(starts))
	for i, b := range starts {
		sum := summaryOf(totals[i], dec)
		out[i] = SalesInterval{Date: repDay(b), Orders: sum.Orders, GrossSales: sum.GrossSales, NetSales: sum.NetSales,
			TotalSales: sum.TotalSales, ItemsSold: sum.ItemsSold, Refunds: sum.Refunds, Discount: sum.Discount,
			Taxes: sum.Taxes, Shipping: sum.Shipping}
	}
	return out
}

// ------------------------------------------------------------------ products / categories

// TopProduct is a product's sales over a period.
type TopProduct struct {
	ProductID uint64  `json:"productId"`
	Name      string  `json:"name"`
	SKU       string  `json:"sku"`
	ItemsSold int64   `json:"itemsSold"`
	NetSales  float64 `json:"netSales"`
	Orders    int64   `json:"orders"`
}

type repProductAgg struct {
	name   string
	qty    float64
	net    float64
	orders map[uint64]bool
}

func productAggregates(orders []repOrder) map[uint64]*repProductAgg {
	agg := map[uint64]*repProductAgg{}
	for _, o := range orders {
		orderID := o.ID
		if o.Refund {
			orderID = o.ParentID
		}
		for _, l := range o.Lines {
			if l.Type != "line_item" || l.ProductID == 0 {
				continue
			}
			a := agg[l.ProductID]
			if a == nil {
				a = &repProductAgg{name: l.Name, orders: map[uint64]bool{}}
				agg[l.ProductID] = a
			}
			a.qty += l.Qty
			a.net += l.Total
			if !o.Refund {
				a.orders[orderID] = true
			}
		}
	}
	return agg
}

func repLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 100 {
		return 100
	}
	return limit
}

// TopProducts is the best-selling products of [from, to) by net sales.
func (s *Store) TopProducts(ctx context.Context, from, to time.Time, limit int) ([]TopProduct, error) {
	orders, err := s.loadReportOrders(ctx, from, to)
	if err != nil {
		return nil, err
	}
	agg := productAggregates(orders)
	ids := make([]uint64, 0, len(agg))
	for id := range agg {
		ids = append(ids, id)
	}
	titles, skus, err := s.repProductTitles(ctx, ids)
	if err != nil {
		return nil, err
	}
	dec := s.Decimals(ctx)
	out := make([]TopProduct, 0, len(agg))
	for id, a := range agg {
		name := titles[id]
		if name == "" {
			name = a.name
		}
		out = append(out, TopProduct{ProductID: id, Name: name, SKU: skus[id], ItemsSold: int64(math.Round(a.qty)),
			NetSales: round(a.net, dec), Orders: int64(len(a.orders))})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].NetSales != out[j].NetSales {
			return out[i].NetSales > out[j].NetSales
		}
		if out[i].ItemsSold != out[j].ItemsSold {
			return out[i].ItemsSold > out[j].ItemsSold
		}
		return out[i].ProductID < out[j].ProductID
	})
	if l := repLimit(limit); len(out) > l {
		out = out[:l]
	}
	return out, nil
}

func (s *Store) repProductTitles(ctx context.Context, ids []uint64) (map[uint64]string, map[uint64]string, error) {
	titles, skus := map[uint64]string{}, map[uint64]string{}
	for _, chunk := range chunks(uniqueIDs(ids), 500) {
		rows, err := s.DB.QueryContext(ctx, `SELECT p.ID, p.post_title, COALESCE((SELECT meta_value FROM `+s.T("postmeta")+
			` m WHERE m.post_id = p.ID AND m.meta_key = '_sku' LIMIT 1),'') FROM `+s.T("posts")+` p WHERE p.ID IN (`+inList(chunk)+`)`)
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			var id uint64
			var title, sku string
			if err := rows.Scan(&id, &title, &sku); err != nil {
				rows.Close()
				return nil, nil, err
			}
			titles[id], skus[id] = title, sku
		}
		rows.Close()
	}
	return titles, skus, nil
}

// TopCategory is a product category's sales over a period.
type TopCategory struct {
	CategoryID    uint64  `json:"categoryId"`
	Name          string  `json:"name"`
	Slug          string  `json:"slug"`
	ItemsSold     int64   `json:"itemsSold"`
	NetSales      float64 `json:"netSales"`
	Orders        int64   `json:"orders"`
	ProductsCount int64   `json:"productsCount"`
}

// TopCategories is the best-selling product categories of [from, to): each
// product's sales count in every category it is in (product_cat terms).
func (s *Store) TopCategories(ctx context.Context, from, to time.Time, limit int) ([]TopCategory, error) {
	orders, err := s.loadReportOrders(ctx, from, to)
	if err != nil {
		return nil, err
	}
	agg := productAggregates(orders)
	ids := make([]uint64, 0, len(agg))
	for id := range agg {
		ids = append(ids, id)
	}
	type cat struct {
		id         uint64
		name, slug string
		qty, net   float64
		orders     map[uint64]bool
		products   int64
	}
	cats := map[uint64]*cat{}
	for _, chunk := range chunks(uniqueIDs(ids), 500) {
		rows, err := s.DB.QueryContext(ctx, `SELECT tr.object_id, t.term_id, t.name, t.slug FROM `+s.T("term_relationships")+` tr
			JOIN `+s.T("term_taxonomy")+` tt ON tt.term_taxonomy_id = tr.term_taxonomy_id AND tt.taxonomy = 'product_cat'
			JOIN `+s.T("terms")+` t ON t.term_id = tt.term_id
			WHERE tr.object_id IN (`+inList(chunk)+`)`)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var pid, tid uint64
			var name, slug string
			if err := rows.Scan(&pid, &tid, &name, &slug); err != nil {
				rows.Close()
				return nil, err
			}
			c := cats[tid]
			if c == nil {
				c = &cat{id: tid, name: name, slug: slug, orders: map[uint64]bool{}}
				cats[tid] = c
			}
			a := agg[pid]
			c.qty += a.qty
			c.net += a.net
			c.products++
			for oid := range a.orders {
				c.orders[oid] = true
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	dec := s.Decimals(ctx)
	out := make([]TopCategory, 0, len(cats))
	for _, c := range cats {
		out = append(out, TopCategory{CategoryID: c.id, Name: c.name, Slug: c.slug, ItemsSold: int64(math.Round(c.qty)),
			NetSales: round(c.net, dec), Orders: int64(len(c.orders)), ProductsCount: c.products})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].NetSales != out[j].NetSales {
			return out[i].NetSales > out[j].NetSales
		}
		return out[i].CategoryID < out[j].CategoryID
	})
	if l := repLimit(limit); len(out) > l {
		out = out[:l]
	}
	return out, nil
}

// ------------------------------------------------------------------ stock

// StockItem is a product or variation's stock.
type StockItem struct {
	ID             uint64   `json:"id"`
	ParentID       uint64   `json:"parentId"`
	Type           string   `json:"type"` // product | variation
	Name           string   `json:"name"`
	SKU            string   `json:"sku"`
	Status         string   `json:"status"`
	ManageStock    bool     `json:"manageStock"`
	StockQuantity  *float64 `json:"stockQuantity"`
	StockStatus    string   `json:"stockStatus"`
	LowStockAmount float64  `json:"lowStockAmount"`
}

// NormalizeStockType is low|out|all (all by default).
func NormalizeStockType(typ string) string {
	switch typ {
	case "low", "out":
		return typ
	}
	return "all"
}

// StockReport lists products and variations by stock: "low" are the
// stock-managed ones at or under their low-stock threshold (the product's
// _low_stock_amount, a variation's falling back to its parent's, else
// woocommerce_notify_low_stock_amount) and still above the out-of-stock
// threshold; "out" are the out-of-stock ones; "all" every one.
func (s *Store) StockReport(ctx context.Context, typ string, page, perPage int) (Paged[StockItem], error) {
	typ = NormalizeStockType(typ)
	page, perPage, offset := Page(page, perPage)
	global := float64(s.OptionInt(ctx, "woocommerce_notify_low_stock_amount", 2))
	noStock := float64(s.OptionInt(ctx, "woocommerce_notify_no_stock_amount", 0))
	pm := s.T("postmeta")
	meta := func(alias, key string) string {
		return `(SELECT meta_value FROM ` + pm + ` WHERE post_id = ` + alias + `.ID AND meta_key = '` + key + `' ORDER BY meta_id LIMIT 1)`
	}
	inner := `SELECT p.ID AS id, p.post_parent AS parent, p.post_type AS ptype, p.post_title AS title, p.post_status AS status,
		COALESCE(` + meta("p", "_sku") + `,'') AS sku,
		COALESCE(` + meta("p", "_manage_stock") + `,'no') AS manage,
		` + meta("p", "_stock") + ` AS stock,
		COALESCE(` + meta("p", "_stock_status") + `,'instock') AS stock_status,
		COALESCE(NULLIF(` + meta("p", "_low_stock_amount") + `,''), NULLIF(` + meta("par", "_low_stock_amount") + `,'')) AS low
		FROM ` + s.T("posts") + ` p
		LEFT JOIN ` + s.T("posts") + ` par ON par.ID = p.post_parent AND p.post_type = 'product_variation'
		WHERE p.post_type IN ('product','product_variation') AND p.post_status NOT IN ('trash','auto-draft')
		AND (p.post_type = 'product' OR par.post_status NOT IN ('trash','auto-draft'))`
	where := "1=1"
	args := []any{}
	switch typ {
	case "low":
		where = `x.manage = 'yes' AND x.stock IS NOT NULL AND x.stock <> '' AND x.stock_status <> 'outofstock'
			AND CAST(x.stock AS DECIMAL(20,4)) <= CAST(COALESCE(x.low, ?) AS DECIMAL(20,4))
			AND CAST(x.stock AS DECIMAL(20,4)) > ?`
		args = append(args, fmtFloat(global), noStock)
	case "out":
		where = `x.stock_status = 'outofstock'`
	}
	var total int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM (`+inner+`) x WHERE `+where, args...).Scan(&total); err != nil {
		return Paged[StockItem]{}, err
	}
	order := "x.title, x.id"
	if typ == "low" {
		order = "CAST(x.stock AS DECIMAL(20,4)), x.title, x.id"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT x.id, x.parent, x.ptype, x.title, x.status, x.sku, x.manage, x.stock, x.stock_status, x.low FROM (`+
		inner+`) x WHERE `+where+` ORDER BY `+order+` LIMIT ? OFFSET ?`, append(args, perPage, offset)...)
	if err != nil {
		return Paged[StockItem]{}, err
	}
	defer rows.Close()
	items := []StockItem{}
	for rows.Next() {
		var it StockItem
		var ptype, manage string
		var stock, low sql.NullString
		if err := rows.Scan(&it.ID, &it.ParentID, &ptype, &it.Name, &it.Status, &it.SKU, &manage, &stock, &it.StockStatus, &low); err != nil {
			return Paged[StockItem]{}, err
		}
		it.Type = "product"
		if ptype == "product_variation" {
			it.Type = "variation"
		}
		it.ManageStock = manage == "yes"
		if stock.Valid && strings.TrimSpace(stock.String) != "" {
			f := parseFloat(stock.String)
			it.StockQuantity = &f
		}
		it.LowStockAmount = global
		if low.Valid && strings.TrimSpace(low.String) != "" {
			it.LowStockAmount = parseFloat(low.String)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return Paged[StockItem]{}, err
	}
	return NewPaged(items, total, page, perPage), nil
}

// ------------------------------------------------------------------ customers

// CustomerReportRow is a customer's (registered or guest) orders over a period.
type CustomerReportRow struct {
	CustomerID        uint64     `json:"customerId"` // 0 for a guest
	Guest             bool       `json:"guest"`
	Name              string     `json:"name"`
	Email             string     `json:"email"`
	Country           string     `json:"country"`
	Orders            int64      `json:"orders"`
	TotalSpend        float64    `json:"totalSpend"`
	AverageOrderValue float64    `json:"averageOrderValue"`
	LastOrderAt       *time.Time `json:"lastOrderAt"`
}

// CustomerReport is the customers of [from, to) by total spend (orders'
// totals, refunds deducted), guests grouped by billing email.
func (s *Store) CustomerReport(ctx context.Context, from, to time.Time, page, perPage int) (Paged[CustomerReportRow], error) {
	page, perPage, offset := Page(page, perPage)
	orders, err := s.loadReportOrders(ctx, from, to)
	if err != nil {
		return Paged[CustomerReportRow]{}, err
	}
	rows := customerRows(orders, s.Decimals(ctx))
	total := int64(len(rows))
	if offset > len(rows) {
		offset = len(rows)
	}
	end := offset + perPage
	if end > len(rows) {
		end = len(rows)
	}
	return NewPaged(rows[offset:end], total, page, perPage), nil
}

func customerRows(orders []repOrder, dec int) []CustomerReportRow {
	byKey := map[string]*CustomerReportRow{}
	orderKey := map[uint64]string{}
	for _, o := range orders {
		if !o.Refund {
			orderKey[o.ID] = o.customerKey()
		}
	}
	for _, o := range orders {
		k := o.customerKey()
		if o.Refund && k == "" {
			k = orderKey[o.ParentID]
		}
		if k == "" {
			continue
		}
		r := byKey[k]
		if r == nil {
			r = &CustomerReportRow{CustomerID: o.CustomerID, Guest: o.CustomerID == 0}
			byKey[k] = r
		}
		r.TotalSpend += o.Total
		if o.Refund {
			continue
		}
		r.Orders++
		created := o.Created
		if r.LastOrderAt == nil || created.After(*r.LastOrderAt) {
			r.LastOrderAt = &created
			if n := strings.TrimSpace(o.FirstName + " " + o.LastName); n != "" {
				r.Name = n
			}
			if o.Email != "" {
				r.Email = o.Email
			}
			if o.Country != "" {
				r.Country = o.Country
			}
		}
	}
	out := make([]CustomerReportRow, 0, len(byKey))
	for _, r := range byKey {
		if r.Orders > 0 {
			r.AverageOrderValue = round(r.TotalSpend/float64(r.Orders), dec)
		}
		r.TotalSpend = round(r.TotalSpend, dec)
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalSpend != out[j].TotalSpend {
			return out[i].TotalSpend > out[j].TotalSpend
		}
		return out[i].Email < out[j].Email
	})
	return out
}
