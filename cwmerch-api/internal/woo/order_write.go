package woo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"cwmerch-api/internal/db"
	"cwmerch-api/internal/php"
)

// OrderEvents receives what WooCommerce would hook emails on.
type OrderEvents interface {
	OrderStatusChanged(ctx context.Context, o Order, from, to string)
	OrderCustomerNote(ctx context.Context, o Order, note string)
	OrderRefunded(ctx context.Context, o Order, r Refund, full bool)
}

// SetEvents wires the email/notification hooks.
func (s *Store) SetEvents(e OrderEvents) { s.events = e }

func randomKey(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		x, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b[i] = alphabet[x.Int64()]
	}
	return string(b)
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

func unixString(t *time.Time) string {
	if t == nil {
		return ""
	}
	return strconv.FormatInt(t.Unix(), 10)
}

// allocateOrderID reserves an id in posts (a placeholder when HPOS is
// authoritative without sync, the real shop_order post otherwise).
func (s *Store) allocateOrderID(ctx context.Context, q db.Querier, postType string, parent uint64, status string) (uint64, error) {
	hpos, sync := s.HPOS(ctx)
	typ := postType
	if hpos && !sync {
		typ = map[string]string{"shop_order": "shop_order_placehold", "shop_order_refund": "shop_order_placehold"}[postType]
	}
	title := "Order &ndash; " + time.Now().UTC().Format("January 2, 2006 @ 03:04 PM")
	if postType == "shop_order_refund" {
		title = "Refund &ndash; " + time.Now().UTC().Format("January 2, 2006 @ 03:04 PM")
	}
	return s.insertPost(ctx, q, typ, wcStatus(status), title, "", "", "", parent, 1, 0, "closed")
}

