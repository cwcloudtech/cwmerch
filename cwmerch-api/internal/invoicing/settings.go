// Package invoicing issues invoices for orders: numbered without gaps, frozen
// as a JSON snapshot, written as Markdown from a Go template the shop can
// edit, and rendered to PDF. Taxes follow the shop's settings: VAT (from the
// order's WooCommerce tax lines), a fiscal stamp (timbre fiscal), both, or
// none, with defaults per country from a decision table.
package invoicing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"cwmerch-api/internal/woo"
)

// Tax modes.
const (
	TaxVAT      = "vat"
	TaxStamp    = "stamp"
	TaxVATStamp = "vat_stamp"
	TaxNone     = "none"
)

func validTaxMode(m string) bool {
	return m == TaxVAT || m == TaxStamp || m == TaxVATStamp || m == TaxNone
}

// HasVAT / HasStamp read a tax mode.
func HasVAT(m string) bool   { return m == TaxVAT || m == TaxVATStamp }
func HasStamp(m string) bool { return m == TaxStamp || m == TaxVATStamp }

// Seller is the issuer block of the invoice.
type Seller struct {
	Name               string `json:"name"`
	Address            string `json:"address"`
	Postcode           string `json:"postcode"`
	City               string `json:"city"`
	Country            string `json:"country"`
	Email              string `json:"email"`
	Phone              string `json:"phone"`
	Website            string `json:"website"`
	RegistrationLabel  string `json:"registrationLabel"`
	RegistrationNumber string `json:"registrationNumber"`
	VatNumberLabel     string `json:"vatNumberLabel"`
	VatNumber          string `json:"vatNumber"`
	IBAN               string `json:"iban"`
	BIC                string `json:"bic"`
}

// Settings is the shop's invoicing configuration (setting "invoicing", JSON).
type Settings struct {
	Enabled             bool    `json:"enabled"`
	AutoGenerate        string  `json:"autoGenerate"` // none | processing | completed
	NumberPrefix        string  `json:"numberPrefix"`
	NextNumber          int     `json:"nextNumber"`
	NumberPadding       int     `json:"numberPadding"`
	ResetYearly         bool    `json:"resetYearly"`
	Country             string  `json:"country"`
	Locale              string  `json:"locale"`
	TaxMode             string  `json:"taxMode"`
	VatLabel            string  `json:"vatLabel"`
	DefaultVatRate      float64 `json:"defaultVatRate"`
	VatExemptionMention string  `json:"vatExemptionMention"`
	StampAmount         float64 `json:"stampAmount"`
	StampLabel          string  `json:"stampLabel"`
	StampChargeCustomer bool    `json:"stampChargeCustomer"`
	Seller              Seller  `json:"seller"`
	LegalMentions       string  `json:"legalMentions"`
	PaymentTerms        string  `json:"paymentTerms"`
	Footer              string  `json:"footer"`
	Template            string  `json:"template"`
}

// CountryDefaults is one row of the decision table.
type CountryDefaults struct {
	Country             string  `json:"country"`
	Name                string  `json:"name"`
	TaxMode             string  `json:"taxMode"`
	VatLabel            string  `json:"vatLabel"`
	DefaultVatRate      float64 `json:"defaultVatRate"`
	StampAmount         float64 `json:"stampAmount"`
	StampLabel          string  `json:"stampLabel"`
	Currency            string  `json:"currency"`
	RegistrationLabel   string  `json:"registrationLabel"`
	VatNumberLabel      string  `json:"vatNumberLabel"`
	NumberPrefix        string  `json:"numberPrefix"`
	Locale              string  `json:"locale"`
	VatExemptionMention string  `json:"vatExemptionMention"`
	LegalMentions       string  `json:"legalMentions"`
	Fallback            bool    `json:"fallback,omitempty"`
}

// Service is invoicing on the shop's database.
type Service struct {
	woo *woo.Store
}

func New(w *woo.Store) *Service { return &Service{woo: w} }

func (s *Service) defaults(ctx context.Context) Settings {
	country, _ := splitCountry(s.woo.OptionOr(ctx, "woocommerce_default_country", "FR"))
	set := Settings{Enabled: true, AutoGenerate: "none", NumberPrefix: "INV-", NextNumber: 1, NumberPadding: 5, ResetYearly: true,
		Country: country, Locale: "en", TaxMode: TaxVAT, VatLabel: "VAT"}
	if row, err := s.CountryDefault(ctx, country); err == nil {
		applyRow(&set, row)
	}
	set.Seller = Seller{Name: s.woo.Option(ctx, "blogname"), Address: s.woo.Option(ctx, "woocommerce_store_address"),
		Postcode: s.woo.Option(ctx, "woocommerce_store_postcode"), City: s.woo.Option(ctx, "woocommerce_store_city"),
		Country: country, Email: s.woo.Option(ctx, "woocommerce_email_from_address"), Website: s.woo.SiteURL,
		RegistrationLabel: set.Seller.RegistrationLabel, VatNumberLabel: set.Seller.VatNumberLabel}
	if row, err := s.CountryDefault(ctx, country); err == nil {
		set.Seller.RegistrationLabel, set.Seller.VatNumberLabel = row.RegistrationLabel, row.VatNumberLabel
	}
	return set
}

