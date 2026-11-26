package woo

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"goocommerce-api/internal/db"
	"goocommerce-api/internal/php"
)

// ProductInput is a product create/update; nil fields keep their value.
type ProductInput struct {
	Name              *string             `json:"name"`
	Slug              *string             `json:"slug"`
	Type              *string             `json:"type"`
	Status            *string             `json:"status"`
	Featured          *bool               `json:"featured"`
	CatalogVisibility *string             `json:"catalogVisibility"`
	Description       *string             `json:"description"`
	ShortDescription  *string             `json:"shortDescription"`
	SKU               *string             `json:"sku"`
	RegularPrice      *string             `json:"regularPrice"`
	SalePrice         *string             `json:"salePrice"`
	DateOnSaleFrom    *string             `json:"dateOnSaleFrom"`
	DateOnSaleTo      *string             `json:"dateOnSaleTo"`
	TaxStatus         *string             `json:"taxStatus"`
	TaxClass          *string             `json:"taxClass"`
	ManageStock       *bool               `json:"manageStock"`
	StockQuantity     *int64              `json:"stockQuantity"`
	StockStatus       *string             `json:"stockStatus"`
	Backorders        *string             `json:"backorders"`
	LowStockAmount    *string             `json:"lowStockAmount"`
	SoldIndividually  *bool               `json:"soldIndividually"`
	Weight            *string             `json:"weight"`
	Length            *string             `json:"length"`
	Width             *string             `json:"width"`
	Height            *string             `json:"height"`
	ShippingClass     *string             `json:"shippingClass"`
	Virtual           *bool               `json:"virtual"`
	Downloadable      *bool               `json:"downloadable"`
	Downloads         *[]Download         `json:"downloads"`
	DownloadLimit     *int64              `json:"downloadLimit"`
	DownloadExpiry    *int64              `json:"downloadExpiry"`
	ExternalURL       *string             `json:"externalUrl"`
	ButtonText        *string             `json:"buttonText"`
	PurchaseNote      *string             `json:"purchaseNote"`
	MenuOrder         *int                `json:"menuOrder"`
	ReviewsAllowed    *bool               `json:"reviewsAllowed"`
	Categories        *[]TermRef          `json:"categories"`
	Tags              *[]TermRef          `json:"tags"`
	Images            *[]Image            `json:"images"`
	Attributes        *[]ProductAttribute `json:"attributes"`
	DefaultAttributes *[]AttributeValue   `json:"defaultAttributes"`
	UpsellIDs         *[]uint64           `json:"upsellIds"`
	CrossSellIDs      *[]uint64           `json:"crossSellIds"`
	GroupedProducts   *[]uint64           `json:"groupedProducts"`
}

// VariationInput is a variation create/update.
type VariationInput struct {
	SKU            *string           `json:"sku"`
	Description    *string           `json:"description"`
	RegularPrice   *string           `json:"regularPrice"`
	SalePrice      *string           `json:"salePrice"`
	DateOnSaleFrom *string           `json:"dateOnSaleFrom"`
	DateOnSaleTo   *string           `json:"dateOnSaleTo"`
	Status         *string           `json:"status"`
	Virtual        *bool             `json:"virtual"`
	Downloadable   *bool             `json:"downloadable"`
	Downloads      *[]Download       `json:"downloads"`
	DownloadLimit  *int64            `json:"downloadLimit"`
	DownloadExpiry *int64            `json:"downloadExpiry"`
	ManageStock    *bool             `json:"manageStock"`
	StockQuantity  *int64            `json:"stockQuantity"`
	StockStatus    *string           `json:"stockStatus"`
	Backorders     *string           `json:"backorders"`
	Weight         *string           `json:"weight"`
	Length         *string           `json:"length"`
	Width          *string           `json:"width"`
	Height         *string           `json:"height"`
	ShippingClass  *string           `json:"shippingClass"`
	TaxStatus      *string           `json:"taxStatus"`
	TaxClass       *string           `json:"taxClass"`
	Image          *Image            `json:"image"`
	Attributes     *[]AttributeValue `json:"attributes"`
	MenuOrder      *int              `json:"menuOrder"`
}

var validTypes = map[string]bool{"simple": true, "variable": true, "grouped": true, "external": true}
var validStatuses = map[string]bool{"publish": true, "draft": true, "pending": true, "private": true, "future": true}

func cleanPrice(v string) string {
	v = strings.TrimSpace(strings.ReplaceAll(v, ",", "."))
	if v == "" {
		return ""
	}
	if _, err := strconv.ParseFloat(v, 64); err != nil {
		return ""
	}
	return v
}

func parseDateUnix(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return strconv.FormatInt(t.Unix(), 10)
		}
	}
	return ""
}

func (s *Store) uniquePostSlug(ctx context.Context, q db.Querier, postType, slug string, exclude uint64) (string, error) {
	if slug == "" {
		slug = "product"
	}
	base := slug
	for i := 2; i < 10000; i++ {
		var n int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.T("posts")+` WHERE post_type = ? AND post_name = ? AND ID <> ?`,
			postType, slug, exclude).Scan(&n); err != nil {
			return "", err
		}
		if n == 0 {
			return slug, nil
		}
		slug = base + "-" + strconv.Itoa(i)
	}
	return slug, nil
}

func (s *Store) skuTaken(ctx context.Context, q db.Querier, sku string, exclude uint64) (bool, error) {
	if sku == "" {
		return false, nil
	}
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.T("postmeta")+` m JOIN `+s.T("posts")+` p ON p.ID = m.post_id
		WHERE m.meta_key = '_sku' AND m.meta_value = ? AND m.post_id <> ? AND p.post_status <> 'trash'
		AND p.post_type IN ('product', 'product_variation')`, sku, exclude).Scan(&n)
	return n > 0, err
}

