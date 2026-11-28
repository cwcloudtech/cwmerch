package woo

// SEO metadata, the cwmerch way: the same data where the most common
// WordPress SEO plugin keeps it, so a WordPress running alongside and cwmerch read and write the same
// values, and nothing is added to the database schema.
//
//   - products: postmeta _yoast_wpseo_* (focuskw, title, metadesc,
//     meta-robots-noindex/nofollow/adv, canonical, bctitle, opengraph-*,
//     twitter-*, is_cornerstone, primary_product_cat, linkdex, content_score)
//   - product categories / tags: the option wpseo_taxonomy_meta
//     ([taxonomy][term_id][wpseo_*])
//   - search appearance: the options wpseo_titles (templates with %%vars%%,
//     separator, noindex per content type, site representation) and
//     wpseo_social (Open Graph, X, profiles)
//
// Unknown keys of those options are kept as they are on save.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cwmerch-api/internal/php"
)

const (
	wpseoPrefix = "_yoast_wpseo_"
	// WPSEOTaxonomyMeta is the option holding the terms' SEO data.
	WPSEOTaxonomyMeta = "wpseo_taxonomy_meta"
	wpseoTitles       = "wpseo_titles"
	wpseoSocial       = "wpseo_social"
)

// SEO robots choices ("" = the content type's default).
const (
	RobotsDefault = ""
	RobotsIndex   = "index"
	RobotsNoIndex = "noindex"
)

// SEOMeta is one product's or term's SEO data (the SEO metabox).
type SEOMeta struct {
	FocusKeyphrase string `json:"focusKeyphrase"`
	// Title and Description may hold %%variables%% (empty = the template of
	// the content type, from the search appearance settings).
	Title       string `json:"title"`
	Description string `json:"description"`
	// NoIndex: "" (content type default), "index" or "noindex".
	NoIndex  string `json:"noIndex"`
	NoFollow bool   `json:"noFollow"`
	// Advanced robots: noimageindex, noarchive, nosnippet.
	Advanced           []string `json:"advanced"`
	Canonical          string   `json:"canonical"`
	BreadcrumbTitle    string   `json:"breadcrumbTitle"`
	OGTitle            string   `json:"ogTitle"`
	OGDescription      string   `json:"ogDescription"`
	OGImage            string   `json:"ogImage"`
	TwitterTitle       string   `json:"twitterTitle"`
	TwitterDescription string   `json:"twitterDescription"`
	TwitterImage       string   `json:"twitterImage"`
	Cornerstone        bool     `json:"cornerstone"`
	// PrimaryCategory is a product's main category (%%primary_category%%,
	// breadcrumbs); 0 = the first one.
	PrimaryCategory uint64 `json:"primaryCategory"`
	// Scores (0-100) of the last analysis, as stored in the postmeta (read only:
	// computed when the SEO data is saved). Nil = never analysed.
	SEOScore         *int `json:"seoScore"`
	ReadabilityScore *int `json:"readabilityScore"`
}

var robotsAdvanced = map[string]bool{"noimageindex": true, "noarchive": true, "nosnippet": true}

func (m *SEOMeta) clean() {
	m.FocusKeyphrase = strings.TrimSpace(m.FocusKeyphrase)
	m.Title = strings.TrimSpace(m.Title)
	m.Description = strings.TrimSpace(m.Description)
	m.Canonical = strings.TrimSpace(m.Canonical)
	m.BreadcrumbTitle = strings.TrimSpace(m.BreadcrumbTitle)
	for _, p := range []*string{&m.OGTitle, &m.OGDescription, &m.OGImage, &m.TwitterTitle, &m.TwitterDescription, &m.TwitterImage} {
		*p = strings.TrimSpace(*p)
	}
	if m.NoIndex != RobotsIndex && m.NoIndex != RobotsNoIndex {
		m.NoIndex = RobotsDefault
	}
	adv := []string{}
	for _, a := range m.Advanced {
		a = strings.ToLower(strings.TrimSpace(a))
		if robotsAdvanced[a] && !hasString(adv, a) {
			adv = append(adv, a)
		}
	}
	m.Advanced = adv
}

func hasString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func scorePtr(v string) *int {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &n
}

func splitAdvanced(v string) []string {
	out := []string{}
	for _, a := range strings.Split(v, ",") {
		a = strings.TrimSpace(a)
		if robotsAdvanced[a] {
			out = append(out, a)
		}
	}
	return out
}

