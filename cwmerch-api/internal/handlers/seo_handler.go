package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"cwmerch-api/internal/models"
	"cwmerch-api/internal/seoanalysis"
	"cwmerch-api/internal/store"
	"cwmerch-api/internal/woo"
)

// SEOService resolves what the SEO screens, the storefront and the AI
// assistant need: a page's SEO data, its resolved head and its analysis.
type SEOService struct {
	woo *woo.Store
}

func NewSEOService(w *woo.Store) *SEOService { return &SEOService{woo: w} }

// seoTarget names the page analysed: by kind and id (taxonomy pages: kind
// "category" or "tag"), or by storefront path. Meta and Draft are the
// editor's unsaved values (live analysis).
type seoTarget struct {
	Kind  string       `json:"kind"`
	ID    uint64       `json:"id"`
	Path  string       `json:"path"`
	Meta  *woo.SEOMeta `json:"meta"`
	Draft *seoDraft    `json:"draft"`
	// headOnly skips the analysis (the storefront's head).
	headOnly bool
}

type seoDraft struct {
	Name             *string             `json:"name"`
	Slug             *string             `json:"slug"`
	Description      *string             `json:"description"`
	ShortDescription *string             `json:"shortDescription"`
	SKU              *string             `json:"sku"`
	Images           []seoanalysis.Image `json:"images"`
	Categories       []woo.TermRef       `json:"categories"`
	Status           *string             `json:"status"`
}

// seoPage is a resolved target.
type seoPage struct {
	Kind     string             `json:"kind"`
	ID       uint64             `json:"id,omitempty"`
	Taxonomy string             `json:"taxonomy,omitempty"`
	Name     string             `json:"name"`
	Path     string             `json:"path"`
	Slug     string             `json:"slug,omitempty"`
	EditPath string             `json:"editPath,omitempty"`
	Meta     woo.SEOMeta        `json:"meta"`
	Head     woo.SEOHead        `json:"head"`
	Analysis seoanalysis.Result `json:"analysis"`
	Input    *seoanalysis.Input `json:"-"`
	Vars     map[string]string  `json:"vars"`
	product  *woo.Product
	term     *woo.Term
}

func taxonomyOf(kind string) string {
	switch kind {
	case seoanalysis.KindCategory, woo.TaxCategory:
		return woo.TaxCategory
	case seoanalysis.KindTag, woo.TaxTag:
		return woo.TaxTag
	}
	return ""
}

func kindOf(taxonomy string) string {
	if taxonomy == woo.TaxTag {
		return seoanalysis.KindTag
	}
	return seoanalysis.KindCategory
}

func notFound() error { return woo.ErrNotFound }

