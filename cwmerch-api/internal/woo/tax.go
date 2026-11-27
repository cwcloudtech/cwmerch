package woo

import (
	"context"
	"database/sql"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// TaxRate is a row of woocommerce_tax_rates with its postcode/city locations.
type TaxRate struct {
	ID        uint64   `json:"id"`
	Country   string   `json:"country"`
	State     string   `json:"state"`
	Postcodes []string `json:"postcodes"`
	Cities    []string `json:"cities"`
	Rate      float64  `json:"rate"`
	Name      string   `json:"name"`
	Priority  int      `json:"priority"`
	Compound  bool     `json:"compound"`
	Shipping  bool     `json:"shipping"`
	Order     int      `json:"order"`
	Class     string   `json:"class"`
}

// TaxClass is a tax class ("" slug = Standard).
type TaxClass struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// TaxClasses lists the classes (Standard first).
func (s *Store) TaxClasses(ctx context.Context) ([]TaxClass, error) {
	out := []TaxClass{{Slug: "", Name: "Standard"}}
	rows, err := s.DB.QueryContext(ctx, `SELECT slug, name FROM `+s.T("wc_tax_rate_classes")+` ORDER BY tax_rate_class_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c TaxClass
		if err := rows.Scan(&c.Slug, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreateTaxClass(ctx context.Context, name string) (TaxClass, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return TaxClass{}, refuse(400, "errors.nameRequired", "A name is required")
	}
	slug := Slugify(name)
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO `+s.T("wc_tax_rate_classes")+` (name, slug) VALUES (?, ?)`, name, slug); err != nil {
		if strings.Contains(err.Error(), "1062") {
			return TaxClass{}, refuse(400, "errors.taxClassExists", "This tax class already exists")
		}
		return TaxClass{}, err
	}
	// WooCommerce also mirrors the class list in an option.
	classes, _ := s.TaxClasses(ctx)
	names := []string{}
	for _, c := range classes[1:] {
		names = append(names, c.Name)
	}
	_ = s.SetOption(ctx, "woocommerce_tax_classes", strings.Join(names, "\n"))
	return TaxClass{Slug: slug, Name: name}, nil
}

func (s *Store) DeleteTaxClass(ctx context.Context, slug string) error {
	if slug == "" {
		return refuse(400, "errors.standardTaxClass", "The standard tax class cannot be deleted")
	}
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM `+s.T("wc_tax_rate_classes")+` WHERE slug = ?`, slug)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.T("woocommerce_tax_rate_locations")+` WHERE tax_rate_id IN (SELECT tax_rate_id FROM `+s.T("woocommerce_tax_rates")+
			` WHERE tax_rate_class = ?)`, slug); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM `+s.T("woocommerce_tax_rates")+` WHERE tax_rate_class = ?`, slug)
		return err
	})
}

