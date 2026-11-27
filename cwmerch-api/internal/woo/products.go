package woo

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"cwmerch-api/internal/db"
	"cwmerch-api/internal/php"
)

// ProductAttribute is an attribute on a product: global (id > 0, slug pa_x,
// options are term names) or custom (id 0, options are free values).
type ProductAttribute struct {
	ID        uint64   `json:"id"`
	Name      string   `json:"name"`
	Slug      string   `json:"slug"`
	Position  int      `json:"position"`
	Visible   bool     `json:"visible"`
	Variation bool     `json:"variation"`
	Options   []string `json:"options"`
}

// AttributeValue is one attribute choice (variation attributes, defaults).
// An empty Option on a variation means "any".
type AttributeValue struct {
	Slug   string `json:"slug"`
	Name   string `json:"name,omitempty"`
	Option string `json:"option"`
}

// Download is a downloadable file of a product.
type Download struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	File string `json:"file"`
}

// Variation is a variable product's variation.
type Variation struct {
	ID             uint64           `json:"id"`
	ParentID       uint64           `json:"parentId"`
	SKU            string           `json:"sku"`
	Description    string           `json:"description"`
	Price          float64          `json:"price"`
	RegularPrice   *float64         `json:"regularPrice"`
	SalePrice      *float64         `json:"salePrice"`
	OnSale         bool             `json:"onSale"`
	DateOnSaleFrom *time.Time       `json:"dateOnSaleFrom"`
	DateOnSaleTo   *time.Time       `json:"dateOnSaleTo"`
	Status         string           `json:"status"`
	Virtual        bool             `json:"virtual"`
	Downloadable   bool             `json:"downloadable"`
	Downloads      []Download       `json:"downloads"`
	DownloadLimit  int64            `json:"downloadLimit"`
	DownloadExpiry int64            `json:"downloadExpiry"`
	ManageStock    bool             `json:"manageStock"`
	StockQuantity  *int64           `json:"stockQuantity"`
	StockStatus    string           `json:"stockStatus"`
	Backorders     string           `json:"backorders"`
	Weight         string           `json:"weight"`
	Length         string           `json:"length"`
	Width          string           `json:"width"`
	Height         string           `json:"height"`
	ShippingClass  string           `json:"shippingClass"`
	TaxStatus      string           `json:"taxStatus"`
	TaxClass       string           `json:"taxClass"`
	Image          *Image           `json:"image"`
	Attributes     []AttributeValue `json:"attributes"`
	MenuOrder      int              `json:"menuOrder"`
	// manageStockParent: "parent" in _manage_stock means the parent's stock is used.
	stockFromParent bool
}

