package woo

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"cwmerch-api/internal/db"
)

// Taxonomies WooCommerce uses.
const (
	TaxCategory      = "product_cat"
	TaxTag           = "product_tag"
	TaxType          = "product_type"
	TaxVisibility    = "product_visibility"
	TaxShippingClass = "product_shipping_class"
)

// Image is a product/category image (an attachment post).
type Image struct {
	ID   uint64 `json:"id"`
	Src  string `json:"src"`
	Name string `json:"name,omitempty"`
	Alt  string `json:"alt,omitempty"`
}

// TermRef is a term as products reference it.
type TermRef struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Term is a taxonomy term (category, tag, attribute term, shipping class).
type Term struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Parent      uint64 `json:"parent"`
	Description string `json:"description"`
	Count       int64  `json:"count"`
	Image       *Image `json:"image"`
	MenuOrder   int    `json:"menuOrder"`
	DisplayType string `json:"displayType,omitempty"`
	// Path is the term's storefront page, following WooCommerce's permalinks
	// (categories are hierarchical: /product-category/parent/child).
	Path       string `json:"path,omitempty"`
	taxonomyID uint64
}

var nonSlug = regexp.MustCompile(`[^a-z0-9-]+`)

// Slugify is sanitize_title's spirit: lowercase ASCII, accents folded, dashes.
func Slugify(s string) string {
	t := norm.NFD.String(strings.ToLower(strings.TrimSpace(s)))
	var b strings.Builder
	for _, r := range t {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	out := nonSlug.ReplaceAllString(strings.ReplaceAll(b.String(), " ", "-"), "-")
	out = strings.Trim(regexp.MustCompile(`-+`).ReplaceAllString(out, "-"), "-")
	if len(out) > 190 {
		out = out[:190]
	}
	return out
}

func (s *Store) termMetas(ctx context.Context, ids []uint64) (map[uint64]map[string]string, error) {
	out := map[uint64]map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT term_id, meta_key, meta_value FROM `+s.T("termmeta")+` WHERE term_id IN (`+inList(ids)+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uint64
		var k string
		var v sql.NullString
		if err := rows.Scan(&id, &k, &v); err != nil {
			return nil, err
		}
		if out[id] == nil {
			out[id] = map[string]string{}
		}
		out[id][k] = v.String
	}
	return out, rows.Err()
}

func (s *Store) setTermMeta(ctx context.Context, q db.Querier, termID uint64, key, value string) error {
	var metaID uint64
	err := q.QueryRowContext(ctx, `SELECT meta_id FROM `+s.T("termmeta")+` WHERE term_id = ? AND meta_key = ? LIMIT 1`, termID, key).Scan(&metaID)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = q.ExecContext(ctx, `INSERT INTO `+s.T("termmeta")+` (term_id, meta_key, meta_value) VALUES (?, ?, ?)`, termID, key, value)
		return err
	}
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `UPDATE `+s.T("termmeta")+` SET meta_value = ? WHERE meta_id = ?`, value, metaID)
	return err
}

// Terms lists a taxonomy's terms (ordered by the "order" term meta, then name).
func (s *Store) Terms(ctx context.Context, taxonomy string) ([]Term, error) {
	terms, err := s.termsWhere(ctx, "tt.taxonomy = ?", taxonomy)
	if err != nil || (taxonomy != TaxCategory && taxonomy != TaxTag) {
		return terms, err
	}
	pl := s.Permalinks(ctx)
	byID := map[uint64]Term{}
	for _, t := range terms {
		byID[t.ID] = t
	}
	for i := range terms {
		if taxonomy == TaxTag {
			terms[i].Path = pl.TagPath(terms[i].Slug)
			continue
		}
		segs := []string{terms[i].Slug}
		seen := map[uint64]bool{terms[i].ID: true}
		for p := terms[i].Parent; p != 0 && !seen[p]; {
			pt, ok := byID[p]
			if !ok {
				break
			}
			seen[p] = true
			segs = append([]string{pt.Slug}, segs...)
			p = pt.Parent
		}
		terms[i].Path = pl.CategoryPath(strings.Join(segs, "/"))
	}
	return terms, nil
}

func (s *Store) termsWhere(ctx context.Context, where string, args ...any) ([]Term, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT t.term_id, t.name, t.slug, tt.parent, tt.description, tt.count, tt.term_taxonomy_id
		FROM `+s.T("terms")+` t JOIN `+s.T("term_taxonomy")+` tt ON tt.term_id = t.term_id WHERE `+where+` ORDER BY t.name`, args...)
	if err != nil {
		return nil, err
	}
	terms := []Term{}
	ids := []uint64{}
	for rows.Next() {
		var t Term
		var desc sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.Parent, &desc, &t.Count, &t.taxonomyID); err != nil {
			rows.Close()
			return nil, err
		}
		t.Description = desc.String
		terms = append(terms, t)
		ids = append(ids, t.ID)
	}
	rows.Close()
	metas, err := s.termMetas(ctx, ids)
	if err != nil {
		return nil, err
	}
	thumbs := []uint64{}
	for i := range terms {
		m := metas[terms[i].ID]
		terms[i].MenuOrder, _ = strconv.Atoi(m["order"])
		terms[i].DisplayType = m["display_type"]
		if id := ParseID(m["thumbnail_id"]); id != 0 {
			terms[i].Image = &Image{ID: id}
			thumbs = append(thumbs, id)
		}
	}
	if len(thumbs) > 0 {
		images, err := s.images(ctx, thumbs)
		if err != nil {
			return nil, err
		}
		for i := range terms {
			if terms[i].Image != nil {
				if img, ok := images[terms[i].Image.ID]; ok {
					terms[i].Image = &img
				} else {
					terms[i].Image = nil
				}
			}
		}
	}
	sortTerms(terms)
	return terms, nil
}

