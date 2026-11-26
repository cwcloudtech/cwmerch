package woo

import (
	"context"
	"strings"
)

// ParseIDList reads "1,2,3".
func ParseIDList(v string) []uint64 { return csvIDs(v) }

// AddressMeta is an address as user/order meta keys with a prefix.
func AddressMeta(a Address, prefix string) map[string]string { return addressMeta(a, prefix) }

// AddressFromMeta reads an address from prefixed meta keys.
func AddressFromMeta(m map[string]string, prefix string) Address { return addressFromMeta(m, prefix) }

// ApplyDisplayPrices converts catalogue prices to the "display prices in the
// shop" setting (incl./excl. tax), using the shop's base location rates,
// like wc_get_price_to_display does for a visitor without an address.
func (s *Store) ApplyDisplayPrices(ctx context.Context, products []Product) {
	set := s.ShopSettings(ctx)
	if !set.CalcTaxes {
		return
	}
	wantIncl := set.TaxDisplayShop == "incl"
	if wantIncl == set.PricesIncludeTax {
		return
	}
	base := s.BaseLocation(ctx)
	rateCache := map[string][]TaxRate{}
	rates := func(class string) []TaxRate {
		if r, ok := rateCache[class]; ok {
			return r
		}
		r, _ := s.FindRates(ctx, base, class, false)
		rateCache[class] = r
		return r
	}
	conv := func(price float64, class string, taxable bool) float64 {
		if !taxable || price == 0 {
			return price
		}
		r := rates(class)
		if wantIncl {
			return round(price+sumTaxes(CalcExclusiveTax(price, r)), set.Decimals)
		}
		return round(price-sumTaxes(CalcInclusiveTax(price, r)), set.Decimals)
	}
	convPtr := func(p *float64, class string, taxable bool) *float64 {
		if p == nil {
			return nil
		}
		v := conv(*p, class, taxable)
		return &v
	}
	for i := range products {
		p := &products[i]
		taxable := p.TaxStatus == "taxable"
		p.Price = conv(p.Price, p.TaxClass, taxable)
		p.PriceMin = conv(p.PriceMin, p.TaxClass, taxable)
		p.PriceMax = conv(p.PriceMax, p.TaxClass, taxable)
		p.RegularPrice = convPtr(p.RegularPrice, p.TaxClass, taxable)
		p.SalePrice = convPtr(p.SalePrice, p.TaxClass, taxable)
		for j := range p.Variations {
			v := &p.Variations[j]
			class := v.TaxClass
			if strings.TrimSpace(class) == "" {
				class = p.TaxClass
			}
			vt := v.TaxStatus == "taxable"
			v.Price = conv(v.Price, class, vt)
			v.RegularPrice = convPtr(v.RegularPrice, class, vt)
			v.SalePrice = convPtr(v.SalePrice, class, vt)
		}
	}
}