// Product is a WooCommerce product.
type Product struct {
	ID                uint64             `json:"id"`
	Name              string             `json:"name"`
	Slug              string             `json:"slug"`
	Permalink         string             `json:"permalink"`
	Type              string             `json:"type"`
	Status            string             `json:"status"`
	Featured          bool               `json:"featured"`
	CatalogVisibility string             `json:"catalogVisibility"`
	Description       string             `json:"description"`
	ShortDescription  string             `json:"shortDescription"`
	SKU               string             `json:"sku"`
	Price             float64            `json:"price"`
	RegularPrice      *float64           `json:"regularPrice"`
	SalePrice         *float64           `json:"salePrice"`
	DateOnSaleFrom    *time.Time         `json:"dateOnSaleFrom"`
	DateOnSaleTo      *time.Time         `json:"dateOnSaleTo"`
	OnSale            bool               `json:"onSale"`
	PriceMin          float64            `json:"priceMin"`
	PriceMax          float64            `json:"priceMax"`
	TaxStatus         string             `json:"taxStatus"`
	TaxClass          string             `json:"taxClass"`
	ManageStock       bool               `json:"manageStock"`
	StockQuantity     *int64             `json:"stockQuantity"`
	StockStatus       string             `json:"stockStatus"`
	Backorders        string             `json:"backorders"`
	LowStockAmount    *int64             `json:"lowStockAmount"`
	SoldIndividually  bool               `json:"soldIndividually"`
	Weight            string             `json:"weight"`
	Length            string             `json:"length"`
	Width             string             `json:"width"`
	Height            string             `json:"height"`
	ShippingClass     string             `json:"shippingClass"`
	ShippingClassID   uint64             `json:"shippingClassId"`
	Virtual           bool               `json:"virtual"`
	Downloadable      bool               `json:"downloadable"`
	Downloads         []Download         `json:"downloads"`
	DownloadLimit     int64              `json:"downloadLimit"`
	DownloadExpiry    int64              `json:"downloadExpiry"`
	ExternalURL       string             `json:"externalUrl"`
	ButtonText        string             `json:"buttonText"`
	PurchaseNote      string             `json:"purchaseNote"`
	MenuOrder         int                `json:"menuOrder"`
	ReviewsAllowed    bool               `json:"reviewsAllowed"`
	AverageRating     float64            `json:"averageRating"`
	RatingCount       int64              `json:"ratingCount"`
	TotalSales        int64              `json:"totalSales"`
	Categories        []TermRef          `json:"categories"`
	Tags              []TermRef          `json:"tags"`
	Images            []Image            `json:"images"`
	Attributes        []ProductAttribute `json:"attributes"`
	DefaultAttributes []AttributeValue   `json:"defaultAttributes"`
	Variations        []Variation        `json:"variations,omitempty"`
	VariationIDs      []uint64           `json:"variationIds"`
	UpsellIDs         []uint64           `json:"upsellIds"`
	CrossSellIDs      []uint64           `json:"crossSellIds"`
	GroupedProducts   []uint64           `json:"groupedProducts"`
	Related           []uint64           `json:"related,omitempty"`
	CreatedAt         time.Time          `json:"createdAt"`
	UpdatedAt         time.Time          `json:"updatedAt"`
}

type postRow struct {
	ID          uint64
	Title       string
	Slug        string
	Status      string
	Content     string
	Excerpt     string
	MenuOrder   int
	Comment     string
	Parent      uint64
	Type        string
	Created     time.Time
	Modified    time.Time
	CommentsCnt int64
}

const postCols = `p.ID, p.post_title, p.post_name, p.post_status, p.post_content, p.post_excerpt, p.menu_order,
	p.comment_status, p.post_parent, p.post_type, p.post_date_gmt, p.post_modified_gmt, p.comment_count`

func scanPost(rows interface{ Scan(...any) error }) (postRow, error) {
	var r postRow
	var created, modified sql.NullTime
	err := rows.Scan(&r.ID, &r.Title, &r.Slug, &r.Status, &r.Content, &r.Excerpt, &r.MenuOrder,
		&r.Comment, &r.Parent, &r.Type, &created, &modified, &r.CommentsCnt)
	r.Created, r.Modified = created.Time, modified.Time
	return r, err
}