// insertPost creates a posts row.
func (s *Store) insertPost(ctx context.Context, q db.Querier, postType, status, title, slug, content, excerpt string, parent, author uint64, menuOrder int, comment string) (uint64, error) {
	t := now()
	local := s.localTime(ctx, t)
	res, err := q.ExecContext(ctx, `INSERT INTO `+s.T("posts")+` (post_author, post_date, post_date_gmt, post_content, post_title, post_excerpt,
		post_status, comment_status, ping_status, post_password, post_name, to_ping, pinged, post_modified, post_modified_gmt,
		post_content_filtered, post_parent, guid, menu_order, post_type, post_mime_type, comment_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'closed', '', ?, '', '', ?, ?, '', ?, '', ?, ?, '', 0)`,
		author, local, t, content, title, excerpt, status, comment, slug, local, t, parent, menuOrder, postType)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	guid := s.SiteURL + "/?post_type=" + postType + "&p=" + strconv.FormatInt(id, 10)
	_, err = q.ExecContext(ctx, `UPDATE `+s.T("posts")+` SET guid = ? WHERE ID = ?`, guid, id)
	return uint64(id), err
}

func (s *Store) touchPost(ctx context.Context, q db.Querier, id uint64) error {
	t := now()
	_, err := q.ExecContext(ctx, `UPDATE `+s.T("posts")+` SET post_modified = ?, post_modified_gmt = ? WHERE ID = ?`, s.localTime(ctx, t), t, id)
	return err
}

