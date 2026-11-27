package woo

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"cwmerch-api/internal/db"
	"cwmerch-api/internal/php"
)

// OrderStatuses are WooCommerce's core statuses (without the wc- prefix).
var OrderStatuses = []string{"pending", "processing", "on-hold", "completed", "cancelled", "refunded", "failed", "checkout-draft"}

var statusLabels = map[string]string{"pending": "Pending payment", "processing": "Processing", "on-hold": "On hold",
	"completed": "Completed", "cancelled": "Cancelled", "refunded": "Refunded", "failed": "Failed", "checkout-draft": "Draft"}

// StatusLabel is the human label of a status.
func StatusLabel(s string) string {
	if l, ok := statusLabels[s]; ok {
		return l
	}
	return s
}

// IsValidStatus reports a core status.
func IsValidStatus(s string) bool { _, ok := statusLabels[s]; return ok }

func wcStatus(s string) string {
	if strings.HasPrefix(s, "wc-") || s == "trash" || s == "draft" || s == "auto-draft" {
		return s
	}
	return "wc-" + s
}

func plainStatus(s string) string { return strings.TrimPrefix(s, "wc-") }

// MetaEntry is a displayed item meta.
type MetaEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// LineItem is an order line.
type LineItem struct {
	ID           uint64             `json:"id"`
	ProductID    uint64             `json:"productId"`
	VariationID  uint64             `json:"variationId"`
	Name         string             `json:"name"`
	SKU          string             `json:"sku"`
	Quantity     int64              `json:"quantity"`
	TaxClass     string             `json:"taxClass"`
	Subtotal     float64            `json:"subtotal"`
	SubtotalTax  float64            `json:"subtotalTax"`
	Total        float64            `json:"total"`
	TotalTax     float64            `json:"totalTax"`
	Taxes        map[uint64]float64 `json:"-"`
	SubTaxes     map[uint64]float64 `json:"-"`
	Meta         []MetaEntry        `json:"meta"`
	Image        string             `json:"image,omitempty"`
	ReducedStock int64              `json:"-"`
	Refunded     float64            `json:"refunded"`
	RefundedQty  int64              `json:"refundedQuantity"`
}

// ShippingLine is an order's shipping line.
type ShippingLine struct {
	ID          uint64             `json:"id"`
	MethodID    string             `json:"methodId"`
	InstanceID  uint64             `json:"instanceId"`
	MethodTitle string             `json:"methodTitle"`
	Total       float64            `json:"total"`
	TotalTax    float64            `json:"totalTax"`
	Taxes       map[uint64]float64 `json:"-"`
}

// FeeLine is a fee line.
type FeeLine struct {
	ID       uint64  `json:"id"`
	Name     string  `json:"name"`
	Total    float64 `json:"total"`
	TotalTax float64 `json:"totalTax"`
}

// CouponLine is an applied coupon.
type CouponLine struct {
	ID          uint64  `json:"id"`
	Code        string  `json:"code"`
	Discount    float64 `json:"discount"`
	DiscountTax float64 `json:"discountTax"`
}

// TaxLine is a tax total line.
type TaxLine struct {
	ID               uint64  `json:"id"`
	RateID           uint64  `json:"rateId"`
	Label            string  `json:"label"`
	Compound         bool    `json:"compound"`
	TaxTotal         float64 `json:"taxTotal"`
	ShippingTaxTotal float64 `json:"shippingTaxTotal"`
	RatePercent      float64 `json:"ratePercent"`
}

// Refund is a refund of an order.
type Refund struct {
	ID         uint64     `json:"id"`
	Reason     string     `json:"reason"`
	Amount     float64    `json:"amount"`
	CreatedAt  time.Time  `json:"createdAt"`
	RefundedBy uint64     `json:"refundedBy"`
	LineItems  []LineItem `json:"lineItems,omitempty"`
}

