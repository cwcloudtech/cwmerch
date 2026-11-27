package payments

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Konnect (Tunisia) redirects the customer to a hosted payment page; it
// notifies a GET webhook carrying payment_ref only, so the status is always
// read back from GET /payments/{ref}.
type Konnect struct {
	BaseURL string // overrides the sandbox/production URL (tests)
}

func (Konnect) ID() string           { return "konnect" }
func (Konnect) Name() string         { return "Konnect" }
func (Konnect) DefaultTitle() string { return "Konnect (card, wallet, e-DINAR)" }
func (Konnect) PublicSettings() []string {
	return []string{"walletId", "acceptedPaymentMethods", "lifespan", "addPaymentFeesToAmount", "theme"}
}
func (Konnect) SecretNames() []string { return []string{"apiKey"} }
func (Konnect) Currencies() []string  { return []string{"TND", "EUR", "USD"} }
func (k Konnect) Configured(c Config) bool {
	return c.Secrets["apiKey"] != "" && c.Str("walletId") != ""
}

func (k Konnect) base(c Config) string {
	if k.BaseURL != "" {
		return strings.TrimRight(k.BaseURL, "/")
	}
	if c.Live() {
		return "https://api.konnect.network/api/v2"
	}
	return "https://api.sandbox.konnect.network/api/v2"
}

func (k Konnect) CreatePayment(ctx context.Context, c Config, req Request) (Session, error) {
	if !k.Configured(c) {
		return Session{}, ErrNotConfigured
	}
	methods := []string{"wallet", "bank_card", "e-DINAR"}
	if raw, ok := c.Settings["acceptedPaymentMethods"].([]any); ok && len(raw) > 0 {
		methods = methods[:0]
		for _, m := range raw {
			if s, ok := m.(string); ok {
				methods = append(methods, s)
			}
		}
	}
	theme := c.Str("theme")
	if theme == "" {
		theme = "light"
	}
	body := map[string]any{
		"receiverWalletId": c.Str("walletId"), "token": strings.ToUpper(req.Currency),
		"amount": MinorUnits(req.Amount, req.Currency), "type": "immediate", "description": req.Description,
		"acceptedPaymentMethods": methods, "lifespan": c.Int("lifespan", 30), "checkoutForm": false,
		"addPaymentFeesToAmount": c.Bool("addPaymentFeesToAmount", false), "firstName": req.FirstName, "lastName": req.LastName,
		"phoneNumber": req.Phone, "email": req.Email, "orderId": "order-" + strconv.FormatUint(req.OrderID, 10), "webhook": req.WebhookURL,
		"successUrl": req.ReturnURL, "failUrl": req.ReturnURL, "theme": theme,
	}
	var out struct {
		PayURL     string `json:"payUrl"`
		PaymentRef string `json:"paymentRef"`
	}
	if _, err := doJSON(ctx, "konnect", http.MethodPost, k.base(c)+"/payments/init-payment",
		map[string]string{"x-api-key": c.Secrets["apiKey"]}, body, &out); err != nil {
		return Session{}, err
	}
	if out.PayURL == "" || out.PaymentRef == "" {
		return Session{}, errors.New("konnect answered without payUrl/paymentRef")
	}
	return Session{Provider: "konnect", Status: "requires_payment", Reference: out.PaymentRef, RedirectURL: out.PayURL}, nil
}

func (k Konnect) Status(ctx context.Context, c Config, reference string) (Result, error) {
	if !k.Configured(c) {
		return Result{}, ErrNotConfigured
	}
	var out struct {
		Payment struct {
			ID             string `json:"id"`
			Status         string `json:"status"`
			Amount         int64  `json:"amount"`
			ReachedAmount  int64  `json:"reachedAmount"`
			Token          string `json:"token"`
			OrderID        string `json:"orderId"`
			ExpirationDate string `json:"expirationDate"`
			Transactions   []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"transactions"`
		} `json:"payment"`
	}
	if _, err := doJSON(ctx, "konnect", http.MethodGet, k.base(c)+"/payments/"+url.PathEscape(reference),
		map[string]string{"x-api-key": c.Secrets["apiKey"]}, nil, &out); err != nil {
		return Result{}, err
	}
	p := out.Payment
	r := Result{Reference: reference, OrderID: orderIDFrom(p.OrderID), AmountMinor: p.ReachedAmount, Currency: strings.ToUpper(p.Token),
		TransactionID: reference, Status: StatusPending}
	if p.Status == "completed" && (p.Amount == 0 || p.ReachedAmount >= p.Amount) {
		r.Status = StatusPaid
		for _, t := range p.Transactions {
			if t.Status == "success" && t.ID != "" {
				r.TransactionID = t.ID
			}
		}
	} else if len(p.Transactions) > 0 {
		failed := true
		for _, t := range p.Transactions {
			if t.Status == "success" || t.Status == "pending" {
				failed = false
			}
		}
		if failed {
			r.Status, r.Detail = StatusFailed, "the payment attempt failed"
		}
	}
	return r, nil
}

func (k Konnect) ParseWebhook(ctx context.Context, c Config, r *http.Request, body []byte) (Event, error) {
	ref := strings.TrimSpace(r.URL.Query().Get("payment_ref"))
	if ref == "" {
		return Event{}, errors.New("missing payment_ref")
	}
	// Konnect does not sign its webhook: the service confirms with Status.
	return Event{Reference: ref, Type: "konnect.payment"}, nil
}

func (k Konnect) Test(ctx context.Context, c Config) error {
	if !k.Configured(c) {
		return ErrNotConfigured
	}
	_, err := doJSON(ctx, "konnect", http.MethodGet, k.base(c)+"/payments/000000000000000000000000",
		map[string]string{"x-api-key": c.Secrets["apiKey"]}, nil, nil)
	var api *APIError
	if errors.As(err, &api) && api.Status == http.StatusNotFound {
		return nil // authenticated: the dummy payment just does not exist
	}
	return err
}