// SaveProduct creates (id 0) or updates a product.
func (s *Store) SaveProduct(ctx context.Context, id uint64, in ProductInput, authorID uint64) (Product, error) {
	var current Product
	if id != 0 {
		var err error
		if current, err = s.Product(ctx, id); err != nil {
			return Product{}, err
		}
	} else {
		if in.Name == nil || strings.TrimSpace(*in.Name) == "" {
			return Product{}, refuse(400, "errors.nameRequired", "A product name is required")
		}
		current = Product{Type: "simple", Status: "publish", TaxStatus: "taxable", StockStatus: "instock", Backorders: "no",
			CatalogVisibility: "visible", ReviewsAllowed: true, DownloadLimit: -1, DownloadExpiry: -1}
	}
	typ := current.Type
	if in.Type != nil {
		if !validTypes[*in.Type] {
			return Product{}, refuse(400, "errors.invalidProductType", "Unknown product type")
		}
		typ = *in.Type
	}
	status := current.Status
	if in.Status != nil {
		if !validStatuses[*in.Status] {
			return Product{}, refuse(400, "errors.invalidStatus", "Unknown status")
		}
		status = *in.Status
	}
	if in.SKU != nil {
		taken, err := s.skuTaken(ctx, s.DB, strings.TrimSpace(*in.SKU), id)
		if err != nil {
			return Product{}, err
		}
		if taken {
			return Product{}, refuse(400, "errors.skuTaken", "This SKU is already used by another product")
		}
	}
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		name := str(in.Name, current.Name)
		content := str(in.Description, current.Description)
		excerpt := str(in.ShortDescription, current.ShortDescription)
		menu := current.MenuOrder
		if in.MenuOrder != nil {
			menu = *in.MenuOrder
		}
		comment := "closed"
		if boolOr(in.ReviewsAllowed, current.ReviewsAllowed) {
			comment = "open"
		}
		slug := current.Slug
		if in.Slug != nil && Slugify(*in.Slug) != "" {
			slug = Slugify(*in.Slug)
		} else if slug == "" {
			slug = Slugify(name)
		}
		var err error
		if slug, err = s.uniquePostSlug(ctx, tx, "product", slug, id); err != nil {
			return err
		}
		if id == 0 {
			if id, err = s.insertPost(ctx, tx, "product", status, name, slug, content, excerpt, 0, authorID, menu, comment); err != nil {
				return err
			}
		} else {
			t := now()
			if _, err := tx.ExecContext(ctx, `UPDATE `+s.T("posts")+` SET post_title = ?, post_name = ?, post_content = ?, post_excerpt = ?,
				post_status = ?, menu_order = ?, comment_status = ?, post_modified = ?, post_modified_gmt = ? WHERE ID = ?`,
				name, slug, content, excerpt, status, menu, comment, s.localTime(ctx, t), t, id); err != nil {
				return err
			}
		}
		meta := map[string]string{}
		set := func(k string, v *string) {
			if v != nil {
				meta[k] = strings.TrimSpace(*v)
			}
		}
		set("_sku", in.SKU)
		if in.RegularPrice != nil {
			meta["_regular_price"] = cleanPrice(*in.RegularPrice)
		}
		if in.SalePrice != nil {
			meta["_sale_price"] = cleanPrice(*in.SalePrice)
		}
		if in.DateOnSaleFrom != nil {
			meta["_sale_price_dates_from"] = parseDateUnix(*in.DateOnSaleFrom)
		}
		if in.DateOnSaleTo != nil {
			meta["_sale_price_dates_to"] = parseDateUnix(*in.DateOnSaleTo)
		}
		set("_tax_status", in.TaxStatus)
		set("_tax_class", in.TaxClass)
		if in.ManageStock != nil {
			meta["_manage_stock"] = yesNo(*in.ManageStock)
		}
		if in.StockQuantity != nil {
			meta["_stock"] = strconv.FormatInt(*in.StockQuantity, 10)
		}
		set("_stock_status", in.StockStatus)
		set("_backorders", in.Backorders)
		set("_low_stock_amount", in.LowStockAmount)
		if in.SoldIndividually != nil {
			meta["_sold_individually"] = yesNo(*in.SoldIndividually)
		}
		set("_weight", in.Weight)
		set("_length", in.Length)
		set("_width", in.Width)
		set("_height", in.Height)
		if in.Virtual != nil {
			meta["_virtual"] = yesNo(*in.Virtual)
		}
		if in.Downloadable != nil {
			meta["_downloadable"] = yesNo(*in.Downloadable)
		}
		if in.Downloads != nil {
			meta["_downloadable_files"] = downloadsSerialized(*in.Downloads)
		}
		if in.DownloadLimit != nil {
			meta["_download_limit"] = strconv.FormatInt(*in.DownloadLimit, 10)
		}
		if in.DownloadExpiry != nil {
			meta["_download_expiry"] = strconv.FormatInt(*in.DownloadExpiry, 10)
		}
		set("_product_url", in.ExternalURL)
		set("_button_text", in.ButtonText)
		set("_purchase_note", in.PurchaseNote)
		if in.UpsellIDs != nil {
			meta["_upsell_ids"] = serializeIDs(*in.UpsellIDs)
		}
		if in.CrossSellIDs != nil {
			meta["_crosssell_ids"] = serializeIDs(*in.CrossSellIDs)
		}
		if in.GroupedProducts != nil {
			meta["_children"] = serializeIDs(*in.GroupedProducts)
		}
		if in.Images != nil {
			imgs := *in.Images
			meta["_thumbnail_id"] = ""
			meta["_product_image_gallery"] = ""
			if len(imgs) > 0 {
				meta["_thumbnail_id"] = strconv.FormatUint(imgs[0].ID, 10)
				gallery := []string{}
				for _, img := range imgs[1:] {
					gallery = append(gallery, strconv.FormatUint(img.ID, 10))
				}
				meta["_product_image_gallery"] = strings.Join(gallery, ",")
			}
		}
		if in.DefaultAttributes != nil {
			def := php.NewArray()
			for _, d := range *in.DefaultAttributes {
				if d.Option != "" {
					def.Set(d.Slug, s.attributeOptionSlug(ctx, d.Slug, d.Option))
				}
			}
			meta["_default_attributes"] = php.Marshal(def)
		}
		// defaults WooCommerce writes on a new product
		if current.ID == 0 {
			for k, v := range map[string]string{"total_sales": "0", "_tax_status": "taxable", "_tax_class": "", "_manage_stock": "no",
				"_backorders": "no", "_sold_individually": "no", "_virtual": "no", "_downloadable": "no", "_download_limit": "-1",
				"_download_expiry": "-1", "_stock": "", "_stock_status": "instock", "_wc_average_rating": "0", "_wc_review_count": "0",
				"_wc_rating_count": "a:0:{}", "_product_version": "9.9.0"} {
				if _, ok := meta[k]; !ok {
					meta[k] = v
				}
			}
		}
		if typ == "external" {
			meta["_manage_stock"], meta["_stock_status"] = "no", "instock"
		}
		if err := s.setPostMetas(ctx, tx, id, meta); err != nil {
			return err
		}
		if in.Attributes != nil {
			if err := s.saveProductAttributes(ctx, tx, id, *in.Attributes); err != nil {
				return err
			}
		}
		// taxonomies
		_, typeTT, err := s.ensureTerm(ctx, tx, TaxType, typ, typ)
		if err != nil {
			return err
		}
		if err := s.setObjectTerms(ctx, tx, id, TaxType, []uint64{typeTT}); err != nil {
			return err
		}
		if in.Categories != nil || current.ID == 0 {
			cats := []uint64{}
			if in.Categories != nil {
				for _, c := range *in.Categories {
					cats = append(cats, c.ID)
				}
			}
			if len(cats) == 0 {
				if def := ParseID(s.Option(ctx, "default_product_cat")); def != 0 {
					cats = []uint64{def}
				}
			}
			tt, err := s.termTaxonomyIDs(ctx, tx, TaxCategory, cats)
			if err != nil {
				return err
			}
			if err := s.setObjectTerms(ctx, tx, id, TaxCategory, tt); err != nil {
				return err
			}
		}
		if in.Tags != nil {
			tt := []uint64{}
			for _, t := range *in.Tags {
				if t.ID != 0 {
					ids, err := s.termTaxonomyIDs(ctx, tx, TaxTag, []uint64{t.ID})
					if err != nil {
						return err
					}
					tt = append(tt, ids...)
				} else if strings.TrimSpace(t.Name) != "" {
					_, ttID, err := s.ensureTerm(ctx, tx, TaxTag, t.Name, Slugify(t.Name))
					if err != nil {
						return err
					}
					tt = append(tt, ttID)
				}
			}
			if err := s.setObjectTerms(ctx, tx, id, TaxTag, tt); err != nil {
				return err
			}
		}
		if in.ShippingClass != nil {
			if err := s.setShippingClass(ctx, tx, id, *in.ShippingClass); err != nil {
				return err
			}
		}
		return s.syncProduct(ctx, tx, id, in.Featured, in.CatalogVisibility)
	})
	if err != nil {
		return Product{}, err
	}
	if typ == "variable" {
		if err := s.syncVariableParent(ctx, id); err != nil {
			return Product{}, err
		}
	}
	return s.Product(ctx, id)
}

func str(v *string, def string) string {
	if v != nil {
		return *v
	}
	return def
}

func boolOr(v *bool, def bool) bool {
	if v != nil {
		return *v
	}
	return def
}

func (s *Store) setShippingClass(ctx context.Context, q db.Querier, objectID uint64, slug string) error {
	if slug == "" {
		return s.setObjectTerms(ctx, q, objectID, TaxShippingClass, nil)
	}
	t, err := s.TermBySlug(ctx, TaxShippingClass, slug)
	if err != nil {
		return refuse(400, "errors.unknownShippingClass", "Unknown shipping class")
	}
	tt, err := s.termTaxonomyIDs(ctx, q, TaxShippingClass, []uint64{t.ID})
	if err != nil {
		return err
	}
	return s.setObjectTerms(ctx, q, objectID, TaxShippingClass, tt)
}

