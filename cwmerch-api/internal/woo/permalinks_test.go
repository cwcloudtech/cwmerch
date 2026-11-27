package woo

import "testing"

func TestProductPathAndRoutes(t *testing.T) {
	def := Permalinks{ProductBase: "product", CategoryBase: "product-category", TagBase: "product-tag", ShopBase: "shop"}
	if p := def.productPath("hoodie", nil, nil); p != "/product/hoodie" {
		t.Errorf("default path %s", p)
	}
	fr := Permalinks{ProductBase: "produit", CategoryBase: "categorie-produit", TagBase: "etiquette-produit", ShopBase: "boutique"}
	cases := map[string][2]string{
		"/":                                {"home", ""},
		"/boutique":                        {"shop", ""},
		"/produit/bouquet-de-roses":        {"product", "bouquet-de-roses"},
		"/categorie-produit/fleurs/roses/": {"category", "fleurs/roses"},
		"/etiquette-produit/ete":           {"tag", "ete"},
		"/my-account":                      {"", ""},
	}
	for path, want := range cases {
		if k, r := fr.RouteKind(path); k != want[0] || r != want[1] {
			t.Errorf("RouteKind(%s) = %s %s, want %v", path, k, r, want)
		}
	}
	withCat := Permalinks{ProductBase: "shop/%product_cat%", CategoryBase: "product-category", TagBase: "product-tag", ShopBase: "shop", ProductCategoryInPath: true}
	cats := []TermRef{{ID: 9, Slug: "hoodies"}, {ID: 4, Slug: "clothing"}}
	paths := map[uint64]string{4: "clothing", 9: "clothing/hoodies"}
	if p := withCat.productPath("hoodie", cats, paths); p != "/shop/clothing/hoodie" {
		t.Errorf("%%product_cat%% path %s (lowest category id wins)", p)
	}
	if p := withCat.productPath("mug", nil, paths); p != "/shop/uncategorized/mug" {
		t.Errorf("uncategorized path %s", p)
	}
	if k, r := withCat.RouteKind("/shop/clothing/hoodies/hoodie"); k != "product" || r != "hoodie" {
		t.Errorf("product with category path: %s %s", k, r)
	}
	if k, _ := withCat.RouteKind("/shop"); k != "shop" {
		t.Errorf("shop page: %s", k)
	}
}
