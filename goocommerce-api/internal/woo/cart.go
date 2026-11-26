package woo

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CartLine is what the client asks for.
type CartLine struct {
	ProductID   uint64 `json:"productId"`
	VariationID uint64 `json:"variationId"`
	Quantity    int64  `json:"quantity"`
	// Attributes picks "any" variation attributes ({"pa_color": "Blue"}).
	Attributes map[string]string `json:"attributes,omitempty"`
}

// CartRequest prices a cart.
type CartRequest struct {
	Items          []CartLine `json:"items"`
	Coupons        []string   `json:"coupons"`
	Customer       Location   `json:"customer"`
	ShippingMethod string     `json:"shippingMethod"`
	// Email is used for coupon email restrictions and per-user limits.
	Email  string `json:"email,omitempty"`
	UserID uint64 `json:"-"`
	// Billing is the billing location when tax is based on billing.
	Billing *Location `json:"-"`
}

// CartItem is a priced line.
type CartItem struct {
	Key          string           `json:"key"`
	ProductID    uint64           `json:"productId"`
	VariationID  uint64           `json:"variationId"`
	Name         string           `json:"name"`
	SKU          string           `json:"sku"`
	Image        string           `json:"image"`
	Permalink    string           `json:"permalink"`
	Attributes   []AttributeValue `json:"attributes"`
	Quantity     int64            `json:"quantity"`
	Price        float64          `json:"price"`
	Subtotal     float64          `json:"subtotal"`
	SubtotalTax  float64          `json:"subtotalTax"`
	Total        float64          `json:"total"`
	TotalTax     float64          `json:"totalTax"`
	MaxQuantity  *int64           `json:"maxQuantity"`
	StockStatus  string           `json:"stockStatus"`
	Virtual      bool             `json:"virtual"`
	Downloadable bool             `json:"downloadable"`

	TaxClass      string             `json:"-"`
	Taxable       bool               `json:"-"`
	SubtotalTaxes map[uint64]float64 `json:"-"`
	TotalTaxes    map[uint64]float64 `json:"-"`
	ShippingClass uint64             `json:"-"`
	OnSale        bool               `json:"-"`
	CategoryIDs   []uint64           `json:"-"`
	entered       float64            // line subtotal as entered (incl. tax when prices include tax)
	discount      float64            // discount as entered
	product       Product
	variation     *Variation
}

// CartCoupon is an applied coupon.
type CartCoupon struct {
	Code         string  `json:"code"`
	DiscountType string  `json:"discountType"`
	Discount     float64 `json:"discount"`
	DiscountTax  float64 `json:"discountTax"`
	FreeShipping bool    `json:"freeShipping"`
	ID           uint64  `json:"-"`
}

// CartTax is a total per tax rate.
type CartTax struct {
	RateID      uint64  `json:"rateId"`
	Label       string  `json:"label"`
	Compound    bool    `json:"compound"`
	RatePercent float64 `json:"ratePercent"`
	Amount      float64 `json:"amount"`
	CartTax     float64 `json:"cartTax"`
	ShippingTax float64 `json:"shippingTax"`
}

// Notice is a message about the cart.
type Notice struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Cart is a priced cart.
type Cart struct {
	Items                []CartItem     `json:"items"`
	Coupons              []CartCoupon   `json:"coupons"`
	Subtotal             float64        `json:"subtotal"`
	SubtotalTax          float64        `json:"subtotalTax"`
	DiscountTotal        float64        `json:"discountTotal"`
	DiscountTax          float64        `json:"discountTax"`
	ShippingTotal        float64        `json:"shippingTotal"`
	ShippingTax          float64        `json:"shippingTax"`
	CartTax              float64        `json:"cartTax"`
	TaxTotal             float64        `json:"taxTotal"`
	Total                float64        `json:"total"`
	Taxes                []CartTax      `json:"taxes"`
	ShippingRates        []ShippingRate `json:"shippingRates"`
	ChosenShippingMethod string         `json:"chosenShippingMethod"`
	ChosenShipping       *ShippingRate  `json:"chosenShipping,omitempty"`
	NeedsShipping        bool           `json:"needsShipping"`
	NeedsPayment         bool           `json:"needsPayment"`
	PricesIncludeTax     bool           `json:"pricesIncludeTax"`
	Currency             string         `json:"currency"`
	Notices              []Notice       `json:"notices"`
	TaxLocation          Location       `json:"taxLocation"`
	rates                map[uint64]TaxRate
}

