// Package payments takes online payments for orders through pluggable
// providers (adapter pattern, like comwork-cloud-api's PaymentAdapter): Stripe
// (PaymentIntent + Stripe.js), Konnect and Flouci (hosted payment pages).
// Adding a provider is one more Adapter registered in New.
package payments

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
)

// Statuses a provider reports for a payment.
const (
	StatusPaid    = "paid"
	StatusPending = "pending"
	StatusFailed  = "failed"
)

// Config is one provider's configuration, secrets already opened.
type Config struct {
	Mode     string            // "test" (sandbox) or "live"
	Settings map[string]any    // public settings
	Secrets  map[string]string // opened secrets
}

// Live reports production mode.
func (c Config) Live() bool { return c.Mode == "live" }

// Str reads a public setting as a string.
func (c Config) Str(name string) string {
	switch v := c.Settings[name].(type) {
	case string:
		return strings.TrimSpace(v)
	case nil:
		return ""
	default:
		return strings.TrimSpace(toString(v))
	}
}

// Bool reads a public setting as a boolean.
func (c Config) Bool(name string, def bool) bool {
	switch v := c.Settings[name].(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "yes" || v == "1"
	}
	return def
}

// Int reads a public setting as an integer.
func (c Config) Int(name string, def int) int {
	switch v := c.Settings[name].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return def
}

// Request is what is needed to start paying an order.
type Request struct {
	OrderID     uint64
	OrderKey    string
	Amount      float64 // in currency units (e.g. 12.5 EUR)
	Currency    string
	Description string
	Email       string
	FirstName   string
	LastName    string
	Phone       string
	ReturnURL   string // where the customer comes back to (UI)
	WebhookURL  string // where the provider notifies (API)
}

// Session is a started payment, for the browser.
type Session struct {
	Provider       string `json:"provider"`
	Status         string `json:"status"`
	Reference      string `json:"reference"`
	ClientSecret   string `json:"clientSecret,omitempty"`
	PublishableKey string `json:"publishableKey,omitempty"`
	RedirectURL    string `json:"redirectUrl,omitempty"`
}

// Result is a payment's state as the provider reports it.
type Result struct {
	Status        string
	Reference     string
	OrderID       uint64 // when the provider echoes our order id
	TransactionID string
	AmountMinor   int64
	Currency      string
	Detail        string
}

// Event is a parsed provider callback: which payment it is about. The
// service always re-asks the provider (Status) before trusting it.
type Event struct {
	Reference string
	OrderID   uint64
	Type      string
}

// Adapter is one payment provider.
type Adapter interface {
	ID() string
	Name() string
	DefaultTitle() string
	// PublicSettings and SecretNames are the configuration fields.
	PublicSettings() []string
	SecretNames() []string
	// Currencies supported ("*" = any).
	Currencies() []string
	Configured(cfg Config) bool
	CreatePayment(ctx context.Context, cfg Config, req Request) (Session, error)
	Status(ctx context.Context, cfg Config, reference string) (Result, error)
	// ParseWebhook authenticates (when the provider signs) and parses a callback.
	ParseWebhook(ctx context.Context, cfg Config, r *http.Request, body []byte) (Event, error)
	Test(ctx context.Context, cfg Config) error
}

// ErrNotConfigured is a provider without its credentials.
var ErrNotConfigured = errors.New("payment provider is not configured")

// ErrBadSignature is a callback that failed authentication.
var ErrBadSignature = errors.New("invalid webhook signature")

// threeDecimals and zeroDecimals are ISO 4217 minor-unit exceptions.
var threeDecimals = map[string]bool{"TND": true, "KWD": true, "BHD": true, "JOD": true, "OMR": true, "IQD": true, "LYD": true}
var zeroDecimals = map[string]bool{"JPY": true, "KRW": true, "XOF": true, "XAF": true, "VND": true, "CLP": true, "XPF": true, "UGX": true,
	"RWF": true, "PYG": true, "MGA": true, "KMF": true, "GNF": true, "DJF": true, "BIF": true, "VUV": true}

// MinorUnits converts an amount to the currency's smallest unit.
func MinorUnits(amount float64, currency string) int64 {
	c := strings.ToUpper(currency)
	switch {
	case threeDecimals[c]:
		return int64(math.Round(amount * 1000))
	case zeroDecimals[c]:
		return int64(math.Round(amount))
	}
	return int64(math.Round(amount * 100))
}

// FromMinor converts back.
func FromMinor(minor int64, currency string) float64 {
	c := strings.ToUpper(currency)
	switch {
	case threeDecimals[c]:
		return float64(minor) / 1000
	case zeroDecimals[c]:
		return float64(minor)
	}
	return float64(minor) / 100
}

func supports(a Adapter, currency string) bool {
	for _, c := range a.Currencies() {
		if c == "*" || strings.EqualFold(c, currency) {
			return true
		}
	}
	return false
}

func toString(v any) string {
	switch x := v.(type) {
	case float64:
		if x == math.Trunc(x) {
			return strings.TrimSuffix(strings.TrimSuffix(fmtFloat(x), "0"), ".")
		}
		return fmtFloat(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	}
	return ""
}
