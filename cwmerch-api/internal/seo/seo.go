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
	h := Head{Title: shop, Description: tagline, Canonical: r.publicUI + path, Image: logo, Type: "website", Lang: lang}
	org := map[string]any{"@context": "https://schema.org", "@type": "Organization", "name": shop, "url": r.publicUI + "/"}
	if logo != "" {
		org["logo"] = logo
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
		h.JSONLD = []any{org, map[string]any{"@context": "https://schema.org", "@type": "WebSite", "name": shop, "url": r.publicUI + "/",
			"potentialAction": map[string]any{"@type": "SearchAction", "target": r.publicUI + pl.ShopPath() + "?search={search_term_string}", "query-input": "required name=search_term_string"}}}
		if len(app.Banner.Slides) > 0 && app.Banner.Slides[0].Image != "" && h.Image == "" {
			h.Image = app.Banner.Slides[0].Image
		}
	case kind == "shop":
		h.Title = titleOf("Shop", shop)
		switch lang {
		case "fr":
			h.Title = titleOf("Boutique", shop)
		case "ar":
			h.Title = titleOf("المتجر", shop)
		}
		h.Canonical = r.publicUI + pl.ShopPath()
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
		h.Title = titleOf(t.Name, shop)
		if t.Description != "" {
			h.Description = PlainText(t.Description, 160)
		}
		if t.Image != nil {
			h.Image = t.Image.Src
		}
		h.Canonical = r.publicUI + t.Path
		h.JSONLD = []any{crumbs([2]string{shop, "/"}, [2]string{t.Name, t.Path})}
	case kind == "product":
		p, err := r.woo.PublicProduct(ctx, ref)
		if err != nil {
			h.NoIndex, h.Title = true, titleOf("Not found", shop)
			break
		}
		h.Title = titleOf(p.Name, shop)
		desc := p.ShortDescription
		if strings.TrimSpace(PlainText(desc, 10)) == "" {
			desc = p.Description
		}
		h.Description = PlainText(desc, 160)
		h.Type = "product"
		h.Canonical = r.publicUI + p.Path
		if len(p.Images) > 0 {
			h.Image = p.Images[0].Src
		}
		h.NoIndex = p.CatalogVisibility == "hidden"
		h.JSONLD = []any{r.productLD(ctx, p), r.crumbsFor(ctx, p, shop)}
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

func (r *Renderer) crumbsFor(ctx context.Context, p woo.Product, shop string) map[string]any {
	base := r.publicUI
	list := []any{map[string]any{"@type": "ListItem", "position": 1, "name": shop, "item": base + "/"}}
	if len(p.Categories) > 0 {
		if t, err := r.woo.Term(ctx, woo.TaxCategory, p.Categories[0].ID); err == nil {
			list = append(list, map[string]any{"@type": "ListItem", "position": 2, "name": t.Name, "item": base + t.Path})
		}
	}
	list = append(list, map[string]any{"@type": "ListItem", "position": len(list) + 1, "name": p.Name, "item": base + p.Path})
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

// RenderHead is the HTML of a head.
func RenderHead(h Head) string {
	e := html.EscapeString
	var b strings.Builder
	b.WriteString("<title>" + e(h.Title) + "</title>\n")
	b.WriteString(`<meta name="description" content="` + e(h.Description) + `">` + "\n")
	b.WriteString(`<link rel="canonical" href="` + e(h.Canonical) + `">` + "\n")
	if h.NoIndex {
		b.WriteString(`<meta name="robots" content="noindex,follow">` + "\n")
	} else {
		b.WriteString(`<meta name="robots" content="index,follow,max-image-preview:large">` + "\n")
	}
	meta := func(attr, k, v string) {
		if v != "" {
			b.WriteString(`<meta ` + attr + `="` + e(k) + `" content="` + e(v) + `">` + "\n")
		}
	}
	meta("property", "og:type", h.Type)
	meta("property", "og:title", h.Title)
	meta("property", "og:description", h.Description)
	meta("property", "og:url", h.Canonical)
	meta("property", "og:image", h.Image)
	meta("property", "og:locale", map[string]string{"fr": "fr_FR", "en": "en_US", "ar": "ar_AR"}[h.Lang])
	card := "summary"
	if h.Image != "" {
		card = "summary_large_image"
	}
	meta("name", "twitter:card", card)
	meta("name", "twitter:title", h.Title)
	meta("name", "twitter:description", h.Description)
	meta("name", "twitter:image", h.Image)
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
	pl := r.woo.Permalinks(ctx)
	set.URLs = append(set.URLs, sURL{Loc: r.publicUI + "/"}, sURL{Loc: r.publicUI + pl.ShopPath()})
	for _, tax := range []string{woo.TaxCategory, woo.TaxTag} {
		terms, err := r.woo.Terms(ctx, tax)
		if err != nil {
			return nil, err
		}
		for _, t := range terms {
			if t.Count > 0 && t.Path != "" {
				set.URLs = append(set.URLs, sURL{Loc: r.publicUI + t.Path})
			}
		}
	}
	for page := 1; page <= 500; page++ {
		res, err := r.woo.Products(ctx, woo.ProductQuery{Public: true, Page: page, PerPage: 100, OrderBy: "id"})
		if err != nil {
			return nil, err
		}
		for _, p := range res.Items {
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
