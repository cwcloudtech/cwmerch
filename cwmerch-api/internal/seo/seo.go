// Package seo renders the storefront's <head> on the server for crawlers and
// link previews: the UI is a single-page app, so without this every product
// would share one title. GET /v1/seo/html?path=... (public) answers the UI's
// own index.html, fetched from its public URL (CWMERCH_UI_URL), with the region
// between <!--cwmerch-seo-start--> and <!--cwmerch-seo-end--> replaced. The
// UI's nginx loads it under the UI's address through the public API URL
// (CWMERCH_API_URL).
package seo

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"cwmerch-api/internal/woo"
)

// Renderer builds pages' head tags from the shop data.
type Renderer struct {
	woo        *woo.Store
	uiURL      string // the UI's public URL, where the API fetches index.html
	publicUI   string // the shop's public URL (canonical links)
	client     *http.Client
	mu         sync.Mutex
	template   string
	templateAt time.Time
}

func New(w *woo.Store, uiURL, publicUIURL string) *Renderer {
	return &Renderer{woo: w, uiURL: strings.TrimRight(uiURL, "/"), publicUI: strings.TrimRight(publicUIURL, "/"),
		client: &http.Client{Timeout: 5 * time.Second}}
}

// Head is what a page declares.
type Head struct {
	Title       string
	Description string
	Canonical   string
	Image       string
	Type        string
	NoIndex     bool
	JSONLD      []any
	Lang        string
	Extra       map[string]string // og:price:amount...
	// SEO is the page's resolved SEO head (robots, social...), when it has one.
	SEO *woo.SEOHead
}

func (h *Head) apply(s woo.SEOHead) {
	h.SEO = &s
	h.Title, h.Description, h.Canonical, h.NoIndex = s.Title, s.Description, s.Canonical, s.NoIndex
	if s.OGImage != "" {
		h.Image = s.OGImage
	}
}

var tags = regexp.MustCompile(`<[^>]+>`)
var spaces = regexp.MustCompile(`\s+`)

// PlainText strips HTML and cuts at max runes on a word boundary.
func PlainText(s string, max int) string {
	t := strings.TrimSpace(spaces.ReplaceAllString(html.UnescapeString(tags.ReplaceAllString(s, " ")), " "))
	r := []rune(t)
	if len(r) <= max {
		return t
	}
	cut := string(r[:max])
	if i := strings.LastIndex(cut, " "); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:") + "…"
}