func sortTerms(terms []Term) {
	for i := 1; i < len(terms); i++ {
		for j := i; j > 0 && (terms[j].MenuOrder < terms[j-1].MenuOrder ||
			(terms[j].MenuOrder == terms[j-1].MenuOrder && strings.ToLower(terms[j].Name) < strings.ToLower(terms[j-1].Name))); j-- {
			terms[j], terms[j-1] = terms[j-1], terms[j]
		}
	}
}

// Term is one term of a taxonomy by id.
func (s *Store) Term(ctx context.Context, taxonomy string, id uint64) (Term, error) {
	terms, err := s.Terms(ctx, taxonomy)
	if err != nil {
		return Term{}, err
	}
	for _, t := range terms {
		if t.ID == id {
			return t, nil
		}
	}
	return Term{}, ErrNotFound
}

// TermBySlug is one term of a taxonomy by slug.
// TermBySlug accepts a slug or a hierarchical path ("parent/child").
func (s *Store) TermBySlug(ctx context.Context, taxonomy, slug string) (Term, error) {
	slug = strings.Trim(slug, "/")
	if i := strings.LastIndex(slug, "/"); i >= 0 {
		slug = slug[i+1:]
	}
	terms, err := s.Terms(ctx, taxonomy)
	if err != nil {
		return Term{}, err
	}
	for _, t := range terms {
		if t.Slug == slug {
			return t, nil
		}
	}
	return Term{}, ErrNotFound
}

// TermInput is a term create/update.
type TermInput struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Parent      *uint64 `json:"parent"`
	Description *string `json:"description"`
	ImageID     *uint64 `json:"imageId"`
	Image       *Image  `json:"image"`
	MenuOrder   *int    `json:"menuOrder"`
	DisplayType *string `json:"displayType"`
}

