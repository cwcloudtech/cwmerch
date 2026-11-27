package woo

import (
	"context"
	"database/sql"
	"math"
	"regexp"
	"strconv"
	"strings"

	"cwmerch-api/internal/php"
)

// ZoneLocation is a zone's location: country "FR", state "US:CA",
// continent "EU" or postcode "75*".
type ZoneLocation struct {
	Code string `json:"code"`
	Type string `json:"type"`
}

// ShippingMethod is a method instance in a zone.
type ShippingMethod struct {
	InstanceID uint64            `json:"instanceId"`
	MethodID   string            `json:"methodId"`
	Title      string            `json:"title"`
	Enabled    bool              `json:"enabled"`
	Order      int               `json:"order"`
	Settings   map[string]string `json:"settings"`
}

// ShippingZone is a WooCommerce shipping zone (id 0 = everywhere else).
type ShippingZone struct {
	ID        uint64           `json:"id"`
	Name      string           `json:"name"`
	Order     int              `json:"order"`
	Locations []ZoneLocation   `json:"locations"`
	Methods   []ShippingMethod `json:"methods"`
}

var methodTitles = map[string]string{"flat_rate": "Flat rate", "free_shipping": "Free shipping", "local_pickup": "Local pickup"}

// ShippingMethodIDs are the methods cwmerch computes.
var ShippingMethodIDs = []string{"flat_rate", "free_shipping", "local_pickup"}

func (s *Store) zoneMethods(ctx context.Context, zoneID uint64) ([]ShippingMethod, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT instance_id, method_id, method_order, is_enabled FROM `+s.T("woocommerce_shipping_zone_methods")+
		` WHERE zone_id = ? ORDER BY method_order, instance_id`, zoneID)
	if err != nil {
		return nil, err
	}
	out := []ShippingMethod{}
	for rows.Next() {
		var m ShippingMethod
		var enabled int
		if err := rows.Scan(&m.InstanceID, &m.MethodID, &m.Order, &enabled); err != nil {
			rows.Close()
			return nil, err
		}
		m.Enabled = enabled == 1
		out = append(out, m)
	}
	rows.Close()
	for i := range out {
		settings := s.OptionArray(ctx, "woocommerce_"+out[i].MethodID+"_"+strconv.FormatUint(out[i].InstanceID, 10)+"_settings")
		out[i].Settings = settings.StringMap()
		out[i].Title = orDefault(out[i].Settings["title"], methodTitles[out[i].MethodID])
		if out[i].Title == "" {
			out[i].Title = out[i].MethodID
		}
	}
	return out, nil
}

// ShippingZones lists the zones, then "rest of the world" (id 0).
func (s *Store) ShippingZones(ctx context.Context) ([]ShippingZone, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT zone_id, zone_name, zone_order FROM `+s.T("woocommerce_shipping_zones")+` ORDER BY zone_order, zone_id`)
	if err != nil {
		return nil, err
	}
	zones := []ShippingZone{}
	for rows.Next() {
		var z ShippingZone
		if err := rows.Scan(&z.ID, &z.Name, &z.Order); err != nil {
			rows.Close()
			return nil, err
		}
		z.Locations = []ZoneLocation{}
		zones = append(zones, z)
	}
	rows.Close()
	lrows, err := s.DB.QueryContext(ctx, `SELECT zone_id, location_code, location_type FROM `+s.T("woocommerce_shipping_zone_locations")+` ORDER BY location_id`)
	if err != nil {
		return nil, err
	}
	idx := map[uint64]int{}
	for i, z := range zones {
		idx[z.ID] = i
	}
	for lrows.Next() {
		var id uint64
		var l ZoneLocation
		if err := lrows.Scan(&id, &l.Code, &l.Type); err != nil {
			lrows.Close()
			return nil, err
		}
		if i, ok := idx[id]; ok {
			zones[i].Locations = append(zones[i].Locations, l)
		}
	}
	lrows.Close()
	zones = append(zones, ShippingZone{ID: 0, Name: "Locations not covered by your other zones", Locations: []ZoneLocation{}})
	for i := range zones {
		if zones[i].Methods, err = s.zoneMethods(ctx, zones[i].ID); err != nil {
			return nil, err
		}
	}
	return zones, nil
}

func (s *Store) ShippingZone(ctx context.Context, id uint64) (ShippingZone, error) {
	zones, err := s.ShippingZones(ctx)
	if err != nil {
		return ShippingZone{}, err
	}
	for _, z := range zones {
		if z.ID == id {
			return z, nil
		}
	}
	return ShippingZone{}, ErrNotFound
}

