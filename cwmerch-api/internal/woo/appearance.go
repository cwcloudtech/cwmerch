package woo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Colors are the storefront's theme colours (CSS variables --cwmerch-*).
type Colors struct {
	Primary     string `json:"primary"`
	PrimaryText string `json:"primaryText"`
	Secondary   string `json:"secondary"`
	Accent      string `json:"accent"`
	Background  string `json:"background"`
	Surface     string `json:"surface"`
	Text        string `json:"text"`
	Muted       string `json:"muted"`
	Border      string `json:"border"`
	Header      string `json:"header"`
	HeaderText  string `json:"headerText"`
	Footer      string `json:"footer"`
	FooterText  string `json:"footerText"`
	Sale        string `json:"sale"`
}

// Fonts are the heading/body font families.
type Fonts struct {
	Heading string `json:"heading"`
	Body    string `json:"body"`
}

// Theme is a colour/font preset.
type Theme struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Colors Colors `json:"colors"`
	Fonts  Fonts  `json:"fonts"`
}

// Themes are the presets offered in the appearance editor.
var Themes = []Theme{
	// The default: cwclock's blues (consistent with the cwmerch logo); the
	// primary is cwclock's 700 shade so white text on it stays readable.
	{ID: "classic", Name: "Classic", Fonts: Fonts{"Inter", "Inter"}, Colors: Colors{Primary: "#0a7fae", PrimaryText: "#ffffff", Secondary: "#334155", Accent: "#1cb9f7",
		Background: "#ffffff", Surface: "#f8fafc", Text: "#0f172a", Muted: "#64748b", Border: "#e2e8f0", Header: "#ffffff", HeaderText: "#0f172a", Footer: "#0f172a", FooterText: "#f1f5f9", Sale: "#dc2626"}},
	{ID: "garden", Name: "Garden", Fonts: Fonts{"Playfair Display", "Lato"}, Colors: Colors{Primary: "#2f6b3a", PrimaryText: "#ffffff", Secondary: "#7a5c3e", Accent: "#c9a227",
		Background: "#fbfaf5", Surface: "#f1efe4", Text: "#24301f", Muted: "#66705f", Border: "#dcd8c6", Header: "#ffffff", HeaderText: "#24301f", Footer: "#1f3b24", FooterText: "#f1efe4", Sale: "#b5452b"}},
	{ID: "rose", Name: "Rose", Fonts: Fonts{"Cormorant Garamond", "Montserrat"}, Colors: Colors{Primary: "#c2185b", PrimaryText: "#ffffff", Secondary: "#5e3a4c", Accent: "#f5b7c8",
		Background: "#fffafb", Surface: "#fdeef3", Text: "#3a2330", Muted: "#86677a", Border: "#f2d6e0", Header: "#ffffff", HeaderText: "#3a2330", Footer: "#5e1d3a", FooterText: "#fdeef3", Sale: "#e53935"}},
	{ID: "ocean", Name: "Ocean", Fonts: Fonts{"Poppins", "Open Sans"}, Colors: Colors{Primary: "#1565c0", PrimaryText: "#ffffff", Secondary: "#0d3b66", Accent: "#26c6da",
		Background: "#ffffff", Surface: "#eef5fc", Text: "#14213d", Muted: "#5c6b80", Border: "#d6e4f2", Header: "#ffffff", HeaderText: "#14213d", Footer: "#0d3b66", FooterText: "#eef5fc", Sale: "#e63946"}},
	{ID: "sunset", Name: "Sunset", Fonts: Fonts{"Merriweather", "Nunito"}, Colors: Colors{Primary: "#e85d04", PrimaryText: "#ffffff", Secondary: "#6a040f", Accent: "#ffba08",
		Background: "#fffbf5", Surface: "#fff1e0", Text: "#2b1a12", Muted: "#7d6658", Border: "#f3dcc4", Header: "#ffffff", HeaderText: "#2b1a12", Footer: "#370617", FooterText: "#fff1e0", Sale: "#d00000"}},
	{ID: "midnight", Name: "Midnight (dark)", Fonts: Fonts{"Space Grotesk", "Inter"}, Colors: Colors{Primary: "#8b5cf6", PrimaryText: "#ffffff", Secondary: "#a1a1aa", Accent: "#22d3ee",
		Background: "#0f1117", Surface: "#181b24", Text: "#e7e7ee", Muted: "#9a9cab", Border: "#2a2e3b", Header: "#11131a", HeaderText: "#e7e7ee", Footer: "#0a0b10", FooterText: "#c9cad6", Sale: "#f87171"}},
	{ID: "minimal", Name: "Minimal", Fonts: Fonts{"DM Sans", "DM Sans"}, Colors: Colors{Primary: "#111111", PrimaryText: "#ffffff", Secondary: "#555555", Accent: "#999999",
		Background: "#ffffff", Surface: "#f5f5f5", Text: "#111111", Muted: "#6b6b6b", Border: "#e5e5e5", Header: "#ffffff", HeaderText: "#111111", Footer: "#ffffff", FooterText: "#111111", Sale: "#c62828"}},
	{ID: "luxury", Name: "Luxury", Fonts: Fonts{"Cinzel", "Raleway"}, Colors: Colors{Primary: "#c9a227", PrimaryText: "#111111", Secondary: "#e8d9a8", Accent: "#8c6d1f",
		Background: "#121212", Surface: "#1c1c1c", Text: "#f2ead3", Muted: "#a89f86", Border: "#333026", Header: "#0b0b0b", HeaderText: "#f2ead3", Footer: "#0b0b0b", FooterText: "#c9b98a", Sale: "#e57373"}},
}