// attributeOptionSlug turns a global attribute's term name into its slug
// (variation meta and defaults store slugs); custom values stay as they are.
func (s *Store) attributeOptionSlug(ctx context.Context, attrSlug, option string) string {
	if !strings.HasPrefix(attrSlug, "pa_") || option == "" {
		return option
	}
	terms, err := s.Terms(ctx, attrSlug)
	if err != nil {
		return option
	}
	for _, t := range terms {
		if t.Name == option || t.Slug == option {
			return t.Slug
		}
	}
	return Slugify(option)
}

// saveProductAttributes writes _product_attributes and the pa_* term relationships.
func (s *Store) saveProductAttributes(ctx context.Context, q db.Querier, id uint64, attrs []ProductAttribute) error {
	arr := php.NewArray()
	globals, err := s.Attributes(ctx)
	if err != nil {
		return err
	}
	byID := map[uint64]Attribute{}
	bySlug := map[string]Attribute{}
	for _, g := range globals {
		byID[g.ID] = g
		bySlug[g.Taxonomy()] = g
	}
	usedTax := map[string]bool{}
	for i, a := range attrs {
		g, isGlobal := byID[a.ID]
		if !isGlobal && a.ID == 0 {
			g, isGlobal = bySlug[a.Slug]
		}
		entry := php.NewArray()
		if isGlobal {
			tax := g.Taxonomy()
			usedTax[tax] = true
			tts := []uint64{}
			for _, opt := range a.Options {
				if strings.TrimSpace(opt) == "" {
					continue
				}
				_, tt, err := s.ensureTerm(ctx, q, tax, opt, Slugify(opt))
				if err != nil {
					return err
				}
				tts = append(tts, tt)
			}
			if err := s.setObjectTerms(ctx, q, id, tax, tts); err != nil {
				return err
			}
			entry.Set("name", tax).Set("value", "").Set("position", int64(i)).Set("is_visible", int64(boolInt(a.Visible))).
				Set("is_variation", int64(boolInt(a.Variation))).Set("is_taxonomy", int64(1))
			arr.Set(tax, entry)
		} else {
			name := strings.TrimSpace(a.Name)
			if name == "" {
				continue
			}
			entry.Set("name", name).Set("value", strings.Join(a.Options, " | ")).Set("position", int64(i)).
				Set("is_visible", int64(boolInt(a.Visible))).Set("is_variation", int64(boolInt(a.Variation))).Set("is_taxonomy", int64(0))
			arr.Set(Slugify(name), entry)
		}
	}
	// drop relationships of global attributes no longer used
	for _, g := range globals {
		if !usedTax[g.Taxonomy()] {
			if err := s.setObjectTerms(ctx, q, id, g.Taxonomy(), nil); err != nil {
				return err
			}
		}
	}
	return s.setPostMeta(ctx, q, id, "_product_attributes", php.Marshal(arr))
}

// stockStatusFor derives the stock status from quantity when stock is managed.
func stockStatusFor(manage bool, qty int64, backorders, current string, threshold int64) string {
	if !manage {
		if current == "" {
			return "instock"
		}
		return current
	}
	if qty > threshold {
		return "instock"
	}
	if backorders == "notify" || backorders == "yes" {
		return "onbackorder"
	}
	return "outofstock"
}

// syncProduct recomputes _price, stock status, visibility terms and the
// lookup row after a write (what WC_Product_Data_Store_CPT does on save).
func (s *Store) syncProduct(ctx context.Context, q db.Querier, id uint64, featured *bool, visibility *string) error {
	m, err := s.postMeta(ctx, q, id)
	if err != nil {
		return err
	}
	threshold := int64(s.OptionInt(ctx, "woocommerce_notify_no_stock_amount", 0))
	status := stockStatusFor(m.Bool("_manage_stock") && s.OptionBool(ctx, "woocommerce_manage_stock", true), m.Int("_stock"), m["_backorders"], m["_stock_status"], threshold)
	price, _ := activePrice(m.FloatPtr("_regular_price"), m.FloatPtr("_sale_price"), unixPtr(m["_sale_price_dates_from"]), unixPtr(m["_sale_price_dates_to"]), time.Now().UTC())
	priceStr := ""
	if m.FloatPtr("_regular_price") != nil || m.FloatPtr("_sale_price") != nil {
		priceStr = fmtFloat(price)
	}
	if err := s.setPostMetas(ctx, q, id, map[string]string{"_price": priceStr, "_stock_status": status}); err != nil {
		return err
	}
	// visibility terms
	terms, err := s.objectTerms(ctx, q, []uint64{id}, TaxVisibility)
	if err != nil {
		return err
	}
	vis := map[string]bool{}
	for _, t := range terms[id][TaxVisibility] {
		vis[t.Slug] = true
	}
	if featured != nil {
		vis["featured"] = *featured
	}
	if visibility != nil {
		vis["exclude-from-catalog"] = *visibility == "search" || *visibility == "hidden"
		vis["exclude-from-search"] = *visibility == "catalog" || *visibility == "hidden"
	}
	vis["outofstock"] = status == "outofstock"
	tts := []uint64{}
	for slug, on := range vis {
		if !on {
			continue
		}
		_, tt, err := s.ensureTerm(ctx, q, TaxVisibility, slug, slug)
		if err != nil {
			return err
		}
		tts = append(tts, tt)
	}
	if err := s.setObjectTerms(ctx, q, id, TaxVisibility, tts); err != nil {
		return err
	}
	return s.syncLookup(ctx, q, id)
}

