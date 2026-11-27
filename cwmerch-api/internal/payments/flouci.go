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

// Flouci (Tunisia, TND only) redirects the customer to a hosted checkout and
// calls a signed webhook (v2: POST JSON, X-Flouci-Signature); the status is
// confirmed with verify_payment.
type Flouci struct {
	BaseURL string // https://developers.flouci.com/api/v2
}

func (Flouci) ID() string           { return "flouci" }
func (Flouci) Name() string         { return "Flouci" }
func (Flouci) DefaultTitle() string { return "Flouci (card, wallet)" }
func (Flouci) PublicSettings() []string {
	return []string{"publicKey", "acceptCard", "sessionTimeoutSecs"}
}
func (Flouci) SecretNames() []string { return []string{"privateKey"} }
func (Flouci) Currencies() []string  { return []string{"TND"} }
func (f Flouci) Configured(c Config) bool {
	return c.Str("publicKey") != "" && c.Secrets["privateKey"] != ""
}

func (f Flouci) base() string {
	if f.BaseURL != "" {
		return strings.TrimRight(f.BaseURL, "/")
	}
	return "https://developers.flouci.com/api/v2"
}

func (f Flouci) auth(c Config) map[string]string {
	return map[string]string{"Authorization": "Bearer " + c.Str("publicKey") + ":" + c.Secrets["privateKey"]}
}

func (f Flouci) CreatePayment(ctx context.Context, c Config, req Request) (Session, error) {
	if !f.Configured(c) {
		return Session{}, ErrNotConfigured
	}
	if !strings.EqualFold(req.Currency, "TND") {
		return Session{}, errors.New("flouci only accepts TND")
	}
	body := map[string]any{
		"amount": strconv.FormatInt(MinorUnits(req.Amount, "TND"), 10), "accept_card": c.Bool("acceptCard", true),
		"session_timeout_secs": c.Int("sessionTimeoutSecs", 1200), "success_link": req.ReturnURL, "fail_link": req.ReturnURL,
		"developer_tracking_id": "order-" + strconv.FormatUint(req.OrderID, 10), "webhook": req.WebhookURL, "webhook_version": 2,
	}
	var out struct {
		Result struct {
			Success   bool   `json:"success"`
			PaymentID string `json:"payment_id"`
			Link      string `json:"link"`
			Message   string `json:"message"`
		} `json:"result"`
	}
	if _, err := doJSON(ctx, "flouci", http.MethodPost, f.base()+"/generate_payment", f.auth(c), body, &out); err != nil {
		return Session{}, err
	}
	if !out.Result.Success || out.Result.Link == "" {
		return Session{}, errors.New("flouci refused the payment: " + out.Result.Message)
	}
	return Session{Provider: "flouci", Status: "requires_payment", Reference: out.Result.PaymentID, RedirectURL: out.Result.Link}, nil
}

func (f Flouci) Status(ctx context.Context, c Config, reference string) (Result, error) {
	if !f.Configured(c) {
		return Result{}, ErrNotConfigured
	}
	var out struct {
		Success bool `json:"success"`
		Result  struct {
			Type                string `json:"type"`
			Amount              int64  `json:"amount"`
			Status              string `json:"status"`
			DeveloperTrackingID string `json:"developer_tracking_id"`
			Details             struct {
				OrderNumber  string `json:"order_number"`
				ApprovalCode string `json:"approval_code"`
			} `json:"details"`
		} `json:"result"`
	}
	if _, err := doJSON(ctx, "flouci", http.MethodGet, f.base()+"/verify_payment/"+url.PathEscape(reference), f.auth(c), nil, &out); err != nil {
		return Result{}, err
	}
	r := Result{Reference: reference, OrderID: orderIDFrom(out.Result.DeveloperTrackingID), AmountMinor: out.Result.Amount,
		Currency: "TND", TransactionID: reference, Status: StatusPending}
	if out.Result.Details.OrderNumber != "" {
		r.TransactionID = out.Result.Details.OrderNumber
	}
	switch out.Result.Status {
	case "SUCCESS":
		if out.Success {
			r.Status = StatusPaid
		}
	case "FAILURE", "EXPIRED", "SYSTEM_FAILURE":
		r.Status, r.Detail = StatusFailed, strings.ToLower(out.Result.Status)
	}
	return r, nil
}

func (f Flouci) ParseWebhook(ctx context.Context, c Config, r *http.Request, body []byte) (Event, error) {
	secret := c.Secrets["privateKey"]
	if r.Method == http.MethodGet {
		// v1 style: ?payment_id=..&success=..; signed over that string
		q := r.URL.Query()
		signed := []byte("payment_id=" + q.Get("payment_id") + "&success=" + q.Get("success"))
		if err := verifyTimestampedHMAC(r.Header.Get("X-Flouci-Signature"), secret, signed, 5*time.Minute); err != nil {
			return Event{}, err
		}
		return Event{Reference: q.Get("payment_id"), Type: "flouci.v1"}, nil
	}
	if err := verifyTimestampedHMAC(r.Header.Get("X-Flouci-Signature"), secret, body, 5*time.Minute); err != nil {
		return Event{}, err
	}
	var ev struct {
		Event               string `json:"event"`
		PaymentID           string `json:"payment_id"`
		DeveloperTrackingID string `json:"developer_tracking_id"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		return Event{}, err
	}
	return Event{Reference: ev.PaymentID, OrderID: orderIDFrom(ev.DeveloperTrackingID), Type: ev.Event}, nil
}

func (f Flouci) Test(ctx context.Context, c Config) error {
	if !f.Configured(c) {
		return ErrNotConfigured
	}
	_, err := doJSON(ctx, "flouci", http.MethodGet, f.base()+"/verify_payment/cwmerch-connection-test", f.auth(c), nil, nil)
	var api *APIError
	if errors.As(err, &api) && (api.Status == http.StatusNotFound || api.Status == http.StatusBadRequest) {
		return nil // authenticated: the dummy payment just does not exist
	}
	return err
}