func splitCountry(v string) (string, string) {
	parts := strings.SplitN(v, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return v, ""
}

func applyRow(set *Settings, row CountryDefaults) {
	set.TaxMode, set.VatLabel, set.DefaultVatRate = row.TaxMode, row.VatLabel, row.DefaultVatRate
	set.StampAmount, set.StampLabel = row.StampAmount, row.StampLabel
	set.VatExemptionMention, set.LegalMentions = row.VatExemptionMention, row.LegalMentions
	if row.NumberPrefix != "" {
		set.NumberPrefix = row.NumberPrefix
	}
	if row.Locale != "" {
		set.Locale = row.Locale
	}
	if row.Country != "*" {
		set.Country = row.Country
		set.Seller.Country = row.Country
	}
	set.Seller.RegistrationLabel, set.Seller.VatNumberLabel = row.RegistrationLabel, row.VatNumberLabel
}

// Settings reads the configuration (defaults from the shop's country).
func (s *Service) Settings(ctx context.Context) Settings {
	set := s.defaults(ctx)
	_, _, _ = s.woo.Setting(ctx, "invoicing", &set)
	if set.NumberPadding <= 0 {
		set.NumberPadding = 5
	}
	if set.NextNumber <= 0 {
		set.NextNumber = 1
	}
	return set
}

var prefixRe = regexp.MustCompile(`^[A-Za-z0-9/_.\-]{0,20}$`)

// SaveSettings validates and writes the configuration.
func (s *Service) SaveSettings(ctx context.Context, set Settings, by uint64) (Settings, error) {
	if !validTaxMode(set.TaxMode) {
		return Settings{}, &woo.Error{Status: 400, Code: "errors.invalidTaxMode", Message: "Tax mode must be vat, stamp, vat_stamp or none"}
	}
	switch set.AutoGenerate {
	case "none", "processing", "completed":
	case "":
		set.AutoGenerate = "none"
	default:
		return Settings{}, &woo.Error{Status: 400, Code: "errors.invalidAutoGenerate", Message: "autoGenerate must be none, processing or completed"}
	}
	if !prefixRe.MatchString(set.NumberPrefix) {
		return Settings{}, &woo.Error{Status: 400, Code: "errors.invalidInvoicePrefix", Message: "The number prefix may only hold letters, digits and - _ / ."}
	}
	if set.StampAmount < 0 || set.DefaultVatRate < 0 || set.DefaultVatRate > 100 {
		return Settings{}, &woo.Error{Status: 400, Code: "errors.invalidInvoiceAmounts", Message: "Invalid VAT rate or stamp amount"}
	}
	// PDF exports are never Arabic: an Arabic-speaking shop gets English invoices.
	if set.Locale != "fr" {
		set.Locale = "en"
	}
	if set.NumberPadding < 1 || set.NumberPadding > 10 {
		set.NumberPadding = 5
	}
	if strings.TrimSpace(set.Template) != "" {
		if _, err := parseTemplate(set.Template); err != nil {
			return Settings{}, &woo.Error{Status: 400, Code: "errors.invalidInvoiceTemplate", Message: "Invalid template: " + err.Error()}
		}
	}
	set.Country = strings.ToUpper(set.Country)
	if err := s.woo.SaveSetting(ctx, "invoicing", set, by); err != nil {
		return Settings{}, err
	}
	return s.Settings(ctx), nil
}

// ------------------------------------------------------------------ decision table

func (s *Service) scanDefaults(ctx context.Context, where string, args ...any) ([]CountryDefaults, error) {
	rows, err := s.woo.DB.QueryContext(ctx, `SELECT country, data FROM `+s.woo.T("cwmerch_country_defaults")+` WHERE `+where+` ORDER BY country = '*' DESC, country`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CountryDefaults{}
	for rows.Next() {
		var c CountryDefaults
		var raw []byte
		var country string
		if err := rows.Scan(&country, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &c)
		c.Country = strings.TrimSpace(country) // the column is the key, never the payload
		out = append(out, c)
	}
	return out, rows.Err()
}

// CountryDefaults lists the decision table.
func (s *Service) CountryDefaults(ctx context.Context) ([]CountryDefaults, error) {
	return s.scanDefaults(ctx, "1=1")
}

// CountryDefault is a country's row, else the '*' fallback (Fallback=true).
func (s *Service) CountryDefault(ctx context.Context, iso string) (CountryDefaults, error) {
	iso = strings.ToUpper(strings.TrimSpace(iso))
	rows, err := s.scanDefaults(ctx, "country IN (?, '*')", iso)
	if err != nil {
		return CountryDefaults{}, err
	}
	var fallback *CountryDefaults
	for i := range rows {
		if rows[i].Country == iso {
			return rows[i], nil
		}
		if rows[i].Country == "*" {
			fallback = &rows[i]
		}
	}
	if fallback == nil {
		return CountryDefaults{}, woo.ErrNotFound
	}
	f := *fallback
	f.Fallback = true
	return f, nil
}

var isoRe = regexp.MustCompile(`^([A-Z]{2}|\*)$`)

// SaveCountryDefault creates or replaces a row.
func (s *Service) SaveCountryDefault(ctx context.Context, iso string, row CountryDefaults) (CountryDefaults, error) {
	iso = strings.ToUpper(strings.TrimSpace(iso))
	if !isoRe.MatchString(iso) {
		return CountryDefaults{}, &woo.Error{Status: 400, Code: "errors.invalidCountry", Message: "Country must be an ISO 3166-1 alpha-2 code or *"}
	}
	if row.TaxMode == "" {
		row.TaxMode = TaxVAT
	}
	if row.Locale != "fr" {
		row.Locale = "en" // invoices are in English or French, never Arabic
	}
	if !validTaxMode(row.TaxMode) {
		return CountryDefaults{}, &woo.Error{Status: 400, Code: "errors.invalidTaxMode", Message: "Tax mode must be vat, stamp, vat_stamp or none"}
	}
	if row.DefaultVatRate < 0 || row.DefaultVatRate > 100 || row.StampAmount < 0 {
		return CountryDefaults{}, &woo.Error{Status: 400, Code: "errors.invalidInvoiceAmounts", Message: "Invalid VAT rate or stamp amount"}
	}
	if row.Name == "" {
		if c, ok := woo.CountryByCode(iso); ok {
			row.Name = c.Name
		}
	}
	row.Country, row.Fallback = "", false
	raw, _ := json.Marshal(row)
	if _, err := s.woo.DB.ExecContext(ctx, `INSERT INTO `+s.woo.T("cwmerch_country_defaults")+` (country, data) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE data = VALUES(data)`, iso, string(raw)); err != nil {
		return CountryDefaults{}, err
	}
	return s.CountryDefault(ctx, iso)
}

// DeleteCountryDefault removes a row (never the fallback).
func (s *Service) DeleteCountryDefault(ctx context.Context, iso string) error {
	iso = strings.ToUpper(strings.TrimSpace(iso))
	if iso == "*" {
		return &woo.Error{Status: 400, Code: "errors.fallbackCountry", Message: "The fallback row cannot be deleted"}
	}
	res, err := s.woo.DB.ExecContext(ctx, `DELETE FROM `+s.woo.T("cwmerch_country_defaults")+` WHERE country = ?`, iso)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return woo.ErrNotFound
	}
	return nil
}

// ApplyDefaults fills the settings from a country's row and, when asked,
// creates the standard WooCommerce tax rate for that country if none exists.
func (s *Service) ApplyDefaults(ctx context.Context, iso string, createTaxRate bool, by uint64) (Settings, error) {
	row, err := s.CountryDefault(ctx, iso)
	if err != nil {
		return Settings{}, err
	}
	set := s.Settings(ctx)
	applyRow(&set, row)
	if row.Fallback {
		set.Country = strings.ToUpper(iso)
		set.Seller.Country = set.Country
	}
	if createTaxRate && HasVAT(row.TaxMode) && row.DefaultVatRate > 0 {
		rates, err := s.woo.TaxRates(ctx, "standard")
		if err != nil {
			return Settings{}, err
		}
		exists := false
		for _, r := range rates {
			if strings.EqualFold(r.Country, set.Country) {
				exists = true
			}
		}
		if !exists {
			if _, err := s.woo.SaveTaxRate(ctx, 0, woo.TaxRate{Country: set.Country, Rate: row.DefaultVatRate, Name: row.VatLabel,
				Priority: 1, Shipping: true}); err != nil {
				return Settings{}, err
			}
			_ = s.woo.SetOption(ctx, "woocommerce_calc_taxes", "yes")
		}
	}
	return s.SaveSettings(ctx, set, by)
}

// StampFee is the cart fee hook: the fiscal stamp charged to the customer.
func (s *Service) StampFee(ctx context.Context, cart woo.Cart) []woo.CartFee {
	set := s.Settings(ctx)
	if !HasStamp(set.TaxMode) || !set.StampChargeCustomer || set.StampAmount <= 0 {
		return nil
	}
	return []woo.CartFee{{Name: orDefault(set.StampLabel, "Timbre fiscal"), Total: set.StampAmount}}
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

var errNoRows = sql.ErrNoRows

func isNoRows(err error) bool { return errors.Is(err, errNoRows) }