// ZoneInput is a zone create/update.
type ZoneInput struct {
	Name      *string         `json:"name"`
	Order     *int            `json:"order"`
	Locations *[]ZoneLocation `json:"locations"`
}

func (s *Store) SaveShippingZone(ctx context.Context, id uint64, in ZoneInput) (ShippingZone, error) {
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		if id == 0 {
			if in.Name == nil || strings.TrimSpace(*in.Name) == "" {
				return refuse(400, "errors.nameRequired", "A zone name is required")
			}
			order := 0
			if in.Order != nil {
				order = *in.Order
			}
			res, err := tx.ExecContext(ctx, `INSERT INTO `+s.T("woocommerce_shipping_zones")+` (zone_name, zone_order) VALUES (?, ?)`, *in.Name, order)
			if err != nil {
				return err
			}
			n, _ := res.LastInsertId()
			id = uint64(n)
		} else {
			if _, err := tx.ExecContext(ctx, `UPDATE `+s.T("woocommerce_shipping_zones")+` SET zone_name = COALESCE(?, zone_name), zone_order = COALESCE(?, zone_order) WHERE zone_id = ?`,
				in.Name, in.Order, id); err != nil {
				return err
			}
		}
		if in.Locations != nil {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.T("woocommerce_shipping_zone_locations")+` WHERE zone_id = ?`, id); err != nil {
				return err
			}
			for _, l := range *in.Locations {
				switch l.Type {
				case "country", "state", "continent", "postcode":
				default:
					return refuse(400, "errors.invalidZoneLocation", "Location type must be country, state, continent or postcode")
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO `+s.T("woocommerce_shipping_zone_locations")+` (zone_id, location_code, location_type) VALUES (?, ?, ?)`,
					id, strings.TrimSpace(l.Code), l.Type); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return ShippingZone{}, err
	}
	return s.ShippingZone(ctx, id)
}

func (s *Store) DeleteShippingZone(ctx context.Context, id uint64) error {
	if id == 0 {
		return refuse(400, "errors.restOfWorldZone", "The \"rest of the world\" zone cannot be deleted")
	}
	z, err := s.ShippingZone(ctx, id)
	if err != nil {
		return err
	}
	for _, m := range z.Methods {
		_ = s.DeleteOption(ctx, "woocommerce_"+m.MethodID+"_"+strconv.FormatUint(m.InstanceID, 10)+"_settings")
	}
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		for _, t := range []string{"woocommerce_shipping_zone_methods", "woocommerce_shipping_zone_locations", "woocommerce_shipping_zones"} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.T(t)+` WHERE zone_id = ?`, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) AddShippingMethod(ctx context.Context, zoneID uint64, methodID string) (ShippingMethod, error) {
	if _, ok := methodTitles[methodID]; !ok {
		return ShippingMethod{}, refuse(400, "errors.unknownShippingMethod", "Unknown shipping method")
	}
	if _, err := s.ShippingZone(ctx, zoneID); err != nil {
		return ShippingMethod{}, err
	}
	var order int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(method_order) + 1, 1) FROM `+s.T("woocommerce_shipping_zone_methods")+` WHERE zone_id = ?`, zoneID).Scan(&order)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO `+s.T("woocommerce_shipping_zone_methods")+` (zone_id, method_id, method_order, is_enabled) VALUES (?, ?, ?, 1)`,
		zoneID, methodID, order)
	if err != nil {
		return ShippingMethod{}, err
	}
	n, _ := res.LastInsertId()
	defaults := map[string]string{"title": methodTitles[methodID]}
	switch methodID {
	case "flat_rate":
		defaults["tax_status"], defaults["cost"] = "taxable", "0"
	case "local_pickup":
		defaults["tax_status"], defaults["cost"] = "taxable", ""
	case "free_shipping":
		defaults["requires"], defaults["min_amount"], defaults["ignore_discounts"] = "", "0", "no"
	}
	if err := s.SetOption(ctx, "woocommerce_"+methodID+"_"+strconv.FormatInt(n, 10)+"_settings", php.Marshal(php.FromStringMap(defaults))); err != nil {
		return ShippingMethod{}, err
	}
	return s.shippingMethod(ctx, zoneID, uint64(n))
}

func (s *Store) shippingMethod(ctx context.Context, zoneID, instanceID uint64) (ShippingMethod, error) {
	methods, err := s.zoneMethods(ctx, zoneID)
	if err != nil {
		return ShippingMethod{}, err
	}
	for _, m := range methods {
		if m.InstanceID == instanceID {
			return m, nil
		}
	}
	return ShippingMethod{}, ErrNotFound
}

// MethodInput is a method instance update.
type MethodInput struct {
	Enabled  *bool             `json:"enabled"`
	Order    *int              `json:"order"`
	Settings map[string]string `json:"settings"`
}

func (s *Store) UpdateShippingMethod(ctx context.Context, zoneID, instanceID uint64, in MethodInput) (ShippingMethod, error) {
	m, err := s.shippingMethod(ctx, zoneID, instanceID)
	if err != nil {
		return ShippingMethod{}, err
	}
	if in.Enabled != nil || in.Order != nil {
		var enabled any
		if in.Enabled != nil {
			enabled = boolInt(*in.Enabled)
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE `+s.T("woocommerce_shipping_zone_methods")+` SET is_enabled = COALESCE(?, is_enabled), method_order = COALESCE(?, method_order) WHERE instance_id = ?`,
			enabled, in.Order, instanceID); err != nil {
			return ShippingMethod{}, err
		}
	}
	if in.Settings != nil {
		key := "woocommerce_" + m.MethodID + "_" + strconv.FormatUint(instanceID, 10) + "_settings"
		arr := s.OptionArray(ctx, key)
		for k, v := range in.Settings {
			arr.Set(k, v)
		}
		if err := s.SetOption(ctx, key, php.Marshal(arr)); err != nil {
			return ShippingMethod{}, err
		}
	}
	return s.shippingMethod(ctx, zoneID, instanceID)
}