// Page computes the head of a storefront path.
func (r *Renderer) Page(ctx context.Context, rawPath string) Head {
	u, _ := url.Parse(rawPath)
	path := "/"
	if u != nil && u.Path != "" {
		path = u.Path
	}
	shop := r.woo.Option(ctx, "blogname")
	tagline := r.woo.Option(ctx, "blogdescription")
	app, _ := r.woo.Appearance(ctx, false)
	logo := ""
	if app.Logo != nil {
		logo = app.Logo.Src
	}
	lang := r.woo.Lang(ctx)
	set := r.woo.SEOSettings(ctx)
	if set.WebsiteName != "" {
		shop = set.WebsiteName
	}
	h := Head{Title: shop, Description: tagline, Canonical: r.publicUI + path, Image: logo, Type: "website", Lang: lang}
	org := map[string]any{"@context": "https://schema.org", "@type": "Organization", "name": firstOf(set.CompanyName, shop), "url": r.publicUI + "/"}
	if l := firstOf(set.CompanyLogo, logo); l != "" {
		org["logo"] = l
	}
	if set.CompanyOrPerson == "person" && set.PersonName != "" {
		org = map[string]any{"@context": "https://schema.org", "@type": "Person", "name": set.PersonName, "url": r.publicUI + "/"}
	}
	sameAs := []string{}
	if set.FacebookSite != "" {
		sameAs = append(sameAs, set.FacebookSite)
	}
	if set.TwitterSite != "" {
		sameAs = append(sameAs, "https://x.com/"+set.TwitterSite)
	}
	sameAs = append(sameAs, set.OtherSocialURLs...)
	if len(sameAs) > 0 {
		org["sameAs"] = sameAs
	}
	crumbs := func(items ...[2]string) map[string]any {
		list := []any{}
		for i, it := range items {
			list = append(list, map[string]any{"@type": "ListItem", "position": i + 1, "name": it[0], "item": r.publicUI + it[1]})
		}
		return map[string]any{"@context": "https://schema.org", "@type": "BreadcrumbList", "itemListElement": list}
	}
	pl := r.woo.Permalinks(ctx)
	kind, ref := pl.RouteKind(path)
	switch {
	case kind == "home":
		image := h.Image
		if len(app.Banner.Slides) > 0 && app.Banner.Slides[0].Image != "" && image == "" {
			image = app.Banner.Slides[0].Image
		}
		h.apply(r.woo.HomeHead(ctx, set, image))
		h.JSONLD = []any{org, map[string]any{"@context": "https://schema.org", "@type": "WebSite", "name": shop, "url": r.publicUI + "/",
			"potentialAction": map[string]any{"@type": "SearchAction", "target": r.publicUI + pl.ShopPath() + "?search={search_term_string}", "query-input": "required name=search_term_string"}}}
		if len(app.Banner.Slides) > 0 && app.Banner.Slides[0].Image != "" && h.Image == "" {
			h.Image = app.Banner.Slides[0].Image
		}
	case kind == "shop":
		h.apply(r.woo.ShopHead(ctx, set, pl.ShopPath()))
		if h.Description == "" {
			h.Description = tagline
		}
		h.JSONLD = []any{crumbs([2]string{shop, "/"}, [2]string{h.Title, pl.ShopPath()})}
	case kind == "category" || kind == "tag":
		tax := woo.TaxCategory
		if kind == "tag" {
			tax = woo.TaxTag
		}
		t, err := r.woo.TermBySlug(ctx, tax, ref)
		if err != nil {
			h.NoIndex = true
			break
		}
		meta, _ := r.woo.TermSEO(ctx, tax, t.ID)
		h.apply(r.woo.TermHead(ctx, set, tax, t, meta))
		if h.Description == "" {
			h.Description = tagline
		}
		h.JSONLD = []any{crumbs([2]string{shop, "/"}, [2]string{firstOf(meta.BreadcrumbTitle, t.Name), t.Path})}
	case kind == "product":
		p, err := r.woo.PublicProduct(ctx, ref)
		if err != nil {
			h.NoIndex, h.Title = true, titleOf("Not found", shop)
			break
		}
		meta, _ := r.woo.ProductSEO(ctx, p.ID)
		h.apply(r.woo.ProductHead(ctx, set, p, meta))
		h.Type = "product"
		h.JSONLD = []any{r.productLD(ctx, p), r.crumbsFor(ctx, p, shop, meta)}
		cur := r.woo.Currency(ctx)
		h.Extra = map[string]string{"product:price:amount": fmt.Sprintf("%.2f", p.Price), "product:price:currency": cur,
			"og:price:amount": fmt.Sprintf("%.2f", p.Price), "og:price:currency": cur,
			"product:availability": map[bool]string{true: "in stock", false: "out of stock"}[p.StockStatus != "outofstock"]}
	case strings.Trim(path, "/") == "contact":
		h.Title = titleOf("Contact", shop)
	default:
		// account, cart, checkout, admin, assistant...: not for search engines
		h.NoIndex = true
		h.Title = shop
	}
	if h.Description == "" {
		h.Description = h.Title
	}
	return h
}

func titleOf(page, shop string) string {
	if shop == "" {
		return page
	}
	return page + " – " + shop
}

