package woo

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"cwmerch-api/internal/php"
)

// The storefront's look is resolved field by field, the first source that
// has a value winning:
//
//  1. cwmerch's own appearance (cwmerch_settings "appearance": only the fields
//     the admin overrode; empty = inherit), so the shop keeps working and stays
//     editable once Divi or WordPress are decommissioned;
//  2. Divi (and Extra / Divi child themes such as divi-cart-pro): its theme
//     options (et_divi) and global colors (et_global_data);
//  3. WooCommerce: its email colors, when changed from WooCommerce's defaults;
//  4. WordPress: a block theme's global styles, or a classic theme's customizer;
//  5. cwmerch's default (the blue classic preset).
//
// Sources 2-4 are only READ, at request time, and never written back to
// cwmerch's settings (several API instances may run; nothing to sync). They
// come from wp_options / wp_posts: a missing option, table or column, or an
// unparsable value just yields nothing.

// WordPressThemeID is the preset made of the detected (inherited) look:
// choosing it in the editor clears the overrides.
const WordPressThemeID = "wordpress"

// Look sources.
const (
	SourceDivi        = "divi"
	SourceWooCommerce = "woocommerce"
	SourceWordPress   = "wordpress"
	SourceDerived     = "derived" // a readable text color for a detected background
	SourceDefault     = "default"
)

// Look is what was detected from Divi, WooCommerce and WordPress, over
// cwmerch's defaults, with where each value comes from ("colors.primary":
// "divi"...). The editor shows it as the placeholder of every field the admin
// didn't override.
type Look struct {
	Name    string            `json:"name"`
	Tagline string            `json:"tagline"`
	Logo    *Image            `json:"logo"`
	Colors  Colors            `json:"colors"`
	Fonts   Fonts             `json:"fonts"`
	Sources map[string]string `json:"sources"`
}

type themeColors struct {
	name   string
	colors Colors
	fonts  Fonts
}

func (t *themeColors) empty() bool { return t.colors == (Colors{}) && t.fonts == (Fonts{}) }

type colorRef struct {
	key string
	p   *string
}

// colorRefs lists a Colors' fields by JSON key.
func colorRefs(c *Colors) []colorRef {
	return []colorRef{{"primary", &c.Primary}, {"primaryText", &c.PrimaryText}, {"secondary", &c.Secondary}, {"accent", &c.Accent},
		{"background", &c.Background}, {"surface", &c.Surface}, {"text", &c.Text}, {"muted", &c.Muted}, {"border", &c.Border},
		{"header", &c.Header}, {"headerText", &c.HeaderText}, {"footer", &c.Footer}, {"footerText", &c.FooterText}, {"sale", &c.Sale}}
}

// textOn pairs a text color with the background it sits on.
var textOn = [][2]string{{"primaryText", "primary"}, {"headerText", "header"}, {"footerText", "footer"}}