// persistHeader writes an order's header (status, totals, addresses,
// operational flags) to the authoritative storage and, when the
// compatibility sync is on, to the other one too.
func (s *Store) persistHeader(ctx context.Context, q db.Querier, o *Order, isNew bool) error {
	hpos, sync := s.HPOS(ctx)
	o.UpdatedAt = now()
	if hpos || sync {
		if err := s.persistHPOS(ctx, q, o, isNew); err != nil {
			return err
		}
	}
	if !hpos || sync {
		if err := s.persistLegacy(ctx, q, o); err != nil {
			return err
		}
	} else {
		// keep the placeholder post's status in step (WooCommerce does)
		if _, err := q.ExecContext(ctx, `UPDATE `+s.T("posts")+` SET post_status = ?, post_modified_gmt = ? WHERE ID = ?`, wcStatus(o.Status), o.UpdatedAt, o.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) persistHPOS(ctx context.Context, q db.Querier, o *Order, isNew bool) error {
	typ := orDefault(o.Type, "shop_order")
	if _, err := q.ExecContext(ctx, `INSERT INTO `+s.T("wc_orders")+` (id, status, currency, type, tax_amount, total_amount, customer_id, billing_email,
		date_created_gmt, date_updated_gmt, parent_order_id, payment_method, payment_method_title, transaction_id, ip_address, user_agent, customer_note)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE status = VALUES(status), currency = VALUES(currency), tax_amount = VALUES(tax_amount), total_amount = VALUES(total_amount),
		customer_id = VALUES(customer_id), billing_email = VALUES(billing_email), date_updated_gmt = VALUES(date_updated_gmt),
		payment_method = VALUES(payment_method), payment_method_title = VALUES(payment_method_title), transaction_id = VALUES(transaction_id),
		customer_note = VALUES(customer_note)`,
		o.ID, wcStatus(o.Status), o.Currency, typ, o.TotalTax, o.Total, o.CustomerID, nullIfEmpty(o.Billing.Email), o.CreatedAt, o.UpdatedAt,
		o.ParentID, o.PaymentMethod, o.PaymentMethodTitle, o.TransactionID, o.CustomerIPAddress, o.CustomerUserAgent, o.CustomerNote); err != nil {
		return err
	}
	if typ == "shop_order" {
		for _, kind := range []string{"billing", "shipping"} {
			a := o.Billing
			if kind == "shipping" {
				a = o.Shipping
			}
			if _, err := q.ExecContext(ctx, `DELETE FROM `+s.T("wc_order_addresses")+` WHERE order_id = ? AND address_type = ?`, o.ID, kind); err != nil {
				return err
			}
			if _, err := q.ExecContext(ctx, `INSERT INTO `+s.T("wc_order_addresses")+` (order_id, address_type, first_name, last_name, company, address_1, address_2,
				city, state, postcode, country, email, phone) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				o.ID, kind, a.FirstName, a.LastName, a.Company, a.Address1, a.Address2, a.City, a.State, a.Postcode, a.Country, nullIfEmpty(a.Email), a.Phone); err != nil {
				return err
			}
		}
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM `+s.T("wc_order_operational_data")+` WHERE order_id = ?`, o.ID); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `INSERT INTO `+s.T("wc_order_operational_data")+` (order_id, created_via, woocommerce_version, prices_include_tax,
		coupon_usages_are_counted, download_permission_granted, cart_hash, new_order_email_sent, order_key, order_stock_reduced, date_paid_gmt,
		date_completed_gmt, shipping_tax_amount, shipping_total_amount, discount_tax_amount, discount_total_amount, recorded_sales)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.ID, o.CreatedVia, s.OptionOr(ctx, "woocommerce_version", "9.9.0"), boolInt(o.PricesIncludeTax), boolInt(o.CouponsCounted),
		boolInt(o.DownloadsGranted), o.CartHash, boolInt(o.NewOrderEmailSent), o.OrderKey, boolInt(o.StockReduced), nullTime(o.PaidAt),
		nullTime(o.CompletedAt), o.ShippingTax, o.ShippingTotal, o.DiscountTax, o.DiscountTotal, boolInt(o.SalesRecorded))
	if err != nil {
		return err
	}
	for k, v := range o.Meta {
		if strings.HasPrefix(k, "_refund") || k == "_refunded_by" || k == "_refunded_payment" || !strings.HasPrefix(k, "_") || k == "_cwmerch_source" {
			if _, err := q.ExecContext(ctx, `DELETE FROM `+s.T("wc_orders_meta")+` WHERE order_id = ? AND meta_key = ?`, o.ID, k); err != nil {
				return err
			}
			if _, err := q.ExecContext(ctx, `INSERT INTO `+s.T("wc_orders_meta")+` (order_id, meta_key, meta_value) VALUES (?, ?, ?)`, o.ID, k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Store) persistLegacy(ctx context.Context, q db.Querier, o *Order) error {
	typ := orDefault(o.Type, "shop_order")
	local := s.localTime(ctx, o.UpdatedAt)
	if _, err := q.ExecContext(ctx, `UPDATE `+s.T("posts")+` SET post_type = ?, post_status = ?, post_excerpt = ?, post_parent = ?,
		post_modified = ?, post_modified_gmt = ?, post_date_gmt = ?, post_date = ?, post_password = ? WHERE ID = ?`,
		typ, wcStatus(o.Status), o.CustomerNote, o.ParentID, local, o.UpdatedAt, o.CreatedAt, s.localTime(ctx, o.CreatedAt),
		o.OrderKey, o.ID); err != nil {
		return err
	}
	meta := map[string]string{
		"_order_currency": o.Currency, "_order_total": fmtFloat(o.Total), "_order_tax": fmtFloat(round(o.TotalTax-o.ShippingTax, 6)),
		"_order_shipping": fmtFloat(o.ShippingTotal), "_order_shipping_tax": fmtFloat(o.ShippingTax),
		"_cart_discount": fmtFloat(o.DiscountTotal), "_cart_discount_tax": fmtFloat(o.DiscountTax),
		"_prices_include_tax": yesNo(o.PricesIncludeTax), "_order_version": s.OptionOr(ctx, "woocommerce_version", "9.9.0"),
	}
	if typ == "shop_order" {
		for k, v := range map[string]string{"_order_key": o.OrderKey, "_customer_user": strconv.FormatUint(o.CustomerID, 10),
			"_payment_method": o.PaymentMethod, "_payment_method_title": o.PaymentMethodTitle, "_transaction_id": o.TransactionID,
			"_customer_ip_address": o.CustomerIPAddress, "_customer_user_agent": o.CustomerUserAgent, "_created_via": o.CreatedVia,
			"_cart_hash": o.CartHash, "_date_paid": unixString(o.PaidAt), "_paid_date": fmtDate(o.PaidAt),
			"_date_completed": unixString(o.CompletedAt), "_completed_date": fmtDate(o.CompletedAt),
			"_order_stock_reduced": yesNo(o.StockReduced), "_recorded_sales": yesNo(o.SalesRecorded),
			"_recorded_coupon_usage_counts": yesNo(o.CouponsCounted), "_download_permissions_granted": yesNo(o.DownloadsGranted),
			"_new_order_email_sent": map[bool]string{true: "true", false: "false"}[o.NewOrderEmailSent]} {
			meta[k] = v
		}
		for k, v := range addressMeta(o.Billing, "_billing_") {
			meta[k] = v
		}
		ship := addressMeta(o.Shipping, "_shipping_")
		delete(ship, "_shipping_email")
		for k, v := range ship {
			meta[k] = v
		}
		meta["_billing_address_index"] = strings.Join([]string{o.Billing.FirstName, o.Billing.LastName, o.Billing.Company, o.Billing.Address1,
			o.Billing.Address2, o.Billing.City, o.Billing.State, o.Billing.Postcode, o.Billing.Country, o.Billing.Email, o.Billing.Phone}, " ")
		meta["_shipping_address_index"] = strings.Join([]string{o.Shipping.FirstName, o.Shipping.LastName, o.Shipping.Company, o.Shipping.Address1,
			o.Shipping.Address2, o.Shipping.City, o.Shipping.State, o.Shipping.Postcode, o.Shipping.Country, o.Shipping.Phone}, " ")
	}
	for k, v := range o.Meta {
		if strings.HasPrefix(k, "_refund") || k == "_refunded_by" || k == "_refunded_payment" || !strings.HasPrefix(k, "_") {
			meta[k] = v
		}
	}
	return s.setPostMetas(ctx, q, o.ID, meta)
}

func fmtDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

func (s *Store) addItem(ctx context.Context, q db.Querier, orderID uint64, name, typ string, meta [][2]string) (uint64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO `+s.T("woocommerce_order_items")+` (order_item_name, order_item_type, order_id) VALUES (?, ?, ?)`, name, typ, orderID)
	if err != nil {
		return 0, err
	}
	id64, _ := res.LastInsertId()
	id := uint64(id64)
	for _, kv := range meta {
		if _, err := q.ExecContext(ctx, `INSERT INTO `+s.T("woocommerce_order_itemmeta")+` (order_item_id, meta_key, meta_value) VALUES (?, ?, ?)`, id, kv[0], kv[1]); err != nil {
			return 0, err
		}
	}
	return id, nil
}

func setItemMeta(ctx context.Context, s *Store, q db.Querier, itemID uint64, key, value string) error {
	res, err := q.ExecContext(ctx, `UPDATE `+s.T("woocommerce_order_itemmeta")+` SET meta_value = ? WHERE order_item_id = ? AND meta_key = ?`, value, itemID, key)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, err = q.ExecContext(ctx, `INSERT INTO `+s.T("woocommerce_order_itemmeta")+` (order_item_id, meta_key, meta_value) VALUES (?, ?, ?)`, itemID, key, value)
	}
	return err
}

func lineTaxData(total, subtotal map[uint64]float64) string {
	arr := func(m map[uint64]float64) *php.Array {
		a := php.NewArray()
		for k, v := range m {
			a.Set(int64(k), fmtFloat(v))
		}
		return a
	}
	return php.Marshal(php.NewArray().Set("total", arr(total)).Set("subtotal", arr(subtotal)))
}

// NewOrderInput is everything needed to place an order from a priced cart.
type NewOrderInput struct {
	Cart          Cart
	Billing       Address
	Shipping      Address
	PaymentMethod string
	CustomerID    uint64
	CustomerNote  string
	CreatedVia    string
	IPAddress     string
	UserAgent     string
	Status        string
	// PaymentTitle names an online gateway (stripe, konnect, flouci) that is
	// not one of WooCommerce's offline gateways.
	PaymentTitle string
}

// CreateOrder writes an order (status "pending") from a priced cart.
func (s *Store) CreateOrder(ctx context.Context, in NewOrderInput) (Order, error) {
	cart := in.Cart
	if len(cart.Items) == 0 {
		return Order{}, refuse(400, "errors.cartEmpty", "Your cart is empty")
	}
	gatewayTitle := in.PaymentTitle
	if in.PaymentMethod != "" && gatewayTitle == "" {
		g, ok := s.Gateway(ctx, in.PaymentMethod)
		if !ok {
			return Order{}, refuse(400, "errors.invalidPaymentMethod", "Invalid payment method")
		}
		gatewayTitle = g.Title
	}
	t := now()
	o := Order{Status: orDefault(in.Status, "pending"), Type: "shop_order", Currency: cart.Currency, CreatedAt: t, UpdatedAt: t,
		CustomerID: in.CustomerID, CustomerNote: in.CustomerNote, Billing: in.Billing, Shipping: in.Shipping,
		PaymentMethod: in.PaymentMethod, PaymentMethodTitle: gatewayTitle, PricesIncludeTax: cart.PricesIncludeTax,
		CreatedVia: orDefault(in.CreatedVia, "checkout"), CustomerIPAddress: in.IPAddress, CustomerUserAgent: truncate(in.UserAgent, 200),
		OrderKey: "wc_order_" + randomKey(13), DiscountTotal: cart.DiscountTotal, DiscountTax: cart.DiscountTax,
		ShippingTotal: cart.ShippingTotal, ShippingTax: cart.ShippingTax, TotalTax: cart.TaxTotal, Total: cart.Total,
		CartHash: randomKey(32), Meta: map[string]string{"_cwmerch_source": "cwmerch"}}
	o.Shipping.Email = ""
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		id, err := s.allocateOrderID(ctx, tx, "shop_order", 0, o.Status)
		if err != nil {
			return err
		}
		o.ID = id
		if err := s.persistHeader(ctx, tx, &o, true); err != nil {
			return err
		}
		for _, it := range cart.Items {
			meta := [][2]string{{"_product_id", strconv.FormatUint(it.ProductID, 10)}, {"_variation_id", strconv.FormatUint(it.VariationID, 10)},
				{"_qty", strconv.FormatInt(it.Quantity, 10)}, {"_tax_class", it.TaxClass},
				{"_line_subtotal", fmtFloat(it.Subtotal)}, {"_line_subtotal_tax", fmtFloat(it.SubtotalTax)},
				{"_line_total", fmtFloat(it.Total)}, {"_line_tax", fmtFloat(it.TotalTax)},
				{"_line_tax_data", lineTaxData(it.TotalTaxes, it.SubtotalTaxes)}}
			for _, a := range it.Attributes {
				meta = append(meta, [2]string{a.Slug, s.attributeOptionSlug(ctx, a.Slug, a.Option)})
			}
			name := it.Name
			if it.variation != nil && len(it.Attributes) > 0 {
				opts := []string{}
				for _, a := range it.Attributes {
					opts = append(opts, a.Option)
				}
				name += " - " + strings.Join(opts, ", ")
			}
			if _, err := s.addItem(ctx, tx, id, name, "line_item", meta); err != nil {
				return err
			}
		}
		if cart.ChosenShipping != nil {
			c := cart.ChosenShipping
			itemsList := []string{}
			for _, it := range cart.Items {
				if !it.Virtual {
					itemsList = append(itemsList, fmt.Sprintf("%s &times; %d", it.Name, it.Quantity))
				}
			}
			taxes := php.NewArray()
			total := php.NewArray()
			for k, v := range c.Taxes {
				total.Set(int64(k), fmtFloat(v))
			}
			taxes.Set("total", total)
			if _, err := s.addItem(ctx, tx, id, c.Label, "shipping", [][2]string{{"method_id", c.MethodID},
				{"instance_id", strconv.FormatUint(c.InstanceID, 10)}, {"cost", fmtFloat(c.Cost)}, {"total_tax", fmtFloat(c.Tax)},
				{"taxes", php.Marshal(taxes)}, {"Items", strings.Join(itemsList, ", ")}}); err != nil {
				return err
			}
		}
		for _, f := range cart.Fees {
			if _, err := s.addItem(ctx, tx, id, f.Name, "fee", [][2]string{{"_fee_amount", fmtFloat(f.Total)}, {"_tax_class", ""},
				{"_tax_status", "none"}, {"_line_subtotal", fmtFloat(f.Total)}, {"_line_subtotal_tax", fmtFloat(f.TotalTax)},
				{"_line_total", fmtFloat(f.Total)}, {"_line_tax", fmtFloat(f.TotalTax)},
				{"_line_tax_data", lineTaxData(map[uint64]float64{}, map[uint64]float64{})}}); err != nil {
				return err
			}
		}
		for _, c := range cart.Coupons {
			info := fmt.Sprintf(`[%d,%q,%q,%s]`, c.ID, c.Code, c.DiscountType, fmtFloat(c.Discount))
			if _, err := s.addItem(ctx, tx, id, c.Code, "coupon", [][2]string{{"discount_amount", fmtFloat(c.Discount)},
				{"discount_amount_tax", fmtFloat(c.DiscountTax)}, {"coupon_info", info}}); err != nil {
				return err
			}
		}
		for _, tx2 := range cart.Taxes {
			r := cart.rates[tx2.RateID]
			code := strings.ToUpper(strings.Join([]string{r.Country, r.State, r.Name, strconv.Itoa(r.Priority)}, "-"))
			if _, err := s.addItem(ctx, tx, id, code, "tax", [][2]string{{"rate_id", strconv.FormatUint(tx2.RateID, 10)},
				{"label", tx2.Label}, {"compound", strconv.Itoa(boolInt(tx2.Compound))}, {"tax_amount", fmtFloat(tx2.CartTax)},
				{"shipping_tax_amount", fmtFloat(tx2.ShippingTax)}, {"rate_code", code}, {"rate_percent", fmtFloat(tx2.RatePercent)}}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Order{}, err
	}
	s.AddNote(ctx, o.ID, "Order created by cwmerch.", false, "", "", 0) //nolint:errcheck
	_ = s.syncAnalytics(ctx, o.ID)
	return s.Order(ctx, o.ID)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ------------------------------------------------------------------ status flow

// SetStatus changes an order's status with WooCommerce's side effects:
// stock reduction/restoration, recorded sales, coupon usage, download
// permissions, paid/completed dates, a system note and the email hooks.
func (s *Store) SetStatus(ctx context.Context, id uint64, to, note string, manual bool) (Order, error) {
	if !IsValidStatus(to) {
		return Order{}, refuse(400, "errors.invalidStatus", "Unknown order status")
	}
	o, err := s.Order(ctx, id)
	if err != nil {
		return Order{}, err
	}
	from := o.Status
	if from == to {
		return o, nil
	}
	t := now()
	paidStatuses := map[string]bool{"processing": true, "completed": true}
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		o.Status = to
		if paidStatuses[to] && o.PaidAt == nil {
			o.PaidAt = &t
		}
		if to == "completed" {
			o.CompletedAt = &t
		}
		// stock: reduced when the order is placed on-hold/processing/completed
		if (to == "processing" || to == "completed" || to == "on-hold") && !o.StockReduced {
			if err := s.reduceStock(ctx, tx, &o); err != nil {
				return err
			}
		}
		if (to == "cancelled" || to == "failed" || to == "pending") && o.StockReduced {
			if err := s.restoreStock(ctx, tx, &o); err != nil {
				return err
			}
		}
		if paidStatuses[to] || to == "on-hold" {
			if !o.SalesRecorded {
				for _, li := range o.LineItems {
					if err := s.AddTotalSales(ctx, tx, li.ProductID, li.Quantity); err != nil {
						return err
					}
				}
				o.SalesRecorded = true
			}
			if !o.CouponsCounted {
				for _, c := range o.CouponLines {
					s.RecordCouponUse(ctx, c.Code, o.CustomerID, o.Billing.Email)
				}
				o.CouponsCounted = true
			}
		}
		if to == "cancelled" && o.CouponsCounted {
			for _, c := range o.CouponLines {
				s.ReleaseCouponUse(ctx, c.Code, o.CustomerID, o.Billing.Email)
			}
			o.CouponsCounted = false
		}
		if paidStatuses[to] && !o.DownloadsGranted {
			if err := s.grantDownloads(ctx, tx, &o); err != nil {
				return err
			}
		}
		return s.persistHeader(ctx, tx, &o, false)
	})
	if err != nil {
		return Order{}, err
	}
	msg := fmt.Sprintf("Order status changed from %s to %s.", StatusLabel(from), StatusLabel(to))
	if note != "" {
		msg = note + " " + msg
	}
	_, _ = s.AddNote(ctx, id, msg, false, "", "", 0)
	_ = s.syncAnalytics(ctx, id)
	updated, err := s.Order(ctx, id)
	if err != nil {
		return Order{}, err
	}
	if s.events != nil {
		s.events.OrderStatusChanged(ctx, updated, from, to)
	}
	return updated, nil
}

func (s *Store) reduceStock(ctx context.Context, q db.Querier, o *Order) error {
	changes := []string{}
	for _, li := range o.LineItems {
		if li.ReducedStock > 0 {
			continue
		}
		target := li.ProductID
		if li.VariationID != 0 {
			target = li.VariationID
		}
		ch, err := s.AdjustStock(ctx, q, target, -li.Quantity)
		if err != nil {
			return err
		}
		if ch.Managed {
			if err := setItemMeta(ctx, s, q, li.ID, "_reduced_stock", strconv.FormatInt(li.Quantity, 10)); err != nil {
				return err
			}
			changes = append(changes, fmt.Sprintf("%s (%d&rarr;%d)", li.Name, ch.From, ch.To))
		}
	}
	o.StockReduced = true
	if len(changes) > 0 {
		_, _ = s.addNoteTx(ctx, q, o.ID, "Stock levels reduced: "+strings.Join(changes, ", "))
	}
	return nil
}

func (s *Store) restoreStock(ctx context.Context, q db.Querier, o *Order) error {
	changes := []string{}
	for _, li := range o.LineItems {
		if li.ReducedStock <= 0 {
			continue
		}
		target := li.ProductID
		if li.VariationID != 0 {
			target = li.VariationID
		}
		ch, err := s.AdjustStock(ctx, q, target, li.ReducedStock)
		if err != nil {
			return err
		}
		if _, err := q.ExecContext(ctx, `DELETE FROM `+s.T("woocommerce_order_itemmeta")+` WHERE order_item_id = ? AND meta_key = '_reduced_stock'`, li.ID); err != nil {
			return err
		}
		if ch.Managed {
			changes = append(changes, fmt.Sprintf("%s (%d&rarr;%d)", li.Name, ch.From, ch.To))
		}
	}
	o.StockReduced = false
	if len(changes) > 0 {
		_, _ = s.addNoteTx(ctx, q, o.ID, "Stock levels increased: "+strings.Join(changes, ", "))
	}
	return nil
}

func (s *Store) addNoteTx(ctx context.Context, q db.Querier, orderID uint64, note string) (uint64, error) {
	t := now()
	res, err := q.ExecContext(ctx, `INSERT INTO `+s.T("comments")+` (comment_post_ID, comment_author, comment_author_email, comment_author_url,
		comment_author_IP, comment_date, comment_date_gmt, comment_content, comment_karma, comment_approved, comment_agent, comment_type, comment_parent, user_id)
		VALUES (?, 'WooCommerce', ?, '', '', ?, ?, ?, 0, '1', 'WooCommerce', 'order_note', 0, 0)`,
		orderID, "woocommerce@"+hostOf(s.SiteURL), s.localTime(ctx, t), t, note)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return uint64(id), nil
}

// grantDownloads writes the download permissions of the order's downloadable items.
func (s *Store) grantDownloads(ctx context.Context, q db.Querier, o *Order) error {
	if !s.OptionBool(ctx, "woocommerce_downloads_grant_access_after_payment", true) && o.Status != "completed" {
		return nil
	}
	for _, li := range o.LineItems {
		id := li.ProductID
		if li.VariationID != 0 {
			id = li.VariationID
		}
		m, err := s.postMeta(ctx, q, id)
		if err != nil {
			return err
		}
		if !m.Bool("_downloadable") {
			continue
		}
		limit := intOr(m["_download_limit"], -1)
		expiry := intOr(m["_download_expiry"], -1)
		for _, d := range downloadsFrom(m["_downloadable_files"]) {
			var remaining any
			if limit >= 0 {
				remaining = limit * li.Quantity
			}
			var expires any
			if expiry > 0 {
				expires = time.Now().UTC().Add(time.Duration(expiry) * 24 * time.Hour)
			}
			if _, err := q.ExecContext(ctx, `INSERT INTO `+s.T("woocommerce_downloadable_product_permissions")+` (download_id, product_id, order_id, order_key,
				user_email, user_id, downloads_remaining, access_granted, access_expires, download_count) VALUES (?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP(), ?, 0)`,
				d.ID, id, o.ID, o.OrderKey, o.Billing.Email, nullIfZero(o.CustomerID), remaining, expires); err != nil {
				return err
			}
		}
	}
	o.DownloadsGranted = true
	return nil
}

func nullIfZero(n uint64) any {
	if n == 0 {
		return nil
	}
	return n
}

// CustomerDownload is a download a customer can use.
type CustomerDownload struct {
	ProductID   uint64     `json:"productId"`
	ProductName string     `json:"productName"`
	DownloadID  string     `json:"downloadId"`
	Name        string     `json:"name"`
	OrderID     uint64     `json:"orderId"`
	Remaining   *int64     `json:"remaining"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	URL         string     `json:"url"`
	file        string
}

// Downloads lists a customer's usable downloads.
func (s *Store) Downloads(ctx context.Context, userID uint64, email string) ([]CustomerDownload, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT permission_id, download_id, product_id, order_id, downloads_remaining, access_expires FROM `+
		s.T("woocommerce_downloadable_product_permissions")+` WHERE (user_id = ? OR (user_email = ? AND ? <> ''))
		AND (access_expires IS NULL OR access_expires > UTC_TIMESTAMP()) ORDER BY access_granted DESC`, userID, email, email)
	if err != nil {
		return nil, err
	}
	type perm struct {
		id        uint64
		dl        string
		product   uint64
		order     uint64
		remaining sql.NullString
		expires   sql.NullTime
	}
	perms := []perm{}
	for rows.Next() {
		var p perm
		if err := rows.Scan(&p.id, &p.dl, &p.product, &p.order, &p.remaining, &p.expires); err != nil {
			rows.Close()
			return nil, err
		}
		perms = append(perms, p)
	}
	rows.Close()
	out := []CustomerDownload{}
	for _, p := range perms {
		if p.remaining.Valid && p.remaining.String != "" && parseFloat(p.remaining.String) <= 0 {
			continue
		}
		m, err := s.postMeta(ctx, s.DB, p.product)
		if err != nil {
			continue
		}
		var title string
		_ = s.DB.QueryRowContext(ctx, `SELECT post_title FROM `+s.T("posts")+` WHERE ID = ?`, p.product).Scan(&title)
		for _, d := range downloadsFrom(m["_downloadable_files"]) {
			if d.ID != p.dl {
				continue
			}
			cd := CustomerDownload{ProductID: p.product, ProductName: title, DownloadID: d.ID, Name: d.Name, OrderID: p.order, file: d.File,
				URL: fmt.Sprintf("/v1/users/me/downloads/%d/%s", p.product, d.ID)}
			if p.remaining.Valid && p.remaining.String != "" {
				n := int64(parseFloat(p.remaining.String))
				cd.Remaining = &n
			}
			cd.ExpiresAt = timePtr(p.expires)
			out = append(out, cd)
		}
	}
	return out, nil
}

// UseDownload consumes one download and answers the file URL.
func (s *Store) UseDownload(ctx context.Context, userID uint64, email string, productID uint64, downloadID string) (string, error) {
	list, err := s.Downloads(ctx, userID, email)
	if err != nil {
		return "", err
	}
	for _, d := range list {
		if d.ProductID == productID && d.DownloadID == downloadID {
			_, _ = s.DB.ExecContext(ctx, `UPDATE `+s.T("woocommerce_downloadable_product_permissions")+` SET download_count = download_count + 1,
				downloads_remaining = IF(downloads_remaining = '' OR downloads_remaining IS NULL, downloads_remaining, downloads_remaining - 1)
				WHERE product_id = ? AND download_id = ? AND order_id = ?`, productID, downloadID, d.OrderID)
			return d.file, nil
		}
	}
	return "", ErrNotFound
}

// OrderUpdate is an admin edit of an order (status handled by SetStatus).
type OrderUpdate struct {
	Status        *string  `json:"status"`
	Billing       *Address `json:"billing"`
	Shipping      *Address `json:"shipping"`
	CustomerNote  *string  `json:"customerNote"`
	TransactionID *string  `json:"transactionId"`
	CustomerID    *uint64  `json:"customerId"`
	PaymentMethod *string  `json:"paymentMethod"`
}

// UpdateOrder applies an admin edit.
func (s *Store) UpdateOrder(ctx context.Context, id uint64, u OrderUpdate) (Order, error) {
	o, err := s.Order(ctx, id)
	if err != nil {
		return Order{}, err
	}
	if u.Billing != nil {
		o.Billing = *u.Billing
	}
	if u.Shipping != nil {
		o.Shipping = *u.Shipping
		o.Shipping.Email = ""
	}
	if u.CustomerNote != nil {
		o.CustomerNote = *u.CustomerNote
	}
	if u.TransactionID != nil {
		o.TransactionID = *u.TransactionID
	}
	if u.CustomerID != nil {
		o.CustomerID = *u.CustomerID
	}
	if u.PaymentMethod != nil {
		if g, ok := s.Gateway(ctx, *u.PaymentMethod); ok {
			o.PaymentMethod, o.PaymentMethodTitle = g.ID, g.Title
		}
	}
	if err := s.DB.InTx(ctx, func(tx *sql.Tx) error { return s.persistHeader(ctx, tx, &o, false) }); err != nil {
		return Order{}, err
	}
	if u.Status != nil && *u.Status != o.Status {
		return s.SetStatus(ctx, id, *u.Status, "", true)
	}
	_ = s.syncAnalytics(ctx, id)
	return s.Order(ctx, id)
}

// DeleteOrder trashes (or force deletes) an order.
func (s *Store) DeleteOrder(ctx context.Context, id uint64, force bool) error {
	o, err := s.Order(ctx, id)
	if err != nil {
		return err
	}
	if !force {
		o.Meta["_wp_trash_meta_status"] = wcStatus(o.Status)
		o.Status = "trash"
		return s.DB.InTx(ctx, func(tx *sql.Tx) error {
			hpos, sync := s.HPOS(ctx)
			if hpos || sync {
				if _, err := tx.ExecContext(ctx, `UPDATE `+s.T("wc_orders")+` SET status = 'trash' WHERE id = ?`, id); err != nil {
					return err
				}
			}
			_, err := tx.ExecContext(ctx, `UPDATE `+s.T("posts")+` SET post_status = 'trash' WHERE ID = ?`, id)
			return err
		})
	}
	refunds, _ := s.refundIDs(ctx, s.DB, []uint64{id})
	ids := append([]uint64{id}, refunds[id]...)
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		for _, oid := range ids {
			for _, stmt := range []string{
				`DELETE FROM ` + s.T("woocommerce_order_itemmeta") + ` WHERE order_item_id IN (SELECT order_item_id FROM ` + s.T("woocommerce_order_items") + ` WHERE order_id = ?)`,
				`DELETE FROM ` + s.T("woocommerce_order_items") + ` WHERE order_id = ?`,
				`DELETE FROM ` + s.T("wc_orders_meta") + ` WHERE order_id = ?`,
				`DELETE FROM ` + s.T("wc_order_addresses") + ` WHERE order_id = ?`,
				`DELETE FROM ` + s.T("wc_order_operational_data") + ` WHERE order_id = ?`,
				`DELETE FROM ` + s.T("wc_orders") + ` WHERE id = ?`,
				`DELETE FROM ` + s.T("postmeta") + ` WHERE post_id = ?`,
				`DELETE FROM ` + s.T("posts") + ` WHERE ID = ?`,
				`DELETE FROM ` + s.T("wc_order_stats") + ` WHERE order_id = ?`,
				`DELETE FROM ` + s.T("wc_order_product_lookup") + ` WHERE order_id = ?`,
				`DELETE FROM ` + s.T("wc_order_coupon_lookup") + ` WHERE order_id = ?`,
				`DELETE FROM ` + s.T("wc_order_tax_lookup") + ` WHERE order_id = ?`,
			} {
				if _, err := tx.ExecContext(ctx, stmt, oid); err != nil {
					return err
				}
			}
		}
		_, err := tx.ExecContext(ctx, `DELETE c, m FROM `+s.T("comments")+` c LEFT JOIN `+s.T("commentmeta")+` m ON m.comment_id = c.comment_ID
			WHERE c.comment_post_ID = ? AND c.comment_type = 'order_note'`, id)
		return err
	})
}