// syncLookup refreshes wc_product_meta_lookup for a product or variation.
func (s *Store) syncLookup(ctx context.Context, q db.Querier, id uint64) error {
	m, err := s.postMeta(ctx, q, id)
	if err != nil {
		return err
	}
	var postType string
	if err := q.QueryRowContext(ctx, `SELECT post_type FROM `+s.T("posts")+` WHERE ID = ?`, id).Scan(&postType); err != nil {
		return err
	}
	minP, maxP := m.Float("_price"), m.Float("_price")
	if postType == "product" {
		rows, err := q.QueryContext(ctx, `SELECT MIN(CAST(pm.meta_value AS DECIMAL(19,4))), MAX(CAST(pm.meta_value AS DECIMAL(19,4))) FROM `+
			s.T("posts")+` v JOIN `+s.T("postmeta")+` pm ON pm.post_id = v.ID AND pm.meta_key = '_price' AND pm.meta_value <> ''
			WHERE v.post_parent = ? AND v.post_type = 'product_variation' AND v.post_status = 'publish'`, id)
		if err != nil {
			return err
		}
		if rows.Next() {
			var lo, hi sql.NullFloat64
			if err := rows.Scan(&lo, &hi); err == nil && lo.Valid {
				minP, maxP = lo.Float64, hi.Float64
			}
		}
		rows.Close()
	}
	price, onSale := activePrice(m.FloatPtr("_regular_price"), m.FloatPtr("_sale_price"), unixPtr(m["_sale_price_dates_from"]), unixPtr(m["_sale_price_dates_to"]), time.Now().UTC())
	_ = price
	var stock any
	if m.Bool("_manage_stock") && strings.TrimSpace(m["_stock"]) != "" {
		stock = m.Int("_stock")
	}
	taxClass := m["_tax_class"]
	_, err = q.ExecContext(ctx, `REPLACE INTO `+s.T("wc_product_meta_lookup")+` (product_id, sku, global_unique_id, `+"`virtual`"+`, downloadable,
		min_price, max_price, onsale, stock_quantity, stock_status, rating_count, average_rating, total_sales, tax_status, tax_class)
		VALUES (?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, m["_sku"], boolInt(m.Bool("_virtual")), boolInt(m.Bool("_downloadable")), minP, maxP, boolInt(onSale), stock,
		orDefault(m["_stock_status"], "instock"), m.Int("_wc_review_count"), m.Float("_wc_average_rating"), m.Int("total_sales"),
		orDefault(m["_tax_status"], "taxable"), taxClass)
	if err != nil && strings.Contains(err.Error(), "global_unique_id") {
		// WooCommerce < 9.2 has no global_unique_id column
		_, err = q.ExecContext(ctx, `REPLACE INTO `+s.T("wc_product_meta_lookup")+` (product_id, sku, `+"`virtual`"+`, downloadable,
			min_price, max_price, onsale, stock_quantity, stock_status, rating_count, average_rating, total_sales, tax_status, tax_class)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, m["_sku"], boolInt(m.Bool("_virtual")), boolInt(m.Bool("_downloadable")), minP, maxP, boolInt(onSale), stock,
			orDefault(m["_stock_status"], "instock"), m.Int("_wc_review_count"), m.Float("_wc_average_rating"), m.Int("total_sales"),
			orDefault(m["_tax_status"], "taxable"), taxClass)
	}
	return err
}

// syncVariableParent recomputes a variable product's price and stock status
// from its variations (WC_Product_Variable::sync).
func (s *Store) syncVariableParent(ctx context.Context, parentID uint64) error {
	vars, err := s.Variations(ctx, parentID)
	if err != nil {
		return err
	}
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		status := "outofstock"
		prices := []float64{}
		for _, v := range vars {
			if v.Status != "publish" {
				continue
			}
			if v.StockStatus == "instock" {
				status = "instock"
			} else if v.StockStatus == "onbackorder" && status != "instock" {
				status = "onbackorder"
			}
			if v.RegularPrice != nil || v.SalePrice != nil {
				prices = append(prices, v.Price)
			}
		}
		if len(vars) == 0 {
			status = "outofstock"
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.T("postmeta")+` WHERE post_id = ? AND meta_key = '_price'`, parentID); err != nil {
			return err
		}
		// WooCommerce keeps one _price row per distinct variation price on the parent.
		seen := map[string]bool{}
		for _, p := range prices {
			k := fmtFloat(p)
			if seen[k] {
				continue
			}
			seen[k] = true
			if _, err := tx.ExecContext(ctx, `INSERT INTO `+s.T("postmeta")+` (post_id, meta_key, meta_value) VALUES (?, '_price', ?)`, parentID, k); err != nil {
				return err
			}
		}
		if err := s.setPostMeta(ctx, tx, parentID, "_stock_status", status); err != nil {
			return err
		}
		return s.syncProduct(ctx, tx, parentID, nil, nil)
	})
}