func (s *Store) DeleteShippingMethod(ctx context.Context, zoneID, instanceID uint64) error {
	m, err := s.shippingMethod(ctx, zoneID, instanceID)
	if err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM `+s.T("woocommerce_shipping_zone_methods")+` WHERE instance_id = ?`, instanceID); err != nil {
		return err
	}
	return s.DeleteOption(ctx, "woocommerce_"+m.MethodID+"_"+strconv.FormatUint(instanceID, 10)+"_settings")
}

// MatchZone is the first zone (by order) covering the destination, else zone 0.
func (s *Store) MatchZone(ctx context.Context, dest Location) (ShippingZone, error) {
	zones, err := s.ShippingZones(ctx)
	if err != nil {
		return ShippingZone{}, err
	}
	country := strings.ToUpper(dest.Country)
	for _, z := range zones {
		if z.ID == 0 {
			continue
		}
		regionOK, hasRegion, postcodeOK, hasPostcode := false, false, false, false
		for _, l := range z.Locations {
			switch l.Type {
			case "country":
				hasRegion = true
				regionOK = regionOK || strings.EqualFold(l.Code, country)
			case "state":
				hasRegion = true
				regionOK = regionOK || strings.EqualFold(l.Code, country+":"+dest.State)
			case "continent":
				hasRegion = true
				regionOK = regionOK || strings.EqualFold(l.Code, ContinentOf(country))
			case "postcode":
				hasPostcode = true
				postcodeOK = postcodeOK || PostcodeMatches(l.Code, dest.Postcode)
			}
		}
		if (!hasRegion || regionOK) && (!hasPostcode || postcodeOK) && (hasRegion || hasPostcode) {
			return z, nil
		}
	}
	return zones[len(zones)-1], nil
}

// ShippingRate is a priced option shown at checkout.
type ShippingRate struct {
	ID         string             `json:"id"`
	MethodID   string             `json:"methodId"`
	InstanceID uint64             `json:"instanceId"`
	Label      string             `json:"label"`
	Cost       float64            `json:"cost"`
	Tax        float64            `json:"tax"`
	Taxes      map[uint64]float64 `json:"-"`
	Taxable    bool               `json:"-"`
}

// shippingPackage is what a method prices.
type shippingPackage struct {
	contentsCost   float64 // line totals after discount, as entered
	subtotalCost   float64 // before discounts
	qty            int64
	classQty       map[uint64]int64 // shipping class term id -> qty (0 = no class)
	classCost      map[uint64]float64
	freeShipCoupon bool
}

var shortcode = regexp.MustCompile(`\[(qty|cost)\]`)
var feeCode = regexp.MustCompile(`\[fee([^\]]*)\]`)
var attrRe = regexp.MustCompile(`(\w+)="([^"]*)"`)