func (s *Store) posts(ctx context.Context, q db.Querier, where string, args ...any) ([]postRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+postCols+` FROM `+s.T("posts")+` p WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []postRow{}
	for rows.Next() {
		r, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func unixPtr(v string) *time.Time {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || n == 0 {
		return nil
	}
	t := time.Unix(n, 0).UTC()
	return &t
}

// activePrice is the price a customer pays now, honouring scheduled sales.
func activePrice(regular, sale *float64, from, to *time.Time, at time.Time) (float64, bool) {
	saleActive := sale != nil && (regular == nil || *sale < *regular || *regular == 0) &&
		(from == nil || !at.Before(*from)) && (to == nil || at.Before(*to))
	switch {
	case saleActive:
		return *sale, regular != nil && *sale < *regular
	case regular != nil:
		return *regular, false
	}
	return 0, false
}

func downloadsFrom(serialized string) []Download {
	out := []Download{}
	arr := php.UnmarshalArray(serialized)
	for _, k := range arr.Keys {
		d, ok := arr.Get(k).(*php.Array)
		if !ok {
			continue
		}
		id := d.String("id")
		if id == "" {
			id = php.ToString(k)
		}
		out = append(out, Download{ID: id, Name: d.String("name"), File: d.String("file")})
	}
	return out
}

func downloadsSerialized(downloads []Download) string {
	arr := php.NewArray()
	for _, d := range downloads {
		id := d.ID
		if id == "" {
			id = uuid.NewString()
		}
		arr.Set(id, php.NewArray().Set("id", id).Set("name", d.Name).Set("file", d.File).Set("enabled", true))
	}
	return php.Marshal(arr)
}

// hydrate turns post rows into products (no variations unless withVariations).
func (s *Store) hydrate(ctx context.Context, rows []postRow, withVariations bool) ([]Product, error) {
	if len(rows) == 0 {
		return []Product{}, nil
	}
	ids := make([]uint64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	metas, err := s.postMetas(ctx, s.DB, ids)
	if err != nil {
		return nil, err
	}
	terms, err := s.objectTerms(ctx, s.DB, ids, TaxCategory, TaxTag, TaxType, TaxVisibility, TaxShippingClass)
	if err != nil {
		return nil, err
	}
	imageIDs := []uint64{}
	for _, m := range metas {
		imageIDs = append(imageIDs, ParseID(m["_thumbnail_id"]))
		for _, g := range strings.Split(m["_product_image_gallery"], ",") {
			imageIDs = append(imageIDs, ParseID(g))
		}
	}
	images, err := s.images(ctx, imageIDs)
	if err != nil {
		return nil, err
	}
	attrs, err := s.Attributes(ctx)
	if err != nil {
		return nil, err
	}
	attrBySlug := map[string]Attribute{}
	for _, a := range attrs {
		attrBySlug[a.Taxonomy()] = a
	}
	// attribute terms per product for global attributes
	attrTax := []string{}
	for t := range attrBySlug {
		attrTax = append(attrTax, t)
	}
	attrTerms := map[uint64]map[string][]TermRef{}
	if len(attrTax) > 0 {
		if attrTerms, err = s.objectTerms(ctx, s.DB, ids, attrTax...); err != nil {
			return nil, err
		}
	}
	at := time.Now().UTC()
	out := make([]Product, 0, len(rows))
	children := map[uint64][]uint64{}
	if children, err = s.variationIDs(ctx, ids); err != nil {
		return nil, err
	}
	for _, r := range rows {
		m := metas[r.ID]
		t := terms[r.ID]
		p := Product{
			ID: r.ID, Name: r.Title, Slug: r.Slug, Status: r.Status, Description: r.Content, ShortDescription: r.Excerpt,
			MenuOrder: r.MenuOrder, ReviewsAllowed: r.Comment == "open", CreatedAt: r.Created, UpdatedAt: r.Modified,
			SKU: m["_sku"], RegularPrice: m.FloatPtr("_regular_price"), SalePrice: m.FloatPtr("_sale_price"),
			DateOnSaleFrom: unixPtr(m["_sale_price_dates_from"]), DateOnSaleTo: unixPtr(m["_sale_price_dates_to"]),
			TaxStatus: orDefault(m["_tax_status"], "taxable"), TaxClass: m["_tax_class"],
			ManageStock: m.Bool("_manage_stock"), StockQuantity: m.IntPtr("_stock"), StockStatus: orDefault(m["_stock_status"], "instock"),
			Backorders: orDefault(m["_backorders"], "no"), LowStockAmount: m.IntPtr("_low_stock_amount"),
			SoldIndividually: m.Bool("_sold_individually"), Weight: m["_weight"], Length: m["_length"], Width: m["_width"], Height: m["_height"],
			Virtual: m.Bool("_virtual"), Downloadable: m.Bool("_downloadable"), Downloads: downloadsFrom(m["_downloadable_files"]),
			DownloadLimit: intOr(m["_download_limit"], -1), DownloadExpiry: intOr(m["_download_expiry"], -1),
			ExternalURL: m["_product_url"], ButtonText: m["_button_text"], PurchaseNote: m["_purchase_note"],
			AverageRating: m.Float("_wc_average_rating"), RatingCount: m.Int("_wc_review_count"), TotalSales: m.Int("total_sales"),
			Categories: orEmpty(t[TaxCategory]), Tags: orEmpty(t[TaxTag]), Images: []Image{},
			UpsellIDs: idsFromSerialized(m["_upsell_ids"]), CrossSellIDs: idsFromSerialized(m["_crosssell_ids"]),
			GroupedProducts: idsFromSerialized(m["_children"]), VariationIDs: orEmptyIDs(children[r.ID]),
			Attributes: []ProductAttribute{}, DefaultAttributes: []AttributeValue{},
		}
		if !p.ManageStock {
			p.StockQuantity = nil
		}
		p.Type = "simple"
		if len(t[TaxType]) > 0 {
			p.Type = t[TaxType][0].Slug
		}
		if sc := t[TaxShippingClass]; len(sc) > 0 {
			p.ShippingClass, p.ShippingClassID = sc[0].Slug, sc[0].ID
		}
		vis := map[string]bool{}
		for _, v := range t[TaxVisibility] {
			vis[v.Slug] = true
		}
		p.Featured = vis["featured"]
		switch {
		case vis["exclude-from-catalog"] && vis["exclude-from-search"]:
			p.CatalogVisibility = "hidden"
		case vis["exclude-from-catalog"]:
			p.CatalogVisibility = "search"
		case vis["exclude-from-search"]:
			p.CatalogVisibility = "catalog"
		default:
			p.CatalogVisibility = "visible"
		}
		if img, ok := images[ParseID(m["_thumbnail_id"])]; ok {
			p.Images = append(p.Images, img)
		}
		for _, g := range strings.Split(m["_product_image_gallery"], ",") {
			if img, ok := images[ParseID(g)]; ok {
				p.Images = append(p.Images, img)
			}
		}
		p.Attributes = s.decodeAttributes(m["_product_attributes"], attrBySlug, attrTerms[r.ID])
		def := m.Array("_default_attributes")
		for _, k := range def.Keys {
			p.DefaultAttributes = append(p.DefaultAttributes, AttributeValue{Slug: php.ToString(k), Option: def.String(k)})
		}
		p.Price, p.OnSale = activePrice(p.RegularPrice, p.SalePrice, p.DateOnSaleFrom, p.DateOnSaleTo, at)
		if p.RegularPrice == nil && p.SalePrice == nil {
			p.Price = m.Float("_price")
		}
		p.PriceMin, p.PriceMax = p.Price, p.Price
		p.Permalink = s.SiteURL + "/product/" + p.Slug
		out = append(out, p)
	}
	// variable products: prices/stock come from their variations
	for i := range out {
		if out[i].Type != "variable" || len(out[i].VariationIDs) == 0 {
			continue
		}
		vars, err := s.Variations(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		applyVariationSummary(&out[i], vars)
		if withVariations {
			out[i].Variations = vars
		}
	}
	return out, nil
}

func applyVariationSummary(p *Product, vars []Variation) {
	first := true
	onSale := false
	for _, v := range vars {
		if v.Status != "publish" {
			continue
		}
		if first || v.Price < p.PriceMin {
			p.PriceMin = v.Price
		}
		if first || v.Price > p.PriceMax {
			p.PriceMax = v.Price
		}
		first = false
		onSale = onSale || v.OnSale
	}
	if !first {
		p.Price = p.PriceMin
	}
	p.OnSale = onSale
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func intOr(v string, def int64) int64 {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return int64(parseFloat(v))
}

func orEmpty(t []TermRef) []TermRef {
	if t == nil {
		return []TermRef{}
	}
	return t
}

func orEmptyIDs(ids []uint64) []uint64 {
	if ids == nil {
		return []uint64{}
	}
	return ids
}

// decodeAttributes reads _product_attributes (PHP array keyed by sanitized name).
func (s *Store) decodeAttributes(serialized string, attrBySlug map[string]Attribute, terms map[string][]TermRef) []ProductAttribute {
	out := []ProductAttribute{}
	arr := php.UnmarshalArray(serialized)
	for _, k := range arr.Keys {
		a, ok := arr.Get(k).(*php.Array)
		if !ok {
			continue
		}
		pa := ProductAttribute{
			Name: a.String("name"), Slug: a.String("name"), Position: int(parseFloat(a.String("position"))),
			Visible: a.String("is_visible") == "1", Variation: a.String("is_variation") == "1", Options: []string{},
		}
		if a.String("is_taxonomy") == "1" {
			if g, ok := attrBySlug[pa.Slug]; ok {
				pa.ID, pa.Name = g.ID, g.Name
			}
			for _, t := range terms[pa.Slug] {
				pa.Options = append(pa.Options, t.Name)
			}
		} else {
			for _, v := range strings.Split(a.String("value"), "|") {
				if v = strings.TrimSpace(v); v != "" {
					pa.Options = append(pa.Options, v)
				}
			}
		}
		out = append(out, pa)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out
}

func (s *Store) variationIDs(ctx context.Context, parents []uint64) (map[uint64][]uint64, error) {
	out := map[uint64][]uint64{}
	if len(parents) == 0 {
		return out, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT ID, post_parent FROM `+s.T("posts")+` WHERE post_type = 'product_variation'
		AND post_status IN ('publish', 'private') AND post_parent IN (`+inList(parents)+`) ORDER BY menu_order, ID`)
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