func (c *Cart) notice(typ, code, format string, args ...any) {
	c.Notices = append(c.Notices, Notice{Type: typ, Code: code, Message: fmt.Sprintf(format, args...)})
}

// HasErrors reports error notices (checkout refuses those carts).
func (c Cart) HasErrors() bool {
	for _, n := range c.Notices {
		if n.Type == "error" {
			return true
		}
	}
	return false
}

func (s *Store) resolveVariation(p Product, line CartLine) (*Variation, error) {
	if line.VariationID != 0 {
		for i := range p.Variations {
			if p.Variations[i].ID == line.VariationID {
				return &p.Variations[i], nil
			}
		}
		return nil, ErrNotFound
	}
	// find by attributes
	for i := range p.Variations {
		v := &p.Variations[i]
		ok := true
		for _, a := range v.Attributes {
			want := line.Attributes[a.Slug]
			if a.Option != "" && !strings.EqualFold(want, a.Option) {
				ok = false
				break
			}
		}
		if ok && len(line.Attributes) > 0 {
			return v, nil
		}
	}
	return nil, ErrNotFound
}

// ComputeCart prices a cart the way WooCommerce's WC_Cart_Totals does.
func (s *Store) ComputeCart(ctx context.Context, req CartRequest) (Cart, error) {
	settings := s.ShopSettings(ctx)
	dec := settings.Decimals
	cart := Cart{Items: []CartItem{}, Coupons: []CartCoupon{}, Taxes: []CartTax{}, ShippingRates: []ShippingRate{},
		Notices: []Notice{}, Currency: settings.Currency, PricesIncludeTax: settings.PricesIncludeTax, rates: map[uint64]TaxRate{}}

	// ---- lines
	productIDs := []uint64{}
	for _, l := range req.Items {
		productIDs = append(productIDs, l.ProductID)
	}
	products, err := s.ProductsByIDs(ctx, productIDs, true)
	if err != nil {
		return cart, err
	}
	byID := map[uint64]Product{}
	for _, p := range products {
		byID[p.ID] = p
	}
	merged := map[string]int{}
	for _, l := range req.Items {
		if l.Quantity <= 0 {
			continue
		}
		p, ok := byID[l.ProductID]
		if !ok || p.Status != "publish" {
			cart.notice("error", "cart.productUnavailable", "A product in your cart is no longer available.")
			continue
		}
		if p.Type == "grouped" || p.Type == "external" {
			cart.notice("error", "cart.notPurchasable", "\"%s\" cannot be purchased here.", p.Name)
			continue
		}
		item := CartItem{ProductID: p.ID, Name: p.Name, SKU: p.SKU, Permalink: p.Permalink, Quantity: l.Quantity, Attributes: []AttributeValue{},
			Price: p.Price, StockStatus: p.StockStatus, Virtual: p.Virtual, Downloadable: p.Downloadable, TaxClass: p.TaxClass,
			Taxable: p.TaxStatus == "taxable", ShippingClass: p.ShippingClassID, OnSale: p.OnSale, product: p}
		for _, c := range p.Categories {
			item.CategoryIDs = append(item.CategoryIDs, c.ID)
		}
		if len(p.Images) > 0 {
			item.Image = p.Images[0].Src
		}
		manage, qty, backorders := p.ManageStock, p.StockQuantity, p.Backorders
		if p.Type == "variable" {
			v, err := s.resolveVariation(p, l)
			if err != nil || v.Status != "publish" {
				cart.notice("error", "cart.chooseOptions", "Please choose product options for \"%s\".", p.Name)
				continue
			}
			item.VariationID, item.variation = v.ID, v
			item.Price, item.SKU, item.StockStatus, item.OnSale = v.Price, orDefault(v.SKU, p.SKU), v.StockStatus, v.OnSale
			item.Virtual, item.Downloadable = v.Virtual, v.Downloadable
			if v.TaxClass != "" {
				item.TaxClass = v.TaxClass
			}
			item.Taxable = v.TaxStatus == "taxable"
			if v.Image != nil {
				item.Image = v.Image.Src
			}
			for _, a := range v.Attributes {
				opt := a.Option
				if opt == "" {
					opt = l.Attributes[a.Slug]
				}
				item.Attributes = append(item.Attributes, AttributeValue{Slug: a.Slug, Name: a.Name, Option: opt})
			}
			if v.ManageStock {
				manage, qty, backorders = true, v.StockQuantity, v.Backorders
			}
			if v.ShippingClass != "" {
				if t, err := s.TermBySlug(ctx, TaxShippingClass, v.ShippingClass); err == nil {
					item.ShippingClass = t.ID
				}
			}
		}
		if !settings.ManageStock {
			manage = false
		}
		if item.StockStatus == "outofstock" {
			cart.notice("error", "cart.outOfStock", "Sorry, \"%s\" is out of stock.", item.Name)
			continue
		}
		if p.SoldIndividually && item.Quantity > 1 {
			item.Quantity = 1
			cart.notice("notice", "cart.soldIndividually", "You can only buy one \"%s\".", item.Name)
		}
		if manage && qty != nil && backorders == "no" {
			max := *qty
			item.MaxQuantity = &max
			if item.Quantity > max {
				if max <= 0 {
					cart.notice("error", "cart.outOfStock", "Sorry, \"%s\" is out of stock.", item.Name)
					continue
				}
				item.Quantity = max
				cart.notice("error", "cart.notEnoughStock", "Only %d of \"%s\" are in stock.", max, item.Name)
			}
		}
		attrKey := []string{}
		for _, a := range item.Attributes {
			attrKey = append(attrKey, a.Slug+"="+a.Option)
		}
		item.Key = strconv.FormatUint(item.ProductID, 10) + "-" + strconv.FormatUint(item.VariationID, 10)
		if len(attrKey) > 0 {
			item.Key += "-" + strings.Join(attrKey, "&")
		}
		if i, dup := merged[item.Key]; dup {
			cart.Items[i].Quantity += item.Quantity
			continue
		}
		merged[item.Key] = len(cart.Items)
		cart.Items = append(cart.Items, item)
	}

	// ---- tax location
	taxLoc := s.taxLocation(ctx, settings, req)
	cart.TaxLocation = taxLoc
	base := s.BaseLocation(ctx)
	ratesFor := func(class string, loc Location, shipping bool) []TaxRate {
		if !settings.CalcTaxes {
			return nil
		}
		r, err := s.FindRates(ctx, loc, class, shipping)
		if err != nil {
			return nil
		}
		for _, x := range r {
			cart.rates[x.ID] = x
		}
		return r
	}

	// ---- subtotals
	for i := range cart.Items {
		it := &cart.Items[i]
		it.entered = it.Price * float64(it.Quantity)
		it.SubtotalTaxes = map[uint64]float64{}
		it.Subtotal, it.SubtotalTaxes = s.splitTax(it.entered, it, settings, ratesFor, base, taxLoc)
		it.Subtotal = round(it.Subtotal, dec)
		it.SubtotalTax = round(sumTaxes(it.SubtotalTaxes), dec)
	}

	// ---- coupons
	if len(req.Coupons) > 0 && !settings.EnableCoupons {
		cart.notice("error", "coupon.disabled", "Coupons are disabled.")
	} else {
		s.applyCoupons(ctx, &cart, req, settings)
	}

	// ---- totals per line after discounts
	for i := range cart.Items {
		it := &cart.Items[i]
		net := it.entered - it.discount
		if net < 0 {
			net = 0
		}
		it.Total, it.TotalTaxes = s.splitTax(net, it, settings, ratesFor, base, taxLoc)
		it.Total = round(it.Total, dec)
		for k, v := range it.TotalTaxes {
			it.TotalTaxes[k] = round(v, dec)
		}
		it.TotalTax = round(sumTaxes(it.TotalTaxes), dec)
		// display price follows the cart tax display setting
		if settings.TaxDisplayCart == "incl" {
			it.Price = round((it.Subtotal+it.SubtotalTax)/float64(it.Quantity), dec)
		} else {
			it.Price = round(it.Subtotal/float64(it.Quantity), dec)
		}
	}
	// coupon tax split: difference between subtotal and total taxes, per coupon proportionally
	totalDiscountEntered := 0.0
	for _, it := range cart.Items {
		totalDiscountEntered += it.discount
	}

	// ---- shipping
	for _, it := range cart.Items {
		if !it.Virtual {
			cart.NeedsShipping = true
			break
		}
	}
	if !settings.ShippingEnabled {
		cart.NeedsShipping = false
	}
	if cart.NeedsShipping {
		s.priceShipping(ctx, &cart, req, settings, ratesFor)
	}

	// ---- totals
	taxByRate := map[uint64]*CartTax{}
	addTax := func(id uint64, cartTax, shipTax float64) {
		r := cart.rates[id]
		t, ok := taxByRate[id]
		if !ok {
			t = &CartTax{RateID: id, Label: r.Name, Compound: r.Compound, RatePercent: r.Rate}
			taxByRate[id] = t
		}
		t.CartTax += cartTax
		t.ShippingTax += shipTax
	}
	for _, it := range cart.Items {
		cart.Subtotal += it.Subtotal
		cart.SubtotalTax += it.SubtotalTax
		cart.DiscountTotal += it.Subtotal - it.Total
		cart.DiscountTax += it.SubtotalTax - it.TotalTax
		for id, v := range it.TotalTaxes {
			addTax(id, v, 0)
		}
	}
	if cart.ChosenShipping != nil {
		cart.ShippingTotal = round(cart.ChosenShipping.Cost, dec)
		for id, v := range cart.ChosenShipping.Taxes {
			addTax(id, 0, round(v, dec))
		}
		cart.ShippingTax = round(cart.ChosenShipping.Tax, dec)
	}
	ids := []uint64{}
	for id := range taxByRate {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return cart.rates[ids[i]].Priority < cart.rates[ids[j]].Priority || ids[i] < ids[j]
	})
	for _, id := range ids {
		t := taxByRate[id]
		t.CartTax, t.ShippingTax = round(t.CartTax, dec), round(t.ShippingTax, dec)
		t.Amount = round(t.CartTax+t.ShippingTax, dec)
		cart.CartTax += t.CartTax
		cart.Taxes = append(cart.Taxes, *t)
	}
	cart.Subtotal, cart.SubtotalTax = round(cart.Subtotal, dec), round(cart.SubtotalTax, dec)
	cart.DiscountTotal, cart.DiscountTax = round(cart.DiscountTotal, dec), round(cart.DiscountTax, dec)
	cart.CartTax = round(cart.CartTax, dec)
	cart.TaxTotal = round(cart.CartTax+cart.ShippingTax, dec)
	itemsTotal := 0.0
	for _, it := range cart.Items {
		itemsTotal += it.Total
	}
	cart.Total = round(itemsTotal+cart.ShippingTotal+cart.TaxTotal, dec)
	if cart.Total < 0 {
		cart.Total = 0
	}
	// split each coupon's discount tax proportionally to its share
	if totalDiscountEntered > 0 {
		for i := range cart.Coupons {
			share := cart.Coupons[i].Discount / totalDiscountEntered
			cart.Coupons[i].DiscountTax = round(cart.DiscountTax*share, dec)
			cart.Coupons[i].Discount = round(cart.DiscountTotal*share, dec)
		}
	}
	cart.NeedsPayment = cart.Total > 0
	if len(cart.Items) == 0 && len(req.Items) > 0 && !cart.HasErrors() {
		cart.notice("error", "cart.empty", "Your cart is empty.")
	}
	return cart, nil
}