// seoFromPostMeta reads the SEO keys of a product's meta.
func seoFromPostMeta(m Meta) SEOMeta {
	g := func(k string) string { return m[wpseoPrefix+k] }
	s := SEOMeta{
		FocusKeyphrase: g("focuskw"), Title: g("title"), Description: g("metadesc"),
		NoFollow: g("meta-robots-nofollow") == "1", Advanced: splitAdvanced(g("meta-robots-adv")),
		Canonical: g("canonical"), BreadcrumbTitle: g("bctitle"),
		OGTitle: g("opengraph-title"), OGDescription: g("opengraph-description"), OGImage: g("opengraph-image"),
		TwitterTitle: g("twitter-title"), TwitterDescription: g("twitter-description"), TwitterImage: g("twitter-image"),
		Cornerstone: g("is_cornerstone") == "1", PrimaryCategory: ParseID(g("primary_product_cat")),
		SEOScore: scorePtr(g("linkdex")), ReadabilityScore: scorePtr(g("content_score")),
	}
	switch g("meta-robots-noindex") {
	case "1":
		s.NoIndex = RobotsNoIndex
	case "2":
		s.NoIndex = RobotsIndex
	}
	return s
}

// ProductSEO is a product's SEO data.
func (s *Store) ProductSEO(ctx context.Context, id uint64) (SEOMeta, error) {
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.T("posts")+` WHERE ID = ? AND post_type = 'product'`, id).Scan(&n); err != nil {
		return SEOMeta{}, err
	}
	if n == 0 {
		return SEOMeta{}, ErrNotFound
	}
	m, err := s.postMeta(ctx, s.DB, id)
	if err != nil {
		return SEOMeta{}, err
	}
	return seoFromPostMeta(m), nil
}

