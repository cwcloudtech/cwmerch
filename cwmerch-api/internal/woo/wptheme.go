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

// The storefront's default colors come from the active WordPress theme when
// cwmerch's appearance was never saved, so a shop moving off WordPress keeps
// its brand: Divi (and Extra / Divi child themes, e.g. divi-cart-pro) theme
// options, a block theme's global styles, or a classic theme's customizer.
// Everything is read from wp_options / wp_posts, which always exist; a
// missing option, an unparsable value or an unknown theme just yields
// nothing, and cwmerch's own defaults apply.

// WordPressThemeID is the preset built from the active WordPress theme.
const WordPressThemeID = "wordpress"

type themeColors struct {
	name   string
	colors Colors
	fonts  Fonts
}

func (t *themeColors) empty() bool { return t.colors == (Colors{}) && t.fonts == (Fonts{}) }

// WordPressTheme is the active WordPress theme's look as a preset, merged over
// cwmerch's default colors (false when nothing usable was found).
func (s *Store) WordPressTheme(ctx context.Context) (t Theme, ok bool) {
	// Best effort only: whatever the database holds (another theme, missing
	// options or tables, odd values), never break the storefront's appearance.
	defer func() {
		if r := recover(); r != nil {
			t, ok = Theme{}, false
		}
	}()
	stylesheet := strings.TrimSpace(s.Option(ctx, "stylesheet"))
	template := strings.TrimSpace(s.Option(ctx, "template"))
	var found themeColors
	switch {
	case isDivi(template) || isDivi(stylesheet):
		found = s.diviColors(ctx)
	default:
		found = s.blockThemeColors(ctx, stylesheet)
		if found.empty() {
			found = s.classicThemeColors(ctx, stylesheet)
		}
	}
	if found.empty() {
		return Theme{}, false
	}
	name := found.name
	if name == "" {
		name = stylesheet
	}
	base := Themes[0]
	t = Theme{ID: WordPressThemeID, Name: "WordPress (" + name + ")", Colors: mergeColors(base.Colors, found.colors), Fonts: base.Fonts}
	if found.fonts.Heading != "" {
		t.Fonts.Heading = found.fonts.Heading
	}
	if found.fonts.Body != "" {
		t.Fonts.Body = found.fonts.Body
	}
	return t, true
}

// ThemesFor is the presets offered in the appearance editor: the WordPress
// theme's first when there is one.
func (s *Store) ThemesFor(ctx context.Context) []Theme {
	if t, ok := s.WordPressTheme(ctx); ok {
		return append([]Theme{t}, Themes...)
	}
	return Themes
}

func isDivi(name string) bool {
	n := strings.ToLower(name)
	return n == "divi" || n == "extra"
}

// diviColors reads Divi's theme options (the et_divi option, serialized) and,
// as a fallback, its global colors (et_global_data).
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

// mergeColors puts the theme's colors over the defaults and picks readable
// text colors for the backgrounds the theme set without their text color.
func mergeColors(base, theme Colors) Colors {
	out := base
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&out.Primary, theme.Primary)
	set(&out.Secondary, theme.Secondary)
	set(&out.Accent, theme.Accent)
	set(&out.Background, theme.Background)
	set(&out.Surface, theme.Surface)
	set(&out.Text, theme.Text)
	set(&out.Muted, theme.Muted)
	set(&out.Border, theme.Border)
	set(&out.Header, theme.Header)
	set(&out.Footer, theme.Footer)
	set(&out.Sale, theme.Sale)
	out.PrimaryText = firstNonEmpty(theme.PrimaryText, readableOn(out.Primary))
	if theme.Header != "" && theme.HeaderText == "" {
		out.HeaderText = readableOn(out.Header)
	} else {
		set(&out.HeaderText, theme.HeaderText)
	}
	if theme.Footer != "" && theme.FooterText == "" {
		out.FooterText = readableOn(out.Footer)
	} else {
		set(&out.FooterText, theme.FooterText)
	}
	if theme.Accent == "" && theme.Primary != "" {
		out.Accent = theme.Primary
	}
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