// resolve loads the target, applies the draft and runs the analysis in lng.
func (s *SEOService) resolve(ctx context.Context, t seoTarget, lng string) (seoPage, error) {
	if t.Path != "" && t.ID == 0 {
		pl := s.woo.Permalinks(ctx)
		path := t.Path
		if i := strings.IndexAny(path, "?#"); i >= 0 {
			path = path[:i]
		}
		kind, ref := pl.RouteKind(path)
		switch kind {
		case "home", "shop":
			t.Kind = kind
		case "product":
			p, err := s.woo.PublicProduct(ctx, ref)
			if err != nil {
				return seoPage{}, err
			}
			t.Kind, t.ID = seoanalysis.KindProduct, p.ID
		case "category", "tag":
			term, err := s.woo.TermBySlug(ctx, taxonomyOf(kind), ref)
			if err != nil {
				return seoPage{}, err
			}
			t.Kind, t.ID = kind, term.ID
		default:
			return seoPage{}, notFound()
		}
	}
	if t.Kind == woo.TaxCategory {
		t.Kind = seoanalysis.KindCategory
	}
	if t.Kind == woo.TaxTag {
		t.Kind = seoanalysis.KindTag
	}
	set := s.woo.SEOSettings(ctx)
	page := seoPage{Kind: t.Kind, ID: t.ID}
	in := seoanalysis.Input{Kind: t.Kind, Locale: s.woo.Lang(ctx), MessageLocale: lng, SiteURL: s.woo.SiteURL}

	switch t.Kind {
	case seoanalysis.KindProduct:
		var p woo.Product
		meta := woo.SEOMeta{}
		if t.ID != 0 {
			var err error
			if p, err = s.woo.ProductForSEO(ctx, t.ID); err != nil {
				return seoPage{}, err
			}
			if meta, err = s.woo.ProductSEO(ctx, t.ID); err != nil {
				return seoPage{}, err
			}
			in.GTIN = s.woo.ProductGTIN(ctx, t.ID)
		}
		if t.Meta != nil {
			scores := meta
			meta = *t.Meta
			meta.SEOScore, meta.ReadabilityScore = scores.SEOScore, scores.ReadabilityScore
		}
		images := []seoanalysis.Image{}
		for _, im := range p.Images {
			images = append(images, seoanalysis.Image{Src: im.Src, Alt: im.Alt})
		}
		if d := t.Draft; d != nil {
			if d.Name != nil {
				p.Name = *d.Name
			}
			if d.Slug != nil {
				p.Slug = *d.Slug
				if p.Path == "" || t.ID == 0 {
					p.Path = "/product/" + p.Slug + "/"
				}
			}
			if d.Description != nil {
				p.Description = *d.Description
			}
			if d.ShortDescription != nil {
				p.ShortDescription = *d.ShortDescription
			}
			if d.SKU != nil {
				p.SKU = *d.SKU
			}
			if d.Images != nil {
				images = d.Images
			}
			if d.Categories != nil {
				p.Categories = d.Categories
			}
			if d.Status != nil {
				p.Status = *d.Status
			}
		}
		page.product = &p
		page.Name, page.Path, page.Slug, page.Meta = p.Name, p.Path, p.Slug, meta
		page.EditPath = "/admin/products/" + itoa(t.ID)
		page.Head = s.woo.ProductHead(ctx, set, p, meta)
		page.Vars = s.woo.ProductSEOVars(ctx, set, p, meta)
		used, _ := s.woo.FocusKeyphraseUses(ctx, meta.FocusKeyphrase, t.ID, "", 0)
		in.Keyphrase, in.Slug, in.Name, in.Text = meta.FocusKeyphrase, p.Slug, p.Name, p.Description
		in.ShortDescription, in.SKU, in.Images, in.KeyphraseUsedBy, in.Cornerstone = p.ShortDescription, p.SKU, images, used, meta.Cornerstone
	case seoanalysis.KindCategory, seoanalysis.KindTag:
		tax := taxonomyOf(t.Kind)
		term, err := s.woo.Term(ctx, tax, t.ID)
		if err != nil {
			return seoPage{}, err
		}
		meta, err := s.woo.TermSEO(ctx, tax, t.ID)
		if err != nil {
			return seoPage{}, err
		}
		if t.Meta != nil {
			scores := meta
			meta = *t.Meta
			meta.SEOScore, meta.ReadabilityScore = scores.SEOScore, scores.ReadabilityScore
		}
		if d := t.Draft; d != nil {
			if d.Name != nil {
				term.Name = *d.Name
			}
			if d.Slug != nil {
				term.Slug = *d.Slug
			}
			if d.Description != nil {
				term.Description = *d.Description
			}
		}
		images := []seoanalysis.Image{}
		if term.Image != nil {
			images = append(images, seoanalysis.Image{Src: term.Image.Src, Alt: term.Image.Alt})
		}
		page.term = &term
		page.Taxonomy = tax
		page.Name, page.Path, page.Slug, page.Meta = term.Name, term.Path, term.Slug, meta
		page.EditPath = map[string]string{woo.TaxCategory: "/admin/categories", woo.TaxTag: "/admin/tags"}[tax]
		page.Head = s.woo.TermHead(ctx, set, tax, term, meta)
		page.Vars = s.woo.TermSEOVars(ctx, set, term, meta)
		used, _ := s.woo.FocusKeyphraseUses(ctx, meta.FocusKeyphrase, 0, tax, t.ID)
		in.Keyphrase, in.Slug, in.Name, in.Text, in.Images, in.KeyphraseUsedBy, in.Cornerstone =
			meta.FocusKeyphrase, term.Slug, term.Name, term.Description, images, used, meta.Cornerstone
	case seoanalysis.KindHome:
		app, _ := s.woo.Appearance(ctx, false)
		image := ""
		if app.Logo != nil {
			image = app.Logo.Src
		}
		if len(app.Banner.Slides) > 0 && app.Banner.Slides[0].Image != "" {
			image = app.Banner.Slides[0].Image
		}
		page.Name, page.Path, page.EditPath = s.woo.Option(ctx, "blogname"), "/", "/admin/seo?tab=settings"
		page.Head = s.woo.HomeHead(ctx, set, image)
	case seoanalysis.KindShop:
		path := s.woo.Permalinks(ctx).ShopPath()
		page.Name, page.Path, page.EditPath = page.Head.Title, path, "/admin/seo?tab=settings"
		page.Head = s.woo.ShopHead(ctx, set, path)
		page.Name = page.Head.Title
	default:
		return seoPage{}, notFound()
	}
	in.Title = page.Head.Title
	in.Description = page.Head.Description
	in.DescriptionSet = !page.Head.DescriptionIsAuto
	page.Input = &in
	if !t.headOnly {
		page.Analysis = seoanalysis.Analyze(in)
	}
	if page.Vars == nil {
		page.Vars = map[string]string{}
	}
	return page, nil
}

