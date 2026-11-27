package woo

import (
	"context"
	"database/sql"
	"sort"
	"strings"

	"cwmerch-api/internal/php"
)

// Permalinks are WooCommerce's URL bases (option woocommerce_permalinks, set
// in WordPress → Settings → Permalinks), so product, category and tag pages
// have the same addresses on cwmerch as on WordPress.
type Permalinks struct {
	// ProductBase is the product URL base, without slashes: "product",
	// "produit", "shop" or "shop/%product_cat%".
	ProductBase string `json:"productBase"`
	// CategoryBase / TagBase / AttributeBase: "product-category", "categorie-produit"...
	CategoryBase  string `json:"categoryBase"`
	TagBase       string `json:"tagBase"`
	AttributeBase string `json:"attributeBase"`
	// ShopBase is the shop page's slug ("shop", "boutique").
	ShopBase string `json:"shopBase"`
	// ProductCategoryInPath: the product base holds %product_cat%.
	ProductCategoryInPath bool `json:"productCategoryInPath"`
}

// defaultBases are WooCommerce's slugs as its installer writes them in each
// language (_x('product', 'slug', 'woocommerce')...).
var defaultBases = map[string][3]string{
	"en": {"product", "product-category", "product-tag"},
	"fr": {"produit", "categorie-produit", "etiquette-produit"},
	"ar": {"product", "product-category", "product-tag"},
}

func trimSlashes(s string) string { return strings.Trim(strings.TrimSpace(s), "/") }

// Permalinks reads the bases (WooCommerce's localized defaults when unset).
func (s *Store) Permalinks(ctx context.Context) Permalinks {
	lang := "en"
	if strings.HasPrefix(strings.ToLower(s.Option(ctx, "WPLANG")), "fr") {
		lang = "fr"
	}
	def := defaultBases[lang]
	arr := s.OptionArray(ctx, "woocommerce_permalinks")
	p := Permalinks{ProductBase: trimSlashes(arr.String("product_base")), CategoryBase: trimSlashes(arr.String("category_base")),
		TagBase: trimSlashes(arr.String("tag_base")), AttributeBase: trimSlashes(arr.String("attribute_base"))}
	if p.ProductBase == "" {
		p.ProductBase = def[0]
	}
	if p.CategoryBase == "" {
		p.CategoryBase = def[1]
	}
	if p.TagBase == "" {
		p.TagBase = def[2]
	}
	p.ProductCategoryInPath = strings.Contains(p.ProductBase, "%product_cat%")
	p.ShopBase = "shop"
	if lang == "fr" {
		p.ShopBase = "boutique"
	}
	if id := ParseID(s.Option(ctx, "woocommerce_shop_page_id")); id != 0 {
		var slug sql.NullString
		if s.DB.QueryRowContext(ctx, `SELECT post_name FROM `+s.T("posts")+` WHERE ID = ? AND post_type = 'page'`, id).Scan(&slug) == nil && slug.String != "" {
			p.ShopBase = slug.String
		}
	}
	return p
}

// PermalinksInput edits the bases.
type PermalinksInput struct {
	ProductBase   string `json:"productBase"`
	CategoryBase  string `json:"categoryBase"`
	TagBase       string `json:"tagBase"`
	AttributeBase string `json:"attributeBase"`
}

// SavePermalinks writes woocommerce_permalinks the way WooCommerce does and
// drops WordPress' cached rewrite rules so WordPress serves the new URLs too.
func (s *Store) SavePermalinks(ctx context.Context, in PermalinksInput) (Permalinks, error) {
	clean := func(v string) string {
		parts := []string{}
		for _, seg := range strings.Split(trimSlashes(v), "/") {
			if seg == "%product_cat%" {
				parts = append(parts, seg)
			} else if sl := Slugify(seg); sl != "" {
				parts = append(parts, sl)
			}
		}
		return strings.Join(parts, "/")
	}
	pb, cb, tb, ab := clean(in.ProductBase), clean(in.CategoryBase), clean(in.TagBase), clean(in.AttributeBase)
	if pb == "" || cb == "" || tb == "" {
		return Permalinks{}, refuse(400, "errors.invalidPermalinks", "Product, category and tag bases are required")
	}
	if pb == cb || pb == tb || cb == tb {
		return Permalinks{}, refuse(400, "errors.invalidPermalinks", "Product, category and tag bases must differ")
	}
	arr := s.OptionArray(ctx, "woocommerce_permalinks")
	arr.Set("product_base", "/"+pb).Set("category_base", cb).Set("tag_base", tb).Set("attribute_base", ab)
	if !arr.Has("use_verbose_page_rules") {
		arr.Set("use_verbose_page_rules", false)
	}
	if err := s.SetOption(ctx, "woocommerce_permalinks", php.Marshal(arr)); err != nil {
		return Permalinks{}, err
	}
	_ = s.DeleteOption(ctx, "rewrite_rules") // WordPress rebuilds them on its next request
	s.ForgetOptions()
	return s.Permalinks(ctx), nil
}