// ------------------------------------------------------------------ refunds

// RefundLineInput refunds part of a line.
type RefundLineInput struct {
	ID          uint64  `json:"id"`
	Quantity    int64   `json:"quantity"`
	RefundTotal float64 `json:"refundTotal"`
	RefundTax   float64 `json:"refundTax"`
}

// RefundInput is a refund request (manual refund: money is returned outside cwmerch).
type RefundInput struct {
	Amount    float64           `json:"amount"`
	Reason    string            `json:"reason"`
	LineItems []RefundLineInput `json:"lineItems"`
	Restock   bool              `json:"restock"`
}

// CreateRefund records a refund; a full refund moves the order to "refunded".
func (s *Store) CreateRefund(ctx context.Context, orderID uint64, in RefundInput, by uint64) (Order, Refund, error) {
	o, err := s.Order(ctx, orderID)
	if err != nil {
		return Order{}, Refund{}, err
	}
	dec := s.Decimals(ctx)
	amount := round(in.Amount, dec)
	if amount <= 0 {
		for _, l := range in.LineItems {
			amount += l.RefundTotal + l.RefundTax
		}
		amount = round(amount, dec)
	}
	if amount <= 0 {
		return Order{}, Refund{}, refuse(400, "errors.invalidRefundAmount", "Invalid refund amount")
	}
	available := round(o.Total-o.TotalRefunded, dec)
	if amount > available+0.00001 {
		return Order{}, Refund{}, refuse(400, "errors.refundTooLarge", "Invalid refund amount: more than the remaining order total")
	}
	t := now()
	var refundID uint64
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		id, err := s.allocateOrderID(ctx, tx, "shop_order_refund", orderID, "completed")
		if err != nil {
			return err
		}
		refundID = id
		r := Order{ID: id, Type: "shop_order_refund", Status: "completed", ParentID: orderID, Currency: o.Currency, CreatedAt: t, UpdatedAt: t,
			Total: -amount, PricesIncludeTax: o.PricesIncludeTax, Meta: map[string]string{"_refund_amount": fmtFloat(amount),
				"_refund_reason": in.Reason, "_refunded_by": strconv.FormatUint(by, 10), "_refunded_payment": ""}}
		taxTotal := 0.0
		for _, l := range in.LineItems {
			for _, li := range o.LineItems {
				if li.ID != l.ID {
					continue
				}
				taxTotal += l.RefundTax
				if _, err := s.addItem(ctx, tx, id, li.Name, "line_item", [][2]string{{"_product_id", strconv.FormatUint(li.ProductID, 10)},
					{"_variation_id", strconv.FormatUint(li.VariationID, 10)}, {"_qty", strconv.FormatInt(-l.Quantity, 10)},
					{"_line_subtotal", fmtFloat(-l.RefundTotal)}, {"_line_total", fmtFloat(-l.RefundTotal)},
					{"_line_subtotal_tax", fmtFloat(-l.RefundTax)}, {"_line_tax", fmtFloat(-l.RefundTax)},
					{"_refunded_item_id", strconv.FormatUint(li.ID, 10)}}); err != nil {
					return err
				}
				if in.Restock && l.Quantity > 0 {
					target := li.ProductID
					if li.VariationID != 0 {
						target = li.VariationID
					}
					if _, err := s.AdjustStock(ctx, tx, target, l.Quantity); err != nil {
						return err
					}
				}
			}
		}
		r.TotalTax = -taxTotal
		return s.persistHeader(ctx, tx, &r, true)
	})
	if err != nil {
		return Order{}, Refund{}, err
	}
	note := fmt.Sprintf("Refunded %s %s.", fmtFloat(amount), o.Currency)
	if in.Reason != "" {
		note += " Reason: " + in.Reason
	}
	_, _ = s.AddNote(ctx, orderID, note, false, "", "", 0)
	full := round(o.TotalRefunded+amount, dec) >= round(o.Total, dec)
	_ = s.syncAnalytics(ctx, refundID)
	var updated Order
	if full && o.Status != "refunded" {
		updated, err = s.SetStatus(ctx, orderID, "refunded", "Order fully refunded.", false)
		if err != nil {
			return Order{}, Refund{}, err
		}
	} else {
		updated, err = s.Order(ctx, orderID)
		if err != nil {
			return Order{}, Refund{}, err
		}
	}
	var refund Refund
	for _, r := range updated.Refunds {
		if r.ID == refundID {
			refund = r
		}
	}
	if s.events != nil {
		s.events.OrderRefunded(ctx, updated, refund, full)
	}
	return updated, refund, nil
}