// TaxRates lists rates, optionally of one class ("standard" or "" = Standard; "*" = all).
func (s *Store) TaxRates(ctx context.Context, class string) ([]TaxRate, error) {
	where, args := "1=1", []any{}
	switch class {
	case "*":
	case "standard":
		where, args = "tax_rate_class = ''", nil
	default:
		where, args = "tax_rate_class = ?", []any{class}
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT tax_rate_id, tax_rate_country, tax_rate_state, tax_rate, tax_rate_name, tax_rate_priority,
		tax_rate_compound, tax_rate_shipping, tax_rate_order, tax_rate_class FROM `+s.T("woocommerce_tax_rates")+` WHERE `+where+
		` ORDER BY tax_rate_order, tax_rate_id`, args...)
	if err != nil {
		return nil, err
	}
	rates := []TaxRate{}
	ids := []uint64{}
	for rows.Next() {
		var r TaxRate
		var rate string
		var compound, shipping int
		if err := rows.Scan(&r.ID, &r.Country, &r.State, &rate, &r.Name, &r.Priority, &compound, &shipping, &r.Order, &r.Class); err != nil {
			rows.Close()
			return nil, err
		}
		r.Rate, r.Compound, r.Shipping = parseFloat(rate), compound == 1, shipping == 1
		r.Postcodes, r.Cities = []string{}, []string{}
		rates = append(rates, r)
		ids = append(ids, r.ID)
	}
	rows.Close()
	if len(ids) == 0 {
		return rates, nil
	}
	lrows, err := s.DB.QueryContext(ctx, `SELECT tax_rate_id, location_code, location_type FROM `+s.T("woocommerce_tax_rate_locations")+
		` WHERE tax_rate_id IN (`+inList(ids)+`)`)
	if err != nil {
		return nil, err
	}
	defer lrows.Close()
	idx := map[uint64]int{}
	for i, r := range rates {
		idx[r.ID] = i
	}
	for lrows.Next() {
		var id uint64
		var code, typ string
		if err := lrows.Scan(&id, &code, &typ); err != nil {
			return nil, err
		}
		i := idx[id]
		if typ == "postcode" {
			rates[i].Postcodes = append(rates[i].Postcodes, code)
		} else {
			rates[i].Cities = append(rates[i].Cities, code)
		}
	}
	return rates, lrows.Err()
}

// SaveTaxRate creates (id 0) or updates a rate.
func (s *Store) SaveTaxRate(ctx context.Context, id uint64, r TaxRate) (TaxRate, error) {
	if r.Name == "" {
		r.Name = "Tax"
	}
	if r.Priority <= 0 {
		r.Priority = 1
	}
	if r.Class == "standard" {
		r.Class = ""
	}
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		vals := []any{strings.ToUpper(strings.TrimSpace(r.Country)), strings.ToUpper(strings.TrimSpace(r.State)),
			strconv.FormatFloat(r.Rate, 'f', 4, 64), r.Name, r.Priority, boolInt(r.Compound), boolInt(r.Shipping), r.Order, r.Class}
		if id == 0 {
			res, err := tx.ExecContext(ctx, `INSERT INTO `+s.T("woocommerce_tax_rates")+` (tax_rate_country, tax_rate_state, tax_rate, tax_rate_name,
				tax_rate_priority, tax_rate_compound, tax_rate_shipping, tax_rate_order, tax_rate_class) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, vals...)
			if err != nil {
				return err
			}
			n, _ := res.LastInsertId()
			id = uint64(n)
		} else {
			res, err := tx.ExecContext(ctx, `UPDATE `+s.T("woocommerce_tax_rates")+` SET tax_rate_country = ?, tax_rate_state = ?, tax_rate = ?,
				tax_rate_name = ?, tax_rate_priority = ?, tax_rate_compound = ?, tax_rate_shipping = ?, tax_rate_order = ?, tax_rate_class = ?
				WHERE tax_rate_id = ?`, append(vals, id)...)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				var exists int
				if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.T("woocommerce_tax_rates")+` WHERE tax_rate_id = ?`, id).Scan(&exists); err != nil || exists == 0 {
					return ErrNotFound
				}
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.T("woocommerce_tax_rate_locations")+` WHERE tax_rate_id = ?`, id); err != nil {
			return err
		}
		for typ, codes := range map[string][]string{"postcode": r.Postcodes, "city": r.Cities} {
			for _, c := range codes {
				c = strings.ToUpper(strings.TrimSpace(c))
				if c == "" {
					continue
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO `+s.T("woocommerce_tax_rate_locations")+` (location_code, tax_rate_id, location_type) VALUES (?, ?, ?)`, c, id, typ); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return TaxRate{}, err
	}
	all, err := s.TaxRates(ctx, "*")
	if err != nil {
		return TaxRate{}, err
	}
	for _, x := range all {
		if x.ID == id {
			return x, nil
		}
	}
	return TaxRate{}, ErrNotFound
}

func (s *Store) DeleteTaxRate(ctx context.Context, id uint64) error {
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM `+s.T("woocommerce_tax_rates")+` WHERE tax_rate_id = ?`, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM `+s.T("woocommerce_tax_rate_locations")+` WHERE tax_rate_id = ?`, id)
		return err
	})
}

// Location is where tax and shipping are computed for.
type Location struct {
	Country  string `json:"country"`
	State    string `json:"state"`
	Postcode string `json:"postcode"`
	City     string `json:"city"`
}

// BaseLocation is the shop's address.
func (s *Store) BaseLocation(ctx context.Context) Location {
	c, st := splitCountry(s.OptionOr(ctx, "woocommerce_default_country", "FR"))
	return Location{Country: c, State: st, Postcode: s.Option(ctx, "woocommerce_store_postcode"), City: s.Option(ctx, "woocommerce_store_city")}
}

var postcodeClean = regexp.MustCompile(`[^A-Z0-9]`)

func normalizePostcode(p string) string {
	return postcodeClean.ReplaceAllString(strings.ToUpper(p), "")
}

// PostcodeMatches implements WooCommerce's postcode patterns: exact, "AB*"
// wildcards and "10000...19999" ranges.
func PostcodeMatches(pattern, postcode string) bool {
	pattern = strings.ToUpper(strings.TrimSpace(pattern))
	pc := normalizePostcode(postcode)
	if pattern == "" {
		return false
	}
	if strings.Contains(pattern, "...") {
		parts := strings.SplitN(pattern, "...", 2)
		lo, hi := normalizePostcode(parts[0]), normalizePostcode(parts[1])
		if ln, err1 := strconv.ParseInt(lo, 10, 64); err1 == nil {
			if hn, err2 := strconv.ParseInt(hi, 10, 64); err2 == nil {
				if n, err3 := strconv.ParseInt(pc, 10, 64); err3 == nil {
					return n >= ln && n <= hn
				}
				return false
			}
		}
		return pc >= lo && pc <= hi
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(pc, normalizePostcode(strings.TrimSuffix(pattern, "*")))
	}
	return normalizePostcode(pattern) == pc
}

// FindRates is WC_Tax::find_rates: for each priority, the first rate (by
// order) whose country/state/postcode/city match the location.
func (s *Store) FindRates(ctx context.Context, loc Location, class string, forShipping bool) ([]TaxRate, error) {
	all, err := s.TaxRates(ctx, map[bool]string{true: "standard", false: class}[class == ""])
	if err != nil {
		return nil, err
	}
	matches := []TaxRate{}
	country, state, city := strings.ToUpper(loc.Country), strings.ToUpper(loc.State), strings.ToUpper(strings.TrimSpace(loc.City))
	for _, r := range all {
		if forShipping && !r.Shipping {
			continue
		}
		if r.Country != "" && r.Country != country {
			continue
		}
		if r.State != "" && r.State != state {
			continue
		}
		if len(r.Postcodes) > 0 {
			ok := false
			for _, p := range r.Postcodes {
				if PostcodeMatches(p, loc.Postcode) {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		if len(r.Cities) > 0 {
			ok := false
			for _, c := range r.Cities {
				if c == city {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		matches = append(matches, r)
	}
	// most specific first within a priority (WC orders by specificity, then order)
	specificity := func(r TaxRate) int {
		n := 0
		if r.Country != "" {
			n++
		}
		if r.State != "" {
			n++
		}
		if len(r.Postcodes) > 0 {
			n++
		}
		if len(r.Cities) > 0 {
			n++
		}
		return n
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Priority != matches[j].Priority {
			return matches[i].Priority < matches[j].Priority
		}
		if specificity(matches[i]) != specificity(matches[j]) {
			return specificity(matches[i]) > specificity(matches[j])
		}
		return matches[i].Order < matches[j].Order
	})
	out := []TaxRate{}
	seen := map[int]bool{}
	for _, r := range matches {
		if seen[r.Priority] {
			continue
		}
		seen[r.Priority] = true
		out = append(out, r)
	}
	return out, nil
}

// CalcExclusiveTax is the tax on a price entered without tax.
func CalcExclusiveTax(price float64, rates []TaxRate) map[uint64]float64 {
	taxes := map[uint64]float64{}
	pre := 0.0
	for _, r := range rates {
		if !r.Compound {
			t := price * r.Rate / 100
			taxes[r.ID] = t
			pre += t
		}
	}
	compoundBase := price + pre
	for _, r := range rates {
		if r.Compound {
			t := compoundBase * r.Rate / 100
			taxes[r.ID] = t
			compoundBase += t
		}
	}
	return taxes
}

// CalcInclusiveTax is the tax contained in a price entered with tax.
func CalcInclusiveTax(price float64, rates []TaxRate) map[uint64]float64 {
	taxes := map[uint64]float64{}
	nonCompound := price
	for i := len(rates) - 1; i >= 0; i-- {
		r := rates[i]
		if r.Compound {
			t := nonCompound - nonCompound/(1+r.Rate/100)
			taxes[r.ID] = t
			nonCompound -= t
		}
	}
	regular := 0.0
	for _, r := range rates {
		if !r.Compound {
			regular += r.Rate
		}
	}
	for _, r := range rates {
		if !r.Compound {
			theRate := (r.Rate / 100) / (1 + regular/100)
			taxes[r.ID] = theRate * nonCompound
		}
	}
	return taxes
}

func sumTaxes(t map[uint64]float64) float64 {
	n := 0.0
	for _, v := range t {
		n += v
	}
	return n
}
