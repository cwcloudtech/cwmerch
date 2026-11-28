package seoanalysis

import (
	"html"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Doc is a parsed HTML text.
type Doc struct {
	HTML        string
	Paragraphs  []string // plain text blocks (p, li, headings, br-separated...)
	Intro       string   // the first paragraph
	Headings    []Heading
	Sections    []string // text between subheadings (h2-h6)
	Links       []Link
	Images      []Image
	Sentences   []string
	Words       []string
	H1Count     int
	PlainText   string
	wordsNorm   []string
	sentenceSet [][]string
}

type Heading struct {
	Level int
	Text  string
}

type Link struct {
	Href     string
	Text     string
	NoFollow bool
}

type Image struct {
	Src string `json:"src"`
	Alt string `json:"alt"`
}

var (
	headingRe = regexp.MustCompile(`(?is)<h([1-6])[^>]*>(.*?)</h[1-6]>`)
	linkRe    = regexp.MustCompile(`(?is)<a\s([^>]*)>(.*?)</a>`)
	hrefRe    = regexp.MustCompile(`(?is)href\s*=\s*["']([^"']*)["']`)
	relRe     = regexp.MustCompile(`(?is)rel\s*=\s*["']([^"']*)["']`)
	imgRe     = regexp.MustCompile(`(?is)<img\s([^>]*)>`)
	srcRe     = regexp.MustCompile(`(?is)\ssrc\s*=\s*["']([^"']*)["']|^src\s*=\s*["']([^"']*)["']`)
	altRe     = regexp.MustCompile(`(?is)alt\s*=\s*["']([^"']*)["']`)
	blockRe   = regexp.MustCompile(`(?is)</?(p|div|li|ul|ol|h[1-6]|blockquote|pre|tr|table|section|article)\b[^>]*>|<br\s*/?>`)
	tagRe     = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRe   = regexp.MustCompile(`[ \t\r\f\v\x{00a0}]+`)
	// A sentence ends with . ! ? … (or the Arabic question mark) followed by a
	// space, or at a block's end.
	sentenceEndRe = regexp.MustCompile(`([.!?…؟]+)(\s+|$)`)
	wordRe        = regexp.MustCompile(`[\p{L}\p{N}]+(?:-[\p{L}\p{N}]+)*`)
	shortcodeRe   = regexp.MustCompile(`\[/?[a-zA-Z_][\w-]*[^\]]*\]`)
)

func clean(s string) string {
	s = html.UnescapeString(tagRe.ReplaceAllString(s, " "))
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// Parse splits an HTML text into what the assessments read.
func Parse(src string) Doc {
	src = shortcodeRe.ReplaceAllString(src, " ")
	d := Doc{HTML: src}
	for _, m := range headingRe.FindAllStringSubmatch(src, -1) {
		lvl := int(m[1][0] - '0')
		if lvl == 1 {
			d.H1Count++
			continue
		}
		d.Headings = append(d.Headings, Heading{Level: lvl, Text: clean(m[2])})
	}
	for _, m := range linkRe.FindAllStringSubmatch(src, -1) {
		l := Link{Text: clean(m[2])}
		if h := hrefRe.FindStringSubmatch(m[1]); h != nil {
			l.Href = strings.TrimSpace(html.UnescapeString(h[1]))
		}
		if r := relRe.FindStringSubmatch(m[1]); r != nil {
			l.NoFollow = strings.Contains(strings.ToLower(r[1]), "nofollow")
		}
		d.Links = append(d.Links, l)
	}
	for _, m := range imgRe.FindAllStringSubmatch(src, -1) {
		img := Image{}
		if s := srcRe.FindStringSubmatch(" " + m[1]); s != nil {
			img.Src = s[1] + s[2]
		}
		if a := altRe.FindStringSubmatch(m[1]); a != nil {
			img.Alt = html.UnescapeString(a[1])
		}
		d.Images = append(d.Images, img)
	}

	// Sections: the text between subheadings.
	cur := []string{}
	flush := func() {
		if t := clean(strings.Join(cur, " ")); t != "" {
			d.Sections = append(d.Sections, t)
		}
		cur = cur[:0]
	}
	rest := src
	for {
		loc := headingRe.FindStringIndex(rest)
		if loc == nil {
			cur = append(cur, rest)
			break
		}
		cur = append(cur, rest[:loc[0]])
		flush()
		rest = rest[loc[1]:]
	}
	flush()

	// Paragraphs: block-level splits, headings excluded.
	body := headingRe.ReplaceAllString(src, "\n")
	for _, block := range strings.Split(blockRe.ReplaceAllString(body, "\n"), "\n") {
		if t := clean(block); t != "" {
			d.Paragraphs = append(d.Paragraphs, t)
		}
	}
	if len(d.Paragraphs) > 0 {
		d.Intro = d.Paragraphs[0]
	}
	d.PlainText = strings.Join(d.Paragraphs, "\n")
	for _, p := range d.Paragraphs {
		d.Sentences = append(d.Sentences, SplitSentences(p)...)
	}
	d.Words = Words(d.PlainText)
	d.wordsNorm = normWords(d.Words)
	for _, s := range d.Sentences {
		d.sentenceSet = append(d.sentenceSet, normWords(Words(s)))
	}
	return d
}

// SplitSentences cuts a paragraph into sentences.
func SplitSentences(p string) []string {
	out := []string{}
	last := 0
	for _, loc := range sentenceEndRe.FindAllStringIndex(p, -1) {
		// "e.g." / "3.5" / "M. Dupont": a dot followed by a lowercase letter or a
		// digit doesn't end the sentence.
		if loc[1] < len(p) {
			next := []rune(p[loc[1]:])[0]
			if unicode.IsLower(next) || unicode.IsDigit(next) {
				continue
			}
		}
		if s := strings.TrimSpace(p[last:loc[1]]); s != "" && len(Words(s)) > 0 {
			out = append(out, s)
		}
		last = loc[1]
	}
	if s := strings.TrimSpace(p[last:]); s != "" && len(Words(s)) > 0 {
		out = append(out, s)
	}
	return out
}

// Words are the text's words.
func Words(s string) []string { return wordRe.FindAllString(s, -1) }

var foldT = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// Fold lowercases and removes the accents ("Été" → "ete").
func Fold(s string) string {
	out, _, err := transform.String(foldT, strings.ToLower(s))
	if err != nil {
		return strings.ToLower(s)
	}
	return strings.ReplaceAll(out, "’", "'")
}

// stem is a light, language-agnostic stemmer: plural and common feminine
// endings, so "chaise" matches "chaises" and "shoe" matches "shoes".
func stem(w string) string {
	r := []rune(w)
	if len(r) <= 3 {
		return w
	}
	switch {
	case strings.HasSuffix(w, "ies") && len(r) > 4:
		return string(r[:len(r)-3]) + "y"
	case strings.HasSuffix(w, "aux") && len(r) > 4: // chevaux → cheval
		return string(r[:len(r)-3]) + "al"
	case strings.HasSuffix(w, "ss"):
		return w
	case strings.HasSuffix(w, "s"), strings.HasSuffix(w, "x"):
		return string(r[:len(r)-1])
	}
	return w
}

func normWords(words []string) []string {
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = stem(Fold(w))
	}
	return out
}

// Keyphrase is a focus keyphrase ready to match.
type Keyphrase struct {
	Raw     string
	Exact   bool     // "between double quotes": the exact phrase only
	Words   []string // all its words, normalized
	Content []string // without the function words (what must match)
}

// NewKeyphrase prepares kw for the locale.
func NewKeyphrase(kw, locale string) Keyphrase {
	k := Keyphrase{Raw: strings.TrimSpace(kw)}
	raw := k.Raw
	if len(raw) > 2 && (strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`) || strings.HasPrefix(raw, "«") && strings.HasSuffix(raw, "»")) {
		k.Exact = true
		raw = strings.Trim(raw, `"«» `)
	}
	k.Words = normWords(Words(raw))
	fw := functionWords(locale)
	for i, w := range Words(raw) {
		if !fw[Fold(w)] {
			k.Content = append(k.Content, k.Words[i])
		}
	}
	if len(k.Content) == 0 {
		k.Content = k.Words
	}
	return k
}

func (k Keyphrase) Empty() bool { return len(k.Words) == 0 }

// In reports whether every content word of the keyphrase is in the text (any
// order), or the exact phrase for a quoted keyphrase.
func (k Keyphrase) In(text string) bool {
	if k.Empty() {
		return false
	}
	words := normWords(Words(text))
	if k.Exact {
		return indexSeq(words, k.Words) >= 0
	}
	return containsAll(words, k.Content)
}

// Count is how many times the keyphrase occurs in the sentences: per sentence,
// the number of times its rarest content word appears.
func (k Keyphrase) count(sentences [][]string) int {
	if k.Empty() {
		return 0
	}
	n := 0
	for _, s := range sentences {
		if k.Exact {
			for i := 0; i+len(k.Words) <= len(s); i++ {
				if equalSeq(s[i:i+len(k.Words)], k.Words) {
					n++
				}
			}
			continue
		}
		min := -1
		for _, w := range k.Content {
			c := 0
			for _, x := range s {
				if x == w {
					c++
				}
			}
			if min < 0 || c < min {
				min = c
			}
		}
		if min > 0 {
			n += min
		}
	}
	return n
}

// StartsWith reports whether text starts with the keyphrase (its words, in
// order, function words allowed in between at most once).
func (k Keyphrase) StartsWith(text string) bool {
	words := normWords(Words(text))
	if k.Empty() || len(words) == 0 {
		return false
	}
	i := indexSeq(words, k.Words)
	if i == 0 {
		return true
	}
	if !k.Exact {
		i = indexSeq(words, k.Content)
		return i == 0
	}
	return false
}

func containsAll(words, needles []string) bool {
	set := map[string]bool{}
	for _, w := range words {
		set[w] = true
	}
	for _, n := range needles {
		if !set[n] {
			return false
		}
	}
	return true
}

func indexSeq(words, seq []string) int {
	for i := 0; i+len(seq) <= len(words); i++ {
		if equalSeq(words[i:i+len(seq)], seq) {
			return i
		}
	}
	return -1
}

func equalSeq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ------------------------------------------------------------- syllables

var vowelGroupRe = regexp.MustCompile(`[aeiouyàâäéèêëîïôöùûüœæ]+`)

// Syllables estimates a word's syllables (English / French rules of thumb).
func Syllables(word, locale string) int {
	w := strings.ToLower(word)
	n := len(vowelGroupRe.FindAllString(w, -1))
	switch locale {
	case "fr":
		// mute final e / es / ent ("table", "tables", "parlent")
		if n > 1 && (strings.HasSuffix(w, "e") || strings.HasSuffix(w, "es") || strings.HasSuffix(w, "ent")) && !strings.HasSuffix(w, "ée") {
			n--
		}
	default:
		if n > 1 && strings.HasSuffix(w, "e") && !strings.HasSuffix(w, "le") && !strings.HasSuffix(w, "ee") {
			n--
		}
		if n > 1 && (strings.HasSuffix(w, "es") || strings.HasSuffix(w, "ed")) && !strings.HasSuffix(w, "ted") && !strings.HasSuffix(w, "ded") {
			n--
		}
	}
	if n < 1 {
		n = 1
	}
	return n
}