// ThemeByID finds a preset.
func ThemeByID(id string) (Theme, bool) {
	for _, t := range Themes {
		if t.ID == id {
			return t, true
		}
	}
	return Theme{}, false
}

// Slide is a home banner slide.
type Slide struct {
	Image       string `json:"image"`
	MobileImage string `json:"mobileImage"`
	Title       string `json:"title"`
	Subtitle    string `json:"subtitle"`
	ButtonText  string `json:"buttonText"`
	ButtonLink  string `json:"buttonLink"`
	Align       string `json:"align"`
	Overlay     int    `json:"overlay"`
	TextColor   string `json:"textColor"`
}

// Banner is the home hero banner/slider.
type Banner struct {
	Enabled         bool    `json:"enabled"`
	Autoplay        bool    `json:"autoplay"`
	IntervalSeconds int     `json:"intervalSeconds"`
	Height          string  `json:"height"`
	Slides          []Slide `json:"slides"`
}

// Announcement is the bar above the header.
type Announcement struct {
	Enabled bool   `json:"enabled"`
	Text    string `json:"text"`
	Link    string `json:"link"`
}

// HomeSection is a block of the home page.
type HomeSection struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Limit   int    `json:"limit"`
	Content string `json:"content"`
}

// Appearance is the storefront's look, edited in the admin.
type Appearance struct {
	Name         string            `json:"name"`
	Tagline      string            `json:"tagline"`
	Logo         *Image            `json:"logo"`
	Favicon      string            `json:"favicon"`
	Theme        string            `json:"theme"`
	Colors       Colors            `json:"colors"`
	Fonts        Fonts             `json:"fonts"`
	Radius       int               `json:"radius"`
	Banner       Banner            `json:"banner"`
	Announcement Announcement      `json:"announcement"`
	HomeSections []HomeSection     `json:"homeSections"`
	Social       map[string]string `json:"social"`
	Contact      map[string]string `json:"contact"`
	FooterText   string            `json:"footerText"`
	CustomCSS    string            `json:"customCss,omitempty"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

func defaultAppearance() Appearance {
	t := Themes[0]
	return Appearance{Theme: t.ID, Colors: t.Colors, Fonts: t.Fonts, Radius: 8,
		Banner: Banner{Enabled: true, Autoplay: true, IntervalSeconds: 6, Height: "large", Slides: []Slide{}},
		HomeSections: []HomeSection{{Type: "categories", Title: "Shop by category", Limit: 8}, {Type: "featured", Title: "Featured", Limit: 8},
			{Type: "latest", Title: "New arrivals", Limit: 8}, {Type: "onSale", Title: "On sale", Limit: 4}},
		Social:  map[string]string{"facebook": "", "instagram": "", "x": "", "tiktok": "", "youtube": "", "whatsapp": "", "pinterest": ""},
		Contact: map[string]string{"phone": "", "email": "", "address": ""}}
}

// Setting reads a cwmerch_settings document into v (false when missing).
func (s *Store) Setting(ctx context.Context, name string, v any) (time.Time, bool, error) {
	var raw string
	var at time.Time
	err := s.DB.QueryRowContext(ctx, `SELECT data, updated_at FROM `+s.T("cwmerch_settings")+` WHERE name = ?`, name).Scan(&raw, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return at, true, json.Unmarshal([]byte(raw), v)
}

// SaveSetting writes a cwmerch_settings document.
func (s *Store) SaveSetting(ctx context.Context, name string, v any, by uint64) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO `+s.T("cwmerch_settings")+` (name, data, updated_by) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE data = VALUES(data), updated_by = VALUES(updated_by)`, name, string(raw), nullIfZero(by))
	return err
}