// SaveVariation creates (id 0) or updates a variation of parentID.
func (s *Store) SaveVariation(ctx context.Context, parentID, id uint64, in VariationInput, authorID uint64) (Variation, error) {
	parent, err := s.Product(ctx, parentID)
	if err != nil {
		return Variation{}, err
	}
	if in.SKU != nil {
		taken, err := s.skuTaken(ctx, s.DB, strings.TrimSpace(*in.SKU), id)
		if err != nil {
			return Variation{}, err
		}
		if taken {
			return Variation{}, refuse(400, "errors.skuTaken", "This SKU is already used by another product")
		}
	}
	if id != 0 {
		v, err := s.Variation(ctx, id)
		if err != nil {
			return Variation{}, err
		}
		if v.ParentID != parentID {
			return Variation{}, ErrNotFound
		}
	}
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		status := "publish"
		if in.Status != nil {
			status = *in.Status
		}
		title := parent.Name
		if id == 0 {
			var err error
			menu := 0
			if in.MenuOrder != nil {
				menu = *in.MenuOrder
			}
			if id, err = s.insertPost(ctx, tx, "product_variation", status, title, Slugify(title)+"-"+strconv.FormatInt(time.Now().UnixNano()%100000, 10), "", "", parentID, authorID, menu, "closed"); err != nil {
				return err
			}
			for k, v := range map[string]string{"total_sales": "0", "_tax_status": "taxable", "_tax_class": "parent", "_manage_stock": "no",
				"_backorders": "no", "_sold_individually": "no", "_virtual": "no", "_downloadable": "no", "_download_limit": "-1",
				"_download_expiry": "-1", "_stock": "", "_stock_status": "instock", "_wc_average_rating": "0", "_wc_review_count": "0"} {
				if err := s.setPostMeta(ctx, tx, id, k, v); err != nil {
					return err
				}
			}
		} else {
			upd := `UPDATE ` + s.T("posts") + ` SET post_status = COALESCE(?, post_status), menu_order = COALESCE(?, menu_order) WHERE ID = ?`
			if _, err := tx.ExecContext(ctx, upd, in.Status, in.MenuOrder, id); err != nil {
				return err
			}
			if err := s.touchPost(ctx, tx, id); err != nil {
				return err
			}
		}
		meta := map[string]string{}
		set := func(k string, v *string) {
			if v != nil {
				meta[k] = strings.TrimSpace(*v)
			}
		}
		set("_sku", in.SKU)
		set("_variation_description", in.Description)
		if in.RegularPrice != nil {
			meta["_regular_price"] = cleanPrice(*in.RegularPrice)
		}
		if in.SalePrice != nil {
			meta["_sale_price"] = cleanPrice(*in.SalePrice)
		}
		if in.DateOnSaleFrom != nil {
			meta["_sale_price_dates_from"] = parseDateUnix(*in.DateOnSaleFrom)
		}
		if in.DateOnSaleTo != nil {
			meta["_sale_price_dates_to"] = parseDateUnix(*in.DateOnSaleTo)
		}
		if in.Virtual != nil {
			meta["_virtual"] = yesNo(*in.Virtual)
		}
		if in.Downloadable != nil {
			meta["_downloadable"] = yesNo(*in.Downloadable)
		}
		if in.Downloads != nil {
			meta["_downloadable_files"] = downloadsSerialized(*in.Downloads)
		}
		if in.DownloadLimit != nil {
			meta["_download_limit"] = strconv.FormatInt(*in.DownloadLimit, 10)
		}
		if in.DownloadExpiry != nil {
			meta["_download_expiry"] = strconv.FormatInt(*in.DownloadExpiry, 10)
		}
		if in.ManageStock != nil {
			meta["_manage_stock"] = yesNo(*in.ManageStock)
		}
		if in.StockQuantity != nil {
			meta["_stock"] = strconv.FormatInt(*in.StockQuantity, 10)
		}
		set("_stock_status", in.StockStatus)
		set("_backorders", in.Backorders)
		set("_weight", in.Weight)
		set("_length", in.Length)
		set("_width", in.Width)
		set("_height", in.Height)
		set("_tax_status", in.TaxStatus)
		if in.TaxClass != nil {
			meta["_tax_class"] = orDefault(*in.TaxClass, "parent")
		}
		if in.Image != nil {
			meta["_thumbnail_id"] = strconv.FormatUint(in.Image.ID, 10)
			if in.Image.ID == 0 {
				meta["_thumbnail_id"] = ""
			}
		}
		if err := s.setPostMetas(ctx, tx, id, meta); err != nil {
			return err
		}
		if in.Attributes != nil {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.T("postmeta")+` WHERE post_id = ? AND meta_key LIKE 'attribute\_%'`, id); err != nil {
				return err
			}
			names := []string{}
			for _, a := range *in.Attributes {
				slug := a.Slug
				if slug == "" {
					slug = Slugify(a.Name)
				}
				if err := s.setPostMeta(ctx, tx, id, "attribute_"+slug, s.attributeOptionSlug(ctx, slug, a.Option)); err != nil {
					return err
				}
				if a.Option != "" {
					names = append(names, a.Option)
				}
			}
			title := parent.Name
			if len(names) > 0 {
				title += " - " + strings.Join(names, ", ")
			}
			if _, err := tx.ExecContext(ctx, `UPDATE `+s.T("posts")+` SET post_title = ?, post_excerpt = ? WHERE ID = ?`,
				title, strings.Join(names, ", "), id); err != nil {
				return err
			}
		}
		if in.ShippingClass != nil {
			if err := s.setShippingClass(ctx, tx, id, *in.ShippingClass); err != nil {
				return err
			}
		}
		m, err := s.postMeta(ctx, tx, id)
		if err != nil {
			return err
		}
		threshold := int64(s.OptionInt(ctx, "woocommerce_notify_no_stock_amount", 0))
		stockStatus := stockStatusFor(m.Bool("_manage_stock"), m.Int("_stock"), m["_backorders"], m["_stock_status"], threshold)
		price, _ := activePrice(m.FloatPtr("_regular_price"), m.FloatPtr("_sale_price"), unixPtr(m["_sale_price_dates_from"]), unixPtr(m["_sale_price_dates_to"]), time.Now().UTC())
		priceStr := ""
		if m.FloatPtr("_regular_price") != nil || m.FloatPtr("_sale_price") != nil {
			priceStr = fmtFloat(price)
		}
		if err := s.setPostMetas(ctx, tx, id, map[string]string{"_price": priceStr, "_stock_status": stockStatus}); err != nil {
			return err
		}
		return s.syncLookup(ctx, tx, id)
	})
	if err != nil {
		return Variation{}, err
	}
	if err := s.syncVariableParent(ctx, parentID); err != nil {
		return Variation{}, err
	}
	return s.Variation(ctx, id)
}