func firstOf(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (r *Renderer) crumbsFor(ctx context.Context, p woo.Product, shop string, meta woo.SEOMeta) map[string]any {
	base := r.publicUI
	list := []any{map[string]any{"@type": "ListItem", "position": 1, "name": shop, "item": base + "/"}}
	if len(p.Categories) > 0 {
		catID := p.Categories[0].ID
		for _, c := range p.Categories {
			if c.ID == meta.PrimaryCategory {
				catID = c.ID
			}
		}
		if t, err := r.woo.Term(ctx, woo.TaxCategory, catID); err == nil {
			name := t.Name
			if tm, err := r.woo.TermSEO(ctx, woo.TaxCategory, t.ID); err == nil && tm.BreadcrumbTitle != "" {
				name = tm.BreadcrumbTitle
			}
			list = append(list, map[string]any{"@type": "ListItem", "position": 2, "name": name, "item": base + t.Path})
		}
	}
	list = append(list, map[string]any{"@type": "ListItem", "position": len(list) + 1, "name": firstOf(meta.BreadcrumbTitle, p.Name), "item": base + p.Path})
	return map[string]any{"@context": "https://schema.org", "@type": "BreadcrumbList", "itemListElement": list}
}

func (r *Renderer) productLD(ctx context.Context, p woo.Product) map[string]any {
	images := []string{}
	for _, img := range p.Images {
		images = append(images, img.Src)
	}
	availability := "https://schema.org/InStock"
	switch p.StockStatus {
	case "outofstock":
		availability = "https://schema.org/OutOfStock"
	case "onbackorder":
		availability = "https://schema.org/BackOrder"
	}
	cur := r.woo.Currency(ctx)
	url := r.publicUI + p.Path
	var offers any = map[string]any{"@type": "Offer", "price": fmt.Sprintf("%.2f", p.Price), "priceCurrency": cur, "availability": availability, "url": url}
	if p.Type == "variable" && p.PriceMin != p.PriceMax {
		offers = map[string]any{"@type": "AggregateOffer", "lowPrice": fmt.Sprintf("%.2f", p.PriceMin), "highPrice": fmt.Sprintf("%.2f", p.PriceMax),
			"priceCurrency": cur, "offerCount": len(p.Variations), "availability": availability}
	}
	ld := map[string]any{"@context": "https://schema.org", "@type": "Product", "name": p.Name, "url": url,
		"description": PlainText(p.ShortDescription+" "+p.Description, 500), "image": images, "offers": offers}
	if p.SKU != "" {
		ld["sku"] = p.SKU
	}
	if len(p.Categories) > 0 {
		ld["category"] = p.Categories[0].Name
	}
	if p.RatingCount > 0 {
		ld["aggregateRating"] = map[string]any{"@type": "AggregateRating", "ratingValue": fmt.Sprintf("%.2f", p.AverageRating), "reviewCount": p.RatingCount}
	}
	return ld
}

// Social is a network's card.
type Social struct {
	Enabled                   bool
	Title, Description, Image string
}

// SocialOf is the Open Graph and X cards of a head.
func SocialOf(h Head) (Social, Social) {
	og := Social{true, h.Title, h.Description, h.Image}
	tw := og
	if s := h.SEO; s != nil {
		og = Social{s.OpenGraph, s.OGTitle, s.OGDescription, s.OGImage}
		tw = Social{s.Twitter, s.TwitterTitle, s.TwitterDescription, s.TwitterImage}
	}
	return og, tw
}

// RenderHead is the HTML of a head.
func RenderHead(h Head) string {
	e := html.EscapeString
	var b strings.Builder
	b.WriteString("<title>" + e(h.Title) + "</title>\n")
	b.WriteString(`<meta name="description" content="` + e(h.Description) + `">` + "\n")
	if !h.NoIndex {
		b.WriteString(`<link rel="canonical" href="` + e(h.Canonical) + `">` + "\n")
	}
	switch {
	case h.SEO != nil:
		b.WriteString(`<meta name="robots" content="` + e(h.SEO.Robots) + `">` + "\n")
	case h.NoIndex:
		b.WriteString(`<meta name="robots" content="noindex,follow">` + "\n")
	default:
		b.WriteString(`<meta name="robots" content="index,follow,max-image-preview:large">` + "\n")
	}
	meta := func(attr, k, v string) {
		if v != "" {
			b.WriteString(`<meta ` + attr + `="` + e(k) + `" content="` + e(v) + `">` + "\n")
		}
	}
	og, tw := SocialOf(h)
	if og.Enabled {
		meta("property", "og:type", h.Type)
		meta("property", "og:title", og.Title)
		meta("property", "og:description", og.Description)
		meta("property", "og:url", h.Canonical)
		meta("property", "og:image", og.Image)
		meta("property", "og:locale", map[string]string{"fr": "fr_FR", "en": "en_US", "ar": "ar_AR"}[h.Lang])
	}
	if tw.Enabled {
		card := "summary"
		if tw.Image != "" {
			card = "summary_large_image"
		}
		meta("name", "twitter:card", card)
		meta("name", "twitter:title", tw.Title)
		meta("name", "twitter:description", tw.Description)
		meta("name", "twitter:image", tw.Image)
		if h.SEO != nil && h.SEO.TwitterSite != "" {
			meta("name", "twitter:site", "@"+h.SEO.TwitterSite)
		}
	}
	for k, v := range h.Extra {
		meta("property", k, v)
	}
	for _, ld := range h.JSONLD {
		raw, err := json.Marshal(ld)
		if err != nil {
			continue
		}
		// keep "</script>" out of the JSON
		b.WriteString(`<script type="application/ld+json">` + strings.ReplaceAll(string(raw), "</", `<\/`) + "</script>\n")
	}
	return b.String()
}

const startMarker, endMarker = "<!--cwmerch-seo-start-->", "<!--cwmerch-seo-end-->"

var htmlLang = regexp.MustCompile(`<html([^>]*)\slang="[^"]*"`)

// Inject puts the head into the index.html template.
func Inject(template string, h Head) string {
	head := startMarker + "\n" + RenderHead(h) + endMarker
	if i, j := strings.Index(template, startMarker), strings.Index(template, endMarker); i >= 0 && j > i {
		template = template[:i] + head + template[j+len(endMarker):]
	} else {
		template = strings.Replace(template, "</head>", head+"\n</head>", 1)
	}
	if h.Lang != "" {
		if htmlLang.MatchString(template) {
			template = htmlLang.ReplaceAllString(template, `<html$1 lang="`+h.Lang+`"`)
		} else {
			template = strings.Replace(template, "<html", `<html lang="`+h.Lang+`"`, 1)
		}
		if h.Lang == "ar" && !strings.Contains(template, `dir="rtl"`) {
			template = strings.Replace(template, "<html", `<html dir="rtl"`, 1)
		}
	}
	return template
}

// Template fetches the UI's index.html from its public URL (cached a minute).
// nginx serves /index.html as is (never proxied back here), so no loop.
func (r *Renderer) Template(ctx context.Context) (string, error) {
	r.mu.Lock()
	if r.template != "" && time.Since(r.templateAt) < time.Minute {
		defer r.mu.Unlock()
		return r.template, nil
	}
	r.mu.Unlock()
	if r.uiURL == "" {
		return "", fmt.Errorf("CWMERCH_UI_URL is not set")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.uiURL+"/index.html", nil)
	if err != nil {
		return "", err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("index.html answered %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	r.template, r.templateAt = string(raw), time.Now()
	r.mu.Unlock()
	return string(raw), nil
}

type urlset struct {
	XMLName xml.Name `xml:"urlset"`
	NS      string   `xml:"xmlns,attr"`
	URLs    []sURL   `xml:"url"`
}

type sURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

// Sitemap lists the home, shop, categories, tags and every visible product.
func (r *Renderer) Sitemap(ctx context.Context) ([]byte, error) {
	set := urlset{NS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	seo := r.woo.SEOSettings(ctx)
	pl := r.woo.Permalinks(ctx)
	set.URLs = append(set.URLs, sURL{Loc: r.publicUI + "/"}, sURL{Loc: r.publicUI + pl.ShopPath()})
	for _, tax := range []string{woo.TaxCategory, woo.TaxTag} {
		terms, err := r.woo.Terms(ctx, tax)
		if err != nil {
			return nil, err
		}
		for _, t := range terms {
			meta, _ := r.woo.TermSEO(ctx, tax, t.ID)
			if t.Count > 0 && t.Path != "" && woo.TermIndexable(seo, tax, meta) {
				set.URLs = append(set.URLs, sURL{Loc: r.publicUI + t.Path})
			}
		}
	}
	for page := 1; page <= 500; page++ {
		res, err := r.woo.Products(ctx, woo.ProductQuery{Public: true, Page: page, PerPage: 100, OrderBy: "id"})
		if err != nil {
			return nil, err
		}
		ids := make([]uint64, len(res.Items))
		for i, p := range res.Items {
			ids[i] = p.ID
		}
		metas, err := r.woo.ProductsSEO(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, p := range res.Items {
			if !woo.ProductIndexable(seo, p, metas[p.ID]) {
				continue
			}
			set.URLs = append(set.URLs, sURL{Loc: r.publicUI + p.Path, LastMod: p.UpdatedAt.Format("2006-01-02")})
		}
		if page >= res.TotalPages {
			break
		}
	}
	out, err := xml.MarshalIndent(set, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

// Robots is robots.txt.
func (r *Renderer) Robots() string {
	return strings.Join([]string{"User-agent: *", "Disallow: /admin", "Disallow: /my-account", "Disallow: /cart", "Disallow: /checkout",
		"Disallow: /assistant", "Allow: /", "", "Sitemap: " + r.publicUI + "/sitemap.xml", ""}, "\n")
}