// DetectedLook resolves sources 2-5 (everything but cwmerch's overrides).
func (s *Store) DetectedLook(ctx context.Context) (look Look) {
	base := Themes[0]
	defaultLook := func() Look {
		l := Look{Colors: base.Colors, Fonts: base.Fonts, Sources: map[string]string{}}
		for _, r := range colorRefs(&l.Colors) {
			l.Sources["colors."+r.key] = SourceDefault
		}
		l.Sources["fonts.heading"], l.Sources["fonts.body"] = SourceDefault, SourceDefault
		return l
	}
	// Best effort only: whatever the database holds, never break the storefront.
	defer func() {
		if r := recover(); r != nil {
			look = defaultLook()
		}
	}()
	stylesheet := strings.TrimSpace(s.Option(ctx, "stylesheet"))
	layers := []struct {
		source string
		tc     themeColors
	}{
		{SourceDivi, s.diviColors(ctx)},
		{SourceWooCommerce, s.wooColors(ctx)},
		{SourceWordPress, s.wordPressColors(ctx, stylesheet)},
	}
	look = Look{Name: s.Option(ctx, "blogname"), Tagline: s.Option(ctx, "blogdescription"), Sources: map[string]string{}}
	values := map[string]string{}
	for _, r := range colorRefs(&look.Colors) {
		for _, l := range layers {
			tc := l.tc
			if v := *colorRefs(&tc.colors)[indexOf(r.key)].p; v != "" {
				*r.p, look.Sources["colors."+r.key] = v, l.source
				break
			}
		}
		values[r.key] = *r.p
	}
	refs := colorRefs(&look.Colors)
	for _, pair := range textOn {
		text, bg := refs[indexOf(pair[0])], refs[indexOf(pair[1])]
		if *text.p == "" && *bg.p != "" {
			*text.p, look.Sources["colors."+pair[0]] = readableOn(*bg.p), SourceDerived
		}
	}
	if look.Colors.Accent == "" && look.Colors.Primary != "" {
		look.Colors.Accent, look.Sources["colors.accent"] = look.Colors.Primary, SourceDerived
	}
	def := base.Colors
	defRefs := colorRefs(&def)
	for i, r := range refs {
		if *r.p == "" {
			*r.p, look.Sources["colors."+r.key] = *defRefs[i].p, SourceDefault
		}
	}
	look.Fonts, look.Sources["fonts.heading"], look.Sources["fonts.body"] = base.Fonts, SourceDefault, SourceDefault
	if logo, src := s.detectedLogo(ctx, stylesheet); logo != nil {
		look.Logo, look.Sources["logo"] = logo, src
	}
	for i := len(layers) - 1; i >= 0; i-- { // first layer wins
		if f := layers[i].tc.fonts.Heading; f != "" {
			look.Fonts.Heading, look.Sources["fonts.heading"] = f, layers[i].source
		}
		if f := layers[i].tc.fonts.Body; f != "" {
			look.Fonts.Body, look.Sources["fonts.body"] = f, layers[i].source
		}
	}
	return look
}

// inherited reports whether anything came from Divi, WooCommerce or WordPress.
func (l Look) inherited() bool {
	for _, src := range l.Sources {
		if src == SourceDivi || src == SourceWooCommerce || src == SourceWordPress {
			return true
		}
	}
	return false
}

// sourceNames lists the sources used, in priority order (for the preset name).
func (l Look) sourceNames() []string {
	var out []string
	for _, s := range []struct{ id, name string }{{SourceDivi, "Divi"}, {SourceWooCommerce, "WooCommerce"}, {SourceWordPress, "WordPress"}} {
		for _, src := range l.Sources {
			if src == s.id {
				out = append(out, s.name)
				break
			}
		}
	}
	return out
}

func indexOf(key string) int {
	for i, r := range colorRefs(&Colors{}) {
		if r.key == key {
			return i
		}
	}
	return -1
}

// effectiveColors puts the admin's overrides over the detected colors; a
// background the admin overrode without its text color gets a readable one.
func effectiveColors(overrides, detected Colors) Colors {
	out := detected
	refs, ov := colorRefs(&out), colorRefs(&overrides)
	for i := range refs {
		if v := strings.TrimSpace(*ov[i].p); v != "" {
			*refs[i].p = v
		}
	}
	for _, pair := range textOn {
		if *ov[indexOf(pair[0])].p == "" && *ov[indexOf(pair[1])].p != "" {
			*refs[indexOf(pair[0])].p = readableOn(*refs[indexOf(pair[1])].p)
		}
	}
	return out
}

func effectiveFonts(overrides, detected Fonts) Fonts {
	out := detected
	if v := strings.TrimSpace(overrides.Heading); v != "" {
		out.Heading = v
	}
	if v := strings.TrimSpace(overrides.Body); v != "" {
		out.Body = v
	}
	return out
}

// WordPressTheme is the detected look as a preset (false when nothing came
// from Divi, WooCommerce or WordPress).
func (s *Store) WordPressTheme(ctx context.Context) (Theme, bool) {
	look := s.DetectedLook(ctx)
	if !look.inherited() {
		return Theme{}, false
	}
	return Theme{ID: WordPressThemeID, Name: strings.Join(look.sourceNames(), " + "), Colors: look.Colors, Fonts: look.Fonts}, true
}