// GenerateVariations creates a variation for every combination of the
// parent's "used for variations" attributes not covered yet.
func (s *Store) GenerateVariations(ctx context.Context, parentID uint64, authorID uint64) ([]Variation, error) {
	p, err := s.Product(ctx, parentID)
	if err != nil {
		return nil, err
	}
	axes := []ProductAttribute{}
	for _, a := range p.Attributes {
		if a.Variation && len(a.Options) > 0 {
			axes = append(axes, a)
		}
	}
	if len(axes) == 0 {
		return nil, refuse(400, "errors.noVariationAttributes", "Add attributes used for variations first")
	}
	existing := map[string]bool{}
	for _, v := range p.Variations {
		existing[comboKey(v.Attributes)] = true
	}
	combos := [][]AttributeValue{{}}
	for _, a := range axes {
		next := [][]AttributeValue{}
		slug := a.Slug
		if a.ID == 0 {
			slug = Slugify(a.Name)
		}
		for _, c := range combos {
			for _, opt := range a.Options {
				n := append(append([]AttributeValue{}, c...), AttributeValue{Slug: slug, Name: a.Name, Option: opt})
				next = append(next, n)
			}
		}
		combos = next
	}
	if len(combos) > 100 {
		return nil, refuse(400, "errors.tooManyVariations", "More than 100 combinations: generate them in smaller sets")
	}
	created := []Variation{}
	for _, c := range combos {
		if existing[comboKey(c)] {
			continue
		}
		attrs := c
		v, err := s.SaveVariation(ctx, parentID, 0, VariationInput{Attributes: &attrs}, authorID)
		if err != nil {
			return nil, err
		}
		created = append(created, v)
	}
	return created, nil
}

func comboKey(values []AttributeValue) string {
	parts := []string{}
	for _, v := range values {
		parts = append(parts, v.Slug+"="+strings.ToLower(v.Option))
	}
	sortStrings(parts)
	return strings.Join(parts, "&")
}

func sortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

// DeleteVariation removes a variation.
func (s *Store) DeleteVariation(ctx context.Context, parentID, id uint64) error {
	v, err := s.Variation(ctx, id)
	if err != nil {
		return err
	}
	if v.ParentID != parentID {
		return ErrNotFound
	}
	if err := s.DB.InTx(ctx, func(tx *sql.Tx) error { return s.deletePostHard(ctx, tx, id) }); err != nil {
		return err
	}
	return s.syncVariableParent(ctx, parentID)
}

func (s *Store) deletePostHard(ctx context.Context, q db.Querier, id uint64) error {
	rows, err := q.QueryContext(ctx, `SELECT tr.term_taxonomy_id FROM `+s.T("term_relationships")+` tr WHERE tr.object_id = ?`, id)
	if err != nil {
		return err
	}
	tts := []uint64{}
	for rows.Next() {
		var tt uint64
		if err := rows.Scan(&tt); err != nil {
			rows.Close()
			return err
		}
		tts = append(tts, tt)
	}
	rows.Close()
	for _, stmt := range []string{
		`DELETE FROM ` + s.T("term_relationships") + ` WHERE object_id = ?`,
		`DELETE FROM ` + s.T("postmeta") + ` WHERE post_id = ?`,
		`DELETE FROM ` + s.T("wc_product_meta_lookup") + ` WHERE product_id = ?`,
		`DELETE FROM ` + s.T("posts") + ` WHERE ID = ?`,
	} {
		if _, err := q.ExecContext(ctx, stmt, id); err != nil {
			return err
		}
	}
	return s.updateTermCounts(ctx, q, tts)
}

// DeleteProduct trashes a product (force deletes it and its variations).
func (s *Store) DeleteProduct(ctx context.Context, id uint64, force bool) error {
	p, err := s.Product(ctx, id)
	if err != nil {
		return err
	}
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		if !force {
			if _, err := tx.ExecContext(ctx, `UPDATE `+s.T("posts")+` SET post_status = 'trash' WHERE ID = ?`, id); err != nil {
				return err
			}
			if err := s.setPostMetas(ctx, tx, id, map[string]string{"_wp_trash_meta_status": p.Status,
				"_wp_trash_meta_time": strconv.FormatInt(time.Now().Unix(), 10)}); err != nil {
				return err
			}
			return s.syncProduct(ctx, tx, id, nil, nil)
		}
		for _, vid := range p.VariationIDs {
			if err := s.deletePostHard(ctx, tx, vid); err != nil {
				return err
			}
		}
		return s.deletePostHard(ctx, tx, id)
	})
}

// RestoreProduct takes a product out of the trash.
func (s *Store) RestoreProduct(ctx context.Context, id uint64) (Product, error) {
	m, err := s.postMeta(ctx, s.DB, id)
	if err != nil {
		return Product{}, err
	}
	status := orDefault(m["_wp_trash_meta_status"], "draft")
	if _, err := s.DB.ExecContext(ctx, `UPDATE `+s.T("posts")+` SET post_status = ? WHERE ID = ? AND post_status = 'trash'`, status, id); err != nil {
		return Product{}, err
	}
	return s.Product(ctx, id)
}