// SaveProductSEO writes a product's SEO data (an empty value deletes the key,
// like WordPress). seoScore/readabilityScore are the analysis' scores (nil = keep).
func (s *Store) SaveProductSEO(ctx context.Context, id uint64, in SEOMeta, seoScore, readabilityScore *int) (SEOMeta, error) {
	if _, err := s.ProductSEO(ctx, id); err != nil {
		return SEOMeta{}, err
	}
	in.clean()
	noindex := map[string]string{RobotsDefault: "", RobotsNoIndex: "1", RobotsIndex: "2"}[in.NoIndex]
	values := map[string]string{
		"focuskw": in.FocusKeyphrase, "title": in.Title, "metadesc": in.Description,
		"meta-robots-noindex": noindex, "meta-robots-nofollow": map[bool]string{true: "1"}[in.NoFollow],
		"meta-robots-adv": strings.Join(in.Advanced, ","), "canonical": in.Canonical, "bctitle": in.BreadcrumbTitle,
		"opengraph-title": in.OGTitle, "opengraph-description": in.OGDescription, "opengraph-image": in.OGImage,
		"twitter-title": in.TwitterTitle, "twitter-description": in.TwitterDescription, "twitter-image": in.TwitterImage,
		"is_cornerstone":      map[bool]string{true: "1"}[in.Cornerstone],
		"primary_product_cat": map[bool]string{true: strconv.FormatUint(in.PrimaryCategory, 10)}[in.PrimaryCategory > 0],
	}
	if seoScore != nil {
		values["linkdex"] = strconv.Itoa(*seoScore)
	}
	if readabilityScore != nil {
		values["content_score"] = strconv.Itoa(*readabilityScore)
	}
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		for k, v := range values {
			var err error
			if v == "" {
				err = s.deletePostMeta(ctx, tx, id, wpseoPrefix+k)
			} else {
				err = s.setPostMeta(ctx, tx, id, wpseoPrefix+k, v)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return SEOMeta{}, err
	}
	saved, err := s.ProductSEO(ctx, id)
	if err == nil {
		s.syncSEOIndexable(ctx, "post", id, saved)
	}
	return saved, err
}

// FocusKeyphraseUses counts the other products and terms using kw as their
// focus keyphrase ("previously used keyphrase").
func (s *Store) FocusKeyphraseUses(ctx context.Context, kw string, exceptProduct uint64, exceptTax string, exceptTerm uint64) ([]string, error) {
	kw = strings.ToLower(strings.TrimSpace(kw))
	if kw == "" {
		return nil, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT p.ID, p.post_title FROM `+s.T("postmeta")+` m JOIN `+s.T("posts")+` p ON p.ID = m.post_id
		WHERE m.meta_key = ? AND LOWER(m.meta_value) = ? AND p.ID <> ? AND p.post_status IN ('publish', 'draft', 'pending', 'private', 'future')
		ORDER BY p.ID LIMIT 20`, wpseoPrefix+"focuskw", kw, exceptProduct)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id uint64
		var title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, err
		}
		out = append(out, title)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	all := s.OptionArray(ctx, WPSEOTaxonomyMeta)
	for _, tax := range []string{TaxCategory, TaxTag} {
		byTerm := all.Arr(tax)
		if byTerm == nil {
			continue
		}
		for _, k := range byTerm.Keys {
			data, _ := byTerm.Get(k).(*php.Array)
			if data == nil || strings.ToLower(strings.TrimSpace(data.String("wpseo_focuskw"))) != kw {
				continue
			}
			if tax == exceptTax && ParseID(php.ToString(k)) == exceptTerm {
				continue
			}
			if t, err := s.Term(ctx, tax, ParseID(php.ToString(k))); err == nil {
				out = append(out, t.Name)
			}
		}
	}
	return out, nil
}

// ------------------------------------------------------------------ terms

var termSEOKeys = map[string]string{
	"focusKeyphrase": "wpseo_focuskw", "title": "wpseo_title", "description": "wpseo_desc", "canonical": "wpseo_canonical",
	"breadcrumbTitle": "wpseo_bctitle", "ogTitle": "wpseo_opengraph-title", "ogDescription": "wpseo_opengraph-description",
	"ogImage": "wpseo_opengraph-image", "twitterTitle": "wpseo_twitter-title", "twitterDescription": "wpseo_twitter-description",
	"twitterImage": "wpseo_twitter-image",
}

// TermSEO is a product category's or tag's SEO data.
func (s *Store) TermSEO(ctx context.Context, taxonomy string, id uint64) (SEOMeta, error) {
	if _, err := s.Term(ctx, taxonomy, id); err != nil {
		return SEOMeta{}, err
	}
	data := s.OptionArray(ctx, WPSEOTaxonomyMeta).Arr(taxonomy).Arr(int64(id))
	g := func(k string) string { return data.String(k) }
	m := SEOMeta{
		FocusKeyphrase: g("wpseo_focuskw"), Title: g("wpseo_title"), Description: g("wpseo_desc"), Canonical: g("wpseo_canonical"),
		BreadcrumbTitle: g("wpseo_bctitle"), OGTitle: g("wpseo_opengraph-title"), OGDescription: g("wpseo_opengraph-description"),
		OGImage: g("wpseo_opengraph-image"), TwitterTitle: g("wpseo_twitter-title"), TwitterDescription: g("wpseo_twitter-description"),
		TwitterImage: g("wpseo_twitter-image"), Cornerstone: g("wpseo_is_cornerstone") == "1",
		SEOScore: scorePtr(g("wpseo_linkdex")), ReadabilityScore: scorePtr(g("wpseo_content_score")), Advanced: []string{},
	}
	switch g("wpseo_noindex") {
	case "noindex":
		m.NoIndex = RobotsNoIndex
	case "index":
		m.NoIndex = RobotsIndex
	}
	return m, nil
}

// SaveTermSEO writes a term's SEO data into wpseo_taxonomy_meta (the other
// terms' and keys' values are kept).
func (s *Store) SaveTermSEO(ctx context.Context, taxonomy string, id uint64, in SEOMeta, seoScore, readabilityScore *int) (SEOMeta, error) {
	if _, err := s.Term(ctx, taxonomy, id); err != nil {
		return SEOMeta{}, err
	}
	in.clean()
	all := s.OptionArray(ctx, WPSEOTaxonomyMeta)
	byTerm := all.Arr(taxonomy)
	if byTerm == nil {
		byTerm = php.NewArray()
	}
	data := byTerm.Arr(int64(id))
	if data == nil {
		data = php.NewArray()
	}
	set := func(k, v string) {
		if v == "" {
			data.Delete(k)
		} else {
			data.Set(k, v)
		}
	}
	set(termSEOKeys["focusKeyphrase"], in.FocusKeyphrase)
	set(termSEOKeys["title"], in.Title)
	set(termSEOKeys["description"], in.Description)
	set(termSEOKeys["canonical"], in.Canonical)
	set(termSEOKeys["breadcrumbTitle"], in.BreadcrumbTitle)
	set(termSEOKeys["ogTitle"], in.OGTitle)
	set(termSEOKeys["ogDescription"], in.OGDescription)
	set(termSEOKeys["ogImage"], in.OGImage)
	set(termSEOKeys["twitterTitle"], in.TwitterTitle)
	set(termSEOKeys["twitterDescription"], in.TwitterDescription)
	set(termSEOKeys["twitterImage"], in.TwitterImage)
	set("wpseo_noindex", map[string]string{RobotsDefault: "default", RobotsIndex: "index", RobotsNoIndex: "noindex"}[in.NoIndex])
	set("wpseo_is_cornerstone", map[bool]string{true: "1"}[in.Cornerstone])
	if seoScore != nil {
		set("wpseo_linkdex", strconv.Itoa(*seoScore))
	}
	if readabilityScore != nil {
		set("wpseo_content_score", strconv.Itoa(*readabilityScore))
	}
	if data.Len() == 0 {
		byTerm.Delete(int64(id))
	} else {
		byTerm.Set(int64(id), data)
	}
	all.Set(taxonomy, byTerm)
	if err := s.SetOption(ctx, WPSEOTaxonomyMeta, php.Marshal(all)); err != nil {
		return SEOMeta{}, err
	}
	saved, err := s.TermSEO(ctx, taxonomy, id)
	if err == nil {
		s.syncSEOIndexable(ctx, "term", id, saved)
	}
	return saved, err
}

// ------------------------------------------------------------------ settings

// SEOSettings is the "search appearance" (wpseo_titles) and social
// (wpseo_social) settings that matter to a shop.
type SEOSettings struct {
	// Separator is the separator id (sc-dash, sc-ndash, sc-pipe...).
	Separator     string `json:"separator"`
	WebsiteName   string `json:"websiteName"`
	AlternateName string `json:"alternateName"`
	// Templates (%%variables%%) per content type; empty description =
	// the content's own excerpt.
	HomeTitle           string `json:"homeTitle"`
	HomeDescription     string `json:"homeDescription"`
	ProductTitle        string `json:"productTitle"`
	ProductDescription  string `json:"productDescription"`
	ProductNoIndex      bool   `json:"productNoIndex"`
	ShopTitle           string `json:"shopTitle"`
	ShopDescription     string `json:"shopDescription"`
	CategoryTitle       string `json:"categoryTitle"`
	CategoryDescription string `json:"categoryDescription"`
	CategoryNoIndex     bool   `json:"categoryNoIndex"`
	TagTitle            string `json:"tagTitle"`
	TagDescription      string `json:"tagDescription"`
	TagNoIndex          bool   `json:"tagNoIndex"`
	// Site representation (schema.org Organization or Person).
	CompanyOrPerson string `json:"companyOrPerson"`
	CompanyName     string `json:"companyName"`
	CompanyLogo     string `json:"companyLogo"`
	PersonName      string `json:"personName"`
	// Social.
	OpenGraph       bool     `json:"openGraph"`
	Twitter         bool     `json:"twitter"`
	TwitterSite     string   `json:"twitterSite"`
	FacebookSite    string   `json:"facebookSite"`
	OtherSocialURLs []string `json:"otherSocialUrls"`
	OGDefaultImage  string   `json:"ogDefaultImage"`
	OGHomeTitle     string   `json:"ogHomeTitle"`
	OGHomeDesc      string   `json:"ogHomeDescription"`
	OGHomeImage     string   `json:"ogHomeImage"`
}

// SEOSeparators are the title separators.
var SEOSeparators = map[string]string{
	"sc-dash": "-", "sc-ndash": "–", "sc-mdash": "—", "sc-colon": ":", "sc-middot": "·", "sc-bull": "•", "sc-star": "*",
	"sc-smstar": "⋆", "sc-pipe": "|", "sc-tilde": "~", "sc-laquo": "«", "sc-raquo": "»", "sc-lt": "<", "sc-gt": ">",
}

// SEOSeparatorIDs is their order in the settings screen.
var SEOSeparatorIDs = []string{"sc-dash", "sc-ndash", "sc-mdash", "sc-colon", "sc-middot", "sc-bull", "sc-star", "sc-smstar",
	"sc-pipe", "sc-tilde", "sc-laquo", "sc-raquo", "sc-lt", "sc-gt"}

// The default templates.
const (
	defHomeTitle     = "%%sitename%% %%page%% %%sep%% %%sitedesc%%"
	defProductTitle  = "%%title%% %%page%% %%sep%% %%sitename%%"
	defShopTitle     = "%%pt_plural%% %%page%% %%sep%% %%sitename%%"
	defTermTitle     = "%%term_title%% %%page%% %%sep%% %%sitename%%"
	settingsTitleKey = "title-"
)

func phpBool(v any) (bool, bool) {
	switch x := v.(type) {
	case nil:
		return false, false
	case bool:
		return x, true
	default:
		s := php.ToString(x)
		return s == "1" || s == "true" || s == "on", true
	}
}

// SEOSettings reads the search appearance settings, with the plugin's defaults
// where nothing is set.
func (s *Store) SEOSettings(ctx context.Context) SEOSettings {
	t := s.OptionArray(ctx, wpseoTitles)
	so := s.OptionArray(ctx, wpseoSocial)
	str := func(a *php.Array, k, def string) string {
		if a.Has(k) {
			return a.String(k)
		}
		return def
	}
	boolean := func(a *php.Array, k string, def bool) bool {
		if v, ok := phpBool(a.Get(k)); ok {
			return v
		}
		return def
	}
	out := SEOSettings{
		Separator:   str(t, "separator", "sc-dash"),
		WebsiteName: str(t, "website_name", ""), AlternateName: str(t, "alternate_website_name", ""),
		HomeTitle: str(t, "title-home-wpseo", defHomeTitle), HomeDescription: str(t, "metadesc-home-wpseo", ""),
		ProductTitle: str(t, "title-product", defProductTitle), ProductDescription: str(t, "metadesc-product", ""),
		ProductNoIndex: boolean(t, "noindex-product", false),
		ShopTitle:      str(t, "title-ptarchive-product", defShopTitle), ShopDescription: str(t, "metadesc-ptarchive-product", ""),
		CategoryTitle: str(t, "title-tax-product_cat", defTermTitle), CategoryDescription: str(t, "metadesc-tax-product_cat", ""),
		CategoryNoIndex: boolean(t, "noindex-tax-product_cat", false),
		TagTitle:        str(t, "title-tax-product_tag", defTermTitle), TagDescription: str(t, "metadesc-tax-product_tag", ""),
		TagNoIndex:      boolean(t, "noindex-tax-product_tag", false),
		CompanyOrPerson: str(t, "company_or_person", "company"), CompanyName: str(t, "company_name", ""),
		CompanyLogo: str(t, "company_logo", ""), PersonName: str(t, "person_name", ""),
		OpenGraph: boolean(so, "opengraph", true), Twitter: boolean(so, "twitter", true),
		TwitterSite: str(so, "twitter_site", ""), FacebookSite: str(so, "facebook_site", ""),
		OGDefaultImage: str(so, "og_default_image", ""),
		OGHomeTitle:    str(t, "open_graph_frontpage_title", ""), OGHomeDesc: str(t, "open_graph_frontpage_desc", ""),
		OGHomeImage: str(t, "open_graph_frontpage_image", ""), OtherSocialURLs: []string{},
	}
	if o := so.Arr("other_social_urls"); o != nil {
		for _, v := range o.StringList() {
			if strings.TrimSpace(v) != "" {
				out.OtherSocialURLs = append(out.OtherSocialURLs, v)
			}
		}
	}
	if _, ok := SEOSeparators[out.Separator]; !ok {
		out.Separator = "sc-dash"
	}
	if out.CompanyOrPerson != "person" {
		out.CompanyOrPerson = "company"
	}
	return out
}

// SaveSEOSettings writes them into wpseo_titles / wpseo_social, keeping the
// keys cwmerch doesn't manage.
func (s *Store) SaveSEOSettings(ctx context.Context, in SEOSettings) (SEOSettings, error) {
	if _, ok := SEOSeparators[in.Separator]; !ok {
		in.Separator = "sc-dash"
	}
	if in.CompanyOrPerson != "person" {
		in.CompanyOrPerson = "company"
	}
	t := s.OptionArray(ctx, wpseoTitles)
	for k, v := range map[string]string{
		"separator": in.Separator, "website_name": in.WebsiteName, "alternate_website_name": in.AlternateName,
		"title-home-wpseo": in.HomeTitle, "metadesc-home-wpseo": in.HomeDescription,
		"title-product": in.ProductTitle, "metadesc-product": in.ProductDescription,
		"title-ptarchive-product": in.ShopTitle, "metadesc-ptarchive-product": in.ShopDescription,
		"title-tax-product_cat": in.CategoryTitle, "metadesc-tax-product_cat": in.CategoryDescription,
		"title-tax-product_tag": in.TagTitle, "metadesc-tax-product_tag": in.TagDescription,
		"company_or_person": in.CompanyOrPerson, "company_name": in.CompanyName, "company_logo": in.CompanyLogo,
		"person_name":                in.PersonName,
		"open_graph_frontpage_title": in.OGHomeTitle, "open_graph_frontpage_desc": in.OGHomeDesc, "open_graph_frontpage_image": in.OGHomeImage,
	} {
		t.Set(k, strings.TrimSpace(v))
	}
	t.Set("noindex-product", in.ProductNoIndex)
	t.Set("noindex-tax-product_cat", in.CategoryNoIndex)
	t.Set("noindex-tax-product_tag", in.TagNoIndex)
	so := s.OptionArray(ctx, wpseoSocial)
	so.Set("opengraph", in.OpenGraph)
	so.Set("twitter", in.Twitter)
	so.Set("twitter_site", strings.TrimPrefix(strings.TrimSpace(in.TwitterSite), "@"))
	so.Set("facebook_site", strings.TrimSpace(in.FacebookSite))
	so.Set("og_default_image", strings.TrimSpace(in.OGDefaultImage))
	urls := []string{}
	for _, u := range in.OtherSocialURLs {
		if u = strings.TrimSpace(u); u != "" {
			urls = append(urls, u)
		}
	}
	so.Set("other_social_urls", php.FromStrings(urls))
	if err := s.SetOption(ctx, wpseoTitles, php.Marshal(t)); err != nil {
		return SEOSettings{}, err
	}
	if err := s.SetOption(ctx, wpseoSocial, php.Marshal(so)); err != nil {
		return SEOSettings{}, err
	}
	return s.SEOSettings(ctx), nil
}

// ------------------------------------------------------------------ variables

// SEOVars are the values of the %%variables%% for one page.
type SEOVars map[string]string

var seoVarRe = regexp.MustCompile(`%%([a-zA-Z0-9_]+)%%`)
var multiSpace = regexp.MustCompile(`\s{2,}`)

// ReplaceSEOVars resolves a template: known variables replaced,
// unknown ones removed, spaces and dangling separators cleaned up.
func ReplaceSEOVars(tpl string, vars SEOVars) string {
	out := seoVarRe.ReplaceAllStringFunc(tpl, func(m string) string {
		return vars[strings.ToLower(strings.Trim(m, "%"))]
	})
	out = strings.TrimSpace(multiSpace.ReplaceAllString(out, " "))
	sep := vars["sep"]
	if sep != "" {
		for {
			trimmed := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(out, sep), sep))
			if trimmed == out {
				break
			}
			out = trimmed
		}
		out = strings.ReplaceAll(out, sep+" "+sep, sep)
	}
	return out
}

// SEOVarNames lists the supported variables (the editor's "insert variable").
var SEOVarNames = []string{"title", "sitename", "sitedesc", "sep", "excerpt", "primary_category", "category", "term_title",
	"term_description", "pt_plural", "pt_single", "currentyear", "currentmonth", "currentday", "currentdate", "focuskw",
	"wc_price", "wc_sku", "wc_shortdesc", "page"}

func plain(s string, max int) string {
	t := strings.TrimSpace(multiSpace.ReplaceAllString(html.UnescapeString(tagRe.ReplaceAllString(s, " ")), " "))
	t = strings.Join(strings.Fields(t), " ")
	r := []rune(t)
	if max <= 0 || len(r) <= max {
		return t
	}
	cut := string(r[:max])
	if i := strings.LastIndex(cut, " "); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:") + "…"
}

// PlainText strips HTML and cuts at max runes (0 = no cut) on a word boundary.
func PlainText(s string, max int) string { return plain(s, max) }

func (s *Store) baseSEOVars(ctx context.Context, set SEOSettings) SEOVars {
	now := time.Now()
	site := set.WebsiteName
	if site == "" {
		site = s.Option(ctx, "blogname")
	}
	shopTitle := ""
	if id := ParseID(s.Option(ctx, "woocommerce_shop_page_id")); id > 0 {
		_ = s.DB.QueryRowContext(ctx, `SELECT post_title FROM `+s.T("posts")+` WHERE ID = ?`, id).Scan(&shopTitle)
	}
	if shopTitle == "" {
		shopTitle = map[string]string{"fr": "Boutique", "ar": "المتجر"}[s.Lang(ctx)]
		if shopTitle == "" {
			shopTitle = "Shop"
		}
	}
	return SEOVars{"sitename": site, "sitedesc": s.Option(ctx, "blogdescription"), "sep": SEOSeparators[set.Separator],
		"currentyear": strconv.Itoa(now.Year()), "currentmonth": now.Format("January"), "currentday": strconv.Itoa(now.Day()),
		"currentdate": now.Format("2006-01-02"), "pt_plural": shopTitle, "pt_single": shopTitle, "page": ""}
}

// ProductSEOVars are a product's variables (draft values may override them).
func (s *Store) ProductSEOVars(ctx context.Context, set SEOSettings, p Product, meta SEOMeta) SEOVars {
	v := s.baseSEOVars(ctx, set)
	v["title"] = p.Name
	excerpt := p.ShortDescription
	if plain(excerpt, 0) == "" {
		excerpt = p.Description
	}
	v["excerpt"] = plain(excerpt, 155)
	v["wc_shortdesc"] = plain(p.ShortDescription, 155)
	v["wc_sku"] = p.SKU
	if p.Price > 0 {
		v["wc_price"] = strconv.FormatFloat(p.Price, 'f', 2, 64) + " " + s.Currency(ctx)
	}
	v["focuskw"] = meta.FocusKeyphrase
	if len(p.Categories) > 0 {
		primary := p.Categories[0].Name
		for _, c := range p.Categories {
			if c.ID == meta.PrimaryCategory {
				primary = c.Name
			}
		}
		v["primary_category"], v["category"] = primary, primary
	}
	return v
}

// TermSEOVars are a term's variables.
func (s *Store) TermSEOVars(ctx context.Context, set SEOSettings, t Term, meta SEOMeta) SEOVars {
	v := s.baseSEOVars(ctx, set)
	v["term_title"], v["title"], v["category"] = t.Name, t.Name, t.Name
	v["term_description"] = plain(t.Description, 155)
	v["excerpt"] = v["term_description"]
	v["focuskw"] = meta.FocusKeyphrase
	return v
}

// ------------------------------------------------------------------ heads

// SEOHead is what a page declares to search engines and social networks,
// resolved from the SEO data, the templates and the content.
type SEOHead struct {
	Kind               string   `json:"kind"` // home, shop, product, category, tag
	ID                 uint64   `json:"id,omitempty"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	DescriptionIsAuto  bool     `json:"descriptionIsAuto"` // no meta description: the excerpt
	Canonical          string   `json:"canonical"`
	NoIndex            bool     `json:"noIndex"`
	NoFollow           bool     `json:"noFollow"`
	Robots             string   `json:"robots"`
	Advanced           []string `json:"advanced,omitempty"`
	OGTitle            string   `json:"ogTitle"`
	OGDescription      string   `json:"ogDescription"`
	OGImage            string   `json:"ogImage"`
	TwitterTitle       string   `json:"twitterTitle"`
	TwitterDescription string   `json:"twitterDescription"`
	TwitterImage       string   `json:"twitterImage"`
	TwitterSite        string   `json:"twitterSite,omitempty"`
	OpenGraph          bool     `json:"openGraph"`
	Twitter            bool     `json:"twitter"`
	BreadcrumbTitle    string   `json:"breadcrumbTitle,omitempty"`
	FocusKeyphrase     string   `json:"focusKeyphrase,omitempty"`
}

func (h *SEOHead) finish(set SEOSettings, meta SEOMeta, image string, vars SEOVars) {
	r := func(v string) string { return ReplaceSEOVars(v, vars) }
	h.OpenGraph, h.Twitter, h.TwitterSite = set.OpenGraph, set.Twitter, set.TwitterSite
	h.NoFollow = meta.NoFollow
	h.Advanced = meta.Advanced
	h.OGTitle = firstNonBlank(r(meta.OGTitle), h.Title)
	h.OGDescription = firstNonBlank(r(meta.OGDescription), h.Description)
	h.OGImage = firstNonBlank(meta.OGImage, image, set.OGDefaultImage)
	h.TwitterTitle = firstNonBlank(r(meta.TwitterTitle), h.OGTitle)
	h.TwitterDescription = firstNonBlank(r(meta.TwitterDescription), h.OGDescription)
	h.TwitterImage = firstNonBlank(meta.TwitterImage, h.OGImage)
	h.BreadcrumbTitle = r(meta.BreadcrumbTitle)
	h.FocusKeyphrase = meta.FocusKeyphrase
	if meta.Canonical != "" {
		h.Canonical = meta.Canonical
	}
	robots := []string{map[bool]string{true: "noindex", false: "index"}[h.NoIndex], map[bool]string{true: "nofollow", false: "follow"}[h.NoFollow]}
	if !h.NoIndex {
		robots = append(robots, "max-snippet:-1", "max-image-preview:large", "max-video-preview:-1")
	}
	h.Robots = strings.Join(append(robots, h.Advanced...), ", ")
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func resolveNoIndex(choice string, typeDefault bool) bool {
	switch choice {
	case RobotsNoIndex:
		return true
	case RobotsIndex:
		return false
	}
	return typeDefault
}

// ProductHead resolves a product's head with meta (the stored one, or a
// draft from the editor).
func (s *Store) ProductHead(ctx context.Context, set SEOSettings, p Product, meta SEOMeta) SEOHead {
	vars := s.ProductSEOVars(ctx, set, p, meta)
	h := SEOHead{Kind: "product", ID: p.ID, Canonical: s.SiteURL + p.Path}
	h.Title = ReplaceSEOVars(firstNonBlank(meta.Title, set.ProductTitle, defProductTitle), vars)
	h.Description = ReplaceSEOVars(firstNonBlank(meta.Description, set.ProductDescription), vars)
	if h.Description == "" {
		h.Description, h.DescriptionIsAuto = vars["excerpt"], true
	}
	h.NoIndex = resolveNoIndex(meta.NoIndex, set.ProductNoIndex) || p.CatalogVisibility == "hidden" || (p.Status != "" && p.Status != "publish")
	image := ""
	if len(p.Images) > 0 {
		image = p.Images[0].Src
	}
	h.finish(set, meta, image, vars)
	return h
}

// TermHead resolves a category's or tag's head.
func (s *Store) TermHead(ctx context.Context, set SEOSettings, taxonomy string, t Term, meta SEOMeta) SEOHead {
	vars := s.TermSEOVars(ctx, set, t, meta)
	tpl, desc, noindex, kind := set.CategoryTitle, set.CategoryDescription, set.CategoryNoIndex, "category"
	if taxonomy == TaxTag {
		tpl, desc, noindex, kind = set.TagTitle, set.TagDescription, set.TagNoIndex, "tag"
	}
	h := SEOHead{Kind: kind, ID: t.ID, Canonical: s.SiteURL + t.Path}
	h.Title = ReplaceSEOVars(firstNonBlank(meta.Title, tpl, defTermTitle), vars)
	h.Description = ReplaceSEOVars(firstNonBlank(meta.Description, desc), vars)
	if h.Description == "" {
		h.Description, h.DescriptionIsAuto = vars["term_description"], true
	}
	h.NoIndex = resolveNoIndex(meta.NoIndex, noindex)
	image := ""
	if t.Image != nil {
		image = t.Image.Src
	}
	h.finish(set, meta, image, vars)
	return h
}

// HomeHead resolves the home page's head.
func (s *Store) HomeHead(ctx context.Context, set SEOSettings, image string) SEOHead {
	vars := s.baseSEOVars(ctx, set)
	h := SEOHead{Kind: "home", Canonical: s.SiteURL + "/"}
	h.Title = ReplaceSEOVars(firstNonBlank(set.HomeTitle, defHomeTitle), vars)
	h.Description = ReplaceSEOVars(set.HomeDescription, vars)
	if h.Description == "" {
		h.Description, h.DescriptionIsAuto = vars["sitedesc"], true
	}
	h.finish(set, SEOMeta{OGTitle: set.OGHomeTitle, OGDescription: set.OGHomeDesc, OGImage: set.OGHomeImage}, image, vars)
	return h
}

// ShopHead resolves the shop (product archive) page's head.
func (s *Store) ShopHead(ctx context.Context, set SEOSettings, path string) SEOHead {
	vars := s.baseSEOVars(ctx, set)
	h := SEOHead{Kind: "shop", Canonical: s.SiteURL + path}
	h.Title = ReplaceSEOVars(firstNonBlank(set.ShopTitle, defShopTitle), vars)
	h.Description = ReplaceSEOVars(set.ShopDescription, vars)
	h.finish(set, SEOMeta{}, "", vars)
	return h
}

// ProductForSEO is a product by id whatever its status (the admin's view).
func (s *Store) ProductForSEO(ctx context.Context, id uint64) (Product, error) {
	p, err := s.Product(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	return p, err
}

// ProductGTIN is the product's GTIN/UPC/EAN/ISBN (WooCommerce 9.2+ field).
func (s *Store) ProductGTIN(ctx context.Context, id uint64) string {
	m, err := s.postMeta(ctx, s.DB, id)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(m["_global_unique_id"])
}

// String helps debugging.
func (h SEOHead) String() string { return fmt.Sprintf("%s #%d %q", h.Kind, h.ID, h.Title) }

// syncSEOIndexable keeps the SEO plugin's own cache (its indexable table, when the plugin is
// installed alongside) in step with what was just saved, so a WordPress
// running the plugin shows the same values without a reindex. Best effort: the
// table (or a column) may not exist when the plugin isn't installed, and the plugin
// rebuilds a missing row from the postmeta/option itself.
func (s *Store) syncSEOIndexable(ctx context.Context, objectType string, id uint64, m SEOMeta) {
	null := func(v string) any {
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return v
	}
	flag := func(on bool) any {
		if on {
			return 1
		}
		return nil
	}
	var noindex any
	switch m.NoIndex {
	case RobotsNoIndex:
		noindex = 1
	case RobotsIndex:
		noindex = 0
	}
	var seoScore, readScore any
	if m.SEOScore != nil {
		seoScore = *m.SEOScore
	}
	if m.ReadabilityScore != nil {
		readScore = *m.ReadabilityScore
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE `+s.T("yoast_indexable")+` SET title = ?, description = ?, breadcrumb_title = ?,
		is_robots_noindex = ?, is_robots_nofollow = ?, is_robots_noarchive = ?, is_robots_noimageindex = ?, is_robots_nosnippet = ?,
		primary_focus_keyword = ?, primary_focus_keyword_score = ?, readability_score = ?, is_cornerstone = ?, canonical = ?,
		open_graph_title = ?, open_graph_description = ?, open_graph_image = ?, twitter_title = ?, twitter_description = ?, twitter_image = ?
		WHERE object_id = ? AND object_type = ?`,
		null(m.Title), null(m.Description), null(m.BreadcrumbTitle), noindex, map[bool]any{true: 1, false: 0}[m.NoFollow],
		flag(hasString(m.Advanced, "noarchive")), flag(hasString(m.Advanced, "noimageindex")), flag(hasString(m.Advanced, "nosnippet")),
		null(m.FocusKeyphrase), seoScore, readScore, map[bool]int{true: 1, false: 0}[m.Cornerstone], null(m.Canonical),
		null(m.OGTitle), null(m.OGDescription), null(m.OGImage), null(m.TwitterTitle), null(m.TwitterDescription), null(m.TwitterImage),
		id, objectType)
	if err != nil && !IsMissingTable(err) {
		slog.DebugContext(ctx, "seo indexable not synced", "type", objectType, "id", id, "error", err)
	}
}

// ProductsSEO is the SEO data of several products (the sitemap's noindex).
func (s *Store) ProductsSEO(ctx context.Context, ids []uint64) (map[uint64]SEOMeta, error) {
	metas, err := s.postMetas(ctx, s.DB, ids)
	if err != nil {
		return nil, err
	}
	out := map[uint64]SEOMeta{}
	for id, m := range metas {
		out[id] = seoFromPostMeta(m)
	}
	return out, nil
}

// ProductIndexable tells whether a product goes to search engines (its own
// robots choice, else the products' default).
func ProductIndexable(set SEOSettings, p Product, m SEOMeta) bool {
	return !resolveNoIndex(m.NoIndex, set.ProductNoIndex) && p.CatalogVisibility != "hidden"
}

// TermIndexable is the same for a category or tag.
func TermIndexable(set SEOSettings, taxonomy string, m SEOMeta) bool {
	def := set.CategoryNoIndex
	if taxonomy == TaxTag {
		def = set.TagNoIndex
	}
	return !resolveNoIndex(m.NoIndex, def)
}
