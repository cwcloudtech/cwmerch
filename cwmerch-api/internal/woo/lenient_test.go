package woo

import (
	"encoding/json"
	"testing"
)

func TestProductInputAcceptsNumbers(t *testing.T) {
	var p ProductInput
	if err := json.Unmarshal([]byte(`{"regularPrice":30,"salePrice":12.5,"sku":1234,"weight":"0.5","stockQuantity":7,"name":"Savon","lowStockAmount":null}`), &p); err != nil {
		t.Fatal(err)
	}
	if *p.RegularPrice != "30" || *p.SalePrice != "12.5" || *p.SKU != "1234" || *p.Weight != "0.5" || *p.StockQuantity != 7 || *p.Name != "Savon" || p.LowStockAmount != nil {
		t.Fatalf("unexpected input: %+v", p)
	}
	var v VariationInput
	if err := json.Unmarshal([]byte(`{"regularPrice":9.9}`), &v); err != nil || *v.RegularPrice != "9.9" {
		t.Fatalf("variation: %v %+v", err, v)
	}
}