// ThemesFor is the presets offered in the appearance editor: the detected
// look first when there is one.
func (s *Store) ThemesFor(ctx context.Context) []Theme {
	if t, ok := s.WordPressTheme(ctx); ok {
		return append([]Theme{t}, Themes...)
	}
	return Themes
}

// detectedLogo is the shop's logo from Divi (divi_logo, not Divi's own
// default), WooCommerce (the email header image), or WordPress (the theme's
// custom_logo, or a block theme's site_logo), in that order. Only an https://
// URL is kept: a plain-http one would be blocked as mixed content.
func (s *Store) detectedLogo(ctx context.Context, stylesheet string) (*Image, string) {
	secure := func(u string) string {
		u = strings.TrimSpace(u)
		if strings.HasPrefix(strings.ToLower(u), "https://") && !strings.ContainsAny(u, " \t\n\"'<>") {
			return u
		}
		return ""
	}
	if opts := php.UnmarshalArray(s.Option(ctx, "et_divi")); opts != nil {
		if u := secure(opts.String("divi_logo")); u != "" && !strings.Contains(strings.ToLower(u), "/themes/divi/images/logo.png") {
			return &Image{Src: u}, SourceDivi
		}
	}
	if u := secure(s.Option(ctx, "woocommerce_email_header_image")); u != "" {
		return &Image{Src: u}, SourceWooCommerce
	}
	var ids []uint64
	if stylesheet != "" {
		if mods := php.UnmarshalArray(s.Option(ctx, "theme_mods_"+stylesheet)); mods != nil {
			if id := ParseID(mods.String("custom_logo")); id != 0 {
				ids = append(ids, id)
			}
		}
	}
	if id := ParseID(s.Option(ctx, "site_logo")); id != 0 {
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		if imgs, err := s.images(ctx, ids); err == nil {
			for _, id := range ids {
				if img, ok := imgs[id]; ok && secure(img.Src) != "" {
					return &img, SourceWordPress
				}
			}
		}
	}
	return nil, ""
}

// wooColors reads WooCommerce's email colors, ignoring WooCommerce's own
// defaults (they say nothing about the shop's brand).
func (s *Store) wooColors(ctx context.Context) themeColors {
	out := themeColors{name: "WooCommerce"}
	opt := func(name string, defaults ...string) string {
		v := normColor(s.Option(ctx, name))
		for _, d := range defaults {
			if v == d {
				return ""
			}
		}
		return v
	}
	out.colors.Primary = opt("woocommerce_email_base_color", "#7f54b3", "#720eec", "#96588a")
	out.colors.Text = opt("woocommerce_email_text_color", "#3c3c3c", "#1e1e1e", "#515151")
	return out
}

// wordPressColors reads the theme: a block theme's global styles, else a
// classic theme's customizer.
func (s *Store) wordPressColors(ctx context.Context, stylesheet string) themeColors {
	if isDivi(stylesheet) || isDivi(s.Option(ctx, "template")) {
		return themeColors{} // Divi's colors are read from its own options
	}
	if tc := s.blockThemeColors(ctx, stylesheet); !tc.empty() {
		tc.name = stylesheet
		return tc
	}
	tc := s.classicThemeColors(ctx, stylesheet)
	tc.name = stylesheet
	return tc
}

func isDivi(name string) bool {
	n := strings.ToLower(name)
	return n == "divi" || n == "extra"
}