// evalCost evaluates a flat-rate cost formula: numbers, + - * / ( ), [qty],
// [cost] and [fee percent="10" min_fee="2" max_fee="20"].
func evalCost(formula string, qty int64, cost float64, decSep string) float64 {
	f := strings.TrimSpace(formula)
	if f == "" {
		return 0
	}
	f = feeCode.ReplaceAllStringFunc(f, func(m string) string {
		attrs := map[string]string{}
		for _, a := range attrRe.FindAllStringSubmatch(m, -1) {
			attrs[a[1]] = a[2]
		}
		fee := cost * parseFloat(attrs["percent"]) / 100
		if v := attrs["min_fee"]; v != "" && fee < parseFloat(v) {
			fee = parseFloat(v)
		}
		if v := attrs["max_fee"]; v != "" && fee > parseFloat(v) {
			fee = parseFloat(v)
		}
		return strconv.FormatFloat(fee, 'f', -1, 64)
	})
	f = shortcode.ReplaceAllStringFunc(f, func(m string) string {
		if m == "[qty]" {
			return strconv.FormatInt(qty, 10)
		}
		return strconv.FormatFloat(cost, 'f', -1, 64)
	})
	if decSep != "" && decSep != "." {
		f = strings.ReplaceAll(f, decSep, ".")
	}
	v, ok := evalArith(f)
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// evalArith is a tiny recursive-descent evaluator for + - * / and parentheses.
func evalArith(expr string) (float64, bool) {
	p := &arith{s: strings.ReplaceAll(expr, " ", "")}
	v := p.sum()
	return v, p.ok && p.i == len(p.s)
}

type arith struct {
	s  string
	i  int
	ok bool
}

func (p *arith) sum() float64 {
	p.ok = true
	v := p.product()
	for p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
		op := p.s[p.i]
		p.i++
		r := p.product()
		if op == '+' {
			v += r
		} else {
			v -= r
		}
	}
	return v
}

func (p *arith) product() float64 {
	v := p.unary()
	for p.i < len(p.s) && (p.s[p.i] == '*' || p.s[p.i] == '/') {
		op := p.s[p.i]
		p.i++
		r := p.unary()
		if op == '*' {
			v *= r
		} else if r != 0 {
			v /= r
		} else {
			p.ok = false
		}
	}
	return v
}

func (p *arith) unary() float64 {
	if p.i < len(p.s) && p.s[p.i] == '-' {
		p.i++
		return -p.unary()
	}
	if p.i < len(p.s) && p.s[p.i] == '(' {
		p.i++
		v := p.sum()
		if p.i < len(p.s) && p.s[p.i] == ')' {
			p.i++
		} else {
			p.ok = false
		}
		return v
	}
	start := p.i
	for p.i < len(p.s) && (p.s[p.i] >= '0' && p.s[p.i] <= '9' || p.s[p.i] == '.') {
		p.i++
	}
	if start == p.i {
		p.ok = false
		return 0
	}
	v, err := strconv.ParseFloat(p.s[start:p.i], 64)
	if err != nil {
		p.ok = false
	}
	return v
}

// rateFor prices one method instance for a package; ok=false when not available.
func (s *Store) rateFor(ctx context.Context, m ShippingMethod, pkg shippingPackage, decSep string) (ShippingRate, bool) {
	r := ShippingRate{ID: m.MethodID + ":" + strconv.FormatUint(m.InstanceID, 10), MethodID: m.MethodID, InstanceID: m.InstanceID,
		Label: m.Title, Taxable: m.Settings["tax_status"] != "none"}
	switch m.MethodID {
	case "free_shipping":
		r.Taxable = false
		amount := pkg.contentsCost
		if m.Settings["ignore_discounts"] == "yes" {
			amount = pkg.subtotalCost
		}
		min := parseFloat(m.Settings["min_amount"])
		hasMin := amount >= min
		switch m.Settings["requires"] {
		case "coupon":
			return r, pkg.freeShipCoupon
		case "min_amount":
			return r, hasMin
		case "either":
			return r, hasMin || pkg.freeShipCoupon
		case "both":
			return r, hasMin && pkg.freeShipCoupon
		}
		return r, true
	case "local_pickup":
		r.Cost = evalCost(m.Settings["cost"], pkg.qty, pkg.contentsCost, decSep)
		return r, true
	case "flat_rate":
		cost := evalCost(m.Settings["cost"], pkg.qty, pkg.contentsCost, decSep)
		classTotal, highest := 0.0, 0.0
		foundClassCost := false
		for classID, qty := range pkg.classQty {
			key := "no_class_cost"
			if classID != 0 {
				key = "class_cost_" + strconv.FormatUint(classID, 10)
			}
			formula, ok := m.Settings[key]
			if !ok || strings.TrimSpace(formula) == "" {
				continue
			}
			foundClassCost = true
			c := evalCost(formula, qty, pkg.classCost[classID], decSep)
			classTotal += c
			if c > highest {
				highest = c
			}
		}
		if foundClassCost {
			if m.Settings["type"] == "order" {
				cost += highest
			} else {
				cost += classTotal
			}
		}
		r.Cost = cost
		return r, true
	}
	return r, false
}