func (s *Store) uniqueTermSlug(ctx context.Context, q db.Querier, taxonomy, slug string, exclude uint64) (string, error) {
	base := slug
	for i := 2; i < 1000; i++ {
		var n int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.T("terms")+` t JOIN `+s.T("term_taxonomy")+
			` tt ON tt.term_id = t.term_id WHERE tt.taxonomy = ? AND t.slug = ? AND t.term_id <> ?`, taxonomy, slug, exclude).Scan(&n); err != nil {
			return "", err
		}
		if n == 0 {
			return slug, nil
		}
		slug = base + "-" + strconv.Itoa(i)
	}
	return slug, nil
}

// CreateTerm adds a term to a taxonomy.
func (s *Store) CreateTerm(ctx context.Context, taxonomy string, in TermInput) (Term, error) {
	if strings.TrimSpace(in.Name) == "" {
		return Term{}, refuse(400, "errors.nameRequired", "A name is required")
	}
	var id uint64
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = s.insertTerm(ctx, tx, taxonomy, in)
		return err
	})
	if err != nil {
		return Term{}, err
	}
	return s.Term(ctx, taxonomy, id)
}

func (s *Store) insertTerm(ctx context.Context, q db.Querier, taxonomy string, in TermInput) (uint64, error) {
	slug := Slugify(in.Slug)
	if slug == "" {
		slug = Slugify(in.Name)
	}
	slug, err := s.uniqueTermSlug(ctx, q, taxonomy, slug, 0)
	if err != nil {
		return 0, err
	}
	res, err := q.ExecContext(ctx, `INSERT INTO `+s.T("terms")+` (name, slug, term_group) VALUES (?, ?, 0)`, in.Name, slug)
	if err != nil {
		return 0, err
	}
	last, _ := res.LastInsertId()
	id := uint64(last)
	parent, desc := uint64(0), ""
	if in.Parent != nil {
		parent = *in.Parent
	}
	if in.Description != nil {
		desc = *in.Description
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO `+s.T("term_taxonomy")+` (term_id, taxonomy, description, parent, count) VALUES (?, ?, ?, ?, 0)`,
		id, taxonomy, desc, parent); err != nil {
		return 0, err
	}
	return id, s.applyTermMeta(ctx, q, id, in)
}

func (s *Store) applyTermMeta(ctx context.Context, q db.Querier, id uint64, in TermInput) error {
	if in.MenuOrder != nil {
		if err := s.setTermMeta(ctx, q, id, "order", strconv.Itoa(*in.MenuOrder)); err != nil {
			return err
		}
	}
	img := in.ImageID
	if img == nil && in.Image != nil {
		img = &in.Image.ID
	}
	if img != nil {
		if err := s.setTermMeta(ctx, q, id, "thumbnail_id", strconv.FormatUint(*img, 10)); err != nil {
			return err
		}
	}
	if in.DisplayType != nil {
		if err := s.setTermMeta(ctx, q, id, "display_type", *in.DisplayType); err != nil {
			return err
		}
	}
	return nil
}

// UpdateTerm edits a term.
func (s *Store) UpdateTerm(ctx context.Context, taxonomy string, id uint64, in TermInput) (Term, error) {
	current, err := s.Term(ctx, taxonomy, id)
	if err != nil {
		return Term{}, err
	}
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		name := current.Name
		if strings.TrimSpace(in.Name) != "" {
			name = in.Name
		}
		slug := current.Slug
		if in.Slug != "" {
			if slug, err = s.uniqueTermSlug(ctx, tx, taxonomy, Slugify(in.Slug), id); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE `+s.T("terms")+` SET name = ?, slug = ? WHERE term_id = ?`, name, slug, id); err != nil {
			return err
		}
		parent, desc := current.Parent, current.Description
		if in.Parent != nil {
			if *in.Parent == id {
				return refuse(400, "errors.invalidParent", "A term cannot be its own parent")
			}
			parent = *in.Parent
		}
		if in.Description != nil {
			desc = *in.Description
		}
		if _, err := tx.ExecContext(ctx, `UPDATE `+s.T("term_taxonomy")+` SET parent = ?, description = ? WHERE term_id = ? AND taxonomy = ?`,
			parent, desc, id, taxonomy); err != nil {
			return err
		}
		return s.applyTermMeta(ctx, tx, id, in)
	})
	if err != nil {
		return Term{}, err
	}
	return s.Term(ctx, taxonomy, id)
}