// storedAppearance is cwmerch's own appearance document: colors and fonts
// hold only the admin's overrides (empty = inherit from the detected look).
func (s *Store) storedAppearance(ctx context.Context) (Appearance, bool, error) {
	a := defaultAppearance()
	a.Theme, a.Colors, a.Fonts = "", Colors{}, Fonts{}
	at, saved, err := s.Setting(ctx, "appearance", &a)
	if err != nil {
		return Appearance{}, false, err
	}
	a.UpdatedAt = at
	if a.Banner.Slides == nil {
		a.Banner.Slides = []Slide{}
	}
	if a.HomeSections == nil {
		a.HomeSections = []HomeSection{}
	}
	return a, saved, nil
}

// Appearance is the storefront appearance as served: each color and font is
// the admin's override, else Divi's, WooCommerce's, WordPress', else
// cwmerch's default (see DetectedLook); the shop name and tagline fall back to
// WordPress' blogname / blogdescription.
func (s *Store) Appearance(ctx context.Context, withCSS bool) (Appearance, error) {
	a, _, err := s.storedAppearance(ctx)
	if err != nil {
		return Appearance{}, err
	}
	look := s.DetectedLook(ctx)
	a.Colors = effectiveColors(a.Colors, look.Colors)
	a.Fonts = effectiveFonts(a.Fonts, look.Fonts)
	if a.Theme == "" {
		a.Theme = Themes[0].ID
		if look.inherited() {
			a.Theme = WordPressThemeID
		}
	}
	if a.Logo == nil || strings.TrimSpace(a.Logo.Src) == "" {
		a.Logo = look.Logo
	}
	if a.Name == "" {
		a.Name = look.Name
	}
	if a.Tagline == "" {
		a.Tagline = look.Tagline
	}
	if !withCSS {
		a.CustomCSS = ""
	}
	return a, nil
}

// AdminAppearance is what the appearance editor edits: the admin's overrides
// (empty colors/fonts/name = inherited) and the detected look, shown as the
// placeholders of the fields left empty.
type AdminAppearance struct {
	Appearance
	Detected Look `json:"detected"`
}

// AdminAppearance reads the overrides and the detected look. Nothing detected
// is ever written to cwmerch's settings.
func (s *Store) AdminAppearance(ctx context.Context) (AdminAppearance, error) {
	a, _, err := s.storedAppearance(ctx)
	if err != nil {
		return AdminAppearance{}, err
	}
	return AdminAppearance{Appearance: a, Detected: s.DetectedLook(ctx)}, nil
}