// termPaths answers each term's hierarchical slug path ("parent/child").
func (s *Store) termPaths(ctx context.Context, taxonomy string) (map[uint64]string, map[uint64]uint64, error) {
	terms, err := s.Terms(ctx, taxonomy)
	if err != nil {
		return nil, nil, err
	}
	byID := map[uint64]Term{}
	for _, t := range terms {
		byID[t.ID] = t
	}
	paths := map[uint64]string{}
	parents := map[uint64]uint64{}
	for _, t := range terms {
		segs := []string{t.Slug}
		seen := map[uint64]bool{t.ID: true}
		for p := t.Parent; p != 0 && !seen[p]; {
			pt, ok := byID[p]
			if !ok {
				break
			}
			seen[p] = true
			segs = append([]string{pt.Slug}, segs...)
			p = pt.Parent
		}
		paths[t.ID] = strings.Join(segs, "/")
		parents[t.ID] = t.Parent
	}
	return paths, parents, nil
}

// CategoryPath is a category page's path ("/product-category/clothing/hoodies").
func (pl Permalinks) CategoryPath(termPath string) string {
	return "/" + pl.CategoryBase + "/" + termPath
}

// TagPath is a tag page's path.
func (pl Permalinks) TagPath(slug string) string { return "/" + pl.TagBase + "/" + slug }

// ShopPath is the shop page's path.
func (pl Permalinks) ShopPath() string { return "/" + pl.ShopBase }

// productPath builds a product's path like wc_product_post_type_link: the
// %product_cat% placeholder becomes the full path of the product's category
// with the lowest id (WooCommerce's choice), "uncategorized" when none.
func (pl Permalinks) productPath(slug string, cats []TermRef, catPaths map[uint64]string) string {
	base := pl.ProductBase
	if pl.ProductCategoryInPath {
		catPath := "uncategorized"
		if len(cats) > 0 {
			sorted := append([]TermRef{}, cats...)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
			if p, ok := catPaths[sorted[0].ID]; ok {
				catPath = p
			} else {
				catPath = sorted[0].Slug
			}
		}
		base = strings.ReplaceAll(base, "%product_cat%", catPath)
	}
	return "/" + base + "/" + slug
}

// RouteKind tells what a storefront path is: "home", "shop", "product",
// "category", "tag" or "" (anything else), with the slug or term path.
func (pl Permalinks) RouteKind(path string) (kind, ref string) {
	p := trimSlashes(path)
	if p == "" {
		return "home", ""
	}
	if p == trimSlashes(pl.ShopBase) {
		return "shop", ""
	}
	if rest, ok := strings.CutPrefix(p, pl.CategoryBase+"/"); ok && rest != "" {
		return "category", rest
	}
	if rest, ok := strings.CutPrefix(p, pl.TagBase+"/"); ok && rest != "" {
		return "tag", rest
	}
	prefix := pl.ProductBase
	if pl.ProductCategoryInPath {
		prefix = trimSlashes(strings.SplitN(pl.ProductBase, "%product_cat%", 2)[0])
	}
	if prefix == "" || strings.HasPrefix(p, prefix+"/") {
		segs := strings.Split(p, "/")
		if len(segs) >= 2 || prefix == "" {
			return "product", segs[len(segs)-1]
		}
	}
	return "", ""
}