// DeleteTerm removes a term, its relationships and meta; children move to its parent.
func (s *Store) DeleteTerm(ctx context.Context, taxonomy string, id uint64) error {
	current, err := s.Term(ctx, taxonomy, id)
	if err != nil {
		return err
	}
	if taxonomy == TaxCategory && ParseID(s.Option(ctx, "default_product_cat")) == id {
		return refuse(400, "errors.defaultCategory", "The default product category cannot be deleted")
	}
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE `+s.T("term_taxonomy")+` SET parent = ? WHERE parent = ? AND taxonomy = ?`, current.Parent, id, taxonomy); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.T("term_relationships")+` WHERE term_taxonomy_id = ?`, current.taxonomyID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.T("term_taxonomy")+` WHERE term_taxonomy_id = ?`, current.taxonomyID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.T("termmeta")+` WHERE term_id = ?`, id); err != nil {
			return err
		}
		// A term row can be shared by several taxonomies (old WP); only drop it when orphaned.
		_, err := tx.ExecContext(ctx, `DELETE FROM `+s.T("terms")+` WHERE term_id = ? AND NOT EXISTS (SELECT 1 FROM `+
			s.T("term_taxonomy")+` WHERE term_id = ?)`, id, id)
		return err
	})
}

// objectTerms is every term of the given taxonomies attached to objects.
func (s *Store) objectTerms(ctx context.Context, q db.Querier, objectIDs []uint64, taxonomies ...string) (map[uint64]map[string][]TermRef, error) {
	out := map[uint64]map[string][]TermRef{}
	if len(objectIDs) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT tr.object_id, tt.taxonomy, t.term_id, t.name, t.slug
		FROM `+s.T("term_relationships")+` tr
		JOIN `+s.T("term_taxonomy")+` tt ON tt.term_taxonomy_id = tr.term_taxonomy_id
		JOIN `+s.T("terms")+` t ON t.term_id = tt.term_id
		WHERE tr.object_id IN (`+inList(objectIDs)+`) AND tt.taxonomy IN (`+placeholders(len(taxonomies))+`)
		ORDER BY tr.term_order, t.name`, strArgs(taxonomies)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var oid uint64
		var tax string
		var t TermRef
		if err := rows.Scan(&oid, &tax, &t.ID, &t.Name, &t.Slug); err != nil {
			return nil, err
		}
		if out[oid] == nil {
			out[oid] = map[string][]TermRef{}
		}
		out[oid][tax] = append(out[oid][tax], t)
	}
	return out, rows.Err()
}

// ensureTerm finds (or creates) a term by slug in a taxonomy and answers its term_taxonomy_id.
func (s *Store) ensureTerm(ctx context.Context, q db.Querier, taxonomy, name, slug string) (uint64, uint64, error) {
	var termID, ttID uint64
	err := q.QueryRowContext(ctx, `SELECT t.term_id, tt.term_taxonomy_id FROM `+s.T("terms")+` t JOIN `+s.T("term_taxonomy")+
		` tt ON tt.term_id = t.term_id WHERE tt.taxonomy = ? AND (t.slug = ? OR t.name = ?) LIMIT 1`, taxonomy, slug, name).Scan(&termID, &ttID)
	if err == nil {
		return termID, ttID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, err
	}
	termID, err = s.insertTerm(ctx, q, taxonomy, TermInput{Name: name, Slug: slug})
	if err != nil {
		return 0, 0, err
	}
	err = q.QueryRowContext(ctx, `SELECT term_taxonomy_id FROM `+s.T("term_taxonomy")+` WHERE term_id = ? AND taxonomy = ?`, termID, taxonomy).Scan(&ttID)
	return termID, ttID, err
}

