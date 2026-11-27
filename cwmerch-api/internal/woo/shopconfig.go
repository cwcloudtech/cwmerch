package woo

import (
	"context"
	"strings"
)

// Languages the storefront, account, back office and emails are available in.
// PDF exports (invoices, accounting) stay in English or French.
var SupportedLanguages = []string{"en", "fr", "ar"}

// ShopConfig switches the public shop (cwmerch can run as an internal
// information system only: back office, accounting, AI, no storefront) and
// lists the languages the shop offers.
type ShopConfig struct {
	ShopEnabled         bool     `json:"shopEnabled"`
	ClosedMessage       string   `json:"closedMessage"`
	RegistrationEnabled bool     `json:"registrationEnabled"`
	Languages           []string `json:"languages"`
	DefaultLanguage     string   `json:"defaultLanguage"`
	// HidePoweredBy hides the storefront footer's "Powered by cwmerch <version>"
	// (shown by default). Omitted in an update: unchanged.
	HidePoweredBy *bool `json:"hidePoweredBy"`
}

func supportedLanguage(l string) bool {
	for _, s := range SupportedLanguages {
		if s == l {
			return true
		}
	}
	return false
}

// ShopConfig reads the switch (open, every language, by default).
func (s *Store) ShopConfig(ctx context.Context) ShopConfig {
	c := ShopConfig{ShopEnabled: true, RegistrationEnabled: true}
	_, _, _ = s.Setting(ctx, "shop", &c)
	if len(c.Languages) == 0 {
		c.Languages = append([]string{}, SupportedLanguages...)
	}
	if c.HidePoweredBy == nil {
		hide := false
		c.HidePoweredBy = &hide
	}
	if c.DefaultLanguage == "" || !contains(strings.Join(c.Languages, ","), c.DefaultLanguage) {
		c.DefaultLanguage = c.Languages[0]
		if wp := strings.ToLower(s.Option(ctx, "WPLANG")); wp != "" {
			for _, l := range c.Languages {
				if strings.HasPrefix(wp, l) {
					c.DefaultLanguage = l
				}
			}
		}
	}
	return c
}

// SaveShopConfig validates and writes the switch and the languages.
func (s *Store) SaveShopConfig(ctx context.Context, c ShopConfig, by uint64) (ShopConfig, error) {
	cur := s.ShopConfig(ctx)
	if c.Languages == nil {
		// languages omitted (a partial update): keep the current ones
		c.Languages = cur.Languages
		if c.DefaultLanguage == "" {
			c.DefaultLanguage = cur.DefaultLanguage
		}
	}
	if c.HidePoweredBy == nil {
		c.HidePoweredBy = cur.HidePoweredBy
	}
	seen := map[string]bool{}
	langs := []string{}
	for _, l := range c.Languages {
		l = strings.ToLower(strings.TrimSpace(l))
		if !supportedLanguage(l) {
			return ShopConfig{}, refuse(400, "errors.unsupportedLanguage", "Unsupported language: "+l)
		}
		if !seen[l] {
			seen[l] = true
			langs = append(langs, l)
		}
	}
	if len(langs) == 0 {
		return ShopConfig{}, refuse(400, "errors.languageRequired", "At least one language must be enabled")
	}
	c.Languages = langs
	if c.DefaultLanguage == "" {
		c.DefaultLanguage = langs[0]
	}
	if !seen[c.DefaultLanguage] {
		return ShopConfig{}, refuse(400, "errors.invalidDefaultLanguage", "The default language must be one of the enabled languages")
	}
	if err := s.SaveSetting(ctx, "shop", c, by); err != nil {
		return ShopConfig{}, err
	}
	return s.ShopConfig(ctx), nil
}