// Order is a WooCommerce order.
type Order struct {
	ID                  uint64         `json:"id"`
	Number              string         `json:"number"`
	OrderKey            string         `json:"orderKey"`
	Status              string         `json:"status"`
	StatusLabel         string         `json:"statusLabel"`
	Currency            string         `json:"currency"`
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"`
	PaidAt              *time.Time     `json:"paidAt"`
	CompletedAt         *time.Time     `json:"completedAt"`
	CustomerID          uint64         `json:"customerId"`
	CustomerNote        string         `json:"customerNote"`
	Billing             Address        `json:"billing"`
	Shipping            Address        `json:"shipping"`
	PaymentMethod       string         `json:"paymentMethod"`
	PaymentMethodTitle  string         `json:"paymentMethodTitle"`
	TransactionID       string         `json:"transactionId"`
	PricesIncludeTax    bool           `json:"pricesIncludeTax"`
	LineItems           []LineItem     `json:"lineItems"`
	ShippingLines       []ShippingLine `json:"shippingLines"`
	FeeLines            []FeeLine      `json:"feeLines"`
	CouponLines         []CouponLine   `json:"couponLines"`
	TaxLines            []TaxLine      `json:"taxLines"`
	Refunds             []Refund       `json:"refunds"`
	DiscountTotal       float64        `json:"discountTotal"`
	DiscountTax         float64        `json:"discountTax"`
	ShippingTotal       float64        `json:"shippingTotal"`
	ShippingTax         float64        `json:"shippingTax"`
	CartTax             float64        `json:"cartTax"`
	TotalTax            float64        `json:"totalTax"`
	Total               float64        `json:"total"`
	TotalRefunded       float64        `json:"totalRefunded"`
	CreatedVia          string         `json:"createdVia"`
	CustomerIPAddress   string         `json:"customerIpAddress"`
	CustomerUserAgent   string         `json:"customerUserAgent"`
	PaymentInstructions string         `json:"paymentInstructions,omitempty"`
	// flags (operational data)
	StockReduced      bool              `json:"-"`
	SalesRecorded     bool              `json:"-"`
	CouponsCounted    bool              `json:"-"`
	DownloadsGranted  bool              `json:"-"`
	NewOrderEmailSent bool              `json:"-"`
	CartHash          string            `json:"-"`
	ParentID          uint64            `json:"-"`
	Type              string            `json:"-"`
	Meta              map[string]string `json:"-"`
}

var addressFields = []string{"first_name", "last_name", "company", "address_1", "address_2", "city", "state", "postcode", "country", "email", "phone"}

func addressFromMeta(m map[string]string, prefix string) Address {
	return Address{FirstName: m[prefix+"first_name"], LastName: m[prefix+"last_name"], Company: m[prefix+"company"],
		Address1: m[prefix+"address_1"], Address2: m[prefix+"address_2"], City: m[prefix+"city"], State: m[prefix+"state"],
		Postcode: m[prefix+"postcode"], Country: m[prefix+"country"], Email: m[prefix+"email"], Phone: m[prefix+"phone"]}
}

func addressMeta(a Address, prefix string) map[string]string {
	return map[string]string{prefix + "first_name": a.FirstName, prefix + "last_name": a.LastName, prefix + "company": a.Company,
		prefix + "address_1": a.Address1, prefix + "address_2": a.Address2, prefix + "city": a.City, prefix + "state": a.State,
		prefix + "postcode": a.Postcode, prefix + "country": a.Country, prefix + "email": a.Email, prefix + "phone": a.Phone}
}

func timePtr(t sql.NullTime) *time.Time {
	if !t.Valid || t.Time.IsZero() || t.Time.Year() < 1971 {
		return nil
	}
	v := t.Time
	return &v
}

// ------------------------------------------------------------------ reading