func itoa(n uint64) string {
	if n == 0 {
		return "new"
	}
	return sid(n)
}

// SEOHandler serves the SEO screens.
type SEOHandler struct {
	auditor
	svc *SEOService
}

func NewSEOHandler(svc *SEOService, audit *store.AuditStore) *SEOHandler {
	return &SEOHandler{auditor: newAuditor(audit), svc: svc}
}

func lngOf(r *http.Request) string {
	if l := strings.TrimSpace(r.URL.Query().Get("lng")); l != "" {
		return l
	}
	return strings.TrimSpace(r.Header.Get("Accept-Language"))
}

// Settings is GET /v1/admin/seo/settings.
func (h *SEOHandler) Settings(w http.ResponseWriter, r *http.Request) {
	seps := []map[string]string{}
	for _, id := range woo.SEOSeparatorIDs {
		seps = append(seps, map[string]string{"id": id, "char": woo.SEOSeparators[id]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": h.svc.woo.SEOSettings(r.Context()), "separators": seps, "variables": woo.SEOVarNames})
}

// UpdateSettings is PUT /v1/admin/seo/settings.
func (h *SEOHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var in woo.SEOSettings
	if !decodeJSON(w, r, &in) {
		return
	}
	saved, err := h.svc.woo.SaveSEOSettings(r.Context(), in)
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditSEOUpdated, models.AuditSubjectSetting, "seo", nil)
	writeJSON(w, http.StatusOK, map[string]any{"settings": saved})
}

// Analyze is POST /v1/admin/seo/analyze: the live analysis of a page (saved
// or with the editor's draft values).
func (h *SEOHandler) Analyze(w http.ResponseWriter, r *http.Request) {
	var t seoTarget
	if !decodeJSON(w, r, &t) {
		return
	}
	page, err := h.svc.resolve(r.Context(), t, lngOf(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// Product is GET /v1/admin/seo/products/{id}.
func (h *SEOHandler) Product(w http.ResponseWriter, r *http.Request) {
	h.get(w, r, seoTarget{Kind: seoanalysis.KindProduct, ID: idParam(r, "id")})
}

// Term is GET /v1/admin/seo/terms/{taxonomy}/{id}.
func (h *SEOHandler) Term(w http.ResponseWriter, r *http.Request) {
	tax := taxonomyOf(chi.URLParam(r, "taxonomy"))
	if tax == "" {
		writeWooError(w, notFound())
		return
	}
	h.get(w, r, seoTarget{Kind: kindOf(tax), ID: idParam(r, "id")})
}

func (h *SEOHandler) get(w http.ResponseWriter, r *http.Request, t seoTarget) {
	page, err := h.svc.resolve(r.Context(), t, lngOf(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// SaveProduct is PUT /v1/admin/seo/products/{id}: saves the SEO data and the
// scores of its analysis (computed on save).
func (h *SEOHandler) SaveProduct(w http.ResponseWriter, r *http.Request) {
	h.save(w, r, seoanalysis.KindProduct, idParam(r, "id"))
}

// SaveTerm is PUT /v1/admin/seo/terms/{taxonomy}/{id}.
func (h *SEOHandler) SaveTerm(w http.ResponseWriter, r *http.Request) {
	tax := taxonomyOf(chi.URLParam(r, "taxonomy"))
	if tax == "" {
		writeWooError(w, notFound())
		return
	}
	h.save(w, r, kindOf(tax), idParam(r, "id"))
}

func (h *SEOHandler) save(w http.ResponseWriter, r *http.Request, kind string, id uint64) {
	var meta woo.SEOMeta
	if !decodeJSON(w, r, &meta) {
		return
	}
	ctx := r.Context()
	page, err := h.svc.resolve(ctx, seoTarget{Kind: kind, ID: id, Meta: &meta}, lngOf(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	seoScore := page.Analysis.SEO.Score
	readScore := page.Analysis.Readability.Score
	var sp, rp *int
	if seoScore >= 0 {
		sp = &seoScore
	}
	if readScore >= 0 {
		rp = &readScore
	}
	if kind == seoanalysis.KindProduct {
		_, err = h.svc.woo.SaveProductSEO(ctx, id, meta, sp, rp)
	} else {
		_, err = h.svc.woo.SaveTermSEO(ctx, taxonomyOf(kind), id, meta, sp, rp)
	}
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditSEOUpdated, map[bool]string{true: models.AuditSubjectProduct, false: models.AuditSubjectTerm}[kind == seoanalysis.KindProduct],
		sid(id), map[string]any{"focusKeyphrase": meta.FocusKeyphrase, "seoScore": seoScore, "readabilityScore": readScore})
	h.get(w, r, seoTarget{Kind: kind, ID: id})
}

// seoRow is one line of the SEO overview.
type seoRow struct {
	ID               uint64 `json:"id"`
	Kind             string `json:"kind"`
	Name             string `json:"name"`
	Path             string `json:"path"`
	Status           string `json:"status,omitempty"`
	FocusKeyphrase   string `json:"focusKeyphrase"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	DescriptionAuto  bool   `json:"descriptionIsAuto"`
	NoIndex          bool   `json:"noIndex"`
	Cornerstone      bool   `json:"cornerstone"`
	SEOScore         int    `json:"seoScore"`
	SEORating        string `json:"seoRating"`
	ReadabilityScore int    `json:"readabilityScore"`
	ReadabilityRate  string `json:"readabilityRating"`
}

func rowOf(p seoPage) seoRow {
	return seoRow{ID: p.ID, Kind: p.Kind, Name: p.Name, Path: p.Path, FocusKeyphrase: p.Meta.FocusKeyphrase, Title: p.Head.Title,
		Description: p.Head.Description, DescriptionAuto: p.Head.DescriptionIsAuto, NoIndex: p.Head.NoIndex, Cornerstone: p.Meta.Cornerstone,
		SEOScore: p.Analysis.SEO.Score, SEORating: p.Analysis.SEO.Rating,
		ReadabilityScore: p.Analysis.Readability.Score, ReadabilityRate: p.Analysis.Readability.Rating}
}

// Overview is GET /v1/admin/seo/overview?type=product|product_cat|product_tag|pages
// &search=&rating=good|ok|bad|na&page=&perPage=: the SEO / readability
// columns, live (the stored scores may be stale).
func (h *SEOHandler) Overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	lng := lngOf(r)
	search := strings.TrimSpace(q.Get("search"))
	page, perPage := queryInt(r, "page", 1), queryInt(r, "perPage", 20)
	typ := q.Get("type")
	switch typ {
	case woo.TaxCategory, woo.TaxTag:
		terms, err := h.svc.woo.Terms(ctx, typ)
		if err != nil {
			writeWooError(w, err)
			return
		}
		filtered := []woo.Term{}
		for _, t := range terms {
			if search == "" || strings.Contains(strings.ToLower(t.Name), strings.ToLower(search)) {
				filtered = append(filtered, t)
			}
		}
		page, perPage, offset := woo.Page(page, perPage)
		rows := []seoRow{}
		for i := offset; i < len(filtered) && i < offset+perPage; i++ {
			p, err := h.svc.resolve(ctx, seoTarget{Kind: kindOf(typ), ID: filtered[i].ID}, lng)
			if err == nil {
				rows = append(rows, rowOf(p))
			}
		}
		writeJSON(w, http.StatusOK, woo.NewPaged(rows, int64(len(filtered)), page, perPage))
	case "pages":
		rows := []seoRow{}
		for _, k := range []string{seoanalysis.KindHome, seoanalysis.KindShop} {
			if p, err := h.svc.resolve(ctx, seoTarget{Kind: k}, lng); err == nil {
				rows = append(rows, rowOf(p))
			}
		}
		writeJSON(w, http.StatusOK, woo.NewPaged(rows, int64(len(rows)), 1, len(rows)))
	default:
		res, err := h.svc.woo.Products(ctx, woo.ProductQuery{Search: search, Status: q.Get("status"), Page: page, PerPage: perPage})
		if err != nil {
			writeWooError(w, err)
			return
		}
		rows := []seoRow{}
		for _, prod := range res.Items {
			p, err := h.svc.resolve(ctx, seoTarget{Kind: seoanalysis.KindProduct, ID: prod.ID}, lng)
			if err == nil {
				row := rowOf(p)
				row.Status = prod.Status
				rows = append(rows, row)
			}
		}
		writeJSON(w, http.StatusOK, woo.NewPaged(rows, res.Total, res.Page, res.PerPage))
	}
}

// Head is GET /v1/seo/head?path= (public): the storefront page's resolved
// head, for the UI to set the same tags as the server-rendered ones.
func (h *SEOHandler) Head(w http.ResponseWriter, r *http.Request) {
	page, err := h.svc.resolve(r.Context(), seoTarget{Path: r.URL.Query().Get("path"), headOnly: true}, "")
	if err != nil {
		writeWooError(w, err)
		return
	}
	head := page.Head
	head.FocusKeyphrase = ""
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, head)
}