var cssColor = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|rgba?\([0-9.,\s%]+\)|hsla?\([0-9.,\s%deg]+\)|[a-zA-Z]+)$`)
var fontName = regexp.MustCompile(`^[A-Za-z0-9 \-]{0,60}$`)

// SaveAppearance validates and writes the admin's overrides; the shop name
// also goes to WordPress' blogname so both stay in step.
func (s *Store) SaveAppearance(ctx context.Context, a Appearance, by uint64) (AdminAppearance, error) {
	if err := validColors(a.Colors); err != nil {
		return AdminAppearance{}, err
	}
	if !fontName.MatchString(a.Fonts.Heading) || !fontName.MatchString(a.Fonts.Body) {
		return AdminAppearance{}, refuse(400, "errors.invalidFont", "Invalid font name")
	}
	if strings.Contains(strings.ToLower(a.CustomCSS), "</style") {
		return AdminAppearance{}, refuse(400, "errors.invalidCss", "Custom CSS cannot contain </style>")
	}
	if len(a.CustomCSS) > 100_000 {
		return AdminAppearance{}, refuse(400, "errors.cssTooLarge", "Custom CSS is too large")
	}
	if a.Radius < 0 || a.Radius > 40 {
		a.Radius = 8
	}
	for i := range a.Banner.Slides {
		if a.Banner.Slides[i].Overlay < 0 || a.Banner.Slides[i].Overlay > 90 {
			a.Banner.Slides[i].Overlay = 30
		}
	}
	if a.Banner.IntervalSeconds < 2 {
		a.Banner.IntervalSeconds = 6
	}
	// Only the overrides are stored (empty = inherit); choosing the detected
	// preset clears them.
	if a.Theme == WordPressThemeID {
		a.Theme = ""
	}
	for _, r := range colorRefs(&a.Colors) {
		*r.p = strings.TrimSpace(*r.p)
	}
	a.Fonts.Heading, a.Fonts.Body = strings.TrimSpace(a.Fonts.Heading), strings.TrimSpace(a.Fonts.Body)
	a.UpdatedAt = time.Time{}
	if err := s.SaveSetting(ctx, "appearance", a, by); err != nil {
		return AdminAppearance{}, err
	}
	if strings.TrimSpace(a.Name) != "" {
		_ = s.SetOption(ctx, "blogname", a.Name)
	}
	if a.Tagline != "" {
		_ = s.SetOption(ctx, "blogdescription", a.Tagline)
	}
	return s.AdminAppearance(ctx)
}

func validColors(c Colors) error {
	for _, v := range []string{c.Primary, c.PrimaryText, c.Secondary, c.Accent, c.Background, c.Surface, c.Text, c.Muted, c.Border,
		c.Header, c.HeaderText, c.Footer, c.FooterText, c.Sale} {
		if v != "" && !cssColor.MatchString(strings.TrimSpace(v)) {
			return refuse(400, "errors.invalidColor", "Invalid colour: "+v)
		}
	}
	return nil
}

// AppearanceCSS is the stylesheet served at /v1/shop/custom.css: the theme
// variables, then the admin's custom CSS.
func (s *Store) AppearanceCSS(ctx context.Context) (string, time.Time, error) {
	a, err := s.Appearance(ctx, true)
	if err != nil {
		return "", time.Time{}, err
	}
	vars := map[string]string{
		"--cwmerch-primary": a.Colors.Primary, "--cwmerch-primary-text": a.Colors.PrimaryText, "--cwmerch-secondary": a.Colors.Secondary,
		"--cwmerch-accent": a.Colors.Accent, "--cwmerch-bg": a.Colors.Background, "--cwmerch-surface": a.Colors.Surface, "--cwmerch-text": a.Colors.Text,
		"--cwmerch-muted": a.Colors.Muted, "--cwmerch-border": a.Colors.Border, "--cwmerch-header": a.Colors.Header, "--cwmerch-header-text": a.Colors.HeaderText,
		"--cwmerch-footer": a.Colors.Footer, "--cwmerch-footer-text": a.Colors.FooterText, "--cwmerch-sale": a.Colors.Sale,
		"--cwmerch-font-heading": fmt.Sprintf("%q, system-ui, sans-serif", a.Fonts.Heading), "--cwmerch-font-body": fmt.Sprintf("%q, system-ui, sans-serif", a.Fonts.Body),
		"--cwmerch-radius": fmt.Sprintf("%dpx", a.Radius),
	}
	keys := make([]string, 0, len(vars))
	for k, v := range vars {
		if strings.TrimSpace(v) != "" && v != `"", system-ui, sans-serif` {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("/* cwmerch theme: " + a.Theme + " */\n:root {\n")
	for _, k := range keys {
		b.WriteString("  " + k + ": " + vars[k] + ";\n")
	}
	b.WriteString("}\n")
	if a.CustomCSS != "" {
		b.WriteString("\n/* custom CSS */\n" + a.CustomCSS + "\n")
	}
	return b.String(), a.UpdatedAt, nil
}