func (s *Store) loadOrdersHPOS(ctx context.Context, q db.Querier, ids []uint64) (map[uint64]*Order, error) {
	out := map[uint64]*Order{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT o.id, o.status, COALESCE(o.currency, ''), COALESCE(o.type, 'shop_order'), COALESCE(o.tax_amount, 0), COALESCE(o.total_amount, 0),
		COALESCE(o.customer_id, 0), COALESCE(o.billing_email, ''), o.date_created_gmt, o.date_updated_gmt, COALESCE(o.parent_order_id, 0),
		COALESCE(o.payment_method, ''), COALESCE(o.payment_method_title, ''), COALESCE(o.transaction_id, ''), COALESCE(o.ip_address, ''),
		COALESCE(o.user_agent, ''), COALESCE(o.customer_note, ''),
		COALESCE(d.created_via, ''), COALESCE(d.prices_include_tax, 0), COALESCE(d.coupon_usages_are_counted, 0), COALESCE(d.download_permission_granted, 0),
		COALESCE(d.cart_hash, ''), COALESCE(d.new_order_email_sent, 0), COALESCE(d.order_key, ''), COALESCE(d.order_stock_reduced, 0),
		d.date_paid_gmt, d.date_completed_gmt, COALESCE(d.shipping_tax_amount, 0), COALESCE(d.shipping_total_amount, 0),
		COALESCE(d.discount_tax_amount, 0), COALESCE(d.discount_total_amount, 0), COALESCE(d.recorded_sales, 0)
		FROM `+s.T("wc_orders")+` o LEFT JOIN `+s.T("wc_order_operational_data")+` d ON d.order_id = o.id
		WHERE o.id IN (`+inList(ids)+`)`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		o := &Order{Meta: map[string]string{}}
		var created, updated, paid, completed sql.NullTime
		var pit, coupons, downloads, newMail, stock, sales int
		var email string
		if err := rows.Scan(&o.ID, &o.Status, &o.Currency, &o.Type, &o.TotalTax, &o.Total, &o.CustomerID, &email, &created, &updated, &o.ParentID,
			&o.PaymentMethod, &o.PaymentMethodTitle, &o.TransactionID, &o.CustomerIPAddress, &o.CustomerUserAgent, &o.CustomerNote,
			&o.CreatedVia, &pit, &coupons, &downloads, &o.CartHash, &newMail, &o.OrderKey, &stock, &paid, &completed,
			&o.ShippingTax, &o.ShippingTotal, &o.DiscountTax, &o.DiscountTotal, &sales); err != nil {
			rows.Close()
			return nil, err
		}
		o.Status = plainStatus(o.Status)
		o.CreatedAt, o.UpdatedAt = created.Time, updated.Time
		o.PaidAt, o.CompletedAt = timePtr(paid), timePtr(completed)
		o.PricesIncludeTax, o.CouponsCounted, o.DownloadsGranted = pit == 1, coupons == 1, downloads == 1
		o.NewOrderEmailSent, o.StockReduced, o.SalesRecorded = newMail == 1, stock == 1, sales == 1
		o.Billing.Email = email
		out[o.ID] = o
	}
	rows.Close()
	arows, err := q.QueryContext(ctx, `SELECT order_id, address_type, COALESCE(first_name,''), COALESCE(last_name,''), COALESCE(company,''), COALESCE(address_1,''),
		COALESCE(address_2,''), COALESCE(city,''), COALESCE(state,''), COALESCE(postcode,''), COALESCE(country,''), COALESCE(email,''), COALESCE(phone,'')
		FROM `+s.T("wc_order_addresses")+` WHERE order_id IN (`+inList(ids)+`)`)
	if err != nil {
		return nil, err
	}
	for arows.Next() {
		var id uint64
		var typ string
		var a Address
		if err := arows.Scan(&id, &typ, &a.FirstName, &a.LastName, &a.Company, &a.Address1, &a.Address2, &a.City, &a.State, &a.Postcode, &a.Country, &a.Email, &a.Phone); err != nil {
			arows.Close()
			return nil, err
		}
		if o, ok := out[id]; ok {
			if typ == "billing" {
				if a.Email == "" {
					a.Email = o.Billing.Email
				}
				o.Billing = a
			} else {
				a.Email = ""
				o.Shipping = a
			}
		}
	}
	arows.Close()
	mrows, err := q.QueryContext(ctx, `SELECT order_id, meta_key, meta_value FROM `+s.T("wc_orders_meta")+` WHERE order_id IN (`+inList(ids)+`) ORDER BY id`)
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
		if o, ok := out[id]; ok {
			if _, seen := o.Meta[k]; !seen {
				o.Meta[k] = v.String
			}
		}
	}
	mrows.Close()
	return out, nil
}

func (s *Store) loadOrdersLegacy(ctx context.Context, q db.Querier, ids []uint64) (map[uint64]*Order, error) {
	out := map[uint64]*Order{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.posts(ctx, q, `p.ID IN (`+inList(ids)+`) AND p.post_type IN ('shop_order', 'shop_order_refund')`)
	if err != nil {
		return nil, err
	}
	metas, err := s.postMetas(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		m := metas[r.ID]
		o := &Order{ID: r.ID, Status: plainStatus(r.Status), Type: r.Type, CreatedAt: r.Created, UpdatedAt: r.Modified, ParentID: r.Parent,
			CustomerNote: r.Excerpt, Currency: m["_order_currency"], CustomerID: ParseID(m["_customer_user"]), OrderKey: m["_order_key"],
			PaymentMethod: m["_payment_method"], PaymentMethodTitle: m["_payment_method_title"], TransactionID: m["_transaction_id"],
			CustomerIPAddress: m["_customer_ip_address"], CustomerUserAgent: m["_customer_user_agent"], CreatedVia: m["_created_via"],
			CartHash: m["_cart_hash"], PricesIncludeTax: m["_prices_include_tax"] == "yes",
			DiscountTotal: m.Float("_cart_discount"), DiscountTax: m.Float("_cart_discount_tax"), ShippingTotal: m.Float("_order_shipping"),
			ShippingTax: m.Float("_order_shipping_tax"), Total: m.Float("_order_total"),
			Billing: addressFromMeta(m, "_billing_"), Shipping: addressFromMeta(m, "_shipping_"),
			StockReduced: m.Bool("_order_stock_reduced"), SalesRecorded: m.Bool("_recorded_sales"), CouponsCounted: m.Bool("_recorded_coupon_usage_counts"),
			DownloadsGranted: m.Bool("_download_permissions_granted"), NewOrderEmailSent: m.Bool("_new_order_email_sent"), Meta: m}
		o.Shipping.Email = ""
		o.TotalTax = m.Float("_order_tax") + o.ShippingTax
		o.PaidAt = unixPtr(m["_date_paid"])
		o.CompletedAt = unixPtr(m["_date_completed"])
		out[o.ID] = o
	}
	return out, nil
}

func (s *Store) loadOrders(ctx context.Context, q db.Querier, ids []uint64) (map[uint64]*Order, error) {
	hpos, _ := s.HPOS(ctx)
	var orders map[uint64]*Order
	var err error
	if hpos {
		orders, err = s.loadOrdersHPOS(ctx, q, ids)
	} else {
		orders, err = s.loadOrdersLegacy(ctx, q, ids)
	}
	if err != nil {
		return nil, err
	}
	if err := s.loadItems(ctx, q, orders); err != nil {
		return nil, err
	}
	// refunds
	refundIDs, err := s.refundIDs(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	all := []uint64{}
	for _, list := range refundIDs {
		all = append(all, list...)
	}
	if len(all) > 0 {
		var refunds map[uint64]*Order
		if hpos {
			refunds, err = s.loadOrdersHPOS(ctx, q, all)
		} else {
			refunds, err = s.loadOrdersLegacy(ctx, q, all)
		}
		if err != nil {
			return nil, err
		}
		if err := s.loadItems(ctx, q, refunds); err != nil {
			return nil, err
		}
		for parent, list := range refundIDs {
			o, ok := orders[parent]
			if !ok {
				continue
			}
			for _, rid := range list {
				r, ok := refunds[rid]
				if !ok {
					continue
				}
				amount := parseFloat(r.Meta["_refund_amount"])
				if amount == 0 {
					amount = -r.Total
				}
				o.Refunds = append(o.Refunds, Refund{ID: r.ID, Reason: r.Meta["_refund_reason"], Amount: amount, CreatedAt: r.CreatedAt,
					RefundedBy: ParseID(r.Meta["_refunded_by"]), LineItems: r.LineItems})
				o.TotalRefunded += amount
				for _, rl := range r.LineItems {
					refItem := ParseID(rl.metaValue("_refunded_item_id"))
					for i := range o.LineItems {
						if o.LineItems[i].ID == refItem {
							o.LineItems[i].Refunded += -(rl.Total + rl.TotalTax)
							o.LineItems[i].RefundedQty += -rl.Quantity
						}
					}
				}
			}
		}
	}
	for _, o := range orders {
		o.Number = strconv.FormatUint(o.ID, 10)
		o.StatusLabel = StatusLabel(o.Status)
		o.CartTax = 0
		for _, t := range o.TaxLines {
			o.CartTax += t.TaxTotal
		}
		if o.Refunds == nil {
			o.Refunds = []Refund{}
		}
		o.TotalRefunded = round(o.TotalRefunded, 6)
	}
	return orders, nil
}

func (li LineItem) metaValue(key string) string {
	for _, m := range li.Meta {
		if m.Key == key {
			return m.Value
		}
	}
	return ""
}

func (s *Store) refundIDs(ctx context.Context, q db.Querier, ids []uint64) (map[uint64][]uint64, error) {
	out := map[uint64][]uint64{}
	if len(ids) == 0 {
		return out, nil
	}
	hpos, _ := s.HPOS(ctx)
	var query string
	if hpos {
		query = `SELECT id, parent_order_id FROM ` + s.T("wc_orders") + ` WHERE type = 'shop_order_refund' AND parent_order_id IN (` + inList(ids) + `) ORDER BY id`
	} else {
		query = `SELECT ID, post_parent FROM ` + s.T("posts") + ` WHERE post_type = 'shop_order_refund' AND post_parent IN (` + inList(ids) + `) ORDER BY ID`
	}
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, parent uint64
		if err := rows.Scan(&id, &parent); err != nil {
			return nil, err
		}
		out[parent] = append(out[parent], id)
	}
	return out, rows.Err()
}

var hiddenItemMeta = map[string]bool{"_product_id": true, "_variation_id": true, "_qty": true, "_tax_class": true, "_line_subtotal": true,
	"_line_subtotal_tax": true, "_line_total": true, "_line_tax": true, "_line_tax_data": true, "_reduced_stock": true, "_restock_refunded_items": true}

func taxData(serialized, key string) map[uint64]float64 {
	out := map[uint64]float64{}
	arr := php.UnmarshalArray(serialized).Arr(key)
	for _, k := range arr.Keys {
		out[ParseID(php.ToString(k))] = parseFloat(arr.String(k))
	}
	return out
}

func (s *Store) loadItems(ctx context.Context, q db.Querier, orders map[uint64]*Order) error {
	ids := []uint64{}
	for id, o := range orders {
		ids = append(ids, id)
		o.LineItems, o.ShippingLines, o.FeeLines, o.CouponLines, o.TaxLines = []LineItem{}, []ShippingLine{}, []FeeLine{}, []CouponLine{}, []TaxLine{}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.QueryContext(ctx, `SELECT order_item_id, order_item_name, order_item_type, order_id FROM `+s.T("woocommerce_order_items")+
		` WHERE order_id IN (`+inList(ids)+`) ORDER BY order_item_id`)
	if err != nil {
		return err
	}
	type item struct {
		id, order uint64
		name, typ string
	}
	items := []item{}
	itemIDs := []uint64{}
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.name, &it.typ, &it.order); err != nil {
			rows.Close()
			return err
		}
		items = append(items, it)
		itemIDs = append(itemIDs, it.id)
	}
	rows.Close()
	metas := map[uint64][]MetaEntry{}
	if len(itemIDs) > 0 {
		for _, chunk := range chunks(itemIDs, 500) {
			mrows, err := q.QueryContext(ctx, `SELECT order_item_id, meta_key, meta_value FROM `+s.T("woocommerce_order_itemmeta")+
				` WHERE order_item_id IN (`+inList(chunk)+`) ORDER BY meta_id`)
			if err != nil {
				return err
			}
			for mrows.Next() {
				var id uint64
				var k string
				var v sql.NullString
				if err := mrows.Scan(&id, &k, &v); err != nil {
					mrows.Close()
					return err
				}
				metas[id] = append(metas[id], MetaEntry{Key: k, Value: v.String})
			}
			mrows.Close()
		}
	}
	productIDs := []uint64{}
	for _, it := range items {
		o := orders[it.order]
		m := map[string]string{}
		for _, e := range metas[it.id] {
			if _, ok := m[e.Key]; !ok {
				m[e.Key] = e.Value
			}
		}
		switch it.typ {
		case "line_item":
			li := LineItem{ID: it.id, Name: it.name, ProductID: ParseID(m["_product_id"]), VariationID: ParseID(m["_variation_id"]),
				Quantity: int64(parseFloat(m["_qty"])), TaxClass: m["_tax_class"], Subtotal: parseFloat(m["_line_subtotal"]),
				SubtotalTax: parseFloat(m["_line_subtotal_tax"]), Total: parseFloat(m["_line_total"]), TotalTax: parseFloat(m["_line_tax"]),
				Taxes: taxData(m["_line_tax_data"], "total"), SubTaxes: taxData(m["_line_tax_data"], "subtotal"),
				ReducedStock: int64(parseFloat(m["_reduced_stock"])), Meta: []MetaEntry{}}
			for _, e := range metas[it.id] {
				if hiddenItemMeta[e.Key] {
					continue
				}
				if e.Key == "_refunded_item_id" {
					li.Meta = append(li.Meta, e)
					continue
				}
				if strings.HasPrefix(e.Key, "_") {
					continue
				}
				li.Meta = append(li.Meta, e)
			}
			o.LineItems = append(o.LineItems, li)
			productIDs = append(productIDs, li.ProductID, li.VariationID)
		case "shipping":
			o.ShippingLines = append(o.ShippingLines, ShippingLine{ID: it.id, MethodID: m["method_id"], InstanceID: ParseID(m["instance_id"]),
				MethodTitle: it.name, Total: parseFloat(m["cost"]), TotalTax: parseFloat(m["total_tax"]), Taxes: taxData(m["taxes"], "total")})
		case "fee":
			o.FeeLines = append(o.FeeLines, FeeLine{ID: it.id, Name: it.name, Total: parseFloat(m["_line_total"]), TotalTax: parseFloat(m["_line_tax"])})
		case "coupon":
			o.CouponLines = append(o.CouponLines, CouponLine{ID: it.id, Code: it.name, Discount: parseFloat(m["discount_amount"]), DiscountTax: parseFloat(m["discount_amount_tax"])})
		case "tax":
			o.TaxLines = append(o.TaxLines, TaxLine{ID: it.id, RateID: ParseID(m["rate_id"]), Label: m["label"], Compound: m["compound"] == "1",
				TaxTotal: parseFloat(m["tax_amount"]), ShippingTaxTotal: parseFloat(m["shipping_tax_amount"]), RatePercent: parseFloat(m["rate_percent"])})
		}
	}
	// SKUs and images for display
	productIDs = uniqueIDs(productIDs)
	if len(productIDs) > 0 {
		pm, err := s.postMetas(ctx, q, productIDs)
		if err != nil {
			return err
		}
		thumbs := []uint64{}
		for _, m := range pm {
			thumbs = append(thumbs, ParseID(m["_thumbnail_id"]))
		}
		imgs, _ := s.images(ctx, thumbs)
		for _, o := range orders {
			for i := range o.LineItems {
				li := &o.LineItems[i]
				src := pm[li.VariationID]
				if li.VariationID == 0 || src["_sku"] == "" {
					src = pm[li.ProductID]
				}
				li.SKU = src["_sku"]
				img := imgs[ParseID(pm[li.VariationID]["_thumbnail_id"])]
				if img.Src == "" {
					img = imgs[ParseID(pm[li.ProductID]["_thumbnail_id"])]
				}
				li.Image = img.Src
			}
		}
	}
	return nil
}

// Order is one order (not a refund).
func (s *Store) Order(ctx context.Context, id uint64) (Order, error) {
	orders, err := s.loadOrders(ctx, s.DB, []uint64{id})
	if err != nil {
		return Order{}, err
	}
	o, ok := orders[id]
	if !ok || o.Type == "shop_order_refund" {
		return Order{}, ErrNotFound
	}
	if o.Status == "on-hold" || o.Status == "pending" {
		o.PaymentInstructions = s.PaymentInstructions(ctx, o.PaymentMethod)
	}
	return *o, nil
}

// OrderQuery filters orders.
type OrderQuery struct {
	Status     string
	Search     string
	CustomerID uint64
	Email      string
	From, To   *time.Time
	Page       int
	PerPage    int
}

// Orders lists orders, newest first.
func (s *Store) Orders(ctx context.Context, q OrderQuery) (Paged[Order], error) {
	page, perPage, offset := Page(q.Page, q.PerPage)
	hpos, _ := s.HPOS(ctx)
	where := []string{}
	args := []any{}
	var from, idCol, statusCol, dateCol, custCol, emailCol string
	if hpos {
		from = s.T("wc_orders") + ` o LEFT JOIN ` + s.T("wc_order_addresses") + ` a ON a.order_id = o.id AND a.address_type = 'billing'`
		idCol, statusCol, dateCol, custCol, emailCol = "o.id", "o.status", "o.date_created_gmt", "o.customer_id", "o.billing_email"
		where = append(where, "o.type = 'shop_order'")
	} else {
		from = s.T("posts") + ` o`
		idCol, statusCol, dateCol = "o.ID", "o.post_status", "o.post_date_gmt"
		where = append(where, "o.post_type = 'shop_order'")
		meta := func(key string) string {
			return `(SELECT meta_value FROM ` + s.T("postmeta") + ` m WHERE m.post_id = o.ID AND m.meta_key = '` + key + `' LIMIT 1)`
		}
		custCol, emailCol = meta("_customer_user"), meta("_billing_email")
	}
	if q.Status != "" && q.Status != "any" {
		statuses := []string{}
		for _, st := range strings.Split(q.Status, ",") {
			statuses = append(statuses, wcStatus(strings.TrimSpace(st)))
		}
		where = append(where, statusCol+" IN ("+placeholders(len(statuses))+")")
		args = append(args, strArgs(statuses)...)
	} else {
		where = append(where, statusCol+" NOT IN ('trash', 'auto-draft', 'draft', 'wc-checkout-draft')")
	}
	if q.CustomerID != 0 {
		where = append(where, custCol+" = ?")
		args = append(args, q.CustomerID)
	}
	if q.Email != "" {
		where = append(where, emailCol+" = ?")
		args = append(args, q.Email)
	}
	if q.From != nil {
		where = append(where, dateCol+" >= ?")
		args = append(args, *q.From)
	}
	if q.To != nil {
		where = append(where, dateCol+" < ?")
		args = append(args, q.To.Add(24*time.Hour))
	}
	if q.Search != "" {
		like := "%" + q.Search + "%"
		if hpos {
			where = append(where, "(CAST(o.id AS CHAR) = ? OR o.billing_email LIKE ? OR a.first_name LIKE ? OR a.last_name LIKE ? OR a.phone LIKE ? OR a.company LIKE ?)")
			args = append(args, q.Search, like, like, like, like, like)
		} else {
			where = append(where, `(CAST(o.ID AS CHAR) = ? OR EXISTS (SELECT 1 FROM `+s.T("postmeta")+` m WHERE m.post_id = o.ID AND m.meta_key IN
				('_billing_email', '_billing_first_name', '_billing_last_name', '_billing_phone', '_billing_company') AND m.meta_value LIKE ?))`)
			args = append(args, q.Search, like)
		}
	}
	w := strings.Join(where, " AND ")
	var total int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+from+` WHERE `+w, args...).Scan(&total); err != nil {
		return Paged[Order]{}, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+idCol+` FROM `+from+` WHERE `+w+` ORDER BY `+dateCol+` DESC, `+idCol+` DESC LIMIT ? OFFSET ?`,
		append(args, perPage, offset)...)
	if err != nil {
		return Paged[Order]{}, err
	}
	ids := []uint64{}
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return Paged[Order]{}, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	loaded, err := s.loadOrders(ctx, s.DB, ids)
	if err != nil {
		return Paged[Order]{}, err
	}
	items := []Order{}
	for _, id := range ids {
		if o, ok := loaded[id]; ok {
			items = append(items, *o)
		}
	}
	return NewPaged(items, total, page, perPage), nil
}

// OrderByKey is the "order received" lookup: id + order key must match.
func (s *Store) OrderByKey(ctx context.Context, id uint64, key string) (Order, error) {
	o, err := s.Order(ctx, id)
	if err != nil {
		return Order{}, err
	}
	if key == "" || o.OrderKey != key {
		return Order{}, ErrNotFound
	}
	return o, nil
}

// OrderNote is a note on an order (a comment of type order_note).
type OrderNote struct {
	ID           uint64    `json:"id"`
	Note         string    `json:"note"`
	CustomerNote bool      `json:"customerNote"`
	Author       string    `json:"author"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Notes lists an order's notes, newest first.
func (s *Store) Notes(ctx context.Context, orderID uint64, customerOnly bool) ([]OrderNote, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT c.comment_ID, c.comment_content, c.comment_author, c.comment_date_gmt,
		COALESCE((SELECT meta_value FROM `+s.T("commentmeta")+` m WHERE m.comment_id = c.comment_ID AND m.meta_key = 'is_customer_note' LIMIT 1), '')
		FROM `+s.T("comments")+` c WHERE c.comment_post_ID = ? AND c.comment_type = 'order_note' ORDER BY c.comment_date_gmt DESC, c.comment_ID DESC`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrderNote{}
	for rows.Next() {
		var n OrderNote
		var at sql.NullTime
		var cust string
		if err := rows.Scan(&n.ID, &n.Note, &n.Author, &at, &cust); err != nil {
			return nil, err
		}
		n.CreatedAt, n.CustomerNote = at.Time, cust == "1"
		if customerOnly && !n.CustomerNote {
			continue
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// AddNote adds an order note (author "WooCommerce" for system notes, like WC).
func (s *Store) AddNote(ctx context.Context, orderID uint64, note string, customerNote bool, author string, authorEmail string, userID uint64) (OrderNote, error) {
	if author == "" {
		author, authorEmail = "WooCommerce", "woocommerce@"+hostOf(s.SiteURL)
	}
	t := now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO `+s.T("comments")+` (comment_post_ID, comment_author, comment_author_email, comment_author_url,
		comment_author_IP, comment_date, comment_date_gmt, comment_content, comment_karma, comment_approved, comment_agent, comment_type, comment_parent, user_id)
		VALUES (?, ?, ?, '', '', ?, ?, ?, 0, '1', 'WooCommerce', 'order_note', 0, ?)`,
		orderID, author, authorEmail, s.localTime(ctx, t), t, note, userID)
	if err != nil {
		return OrderNote{}, err
	}
	id, _ := res.LastInsertId()
	if customerNote {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO `+s.T("commentmeta")+` (comment_id, meta_key, meta_value) VALUES (?, 'is_customer_note', '1')`, id); err != nil {
			return OrderNote{}, err
		}
	}
	return OrderNote{ID: uint64(id), Note: note, CustomerNote: customerNote, Author: author, CreatedAt: t}, nil
}

// DeleteNote removes a note of an order.
func (s *Store) DeleteNote(ctx context.Context, orderID, noteID uint64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM `+s.T("comments")+` WHERE comment_ID = ? AND comment_post_ID = ? AND comment_type = 'order_note'`, noteID, orderID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM `+s.T("commentmeta")+` WHERE comment_id = ?`, noteID)
	return err
}

func hostOf(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if i := strings.IndexAny(u, "/:"); i >= 0 {
		u = u[:i]
	}
	if u == "" {
		return "localhost"
	}
	return u
}