func (s *Store) taxLocation(ctx context.Context, settings ShopSettings, req CartRequest) Location {
	base := s.BaseLocation(ctx)
	switch settings.TaxBasedOn {
	case "base":
		return base
	case "billing":
		if req.Billing != nil && req.Billing.Country != "" {
			return *req.Billing
		}
	}
	if req.Customer.Country != "" {
		return req.Customer
	}
	if s.OptionOr(ctx, "woocommerce_default_customer_address", "base") == "" {
		return Location{}
	}
	return base
}

// splitTax splits an amount "as entered" into (net, taxes) for a line.
func (s *Store) splitTax(entered float64, it *CartItem, settings ShopSettings, ratesFor func(string, Location, bool) []TaxRate, base, loc Location) (float64, map[uint64]float64) {
	taxes := map[uint64]float64{}
	if !settings.CalcTaxes || !it.Taxable {
		return entered, taxes
	}
	rates := ratesFor(it.TaxClass, loc, false)
	if settings.PricesIncludeTax {
		// WooCommerce removes the BASE rates from an inclusive price, then
		// applies the customer's rates on the net (adjust_non_base_location_prices).
		baseRates := ratesFor(it.TaxClass, base, false)
		net := entered - sumTaxes(CalcInclusiveTax(entered, baseRates))
		if sameRates(baseRates, rates) {
			taxes = CalcInclusiveTax(entered, rates)
			return entered - sumTaxes(taxes), taxes
		}
		taxes = CalcExclusiveTax(net, rates)
		return net, taxes
	}
	return entered, CalcExclusiveTax(entered, rates)
}