// diviColors reads Divi's theme options (the et_divi option, serialized) and,
// as a fallback, its global colors (et_global_data); whenever they exist,
// even once Divi is no longer the active theme.
func (s *Store) diviColors(ctx context.Context) themeColors {
	out := themeColors{name: "Divi"}
	opts := php.UnmarshalArray(s.Option(ctx, "et_divi"))
	get := func(keys ...string) string {
		if opts == nil {
			return ""
		}
		for _, k := range keys {
			if c := normColor(opts.String(k)); c != "" {
				return c
			}
		}
		return ""
	}
	c := &out.colors
	c.Primary = get("accent_color", "all_buttons_bg_color", "menu_link_active")
	c.Accent = get("menu_link_active", "accent_color")
	c.Text = get("font_color")
	c.Secondary = get("header_color")
	c.Header = get("primary_nav_bg")
	c.HeaderText = get("menu_link")
	c.Footer = get("footer_bg")
	c.FooterText = get("footer_widget_text_color", "bottom_bar_text_color")
	if opts != nil {
		out.fonts = Fonts{Heading: fontOrEmpty(opts.String("heading_font")), Body: fontOrEmpty(opts.String("body_font"))}
	}
	// Divi's global colors (Divi 4: et_global_data[global_colors][gcid-*][color]).
	if global := php.UnmarshalArray(s.Option(ctx, "et_global_data")); global != nil {
		if colors, ok := global.Get("global_colors").(*php.Array); ok {
			gc := func(id string) string {
				if e, ok := colors.Get(id).(*php.Array); ok {
					return normColor(e.String("color"))
				}
				return ""
			}
			c.Primary = firstNonEmpty(c.Primary, gc("gcid-primary-color"))
			c.Secondary = firstNonEmpty(c.Secondary, gc("gcid-secondary-color"), gc("gcid-heading-color"))
			c.Text = firstNonEmpty(c.Text, gc("gcid-body-color"))
		}
	}
	return out
}

// blockThemeColors reads a block theme's user global styles (the
// wp_global_styles post: palette + styles.color).
func (s *Store) blockThemeColors(ctx context.Context, stylesheet string) themeColors {
	out := themeColors{}
	if stylesheet == "" {
		return out
	}
	var raw string
	err := s.DB.QueryRowContext(ctx, `SELECT post_content FROM `+s.T("posts")+` WHERE post_type = 'wp_global_styles' AND post_status = 'publish'
		AND post_name IN (?, ?) ORDER BY ID DESC LIMIT 1`, "wp-global-styles-"+stylesheet, "wp-global-styles-"+url.PathEscape(stylesheet)).Scan(&raw)
	if err != nil || raw == "" {
		return out
	}
	var gs struct {
		Settings struct {
			Color struct {
				Palette json.RawMessage `json:"palette"`
			} `json:"color"`
		} `json:"settings"`
		Styles struct {
			Color struct {
				Background string `json:"background"`
				Text       string `json:"text"`
			} `json:"color"`
			Elements struct {
				Link struct {
					Color struct {
						Text string `json:"text"`
					} `json:"color"`
				} `json:"link"`
				Button struct {
					Color struct {
						Background string `json:"background"`
						Text       string `json:"text"`
					} `json:"color"`
				} `json:"button"`
			} `json:"elements"`
		} `json:"styles"`
	}
	if json.Unmarshal([]byte(raw), &gs) != nil {
		return out
	}
	type entry struct {
		Slug  string `json:"slug"`
		Color string `json:"color"`
	}
	palette := map[string]string{}
	var origins map[string][]entry
	var flat []entry
	if json.Unmarshal(gs.Settings.Color.Palette, &origins) == nil {
		for _, origin := range []string{"default", "theme", "custom"} { // custom wins
			for _, e := range origins[origin] {
				palette[e.Slug] = e.Color
			}
		}
	} else if json.Unmarshal(gs.Settings.Color.Palette, &flat) == nil {
		for _, e := range flat {
			palette[e.Slug] = e.Color
		}
	}
	// var(--wp--preset--color--primary) or var:preset|color|primary → palette.
	resolve := func(v string) string {
		v = strings.TrimSpace(v)
		for _, prefix := range []string{"var(--wp--preset--color--", "var:preset|color|"} {
			if strings.HasPrefix(v, prefix) {
				v = palette[strings.TrimSuffix(strings.TrimPrefix(v, prefix), ")")]
			}
		}
		return normColor(v)
	}
	c := &out.colors
	c.Primary = firstNonEmpty(resolve(gs.Styles.Elements.Button.Color.Background), resolve(palette["primary"]), resolve(palette["accent"]), resolve(gs.Styles.Elements.Link.Color.Text))
	c.PrimaryText = resolve(gs.Styles.Elements.Button.Color.Text)
	c.Secondary = resolve(palette["secondary"])
	c.Accent = firstNonEmpty(resolve(palette["accent"]), resolve(palette["tertiary"]))
	c.Background = firstNonEmpty(resolve(gs.Styles.Color.Background), resolve(palette["base"]), resolve(palette["background"]))
	c.Text = firstNonEmpty(resolve(gs.Styles.Color.Text), resolve(palette["contrast"]), resolve(palette["foreground"]))
	return out
}