// Variations is every variation of a product.
func (s *Store) Variations(ctx context.Context, parentID uint64) ([]Variation, error) {
	rows, err := s.posts(ctx, s.DB, `p.post_type = 'product_variation' AND p.post_parent = ? AND p.post_status IN ('publish', 'private') ORDER BY p.menu_order, p.ID`, parentID)
	if err != nil {
		return nil, err
	}
	return s.hydrateVariations(ctx, rows)
}

// Variation is one variation by id.
func (s *Store) Variation(ctx context.Context, id uint64) (Variation, error) {
	rows, err := s.posts(ctx, s.DB, `p.post_type = 'product_variation' AND p.ID = ?`, id)
	if err != nil {
		return Variation{}, err
	}
	vars, err := s.hydrateVariations(ctx, rows)
	if err != nil {
		return Variation{}, err
	}
	if len(vars) == 0 {
		return Variation{}, ErrNotFound
	}
	return vars[0], nil
}

func (s *Store) hydrateVariations(ctx context.Context, rows []postRow) ([]Variation, error) {
	out := []Variation{}
	if len(rows) == 0 {
		return out, nil
	}
	ids := []uint64{}
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	metas, err := s.postMetas(ctx, s.DB, ids)
	if err != nil {
		return nil, err
	}
	imgIDs := []uint64{}
	for _, m := range metas {
		imgIDs = append(imgIDs, ParseID(m["_thumbnail_id"]))
	}
	images, err := s.images(ctx, imgIDs)
	if err != nil {
		return nil, err
	}
	terms, err := s.objectTerms(ctx, s.DB, ids, TaxShippingClass)
	if err != nil {
		return nil, err
	}
	// attribute term names: variation meta stores the term slug
	termNames := map[string]string{}
	attrs, _ := s.Attributes(ctx)
	attrName := map[string]string{}
	for _, a := range attrs {
		attrName[a.Taxonomy()] = a.Name
		if ts, err := s.Terms(ctx, a.Taxonomy()); err == nil {
			for _, t := range ts {
				termNames[a.Taxonomy()+"|"+t.Slug] = t.Name
			}
		}
	}
	at := time.Now().UTC()
	for _, r := range rows {
		m := metas[r.ID]
		v := Variation{
			ID: r.ID, ParentID: r.Parent, SKU: m["_sku"], Description: m["_variation_description"],
			RegularPrice: m.FloatPtr("_regular_price"), SalePrice: m.FloatPtr("_sale_price"),
			DateOnSaleFrom: unixPtr(m["_sale_price_dates_from"]), DateOnSaleTo: unixPtr(m["_sale_price_dates_to"]),
			Status: r.Status, Virtual: m.Bool("_virtual"), Downloadable: m.Bool("_downloadable"),
			Downloads: downloadsFrom(m["_downloadable_files"]), DownloadLimit: intOr(m["_download_limit"], -1), DownloadExpiry: intOr(m["_download_expiry"], -1),
			ManageStock: m.Bool("_manage_stock"), StockQuantity: m.IntPtr("_stock"), StockStatus: orDefault(m["_stock_status"], "instock"),
			Backorders: orDefault(m["_backorders"], "no"), Weight: m["_weight"], Length: m["_length"], Width: m["_width"], Height: m["_height"],
			TaxStatus: orDefault(m["_tax_status"], "taxable"), TaxClass: m["_tax_class"], MenuOrder: r.MenuOrder,
			Attributes: []AttributeValue{}, stockFromParent: m["_manage_stock"] == "parent",
		}
		if v.TaxClass == "parent" {
			v.TaxClass = ""
		}
		if !v.ManageStock {
			v.StockQuantity = nil
		}
		if sc := terms[r.ID][TaxShippingClass]; len(sc) > 0 {
			v.ShippingClass = sc[0].Slug
		}
		if img, ok := images[ParseID(m["_thumbnail_id"])]; ok {
			v.Image = &img
		}
		keys := []string{}
		for k := range m {
			if strings.HasPrefix(k, "attribute_") {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			slug := strings.TrimPrefix(k, "attribute_")
			opt := m[k]
			name := slug
			if n, ok := attrName[slug]; ok {
				name = n
				if tn, ok := termNames[slug+"|"+opt]; ok {
					opt = tn
				}
			}
			v.Attributes = append(v.Attributes, AttributeValue{Slug: slug, Name: name, Option: opt})
		}
		v.Price, v.OnSale = activePrice(v.RegularPrice, v.SalePrice, v.DateOnSaleFrom, v.DateOnSaleTo, at)
		out = append(out, v)
	}
	return out, nil
}

// ------------------------------------------------------------------ queries

// ProductQuery filters a product list.
type ProductQuery struct {
	Search      string
	Category    string
	Tag         string
	MinPrice    *float64
	MaxPrice    *float64
	OnSale      bool
	Featured    bool
	Status      string
	Type        string
	StockStatus string
	OrderBy     string
	Order       string
	Include     []uint64
	Exclude     []uint64
	Page        int
	PerPage     int
	// Public restricts to what a visitor may see (published, visible in the catalog).
	Public bool
}

func (s *Store) termAndChildren(ctx context.Context, taxonomy, ref string) ([]uint64, error) {
	var root Term
	var err error
	if id := ParseID(ref); id != 0 {
		root, err = s.Term(ctx, taxonomy, id)
	} else {
		root, err = s.TermBySlug(ctx, taxonomy, ref)
	}
	if err != nil {
		return nil, err
	}
	all, err := s.Terms(ctx, taxonomy)
	if err != nil {
		return nil, err
	}
	ids := []uint64{root.ID}
	for changed := true; changed; {
		changed = false
		for _, t := range all {
			if containsID(ids, t.Parent) && !containsID(ids, t.ID) {
				ids = append(ids, t.ID)
				changed = true
			}
		}
	}
	return ids, nil
}

func containsID(ids []uint64, id uint64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// Products lists products.
func (s *Store) Products(ctx context.Context, q ProductQuery) (Paged[Product], error) {
	page, perPage, offset := Page(q.Page, q.PerPage)
	where := []string{"p.post_type = 'product'"}
	args := []any{}
	visTerms := func(slugs ...string) string {
		return `p.ID IN (SELECT tr.object_id FROM ` + s.T("term_relationships") + ` tr JOIN ` + s.T("term_taxonomy") +
			` tt ON tt.term_taxonomy_id = tr.term_taxonomy_id JOIN ` + s.T("terms") + ` t ON t.term_id = tt.term_id
			WHERE tt.taxonomy = 'product_visibility' AND t.slug IN ('` + strings.Join(slugs, "','") + `'))`
	}
	if q.Public {
		where = append(where, "p.post_status = 'publish'")
		if q.Search != "" {
			where = append(where, "NOT "+visTerms("exclude-from-search"))
		} else {
			where = append(where, "NOT "+visTerms("exclude-from-catalog"))
		}
		if s.OptionBool(ctx, "woocommerce_hide_out_of_stock_items", false) {
			where = append(where, "NOT "+visTerms("outofstock"))
		}
	} else if q.Status != "" && q.Status != "any" {
		where = append(where, "p.post_status = ?")
		args = append(args, q.Status)
	} else {
		where = append(where, "p.post_status IN ('publish', 'draft', 'pending', 'private', 'future')")
	}
	if q.Search != "" {
		like := "%" + q.Search + "%"
		where = append(where, `(p.post_title LIKE ? OR p.post_excerpt LIKE ? OR p.post_content LIKE ? OR EXISTS (SELECT 1 FROM `+s.T("postmeta")+
			` sm WHERE sm.post_id = p.ID AND sm.meta_key = '_sku' AND sm.meta_value LIKE ?) OR EXISTS (SELECT 1 FROM `+s.T("posts")+
			` v JOIN `+s.T("postmeta")+` vm ON vm.post_id = v.ID AND vm.meta_key = '_sku' WHERE v.post_parent = p.ID AND vm.meta_value LIKE ?))`)
		args = append(args, like, like, like, like, like)
	}
	taxFilter := func(taxonomy, ref string) error {
		ids, err := s.termAndChildren(ctx, taxonomy, ref)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				where = append(where, "1=0")
				return nil
			}
			return err
		}
		where = append(where, `p.ID IN (SELECT tr.object_id FROM `+s.T("term_relationships")+` tr JOIN `+s.T("term_taxonomy")+
			` tt ON tt.term_taxonomy_id = tr.term_taxonomy_id WHERE tt.taxonomy = ? AND tt.term_id IN (`+inList(ids)+`))`)
		args = append(args, taxonomy)
		return nil
	}
	if q.Category != "" {
		if err := taxFilter(TaxCategory, q.Category); err != nil {
			return Paged[Product]{}, err
		}
	}
	if q.Tag != "" {
		if err := taxFilter(TaxTag, q.Tag); err != nil {
			return Paged[Product]{}, err
		}
	}
	if q.Type != "" {
		if err := taxFilter(TaxType, q.Type); err != nil {
			return Paged[Product]{}, err
		}
	}
	if q.Featured {
		where = append(where, visTerms("featured"))
	}
	if q.StockStatus != "" {
		where = append(where, "COALESCE(l.stock_status, 'instock') = ?")
		args = append(args, q.StockStatus)
	}
	if q.OnSale {
		where = append(where, "l.onsale = 1")
	}
	if q.MinPrice != nil {
		where = append(where, "COALESCE(l.max_price, 0) >= ?")
		args = append(args, *q.MinPrice)
	}
	if q.MaxPrice != nil {
		where = append(where, "COALESCE(l.min_price, 0) <= ?")
		args = append(args, *q.MaxPrice)
	}
	if len(q.Include) > 0 {
		where = append(where, "p.ID IN ("+inList(q.Include)+")")
	}
	if len(q.Exclude) > 0 {
		where = append(where, "p.ID NOT IN ("+inList(q.Exclude)+")")
	}
	desc := strings.EqualFold(q.Order, "desc")
	dir := func(def bool) string {
		if q.Order == "" {
			desc = def
		}
		if desc {
			return "DESC"
		}
		return "ASC"
	}
	var order string
	switch q.OrderBy {
	case "date":
		order = "p.post_date_gmt " + dir(true) + ", p.ID " + dir(true)
	case "price":
		order = "COALESCE(l.min_price, 0) " + dir(false) + ", p.ID"
	case "price-desc":
		order = "COALESCE(l.max_price, 0) DESC, p.ID"
	case "popularity":
		order = "COALESCE(l.total_sales, 0) DESC, p.ID DESC"
	case "rating":
		order = "COALESCE(l.average_rating, 0) DESC, COALESCE(l.rating_count, 0) DESC, p.ID DESC"
	case "title":
		order = "p.post_title " + dir(false)
	case "id":
		order = "p.ID " + dir(true)
	case "include":
		if len(q.Include) > 0 {
			order = "FIELD(p.ID, " + inList(q.Include) + ")"
			break
		}
		fallthrough
	default:
		if q.Public {
			order = "p.menu_order ASC, p.post_title ASC"
		} else {
			order = "p.post_date_gmt DESC, p.ID DESC"
		}
	}
	from := ` FROM ` + s.T("posts") + ` p LEFT JOIN ` + s.T("wc_product_meta_lookup") + ` l ON l.product_id = p.ID WHERE ` + strings.Join(where, " AND ")
	var total int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*)`+from, args...).Scan(&total); err != nil {
		return Paged[Product]{}, err
	}
	listed, err := s.DB.QueryContext(ctx, `SELECT `+postCols+from+` ORDER BY `+order+` LIMIT ? OFFSET ?`, append(args, perPage, offset)...)
	if err != nil {
		return Paged[Product]{}, err
	}
	rows := []postRow{}
	for listed.Next() {
		r, err := scanPost(listed)
		if err != nil {
			listed.Close()
			return Paged[Product]{}, err
		}
		rows = append(rows, r)
	}
	listed.Close()
	products, err := s.hydrate(ctx, rows, false)
	if err != nil {
		return Paged[Product]{}, err
	}
	return NewPaged(products, total, page, perPage), nil
}

// Product is one product by id (any status), with variations.
func (s *Store) Product(ctx context.Context, id uint64) (Product, error) {
	rows, err := s.posts(ctx, s.DB, `p.post_type = 'product' AND p.ID = ?`, id)
	if err != nil {
		return Product{}, err
	}
	ps, err := s.hydrate(ctx, rows, true)
	if err != nil {
		return Product{}, err
	}
	if len(ps) == 0 {
		return Product{}, ErrNotFound
	}
	return ps[0], nil
}

// PublicProduct is a published product by id or slug, with variations and related ids.
func (s *Store) PublicProduct(ctx context.Context, ref string) (Product, error) {
	var rows []postRow
	var err error
	if id := ParseID(ref); id != 0 {
		rows, err = s.posts(ctx, s.DB, `p.post_type = 'product' AND p.post_status = 'publish' AND p.ID = ?`, id)
	} else {
		rows, err = s.posts(ctx, s.DB, `p.post_type = 'product' AND p.post_status = 'publish' AND p.post_name = ?`, ref)
	}
	if err != nil {
		return Product{}, err
	}
	ps, err := s.hydrate(ctx, rows, true)
	if err != nil {
		return Product{}, err
	}
	if len(ps) == 0 {
		return Product{}, ErrNotFound
	}
	p := ps[0]
	if p.CatalogVisibility == "hidden" {
		// Hidden products stay reachable by direct link in WooCommerce.
		p.Related = []uint64{}
		return p, nil
	}
	p.Related, err = s.relatedIDs(ctx, p, 4)
	return p, err
}

func (s *Store) relatedIDs(ctx context.Context, p Product, limit int) ([]uint64, error) {
	termIDs := []uint64{}
	for _, c := range p.Categories {
		termIDs = append(termIDs, c.ID)
	}
	for _, t := range p.Tags {
		termIDs = append(termIDs, t.ID)
	}
	out := []uint64{}
	if len(termIDs) == 0 {
		return out, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT p.ID FROM `+s.T("posts")+` p
		JOIN `+s.T("term_relationships")+` tr ON tr.object_id = p.ID
		JOIN `+s.T("term_taxonomy")+` tt ON tt.term_taxonomy_id = tr.term_taxonomy_id
		LEFT JOIN `+s.T("wc_product_meta_lookup")+` l ON l.product_id = p.ID
		WHERE p.post_type = 'product' AND p.post_status = 'publish' AND p.ID <> ?
		AND tt.taxonomy IN ('product_cat', 'product_tag') AND tt.term_id IN (`+inList(termIDs)+`)
		GROUP BY p.ID ORDER BY MAX(COALESCE(l.total_sales, 0)) DESC, p.ID DESC LIMIT ?`, p.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ProductsByIDs hydrates products in the given order (missing ids skipped).
func (s *Store) ProductsByIDs(ctx context.Context, ids []uint64, withVariations bool) ([]Product, error) {
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return []Product{}, nil
	}
	rows, err := s.posts(ctx, s.DB, `p.post_type = 'product' AND p.ID IN (`+inList(ids)+`)`)
	if err != nil {
		return nil, err
	}
	ps, err := s.hydrate(ctx, rows, withVariations)
	if err != nil {
		return nil, err
	}
	byID := map[uint64]Product{}
	for _, p := range ps {
		byID[p.ID] = p
	}
	out := []Product{}
	for _, id := range ids {
		if p, ok := byID[id]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}