func sameRates(a, b []TaxRate) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			return false
		}
	}
	return true
}

// applyCoupons validates and applies coupons (discounts "as entered").
func (s *Store) applyCoupons(ctx context.Context, cart *Cart, req CartRequest, settings ShopSettings) {
	codes := []string{}
	seen := map[string]bool{}
	for _, c := range req.Coupons {
		c = strings.ToLower(strings.TrimSpace(c))
		if c != "" && !seen[c] {
			seen[c] = true
			codes = append(codes, c)
		}
	}
	coupons := []Coupon{}
	for _, code := range codes {
		c, err := s.CouponByCode(ctx, code)
		if err != nil {
			cart.notice("error", "coupon.notExist", "Coupon \"%s\" does not exist!", code)
			continue
		}
		coupons = append(coupons, c)
	}
	// individual use: keeps only that coupon
	for _, c := range coupons {
		if c.IndividualUse && len(coupons) > 1 {
			cart.notice("error", "coupon.individualUse", "Coupon \"%s\" cannot be used in conjunction with other coupons.", c.Code)
			coupons = []Coupon{c}
			break
		}
	}
	subtotalEntered := 0.0
	for _, it := range cart.Items {
		subtotalEntered += it.entered
	}
	at := time.Now().UTC()
	for _, c := range coupons {
		fail := func(code, msg string) { cart.notice("error", code, msg, c.Code) }
		if c.DateExpires != nil && at.After(*c.DateExpires) {
			fail("coupon.expired", "Coupon \"%s\" has expired.")
			continue
		}
		if c.UsageLimit != nil && c.UsageCount >= *c.UsageLimit {
			fail("coupon.usageLimit", "Usage limit for coupon \"%s\" has been reached.")
			continue
		}
		if c.UsageLimitPerUser != nil && (req.UserID != 0 || req.Email != "") && couponUsesBy(c, req.UserID, req.Email) >= *c.UsageLimitPerUser {
			fail("coupon.usageLimitUser", "Coupon usage limit for \"%s\" has been reached.")
			continue
		}
		if c.MinimumAmount != nil && subtotalEntered < *c.MinimumAmount {
			fail("coupon.minimum", "The minimum spend for coupon \"%s\" has not been met.")
			continue
		}
		if c.MaximumAmount != nil && subtotalEntered > *c.MaximumAmount {
			fail("coupon.maximum", "The maximum spend for coupon \"%s\" has been exceeded.")
			continue
		}
		if len(c.EmailRestrictions) > 0 && req.Email != "" && !emailAllowed(c.EmailRestrictions, req.Email) {
			fail("coupon.email", "Coupon \"%s\" is not valid for your email address.")
			continue
		}
		eligible := []int{}
		for i, it := range cart.Items {
			if couponAppliesTo(c, it) {
				eligible = append(eligible, i)
			}
		}
		if len(eligible) == 0 && c.DiscountType != "fixed_cart" {
			fail("coupon.notApplicable", "Coupon \"%s\" is not applicable to the products in your cart.")
			continue
		}
		if len(eligible) == 0 && (len(c.ProductIDs) > 0 || len(c.ProductCategories) > 0) {
			fail("coupon.notApplicable", "Coupon \"%s\" is not applicable to the products in your cart.")
			continue
		}
		applied := CartCoupon{Code: c.Code, DiscountType: c.DiscountType, FreeShipping: c.FreeShipping, ID: c.ID}
		applied.Discount = discountLines(cart, c, eligible, settings.Decimals)
		cart.Coupons = append(cart.Coupons, applied)
	}
}

