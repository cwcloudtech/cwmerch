// Package seoanalysis is cwmerch's content analysis, the WordPress SEO way: the
// SEO assessments (focus keyphrase, title, meta description, slug, links,
// images, text length...) and the readability ones (sentence and paragraph
// length, subheadings, transition words, passive voice, Flesch reading ease),
// with the usual thresholds and good / ok / bad traffic lights, plus the
// WooCommerce SEO product checks (short description, SKU, GTIN, images).
//
// It is pure: the caller hands the resolved title/description and the text.
package seoanalysis

import (
	"fmt"
	"math"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Ratings.
const (
	Good = "good"
	OK   = "ok"
	Bad  = "bad"
	// NA: the assessment couldn't run (no keyphrase...).
	NA = "na"
)

// Kinds of content.
const (
	KindProduct  = "product"
	KindCategory = "category"
	KindTag      = "tag"
	KindHome     = "home"
	KindShop     = "shop"
)

// Input is what is analysed.
type Input struct {
	Kind string `json:"kind"`
	// Locale is the text's language (word lists, readability formulas);
	// MessageLocale the language of the messages (default: Locale).
	Locale        string `json:"locale"`
	MessageLocale string `json:"messageLocale"`
	// Keyphrase is the focus keyphrase ("quoted" = exact phrase).
	Keyphrase string `json:"keyphrase"`
	// Title and Description are the resolved SEO title and meta description
	// (DescriptionSet false: none written, search engines pick the text).
	Title          string `json:"title"`
	Description    string `json:"description"`
	DescriptionSet bool   `json:"descriptionSet"`
	Slug           string `json:"slug"`
	// Name is the page's H1 (the product or term name).
	Name string `json:"name"`
	// Text is the main HTML text (product description, term description).
	Text             string `json:"text"`
	ShortDescription string `json:"shortDescription"`
	// Images are the page's own images (product gallery, term image) besides
	// the ones in the text.
	Images []Image `json:"images"`
	SKU    string  `json:"sku"`
	GTIN   string  `json:"gtin"`
	// SiteURL tells internal links from outbound ones.
	SiteURL string `json:"siteUrl"`
	// KeyphraseUsedBy are the other pages with the same focus keyphrase.
	KeyphraseUsedBy []string `json:"keyphraseUsedBy"`
	Cornerstone     bool     `json:"cornerstone"`
	// TitleWidth is the SEO title's width in pixels as Google shows it; 0 = estimated.
	TitleWidth int `json:"titleWidth"`
}

// Assessment is one check.
type Assessment struct {
	ID     string `json:"id"`
	Rating string `json:"rating"`
	Score  int    `json:"score"` // 0-9 (9 good, 6 ok, 3 bad)
	Title  string `json:"title"`
	Text   string `json:"text"`
}

// Report is one analysis (SEO or readability).
type Report struct {
	// Score 0-100 (stored as linkdex / content_score); -1 when it couldn't run.
	Score       int          `json:"score"`
	Rating      string       `json:"rating"`
	Assessments []Assessment `json:"assessments"`
}

// Result is the whole analysis.
type Result struct {
	SEO         Report `json:"seo"`
	Readability Report `json:"readability"`
	WordCount   int    `json:"wordCount"`
	// Flesch is the Flesch reading ease (Kandel-Moles in French), -1 if n/a.
	Flesch float64 `json:"flesch"`
}

type assessor struct {
	in    Input
	doc   Doc
	kp    Keyphrase
	lang  string
	msg   string
	items []Assessment
}

func (a *assessor) add(id, rating string, key string, args ...any) {
	score := map[string]int{Good: 9, OK: 6, Bad: 3, NA: 0}[rating]
	title, text := message(a.msg, id, key, args...)
	a.items = append(a.items, Assessment{ID: id, Rating: rating, Score: score, Title: title, Text: text})
}

func report(items []Assessment) Report {
	r := Report{Score: -1, Rating: NA, Assessments: items}
	total, n := 0, 0
	for _, it := range items {
		if it.Rating == NA {
			continue
		}
		total += it.Score
		n++
	}
	if n == 0 {
		return r
	}
	r.Score = int(math.Round(float64(total) / float64(n*9) * 100))
	switch {
	case r.Score > 70:
		r.Rating = Good
	case r.Score > 40:
		r.Rating = OK
	default:
		r.Rating = Bad
	}
	// Bad first, then ok, then good ("Problems", "Improvements", "Good results").
	order := map[string]int{Bad: 0, OK: 1, NA: 2, Good: 3}
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && order[items[j].Rating] < order[items[j-1].Rating]; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
	return r
}

func lang(locale string) string {
	switch {
	case strings.HasPrefix(locale, "fr"):
		return "fr"
	case strings.HasPrefix(locale, "ar"):
		return "ar"
	}
	return "en"
}

// Analyze runs both analyses.
func Analyze(in Input) Result {
	a := &assessor{in: in, doc: Parse(in.Text), lang: lang(in.Locale), msg: lang(in.Locale)}
	if in.MessageLocale != "" {
		a.msg = lang(in.MessageLocale)
	}
	a.kp = NewKeyphrase(in.Keyphrase, a.lang)
	res := Result{WordCount: len(a.doc.Words), Flesch: -1}
	res.SEO = report(a.seo())
	if a.kp.Empty() && a.hasText() {
		// No focus keyphrase: the checks are listed, but there is no score.
		res.SEO.Score, res.SEO.Rating = -1, NA
	}
	a.items = nil
	items, flesch := a.readability()
	res.Readability = report(items)
	res.Flesch = flesch
	return res
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

func (a *assessor) hasText() bool { return a.in.Kind != KindHome && a.in.Kind != KindShop }

// ------------------------------------------------------------------ SEO

func (a *assessor) seo() []Assessment {
	in, kp := a.in, a.kp
	if kp.Empty() && a.hasText() {
		a.add("keyphraseLength", Bad, "none")
	} else if a.hasText() {
		n := len(kp.Content)
		switch {
		case n <= 4:
			a.add("keyphraseLength", Good, "good")
		case n <= 8:
			a.add("keyphraseLength", OK, "long", n)
		default:
			a.add("keyphraseLength", Bad, "tooLong", n)
		}
	}

	if !kp.Empty() {
		// Keyphrase in introduction.
		if a.hasText() {
			intro := a.doc.Intro
			inSentence := false
			for _, s := range SplitSentences(intro) {
				if kp.In(s) {
					inSentence = true
				}
			}
			switch {
			case inSentence:
				a.add("keyphraseIntroduction", Good, "good")
			case kp.In(intro):
				a.add("keyphraseIntroduction", OK, "notSameSentence")
			default:
				a.add("keyphraseIntroduction", Bad, "bad")
			}

			// Keyphrase density.
			words := len(a.doc.Words)
			count := kp.count(a.doc.sentenceSet)
			density := pct(count, words)
			switch {
			case count == 0:
				a.add("keyphraseDensity", Bad, "none")
			case words >= 100 && density < 0.5:
				a.add("keyphraseDensity", OK, "low", count, fmt.Sprintf("%.1f", density))
			case words >= 100 && density > 3.5:
				a.add("keyphraseDensity", Bad, "high", count, fmt.Sprintf("%.1f", density))
			case words >= 100 && density > 3:
				a.add("keyphraseDensity", OK, "high", count, fmt.Sprintf("%.1f", density))
			default:
				a.add("keyphraseDensity", Good, "good", count, fmt.Sprintf("%.1f", density))
			}
		}

		// Keyphrase in meta description.
		if in.DescriptionSet && strings.TrimSpace(in.Description) != "" {
			n := kp.count([][]string{normWords(Words(in.Description))})
			switch {
			case n == 0:
				a.add("metaDescriptionKeyphrase", Bad, "none")
			case n <= 2:
				a.add("metaDescriptionKeyphrase", Good, "good")
			default:
				a.add("metaDescriptionKeyphrase", Bad, "tooMany", n)
			}
		}

		// Keyphrase in SEO title.
		switch {
		case kp.StartsWith(in.Title):
			a.add("titleKeyphrase", Good, "good")
		case kp.In(in.Title):
			a.add("titleKeyphrase", OK, "notAtStart")
		default:
			a.add("titleKeyphrase", Bad, "bad")
		}

		// Keyphrase in slug.
		if in.Slug != "" && a.hasText() {
			slug, _ := url.PathUnescape(in.Slug)
			if kp.In(strings.ReplaceAll(slug, "-", " ")) {
				a.add("slugKeyphrase", Good, "good")
			} else {
				a.add("slugKeyphrase", OK, "bad")
			}
		}

		// Keyphrase in subheadings.
		if len(a.doc.Headings) > 0 {
			with := 0
			for _, h := range a.doc.Headings {
				if kp.In(h.Text) {
					with++
				}
			}
			p := pct(with, len(a.doc.Headings))
			switch {
			case with == 0:
				a.add("subheadingsKeyphrase", OK, "none")
			case p > 75 && len(a.doc.Headings) > 1:
				a.add("subheadingsKeyphrase", Bad, "tooMany", with, len(a.doc.Headings))
			case p >= 30 || len(a.doc.Headings) == 1:
				a.add("subheadingsKeyphrase", Good, "good", with, len(a.doc.Headings))
			default:
				a.add("subheadingsKeyphrase", OK, "few", with, len(a.doc.Headings))
			}
		}

		// Keyphrase in image alt attributes.
		images := append(append([]Image{}, in.Images...), a.doc.Images...)
		if len(images) > 0 && a.hasText() {
			with := 0
			for _, im := range images {
				if kp.In(im.Alt) {
					with++
				}
			}
			p := pct(with, len(images))
			switch {
			case with == 0:
				a.add("imageKeyphrase", OK, "none")
			case len(images) >= 5 && p > 70:
				a.add("imageKeyphrase", OK, "tooMany", with, len(images))
			case len(images) >= 5 && p < 30:
				a.add("imageKeyphrase", OK, "few", with, len(images))
			default:
				a.add("imageKeyphrase", Good, "good", with, len(images))
			}
		}

		// Previously used keyphrase.
		switch n := len(in.KeyphraseUsedBy); {
		case n == 0:
			a.add("previouslyUsedKeyphrase", Good, "good")
		case n == 1:
			a.add("previouslyUsedKeyphrase", OK, "once", in.KeyphraseUsedBy[0])
		default:
			a.add("previouslyUsedKeyphrase", Bad, "many", n, strings.Join(firstN(in.KeyphraseUsedBy, 3), ", "))
		}

		// Competing links (links whose anchor is the keyphrase).
		competing := 0
		for _, l := range a.doc.Links {
			if kp.In(l.Text) {
				competing++
			}
		}
		if competing > 0 {
			a.add("competingLinks", OK, "bad", competing)
		}
	}

	// Meta description length.
	desc := strings.TrimSpace(in.Description)
	switch n := utf8.RuneCountInString(desc); {
	case !in.DescriptionSet || n == 0:
		a.add("metaDescriptionLength", Bad, "none")
	case n < 120:
		a.add("metaDescriptionLength", OK, "short", n)
	case n > 156:
		a.add("metaDescriptionLength", OK, "long", n)
	default:
		a.add("metaDescriptionLength", Good, "good", n)
	}

	// SEO title width.
	width := in.TitleWidth
	if width == 0 {
		width = TitleWidth(in.Title)
	}
	switch {
	case strings.TrimSpace(in.Title) == "":
		a.add("titleWidth", Bad, "none")
	case width < 400:
		a.add("titleWidth", OK, "short")
	case width > 600:
		a.add("titleWidth", OK, "long")
	default:
		a.add("titleWidth", Good, "good")
	}

	if !a.hasText() {
		return a.items
	}

	// Text length.
	words := len(a.doc.Words)
	good, ok := 300, 200
	switch {
	case in.Kind == KindProduct && in.Cornerstone:
		good, ok = 900, 400
	case in.Kind == KindProduct:
		good, ok = 200, 100
	case in.Cornerstone:
		good, ok = 400, 250
	default: // categories and tags
		good, ok = 250, 150
	}
	switch {
	case words >= good:
		a.add("textLength", Good, "good", words)
	case words >= ok:
		a.add("textLength", OK, "short", words, good)
	default:
		a.add("textLength", Bad, "tooShort", words, good)
	}

	// Links.
	internal, outbound, followed := 0, 0, 0
	site, _ := url.Parse(in.SiteURL)
	for _, l := range a.doc.Links {
		u, err := url.Parse(l.Href)
		if err != nil || l.Href == "" || strings.HasPrefix(l.Href, "#") || u.Scheme == "mailto" || u.Scheme == "tel" {
			continue
		}
		if u.Host == "" || (site != nil && strings.EqualFold(strings.TrimPrefix(u.Host, "www."), strings.TrimPrefix(site.Host, "www."))) {
			internal++
			continue
		}
		outbound++
		if !l.NoFollow {
			followed++
		}
	}
	switch {
	case internal > 0:
		a.add("internalLinks", Good, "good", internal)
	default:
		a.add("internalLinks", OK, "none")
	}
	switch {
	case outbound == 0:
		a.add("outboundLinks", OK, "none")
	case followed == 0:
		a.add("outboundLinks", OK, "allNofollow")
	default:
		a.add("outboundLinks", Good, "good", outbound)
	}

	// Images.
	images := len(in.Images) + len(a.doc.Images)
	switch {
	case images > 0:
		a.add("images", Good, "good", images)
	case in.Kind == KindProduct:
		a.add("images", Bad, "none")
	default:
		a.add("images", OK, "none")
	}

	// Single title: the name is the page's H1, the text shouldn't add one.
	if a.doc.H1Count > 0 {
		a.add("singleH1", Bad, "bad", a.doc.H1Count)
	}

	if in.Kind == KindProduct {
		// WooCommerce SEO: short description, SKU, identifiers.
		sw := len(Words(clean(in.ShortDescription)))
		switch {
		case sw == 0:
			a.add("shortDescription", Bad, "none")
		case sw < 20:
			a.add("shortDescription", OK, "short", sw)
		case sw > 50:
			a.add("shortDescription", OK, "long", sw)
		default:
			a.add("shortDescription", Good, "good", sw)
		}
		if strings.TrimSpace(in.SKU) != "" {
			a.add("sku", Good, "good")
		} else {
			a.add("sku", OK, "none")
		}
		if strings.TrimSpace(in.GTIN) != "" {
			a.add("productIdentifiers", Good, "good")
		} else {
			a.add("productIdentifiers", OK, "none")
		}
	}
	return a.items
}

func firstN(list []string, n int) []string {
	if len(list) > n {
		return list[:n]
	}
	return list
}

// ------------------------------------------------------------------ readability

func (a *assessor) readability() ([]Assessment, float64) {
	if !a.hasText() {
		return nil, -1
	}
	d := a.doc
	words := len(d.Words)
	if words < 50 {
		a.add("textPresence", Bad, "bad", words)
		return a.items, -1
	}

	// Subheading distribution.
	longest := 0
	for _, s := range d.Sections {
		if n := len(Words(s)); n > longest {
			longest = n
		}
	}
	switch {
	case len(d.Headings) == 0 && words <= 300:
		a.add("subheadingDistribution", Good, "shortText")
	case len(d.Headings) == 0:
		a.add("subheadingDistribution", Bad, "none")
	case longest > 350:
		a.add("subheadingDistribution", Bad, "long", longest)
	case longest > 300:
		a.add("subheadingDistribution", OK, "long", longest)
	default:
		a.add("subheadingDistribution", Good, "good")
	}

	// Paragraph length.
	long := 0
	for _, p := range d.Paragraphs {
		if len(Words(p)) > 150 {
			long++
		}
	}
	maxP := 0
	for _, p := range d.Paragraphs {
		if n := len(Words(p)); n > maxP {
			maxP = n
		}
	}
	switch {
	case long == 0:
		a.add("paragraphLength", Good, "good")
	case maxP > 200:
		a.add("paragraphLength", Bad, "long", long)
	default:
		a.add("paragraphLength", OK, "long", long)
	}

	// Sentence length.
	longS := 0
	for _, s := range d.Sentences {
		if len(Words(s)) > 20 {
			longS++
		}
	}
	p := pct(longS, len(d.Sentences))
	switch {
	case p <= 25:
		a.add("sentenceLength", Good, "good")
	case p <= 30:
		a.add("sentenceLength", OK, "long", fmt.Sprintf("%.0f", p))
	default:
		a.add("sentenceLength", Bad, "long", fmt.Sprintf("%.0f", p))
	}

	// Consecutive sentences starting with the same word.
	groups, run := 0, 1
	prev := ""
	for _, s := range d.Sentences {
		w := Words(s)
		first := ""
		if len(w) > 0 {
			first = Fold(w[0])
		}
		if first != "" && first == prev {
			run++
			if run == 3 {
				groups++
			}
		} else {
			run = 1
		}
		prev = first
	}
	switch {
	case groups == 0:
		a.add("consecutiveSentences", Good, "good")
	case groups == 1:
		a.add("consecutiveSentences", OK, "bad", groups)
	default:
		a.add("consecutiveSentences", Bad, "bad", groups)
	}

	flesch := -1.0
	if a.lang == "en" || a.lang == "fr" {
		// Passive voice.
		passive := 0
		for _, s := range d.Sentences {
			if isPassive(s, a.lang) {
				passive++
			}
		}
		pp := pct(passive, len(d.Sentences))
		switch {
		case pp <= 10:
			a.add("passiveVoice", Good, "good")
		case pp <= 15:
			a.add("passiveVoice", OK, "high", fmt.Sprintf("%.0f", pp))
		default:
			a.add("passiveVoice", Bad, "high", fmt.Sprintf("%.0f", pp))
		}

		// Transition words (texts of 200 words or more).
		if words >= 200 {
			with := 0
			for _, s := range d.Sentences {
				if hasTransition(s, a.lang) {
					with++
				}
			}
			tp := pct(with, len(d.Sentences))
			switch {
			case tp >= 30:
				a.add("transitionWords", Good, "good")
			case tp >= 20:
				a.add("transitionWords", OK, "few", fmt.Sprintf("%.0f", tp))
			default:
				a.add("transitionWords", Bad, "few", fmt.Sprintf("%.0f", tp))
			}
		}

		// Flesch reading ease.
		syllables := 0
		for _, w := range d.Words {
			syllables += Syllables(w, a.lang)
		}
		wps := float64(words) / math.Max(1, float64(len(d.Sentences)))
		spw := float64(syllables) / float64(words)
		if a.lang == "fr" {
			flesch = 207 - 1.015*wps - 73.6*spw // Kandel & Moles
		} else {
			flesch = 206.835 - 1.015*wps - 84.6*spw
		}
		flesch = math.Max(0, math.Min(100, math.Round(flesch*10)/10))
		f := fmt.Sprintf("%.1f", flesch)
		switch {
		case flesch >= 90:
			a.add("fleschReadingEase", Good, "veryEasy", f)
		case flesch >= 70:
			a.add("fleschReadingEase", Good, "easy", f)
		case flesch >= 60:
			a.add("fleschReadingEase", Good, "ok", f)
		case flesch >= 50:
			a.add("fleschReadingEase", OK, "fairlyDifficult", f)
		case flesch >= 30:
			a.add("fleschReadingEase", Bad, "difficult", f)
		default:
			a.add("fleschReadingEase", Bad, "veryDifficult", f)
		}
	}
	return a.items, flesch
}

// ------------------------------------------------------------------ title width

// TitleWidth estimates the pixel width of a title in Google's results (Arial
// 20px): about 600px fit.
func TitleWidth(s string) int {
	w := 0.0
	for _, r := range s {
		switch {
		case strings.ContainsRune("iljI.,:;|!'’`ıìíîï", r):
			w += 5
		case strings.ContainsRune("ftr()[]{}\"-/\\ ", r):
			w += 7
		case strings.ContainsRune("mwMW@%", r):
			w += 16
		case r >= 'A' && r <= 'Z':
			w += 13
		case r >= '0' && r <= '9':
			w += 11
		case r > 0x2E80: // CJK / wide
			w += 20
		default:
			w += 10.5
		}
	}
	return int(math.Round(w))
}