func (s *Store) termTaxonomyIDs(ctx context.Context, q db.Querier, taxonomy string, termIDs []uint64) ([]uint64, error) {
	if len(termIDs) == 0 {
		return nil, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT term_taxonomy_id FROM `+s.T("term_taxonomy")+` WHERE taxonomy = ? AND term_id IN (`+inList(termIDs)+`)`, taxonomy)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []uint64{}
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// setObjectTerms replaces an object's terms in a taxonomy (wp_set_object_terms)
// and refreshes the affected counts.
func (s *Store) setObjectTerms(ctx context.Context, q db.Querier, objectID uint64, taxonomy string, ttIDs []uint64) error {
	rows, err := q.QueryContext(ctx, `SELECT tr.term_taxonomy_id FROM `+s.T("term_relationships")+` tr JOIN `+s.T("term_taxonomy")+
		` tt ON tt.term_taxonomy_id = tr.term_taxonomy_id WHERE tr.object_id = ? AND tt.taxonomy = ?`, objectID, taxonomy)
	if err != nil {
		return err
	}
	old := []uint64{}
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		old = append(old, id)
	}
	rows.Close()
	if len(old) > 0 {
		if _, err := q.ExecContext(ctx, `DELETE FROM `+s.T("term_relationships")+` WHERE object_id = ? AND term_taxonomy_id IN (`+inList(old)+`)`, objectID); err != nil {
			return err
		}
	}
	for i, tt := range uniqueIDs(ttIDs) {
		if _, err := q.ExecContext(ctx, `INSERT IGNORE INTO `+s.T("term_relationships")+` (object_id, term_taxonomy_id, term_order) VALUES (?, ?, ?)`,
			objectID, tt, i); err != nil {
			return err
		}
	}
	return s.updateTermCounts(ctx, q, uniqueIDs(append(old, ttIDs...)))
}

// updateTermCounts recounts published products (WooCommerce counts published
// products for product taxonomies).
func (s *Store) updateTermCounts(ctx context.Context, q db.Querier, ttIDs []uint64) error {
	if len(ttIDs) == 0 {
		return nil
	}
	_, err := q.ExecContext(ctx, `UPDATE `+s.T("term_taxonomy")+` tt SET count = (SELECT COUNT(*) FROM `+s.T("term_relationships")+
		` tr JOIN `+s.T("posts")+` p ON p.ID = tr.object_id WHERE tr.term_taxonomy_id = tt.term_taxonomy_id
		AND p.post_status = 'publish' AND p.post_type IN ('product', 'product_variation'))
		WHERE tt.term_taxonomy_id IN (`+inList(ttIDs)+`)`)
	return err
}

// ------------------------------------------------------------------ attributes

// Attribute is a global product attribute (woocommerce_attribute_taxonomies);
// its terms live in the taxonomy "pa_<slug>".
type Attribute struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Type        string `json:"type"`
	OrderBy     string `json:"orderBy"`
	HasArchives bool   `json:"hasArchives"`
}

// Taxonomy is the attribute's term taxonomy (pa_color).
func (a Attribute) Taxonomy() string { return "pa_" + a.Slug }

func (s *Store) Attributes(ctx context.Context) ([]Attribute, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT attribute_id, attribute_label, attribute_name, attribute_type, attribute_orderby, attribute_public FROM `+
		s.T("woocommerce_attribute_taxonomies")+` ORDER BY attribute_label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attribute{}
	for rows.Next() {
		var a Attribute
		var label sql.NullString
		var public int
		if err := rows.Scan(&a.ID, &label, &a.Slug, &a.Type, &a.OrderBy, &public); err != nil {
			return nil, err
		}
		a.Name = label.String
		if a.Name == "" {
			a.Name = a.Slug
		}
		a.HasArchives = public == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Attribute(ctx context.Context, id uint64) (Attribute, error) {
	all, err := s.Attributes(ctx)
	if err != nil {
		return Attribute{}, err
	}
	for _, a := range all {
		if a.ID == id {
			return a, nil
		}
	}
	return Attribute{}, ErrNotFound
}

// AttributeBySlug accepts "color" or "pa_color".
func (s *Store) AttributeBySlug(ctx context.Context, slug string) (Attribute, error) {
	slug = strings.TrimPrefix(slug, "pa_")
	all, err := s.Attributes(ctx)
	if err != nil {
		return Attribute{}, err
	}
	for _, a := range all {
		if a.Slug == slug {
			return a, nil
		}
	}
	return Attribute{}, ErrNotFound
}

// AttributeInput is an attribute create/update.
type AttributeInput struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Type        string `json:"type"`
	OrderBy     string `json:"orderBy"`
	HasArchives bool   `json:"hasArchives"`
}

func (s *Store) forgetAttributeCache(ctx context.Context) {
	_ = s.DeleteOption(ctx, "_transient_wc_attribute_taxonomies")
	_ = s.DeleteOption(ctx, "_transient_timeout_wc_attribute_taxonomies")
}

func (s *Store) CreateAttribute(ctx context.Context, in AttributeInput) (Attribute, error) {
	if strings.TrimSpace(in.Name) == "" {
		return Attribute{}, refuse(400, "errors.nameRequired", "A name is required")
	}
	slug := Slugify(in.Slug)
	if slug == "" {
		slug = Slugify(in.Name)
	}
	if len(slug) > 28 {
		slug = slug[:28]
	}
	if _, err := s.AttributeBySlug(ctx, slug); err == nil {
		return Attribute{}, refuse(400, "errors.attributeSlugTaken", "An attribute with this slug already exists")
	}
	typ := in.Type
	if typ == "" {
		typ = "select"
	}
	orderBy := in.OrderBy
	if orderBy == "" {
		orderBy = "menu_order"
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO `+s.T("woocommerce_attribute_taxonomies")+
		` (attribute_name, attribute_label, attribute_type, attribute_orderby, attribute_public) VALUES (?, ?, ?, ?, ?)`,
		slug, in.Name, typ, orderBy, boolInt(in.HasArchives))
	if err != nil {
		return Attribute{}, err
	}
	s.forgetAttributeCache(ctx)
	id, _ := res.LastInsertId()
	return s.Attribute(ctx, uint64(id))
}