// ------------------------------------------------------------------ analytics

// syncAnalytics refreshes WooCommerce Analytics' lookup tables for an order
// (what the wc-admin Action Scheduler jobs do), so WooCommerce's own reports
// include orders cwmerch created.
func (s *Store) syncAnalytics(ctx context.Context, id uint64) error {
	orders, err := s.loadOrders(ctx, s.DB, []uint64{id})
	if err != nil {
		return err
	}
	o, ok := orders[id]
	if !ok {
		return nil
	}
	customerID, err := s.ensureCustomerLookup(ctx, o)
	if err != nil {
		return err
	}
	items := int64(0)
	net := o.Total - o.TotalTax - o.ShippingTotal
	for _, li := range o.LineItems {
		items += li.Quantity
	}
	created := o.CreatedAt
	returning := 0
	if customerID != 0 {
		var n int
		_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.T("wc_order_stats")+` WHERE customer_id = ? AND order_id <> ? AND date_created_gmt < ?`, customerID, id, created).Scan(&n)
		if n > 0 {
			returning = 1
		}
	}
	paid, completed := any(nil), any(nil)
	if o.PaidAt != nil {
		paid = s.localTime(ctx, *o.PaidAt)
	}
	if o.CompletedAt != nil {
		completed = s.localTime(ctx, *o.CompletedAt)
	}
	if _, err := s.DB.ExecContext(ctx, `REPLACE INTO `+s.T("wc_order_stats")+` (order_id, parent_id, date_created, date_created_gmt, date_paid, date_completed,
		num_items_sold, total_sales, tax_total, shipping_total, net_total, returning_customer, status, customer_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, o.ParentID, s.localTime(ctx, created), created, paid, completed, items, o.Total, o.TotalTax-o.ShippingTax, o.ShippingTotal,
		net, returning, wcStatus(o.Status), customerID); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM `+s.T("wc_order_product_lookup")+` WHERE order_id = ?`, id); err != nil {
		return err
	}
	for _, li := range o.LineItems {
		var coupon float64
		if li.Subtotal > li.Total {
			coupon = li.Subtotal - li.Total
		}
		var shipShare, shipTaxShare float64
		if items > 0 && o.Type != "shop_order_refund" {
			shipShare = o.ShippingTotal * float64(li.Quantity) / float64(items)
			shipTaxShare = o.ShippingTax * float64(li.Quantity) / float64(items)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO `+s.T("wc_order_product_lookup")+` (order_item_id, order_id, product_id, variation_id, customer_id,
			date_created, product_qty, product_net_revenue, product_gross_revenue, coupon_amount, tax_amount, shipping_amount, shipping_tax_amount)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE product_qty = VALUES(product_qty)`,
			li.ID, id, li.ProductID, li.VariationID, nullIfZero(customerID), s.localTime(ctx, created), li.Quantity, li.Total,
			li.Total+li.TotalTax+shipShare+shipTaxShare, coupon, li.TotalTax, shipShare, shipTaxShare); err != nil {
			return err
		}
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM `+s.T("wc_order_coupon_lookup")+` WHERE order_id = ?`, id); err != nil {
		return err
	}
	for _, c := range o.CouponLines {
		cp, err := s.CouponByCode(ctx, c.Code)
		if err != nil {
			continue
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT IGNORE INTO `+s.T("wc_order_coupon_lookup")+` (order_id, coupon_id, date_created, discount_amount) VALUES (?, ?, ?, ?)`,
			id, cp.ID, s.localTime(ctx, created), c.Discount); err != nil {
			return err
		}
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM `+s.T("wc_order_tax_lookup")+` WHERE order_id = ?`, id); err != nil {
		return err
	}
	for _, t := range o.TaxLines {
		if _, err := s.DB.ExecContext(ctx, `INSERT IGNORE INTO `+s.T("wc_order_tax_lookup")+` (order_id, tax_rate_id, date_created, shipping_tax, order_tax, total_tax)
			VALUES (?, ?, ?, ?, ?, ?)`, id, t.RateID, s.localTime(ctx, created), t.ShippingTaxTotal, t.TaxTotal, t.TaxTotal+t.ShippingTaxTotal); err != nil {
			return err
		}
	}
	return nil
}

// ensureCustomerLookup finds or creates the wc_customer_lookup row of an
// order's customer (registered by user id, guests by email).
func (s *Store) ensureCustomerLookup(ctx context.Context, o *Order) (uint64, error) {
	if o.Type == "shop_order_refund" {
		var id uint64
		_ = s.DB.QueryRowContext(ctx, `SELECT customer_id FROM `+s.T("wc_order_stats")+` WHERE order_id = ?`, o.ParentID).Scan(&id)
		return id, nil
	}
	var id uint64
	var err error
	if o.CustomerID != 0 {
		err = s.DB.QueryRowContext(ctx, `SELECT customer_id FROM `+s.T("wc_customer_lookup")+` WHERE user_id = ? LIMIT 1`, o.CustomerID).Scan(&id)
	} else if o.Billing.Email != "" {
		err = s.DB.QueryRowContext(ctx, `SELECT customer_id FROM `+s.T("wc_customer_lookup")+` WHERE email = ? AND user_id IS NULL LIMIT 1`, o.Billing.Email).Scan(&id)
	} else {
		return 0, nil
	}
	b := o.Billing
	if err == nil {
		_, err = s.DB.ExecContext(ctx, `UPDATE `+s.T("wc_customer_lookup")+` SET first_name = ?, last_name = ?, email = ?, date_last_active = UTC_TIMESTAMP(),
			country = ?, postcode = ?, city = ?, state = ? WHERE customer_id = ?`, b.FirstName, b.LastName, b.Email, b.Country, b.Postcode, b.City, b.State, id)
		return id, err
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	var username any
	var registered any
	if o.CustomerID != 0 {
		var login string
		var reg sql.NullTime
		if s.DB.QueryRowContext(ctx, `SELECT user_login, user_registered FROM `+s.T("users")+` WHERE ID = ?`, o.CustomerID).Scan(&login, &reg) == nil {
			username = login
			if reg.Valid {
				registered = reg.Time
			}
		}
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO `+s.T("wc_customer_lookup")+` (user_id, username, first_name, last_name, email, date_last_active,
		date_registered, country, postcode, city, state) VALUES (?, ?, ?, ?, ?, UTC_TIMESTAMP(), ?, ?, ?, ?, ?)`,
		nullIfZero(o.CustomerID), orEmptyAny(username), b.FirstName, b.LastName, b.Email, registered, b.Country, b.Postcode, b.City, b.State)
	if err != nil {
		return 0, err
	}
	n, _ := res.LastInsertId()
	return uint64(n), nil
}

func orEmptyAny(v any) any {
	if v == nil {
		return ""
	}
	return v
}

// MarkNewOrderEmailSent flags the order like WooCommerce does after new_order.
func (s *Store) MarkNewOrderEmailSent(ctx context.Context, id uint64) error {
	hpos, sync := s.HPOS(ctx)
	if hpos || sync {
		if _, err := s.DB.ExecContext(ctx, `UPDATE `+s.T("wc_order_operational_data")+` SET new_order_email_sent = 1 WHERE order_id = ?`, id); err != nil {
			return err
		}
	}
	if !hpos || sync {
		return s.setPostMeta(ctx, s.DB, id, "_new_order_email_sent", "true")
	}
	return nil
}