// DuplicateProduct copies a product (as a draft, "(Copy)", no SKU), with its
// variations, like WooCommerce's "Duplicate".
func (s *Store) DuplicateProduct(ctx context.Context, id uint64, authorID uint64) (Product, error) {
	p, err := s.Product(ctx, id)
	if err != nil {
		return Product{}, err
	}
	var newID uint64
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		slug, err := s.uniquePostSlug(ctx, tx, "product", p.Slug+"-copy", 0)
		if err != nil {
			return err
		}
		comment := "closed"
		if p.ReviewsAllowed {
			comment = "open"
		}
		if newID, err = s.insertPost(ctx, tx, "product", "draft", p.Name+" (Copy)", slug, p.Description, p.ShortDescription, 0, authorID, p.MenuOrder, comment); err != nil {
			return err
		}
		if err := s.copyMetaAndTerms(ctx, tx, id, newID); err != nil {
			return err
		}
		for _, k := range []string{"_sku", "total_sales", "_wc_average_rating", "_wc_review_count", "_wc_rating_count"} {
			v := ""
			switch k {
			case "total_sales", "_wc_average_rating", "_wc_review_count":
				v = "0"
			case "_wc_rating_count":
				v = "a:0:{}"
			}
			if err := s.setPostMeta(ctx, tx, newID, k, v); err != nil {
				return err
			}
		}
		for _, v := range p.Variations {
			vid, err := s.insertPost(ctx, tx, "product_variation", v.Status, p.Name+" (Copy)", slug+"-"+strconv.FormatUint(v.ID, 10), "", "", newID, authorID, v.MenuOrder, "closed")
			if err != nil {
				return err
			}
			if err := s.copyMetaAndTerms(ctx, tx, v.ID, vid); err != nil {
				return err
			}
			if err := s.setPostMeta(ctx, tx, vid, "_sku", ""); err != nil {
				return err
			}
			if err := s.syncLookup(ctx, tx, vid); err != nil {
				return err
			}
		}
		return s.syncProduct(ctx, tx, newID, nil, nil)
	})
	if err != nil {
		return Product{}, err
	}
	return s.Product(ctx, newID)
}

func (s *Store) copyMetaAndTerms(ctx context.Context, q db.Querier, from, to uint64) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO `+s.T("postmeta")+` (post_id, meta_key, meta_value) SELECT ?, meta_key, meta_value FROM `+
		s.T("postmeta")+` WHERE post_id = ? AND meta_key NOT IN ('_edit_lock', '_edit_last', '_wp_old_slug')`, to, from); err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, `INSERT IGNORE INTO `+s.T("term_relationships")+` (object_id, term_taxonomy_id, term_order) SELECT ?, term_taxonomy_id, term_order FROM `+
		s.T("term_relationships")+` WHERE object_id = ?`, to, from); err != nil {
		return err
	}
	return nil
}

// ------------------------------------------------------------------ stock

// StockChange is the result of a stock adjustment.
type StockChange struct {
	ProductID uint64
	Name      string
	From, To  int64
	Managed   bool
}

// AdjustStock changes a product's or variation's stock by delta when stock
// is managed (a variation with "parent" stock adjusts its parent), and keeps
// stock status, visibility and lookup in sync.
func (s *Store) AdjustStock(ctx context.Context, q db.Querier, productID uint64, delta int64) (StockChange, error) {
	if !s.OptionBool(ctx, "woocommerce_manage_stock", true) {
		return StockChange{ProductID: productID}, nil
	}
	m, err := s.postMeta(ctx, q, productID)
	if err != nil {
		return StockChange{}, err
	}
	var postType string
	var parent uint64
	var title string
	if err := q.QueryRowContext(ctx, `SELECT post_type, post_parent, post_title FROM `+s.T("posts")+` WHERE ID = ?`, productID).Scan(&postType, &parent, &title); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return StockChange{ProductID: productID}, nil
		}
		return StockChange{}, err
	}
	target := productID
	if postType == "product_variation" && !m.Bool("_manage_stock") {
		if m["_manage_stock"] == "parent" || parent != 0 {
			pm, err := s.postMeta(ctx, q, parent)
			if err != nil {
				return StockChange{}, err
			}
			if !pm.Bool("_manage_stock") {
				return StockChange{ProductID: productID}, nil
			}
			target, m = parent, pm
		}
	} else if !m.Bool("_manage_stock") {
		return StockChange{ProductID: productID}, nil
	}
	// atomic update like wc_update_product_stock
	if _, err := q.ExecContext(ctx, `UPDATE `+s.T("postmeta")+` SET meta_value = CAST(COALESCE(NULLIF(meta_value, ''), '0') AS SIGNED) + ? WHERE post_id = ? AND meta_key = '_stock'`,
		delta, target); err != nil {
		return StockChange{}, err
	}
	nm, err := s.postMeta(ctx, q, target)
	if err != nil {
		return StockChange{}, err
	}
	threshold := int64(s.OptionInt(ctx, "woocommerce_notify_no_stock_amount", 0))
	status := stockStatusFor(true, nm.Int("_stock"), nm["_backorders"], nm["_stock_status"], threshold)
	if err := s.setPostMeta(ctx, q, target, "_stock_status", status); err != nil {
		return StockChange{}, err
	}
	var targetType string
	_ = q.QueryRowContext(ctx, `SELECT post_type FROM `+s.T("posts")+` WHERE ID = ?`, target).Scan(&targetType)
	if targetType == "product" {
		if err := s.syncProduct(ctx, q, target, nil, nil); err != nil {
			return StockChange{}, err
		}
	} else if err := s.syncLookup(ctx, q, target); err != nil {
		return StockChange{}, err
	}
	return StockChange{ProductID: target, Name: title, From: nm.Int("_stock") - delta, To: nm.Int("_stock"), Managed: true}, nil
}

// AddTotalSales bumps total_sales on a product (and its lookup row).
func (s *Store) AddTotalSales(ctx context.Context, q db.Querier, productID uint64, qty int64) error {
	if _, err := q.ExecContext(ctx, `UPDATE `+s.T("postmeta")+` SET meta_value = CAST(COALESCE(NULLIF(meta_value, ''), '0') AS SIGNED) + ? WHERE post_id = ? AND meta_key = 'total_sales'`,
		qty, productID); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `UPDATE `+s.T("wc_product_meta_lookup")+` SET total_sales = total_sales + ? WHERE product_id = ?`, qty, productID)
	return err
}
