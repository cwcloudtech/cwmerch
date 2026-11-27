package payments

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Stripe takes card (and other Payment Element) payments with a PaymentIntent
// confirmed in the browser by Stripe.js; the result comes back through the
// signed webhook (payment_intent.succeeded / payment_intent.payment_failed)
// and can always be re-read with Status.
type Stripe struct {
	BaseURL string // https://api.stripe.com/v1 (overridable for tests)
}

func (Stripe) ID() string               { return "stripe" }
func (Stripe) Name() string             { return "Stripe" }
func (Stripe) DefaultTitle() string     { return "Credit card (Stripe)" }
func (Stripe) PublicSettings() []string { return []string{"publishableKey"} }
func (Stripe) SecretNames() []string    { return []string{"secretKey", "webhookSecret"} }
func (Stripe) Currencies() []string     { return []string{"*"} }
func (s Stripe) Configured(c Config) bool {
	return c.Secrets["secretKey"] != "" && c.Str("publishableKey") != ""
}

func (s Stripe) base() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return "https://api.stripe.com/v1"
}

func (s Stripe) headers(c Config) map[string]string {
	return map[string]string{"Authorization": "Bearer " + c.Secrets["secretKey"], "Stripe-Version": "2024-06-20"}
}

type stripeIntent struct {
	ID               string            `json:"id"`
	ClientSecret     string            `json:"client_secret"`
	Status           string            `json:"status"`
	Amount           int64             `json:"amount"`
	AmountReceived   int64             `json:"amount_received"`
	Currency         string            `json:"currency"`
	LatestCharge     string            `json:"latest_charge"`
	Metadata         map[string]string `json:"metadata"`
	LastPaymentError *struct {
		Message string `json:"message"`
	} `json:"last_payment_error"`
}

func (s Stripe) CreatePayment(ctx context.Context, c Config, req Request) (Session, error) {
	if !s.Configured(c) {
		return Session{}, ErrNotConfigured
	}
	amount := MinorUnits(req.Amount, req.Currency)
	if threeDecimals[strings.ToUpper(req.Currency)] {
		amount = (amount + 5) / 10 * 10 // Stripe requires a multiple of 10 for 3-decimal currencies
	}
	form := url.Values{}
	form.Set("amount", strconv.FormatInt(amount, 10))
	form.Set("currency", strings.ToLower(req.Currency))
	form.Set("automatic_payment_methods[enabled]", "true")
	form.Set("description", req.Description)
	if req.Email != "" {
		form.Set("receipt_email", req.Email)
	}
	form.Set("metadata[order_id]", strconv.FormatUint(req.OrderID, 10))
	form.Set("metadata[order_key]", req.OrderKey)
	form.Set("metadata[source]", "cwmerch")
	h := s.headers(c)
	h["Idempotency-Key"] = "cwmerch-" + strconv.FormatUint(req.OrderID, 10) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var pi stripeIntent
	if _, err := do(ctx, "stripe", http.MethodPost, s.base()+"/payment_intents", h, strings.NewReader(form.Encode()),
		"application/x-www-form-urlencoded", &pi); err != nil {
		return Session{}, err
	}
	return Session{Provider: "stripe", Status: "requires_payment", Reference: pi.ID, ClientSecret: pi.ClientSecret,
		PublishableKey: c.Str("publishableKey")}, nil
}

func (s Stripe) Status(ctx context.Context, c Config, reference string) (Result, error) {
	if !s.Configured(c) {
		return Result{}, ErrNotConfigured
	}
	var pi stripeIntent
	if _, err := doJSON(ctx, "stripe", http.MethodGet, s.base()+"/payment_intents/"+url.PathEscape(reference), s.headers(c), nil, &pi); err != nil {
		return Result{}, err
	}
	r := Result{Reference: pi.ID, AmountMinor: pi.AmountReceived, Currency: strings.ToUpper(pi.Currency), TransactionID: pi.ID,
		OrderID: orderIDFrom(pi.Metadata["order_id"])}
	switch pi.Status {
	case "succeeded":
		r.Status = StatusPaid
		if pi.LatestCharge != "" {
			r.TransactionID = pi.LatestCharge
		}
	case "canceled":
		r.Status = StatusFailed
	case "requires_payment_method":
		r.Status = StatusPending
		if pi.LastPaymentError != nil {
			r.Status, r.Detail = StatusFailed, pi.LastPaymentError.Message
		}
	default:
		r.Status = StatusPending
	}
	return r, nil
}

func (s Stripe) ParseWebhook(ctx context.Context, c Config, r *http.Request, body []byte) (Event, error) {
	secret := c.Secrets["webhookSecret"]
	if secret == "" {
		return Event{}, errors.New("no Stripe webhook secret configured")
	}
	if err := verifyTimestampedHMAC(r.Header.Get("Stripe-Signature"), secret, body, 5*time.Minute); err != nil {
		return Event{}, err
	}
	var ev struct {
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID       string            `json:"id"`
				Object   string            `json:"object"`
				Metadata map[string]string `json:"metadata"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		return Event{}, err
	}
	if ev.Data.Object.Object != "payment_intent" || ev.Data.Object.Metadata["source"] != "cwmerch" {
		return Event{Type: ev.Type}, nil // not ours: acknowledged, ignored
	}
	return Event{Reference: ev.Data.Object.ID, OrderID: orderIDFrom(ev.Data.Object.Metadata["order_id"]), Type: ev.Type}, nil
}

func (s Stripe) Test(ctx context.Context, c Config) error {
	if !s.Configured(c) {
		return ErrNotConfigured
	}
	_, err := doJSON(ctx, "stripe", http.MethodGet, s.base()+"/balance", s.headers(c), nil, nil)
	if err == nil && strings.HasPrefix(c.Secrets["secretKey"], "sk_live") != c.Live() {
		return errors.New("the secret key does not match the selected mode (test keys start with sk_test_, live keys with sk_live_)")
	}
	return err
}