func couponAppliesTo(c Coupon, it CartItem) bool {
	ids := []uint64{it.ProductID}
	if it.VariationID != 0 {
		ids = append(ids, it.VariationID)
	}
	anyIn := func(list []uint64, values []uint64) bool {
		for _, v := range values {
			if containsID(list, v) {
				return true
			}
		}
		return false
	}
	if len(c.ProductIDs) > 0 && !anyIn(c.ProductIDs, ids) {
		return false
	}
	if len(c.ProductCategories) > 0 && !anyIn(c.ProductCategories, it.CategoryIDs) {
		return false
	}
	if anyIn(c.ExcludedProductIDs, ids) || anyIn(c.ExcludedProductCategories, it.CategoryIDs) {
		return false
	}
	if c.ExcludeSaleItems && it.OnSale {
		return false
	}
	return true
}

// discountLines applies a coupon to the eligible lines, answering the
// discount "as entered".
func discountLines(cart *Cart, c Coupon, eligible []int, dec int) float64 {
	remaining := func(i int) float64 { return cart.Items[i].entered - cart.Items[i].discount }
	total := 0.0
	switch c.DiscountType {
	case "percent", "fixed_product":
		// limit_usage_to_x_items: the most expensive units first
		sort.SliceStable(eligible, func(a, b int) bool {
			return cart.Items[eligible[a]].entered/float64(cart.Items[eligible[a]].Quantity) > cart.Items[eligible[b]].entered/float64(cart.Items[eligible[b]].Quantity)
		})
		left := int64(-1)
		if c.LimitUsageToXItems != nil {
			left = *c.LimitUsageToXItems
		}
		for _, i := range eligible {
			it := &cart.Items[i]
			qty := it.Quantity
			if left >= 0 {
				if left == 0 {
					break
				}
				if qty > left {
					qty = left
				}
				left -= qty
			}
			unit := it.entered / float64(it.Quantity)
			var d float64
			if c.DiscountType == "percent" {
				d = unit * float64(qty) * c.Amount / 100
			} else {
				d = minF(c.Amount, unit) * float64(qty)
			}
			d = minF(round(d, dec), remaining(i))
			it.discount += d
			total += d
		}
	case "fixed_cart":
		base := 0.0
		lines := eligible
		if len(lines) == 0 {
			for i := range cart.Items {
				lines = append(lines, i)
			}
		}
		for _, i := range lines {
			base += remaining(i)
		}
		amount := minF(c.Amount, base)
		if base <= 0 {
			return 0
		}
		spread := 0.0
		for n, i := range lines {
			share := round(amount*remaining(i)/base, dec)
			if n == len(lines)-1 {
				share = round(amount-spread, dec)
			}
			share = minF(share, remaining(i))
			cart.Items[i].discount += share
			spread += share
		}
		total = spread
	}
	return total
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// priceShipping finds the zone for the destination and prices its methods.
func (s *Store) priceShipping(ctx context.Context, cart *Cart, req CartRequest, settings ShopSettings, ratesFor func(string, Location, bool) []TaxRate) {
	dest := req.Customer
	if dest.Country == "" {
		if s.Option(ctx, "woocommerce_default_customer_address") == "" || s.OptionBool(ctx, "woocommerce_shipping_cost_requires_address", false) {
			return
		}
		dest = s.BaseLocation(ctx)
	}
	allowed := false
	for _, c := range settings.ShipToCountries {
		if strings.EqualFold(c, dest.Country) {
			allowed = true
			break
		}
	}
	if !allowed {
		cart.notice("error", "shipping.unavailableCountry", "Unfortunately we do not ship to %s.", dest.Country)
		return
	}
	zone, err := s.MatchZone(ctx, dest)
	if err != nil {
		return
	}
	pkg := shippingPackage{classQty: map[uint64]int64{}, classCost: map[uint64]float64{}}
	for _, it := range cart.Items {
		if it.Virtual {
			continue
		}
		after := it.entered - it.discount
		pkg.contentsCost += after
		pkg.subtotalCost += it.entered
		pkg.qty += it.Quantity
		pkg.classQty[it.ShippingClass] += it.Quantity
		pkg.classCost[it.ShippingClass] += after
	}
	for _, c := range cart.Coupons {
		if c.FreeShipping {
			pkg.freeShipCoupon = true
		}
	}
	shipClass := s.OptionOr(ctx, "woocommerce_shipping_tax_class", "inherit")
	if shipClass == "inherit" {
		// the item class with the highest rate wins (WooCommerce's behaviour)
		best, bestRate := "", -1.0
		for _, it := range cart.Items {
			if !it.Taxable || it.Virtual {
				continue
			}
			r := 0.0
			for _, x := range ratesFor(it.TaxClass, cart.TaxLocation, true) {
				r += x.Rate
			}
			if r > bestRate {
				best, bestRate = it.TaxClass, r
			}
		}
		shipClass = best
	}
	for _, m := range zone.Methods {
		if !m.Enabled {
			continue
		}
		rate, ok := s.rateFor(ctx, m, pkg, settings.DecimalSeparator)
		if !ok {
			continue
		}
		rate.Cost = round(rate.Cost, settings.Decimals)
		rate.Taxes = map[uint64]float64{}
		if rate.Taxable && settings.CalcTaxes && rate.Cost > 0 {
			rate.Taxes = CalcExclusiveTax(rate.Cost, ratesFor(shipClass, cart.TaxLocation, true))
			for k, v := range rate.Taxes {
				rate.Taxes[k] = round(v, settings.Decimals)
			}
		}
		rate.Tax = round(sumTaxes(rate.Taxes), settings.Decimals)
		cart.ShippingRates = append(cart.ShippingRates, rate)
	}
	if len(cart.ShippingRates) == 0 {
		cart.notice("error", "shipping.noMethods", "No shipping options were found for your address.")
		return
	}
	chosen := 0
	for i, r := range cart.ShippingRates {
		if r.ID == req.ShippingMethod {
			chosen = i
		}
	}
	c := cart.ShippingRates[chosen]
	cart.ChosenShippingMethod, cart.ChosenShipping = c.ID, &c
}

// UnmarshalJSON accepts attributes as a map ({"pa_color":"Blue"}) or a list
// ([{"slug":"pa_color","option":"Blue"}]).
func (l *CartLine) UnmarshalJSON(b []byte) error {
	var raw struct {
		ProductID   uint64          `json:"productId"`
		VariationID uint64          `json:"variationId"`
		Quantity    int64           `json:"quantity"`
		Attributes  json.RawMessage `json:"attributes"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	l.ProductID, l.VariationID, l.Quantity = raw.ProductID, raw.VariationID, raw.Quantity
	l.Attributes = map[string]string{}
	if len(raw.Attributes) == 0 || string(raw.Attributes) == "null" {
		return nil
	}
	if raw.Attributes[0] == '{' {
		return json.Unmarshal(raw.Attributes, &l.Attributes)
	}
	var list []AttributeValue
	if err := json.Unmarshal(raw.Attributes, &list); err != nil {
		return err
	}
	for _, a := range list {
		l.Attributes[a.Slug] = a.Option
	}
	return nil
}