func (s *Store) UpdateAttribute(ctx context.Context, id uint64, in AttributeInput) (Attribute, error) {
	current, err := s.Attribute(ctx, id)
	if err != nil {
		return Attribute{}, err
	}
	name := current.Name
	if strings.TrimSpace(in.Name) != "" {
		name = in.Name
	}
	typ, orderBy := current.Type, current.OrderBy
	if in.Type != "" {
		typ = in.Type
	}
	if in.OrderBy != "" {
		orderBy = in.OrderBy
	}
	// The slug is the taxonomy name: renaming it would orphan every term, so it stays.
	if _, err := s.DB.ExecContext(ctx, `UPDATE `+s.T("woocommerce_attribute_taxonomies")+
		` SET attribute_label = ?, attribute_type = ?, attribute_orderby = ?, attribute_public = ? WHERE attribute_id = ?`,
		name, typ, orderBy, boolInt(in.HasArchives), id); err != nil {
		return Attribute{}, err
	}
	s.forgetAttributeCache(ctx)
	return s.Attribute(ctx, id)
}

func (s *Store) DeleteAttribute(ctx context.Context, id uint64) error {
	a, err := s.Attribute(ctx, id)
	if err != nil {
		return err
	}
	terms, err := s.Terms(ctx, a.Taxonomy())
	if err != nil {
		return err
	}
	for _, t := range terms {
		if err := s.DeleteTerm(ctx, a.Taxonomy(), t.ID); err != nil {
			return err
		}
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM `+s.T("woocommerce_attribute_taxonomies")+` WHERE attribute_id = ?`, id); err != nil {
		return err
	}
	s.forgetAttributeCache(ctx)
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