// classicThemeColors reads a classic theme's customizer (theme_mods_<theme>).
func (s *Store) classicThemeColors(ctx context.Context, stylesheet string) themeColors {
	out := themeColors{}
	if stylesheet == "" {
		return out
	}
	mods := php.UnmarshalArray(s.Option(ctx, "theme_mods_"+stylesheet))
	if mods == nil {
		return out
	}
	hex := func(k string) string {
		v := strings.TrimSpace(mods.String(k))
		if v != "" && !strings.HasPrefix(v, "#") {
			v = "#" + v
		}
		return normColor(v)
	}
	out.colors.Background = hex("background_color")
	out.colors.Primary = firstNonEmpty(hex("primary_color"), hex("accent_color"), hex("link_color"))
	out.colors.HeaderText = hex("header_textcolor")
	return out
}

var hexColor = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
var rgbColor = regexp.MustCompile(`^rgba?\(\s*([\d.]+)\s*,\s*([\d.]+)\s*,\s*([\d.]+)\s*(?:,\s*([\d.]+%?)\s*)?\)$`)

// normColor turns a CSS color (#rgb, #rrggbb, #rrggbbaa, rgb(), rgba()) into
// #rrggbb, translucent colors blended over white; "" when it isn't one.
func normColor(v string) string {
	r, g, b, ok := parseColor(v)
	if !ok {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func parseColor(v string) (int, int, int, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	alpha := 1.0
	var r, g, b float64
	switch {
	case hexColor.MatchString(v):
		h := v[1:]
		if len(h) <= 4 {
			var full strings.Builder
			for _, ch := range h {
				full.WriteRune(ch)
				full.WriteRune(ch)
			}
			h = full.String()
		}
		n, _ := strconv.ParseUint(h[:6], 16, 32)
		r, g, b = float64(n>>16&0xff), float64(n>>8&0xff), float64(n&0xff)
		if len(h) == 8 {
			a, _ := strconv.ParseUint(h[6:8], 16, 8)
			alpha = float64(a) / 255
		}
	case rgbColor.MatchString(v):
		m := rgbColor.FindStringSubmatch(v)
		r, _ = strconv.ParseFloat(m[1], 64)
		g, _ = strconv.ParseFloat(m[2], 64)
		b, _ = strconv.ParseFloat(m[3], 64)
		if m[4] != "" {
			if strings.HasSuffix(m[4], "%") {
				p, _ := strconv.ParseFloat(strings.TrimSuffix(m[4], "%"), 64)
				alpha = p / 100
			} else {
				alpha, _ = strconv.ParseFloat(m[4], 64)
			}
		}
	default:
		return 0, 0, 0, false
	}
	alpha = math.Max(0, math.Min(1, alpha))
	blend := func(c float64) int { return int(math.Round(math.Min(255, c)*alpha + 255*(1-alpha))) }
	return blend(r), blend(g), blend(b), true
}

// readableOn is white or near-black, whichever contrasts more with bg.
func readableOn(bg string) string {
	r, g, b, ok := parseColor(bg)
	if !ok {
		return "#ffffff"
	}
	lin := func(c int) float64 {
		x := float64(c) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	l := 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
	if (1.05)/(l+0.05) >= (l+0.05)/(0.0118+0.05) { // white vs #0f172a
		return "#ffffff"
	}
	return "#0f172a"
}

// fontOrEmpty keeps a font family name (Divi stores "none"/"" for the default).
func fontOrEmpty(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.EqualFold(v, "none") || strings.EqualFold(v, "default") || !fontName.MatchString(v) {
		return ""
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
