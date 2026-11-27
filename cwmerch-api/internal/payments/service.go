package payments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"cwmerch-api/internal/secrets"
	"cwmerch-api/internal/woo"
)

// Order meta keys the service keeps on an order.
const (
	MetaProvider  = "_cwmerch_payment_provider"
	MetaReference = "_cwmerch_payment_ref"
)

// ProviderConfig is one provider as stored (setting "payments", JSON).
type ProviderConfig struct {
	Enabled     bool              `json:"enabled"`
	Mode        string            `json:"mode"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Settings    map[string]any    `json:"settings"`
	Secrets     map[string]string `json:"secrets"` // sealed
}

type stored struct {
	Providers map[string]ProviderConfig `json:"providers"`
}

// Provider is a provider as the administration reads it (no secret values).
type Provider struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Enabled     bool            `json:"enabled"`
	Configured  bool            `json:"configured"`
	Mode        string          `json:"mode"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Settings    map[string]any  `json:"settings"`
	Secrets     map[string]bool `json:"secrets"`
	WebhookURL  string          `json:"webhookUrl"`
	Currencies  []string        `json:"currencies"`
	FromEnv     bool            `json:"fromEnv"`
}

// ProviderUpdate is the administration's edit.
type ProviderUpdate struct {
	Enabled     *bool             `json:"enabled"`
	Mode        *string           `json:"mode"`
	Title       *string           `json:"title"`
	Description *string           `json:"description"`
	Settings    map[string]any    `json:"settings"`
	Secrets     map[string]string `json:"secrets"`
}

// Gateway is an online payment method offered at checkout.
type Gateway struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Type           string `json:"type"`
	PublishableKey string `json:"publishableKey,omitempty"`
}

// Service runs the providers for the shop.
type Service struct {
	woo      *woo.Store
	sealer   *secrets.Sealer
	adapters map[string]Adapter
	order    []string
	apiURL   string
	uiURL    string
	mu       sync.Mutex // serialises payment syncs of one process
	// OnPaid / OnEvent notify (Discord), optional.
	OnPaid func(ctx context.Context, o woo.Order, provider, reference string)
}

// New builds the service with Stripe, Konnect and Flouci.
func New(w *woo.Store, sealer *secrets.Sealer, apiURL, uiURL string, adapters ...Adapter) *Service {
	if len(adapters) == 0 {
		adapters = []Adapter{Stripe{BaseURL: os.Getenv("STRIPE_API_BASE")}, Konnect{BaseURL: os.Getenv("KONNECT_API_BASE")},
			Flouci{BaseURL: os.Getenv("FLOUCI_API_BASE")}}
	}
	s := &Service{woo: w, sealer: sealer, adapters: map[string]Adapter{}, apiURL: strings.TrimRight(apiURL, "/"), uiURL: strings.TrimRight(uiURL, "/")}
	for _, a := range adapters {
		s.adapters[a.ID()] = a
		s.order = append(s.order, a.ID())
	}
	return s
}

// IsOnline reports a payment method handled by an adapter.
func (s *Service) IsOnline(id string) bool { _, ok := s.adapters[id]; return ok }

func (s *Service) load(ctx context.Context) stored {
	st := stored{Providers: map[string]ProviderConfig{}}
	_, _, _ = s.woo.Setting(ctx, "payments", &st)
	if st.Providers == nil {
		st.Providers = map[string]ProviderConfig{}
	}
	return st
}

// envFallback lets credentials come from the environment
// (STRIPE_SECRET_KEY, KONNECT_API_KEY, FLOUCI_PRIVATE_KEY...).
func envName(provider, field string) string {
	var b strings.Builder
	for i, r := range field {
		if r >= 'A' && r <= 'Z' && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToUpper(provider + "_" + b.String())
}

// config opens a provider's configuration.
func (s *Service) config(ctx context.Context, id string) (ProviderConfig, Config, bool) {
	a := s.adapters[id]
	pc := s.load(ctx).Providers[id]
	cfg := Config{Mode: orDefault(pc.Mode, "test"), Settings: map[string]any{}, Secrets: map[string]string{}}
	fromEnv := false
	for k, v := range pc.Settings {
		cfg.Settings[k] = v
	}
	for _, name := range a.PublicSettings() {
		if v := os.Getenv(envName(id, name)); v != "" && cfg.Str(name) == "" {
			cfg.Settings[name], fromEnv = v, true
		}
	}
	for _, name := range a.SecretNames() {
		if sealed := pc.Secrets[name]; sealed != "" {
			if plain, err := s.sealer.Open(sealed); err == nil {
				cfg.Secrets[name] = plain
			} else {
				slog.Warn("payment secret cannot be opened", "provider", id, "secret", name, "error", err)
			}
		} else if v := os.Getenv(envName(id, name)); v != "" {
			cfg.Secrets[name], fromEnv = v, true
		}
	}
	if m := os.Getenv(strings.ToUpper(id) + "_MODE"); m != "" && pc.Mode == "" {
		cfg.Mode = m
	}
	return pc, cfg, fromEnv
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// WebhookURL is where a provider notifies this API.
func (s *Service) WebhookURL(id string) string { return s.apiURL + "/v1/payments/" + id + "/webhook" }

// Providers lists every provider for the administration.
func (s *Service) Providers(ctx context.Context) []Provider {
	out := []Provider{}
	for _, id := range s.order {
		out = append(out, s.provider(ctx, id))
	}
	return out
}

func (s *Service) provider(ctx context.Context, id string) Provider {
	a := s.adapters[id]
	pc, cfg, fromEnv := s.config(ctx, id)
	p := Provider{ID: id, Name: a.Name(), Enabled: pc.Enabled, Configured: a.Configured(cfg), Mode: cfg.Mode,
		Title: orDefault(pc.Title, a.DefaultTitle()), Description: pc.Description, Settings: map[string]any{},
		Secrets: map[string]bool{}, WebhookURL: s.WebhookURL(id), Currencies: a.Currencies(), FromEnv: fromEnv}
	for _, k := range a.PublicSettings() {
		if v, ok := cfg.Settings[k]; ok {
			p.Settings[k] = v
		}
	}
	for _, k := range a.SecretNames() {
		p.Secrets[k] = cfg.Secrets[k] != ""
	}
	return p
}

// Update applies the administration's edit; a blank secret keeps the stored one.
func (s *Service) Update(ctx context.Context, id string, u ProviderUpdate, by uint64) (Provider, error) {
	a, ok := s.adapters[id]
	if !ok {
		return Provider{}, woo.ErrNotFound
	}
	st := s.load(ctx)
	pc := st.Providers[id]
	if pc.Settings == nil {
		pc.Settings = map[string]any{}
	}
	if pc.Secrets == nil {
		pc.Secrets = map[string]string{}
	}
	if u.Enabled != nil {
		pc.Enabled = *u.Enabled
	}
	if u.Mode != nil {
		if *u.Mode != "test" && *u.Mode != "live" {
			return Provider{}, &woo.Error{Status: 400, Code: "errors.invalidPaymentMode", Message: "Mode must be test or live"}
		}
		pc.Mode = *u.Mode
	}
	if u.Title != nil {
		pc.Title = strings.TrimSpace(*u.Title)
	}
	if u.Description != nil {
		pc.Description = *u.Description
	}
	allowed := map[string]bool{}
	for _, k := range a.PublicSettings() {
		allowed[k] = true
	}
	for k, v := range u.Settings {
		if allowed[k] {
			pc.Settings[k] = v
		}
	}
	secretAllowed := map[string]bool{}
	for _, k := range a.SecretNames() {
		secretAllowed[k] = true
	}
	for k, v := range u.Secrets {
		v = strings.TrimSpace(v)
		if !secretAllowed[k] || v == "" {
			continue
		}
		sealed, err := s.sealer.Seal(v)
		if err != nil {
			return Provider{}, &woo.Error{Status: 400, Code: "errors.secretKeyMissing",
				Message: "Set CWMERCH_SECRET_KEY on the API to store payment credentials"}
		}
		pc.Secrets[k] = sealed
	}
	st.Providers[id] = pc
	if err := s.woo.SaveSetting(ctx, "payments", st, by); err != nil {
		return Provider{}, err
	}
	return s.provider(ctx, id), nil
}

// Test checks a provider's credentials against its API.
func (s *Service) Test(ctx context.Context, id string) error {
	a, ok := s.adapters[id]
	if !ok {
		return woo.ErrNotFound
	}
	_, cfg, _ := s.config(ctx, id)
	return a.Test(ctx, cfg)
}

// Gateways are the enabled, configured providers usable for currency.
func (s *Service) Gateways(ctx context.Context, currency string) []Gateway {
	out := []Gateway{}
	for _, id := range s.order {
		a := s.adapters[id]
		pc, cfg, _ := s.config(ctx, id)
		if !pc.Enabled || !a.Configured(cfg) || !supports(a, currency) {
			continue
		}
		g := Gateway{ID: id, Title: orDefault(pc.Title, a.DefaultTitle()), Description: pc.Description, Type: "online"}
		if id == "stripe" {
			g.PublishableKey = cfg.Str("publishableKey")
		}
		out = append(out, g)
	}
	sort.SliceStable(out, func(i, j int) bool { return false })
	return out
}

// Title is a provider's checkout title.
func (s *Service) Title(ctx context.Context, id string) string {
	a, ok := s.adapters[id]
	if !ok {
		return id
	}
	pc, _, _ := s.config(ctx, id)
	return orDefault(pc.Title, a.DefaultTitle())
}

// Start opens a payment for a pending order and remembers its reference.
func (s *Service) Start(ctx context.Context, o woo.Order) (Session, error) {
	id := o.PaymentMethod
	a, ok := s.adapters[id]
	if !ok {
		return Session{}, &woo.Error{Status: 400, Code: "errors.invalidPaymentMethod", Message: "Not an online payment method"}
	}
	if o.Status != "pending" && o.Status != "failed" {
		return Session{}, &woo.Error{Status: 409, Code: "errors.orderNotPayable", Message: "This order does not need a payment"}
	}
	pc, cfg, _ := s.config(ctx, id)
	if !pc.Enabled || !a.Configured(cfg) {
		return Session{}, &woo.Error{Status: 400, Code: "errors.paymentProviderUnavailable", Message: "This payment method is not available"}
	}
	return s.start(ctx, a, cfg, o)
}

func (s *Service) start(ctx context.Context, a Adapter, cfg Config, o woo.Order) (Session, error) {
	name := s.woo.Option(ctx, "blogname")
	sess, err := a.CreatePayment(ctx, cfg, Request{
		OrderID: o.ID, OrderKey: o.OrderKey, Amount: o.Total - o.TotalRefunded, Currency: o.Currency,
		Description: fmt.Sprintf("%s – order #%s", name, o.Number), Email: o.Billing.Email,
		FirstName: o.Billing.FirstName, LastName: o.Billing.LastName, Phone: o.Billing.Phone,
		ReturnURL:  fmt.Sprintf("%s/checkout/payment/%d?key=%s", s.uiURL, o.ID, o.OrderKey),
		WebhookURL: s.WebhookURL(a.ID()),
	})
	if err != nil {
		slog.ErrorContext(ctx, "payment could not be started", "provider", a.ID(), "order", o.ID, "error", err)
		return Session{}, &woo.Error{Status: 502, Code: "errors.paymentProviderError", Message: "The payment provider refused the payment: " + err.Error()}
	}
	_ = s.woo.SetOrderMeta(ctx, o.ID, MetaProvider, a.ID())
	_ = s.woo.SetOrderMeta(ctx, o.ID, MetaReference, sess.Reference)
	_, _ = s.woo.AddNote(ctx, o.ID, fmt.Sprintf("%s payment started (reference %s).", a.Name(), sess.Reference), false, "", "", 0)
	return sess, nil
}

// Sync asks the provider about an order's payment and applies the result:
// paid → transaction id + "processing" (stock, emails...), failed → "failed".
// Idempotent: a paid order is left alone.
func (s *Service) Sync(ctx context.Context, orderID uint64) (string, woo.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, err := s.woo.Order(ctx, orderID)
	if err != nil {
		return "", woo.Order{}, err
	}
	if o.PaidAt != nil || o.Status == "processing" || o.Status == "completed" {
		return StatusPaid, o, nil
	}
	provider := orDefault(s.woo.OrderMeta(ctx, orderID, MetaProvider), o.PaymentMethod)
	ref := s.woo.OrderMeta(ctx, orderID, MetaReference)
	a, ok := s.adapters[provider]
	if !ok || ref == "" {
		return StatusPending, o, nil
	}
	_, cfg, _ := s.config(ctx, provider)
	res, err := a.Status(ctx, cfg, ref)
	if err != nil {
		return "", o, &woo.Error{Status: 502, Code: "errors.paymentProviderError", Message: err.Error()}
	}
	switch res.Status {
	case StatusPaid:
		paid := FromMinor(res.AmountMinor, orDefault(res.Currency, o.Currency))
		if res.AmountMinor > 0 && paid+0.01 < o.Total-o.TotalRefunded && strings.EqualFold(res.Currency, o.Currency) {
			_, _ = s.woo.AddNote(ctx, orderID, fmt.Sprintf("%s reports %.3f %s paid, less than the order total: left pending for review.",
				a.Name(), paid, res.Currency), false, "", "", 0)
			return StatusPending, o, nil
		}
		tx := res.TransactionID
		if _, err := s.woo.UpdateOrder(ctx, orderID, woo.OrderUpdate{TransactionID: &tx}); err != nil {
			return "", o, err
		}
		o, err = s.woo.SetStatus(ctx, orderID, "processing", fmt.Sprintf("Payment received via %s (%s).", a.Name(), tx), false)
		if err != nil {
			return "", o, err
		}
		if s.OnPaid != nil {
			s.OnPaid(ctx, o, provider, ref)
		}
		return StatusPaid, o, nil
	case StatusFailed:
		if o.Status == "pending" {
			o, err = s.woo.SetStatus(ctx, orderID, "failed", fmt.Sprintf("%s payment failed: %s.", a.Name(), orDefault(res.Detail, "refused")), false)
			if err != nil {
				return "", o, err
			}
		}
		return StatusFailed, o, nil
	}
	return StatusPending, o, nil
}

// Webhook authenticates and handles a provider callback.
func (s *Service) Webhook(ctx context.Context, id string, r *http.Request, body []byte) (int, error) {
	a, ok := s.adapters[id]
	if !ok {
		return http.StatusNotFound, woo.ErrNotFound
	}
	_, cfg, _ := s.config(ctx, id)
	ev, err := a.ParseWebhook(ctx, cfg, r, body)
	if err != nil {
		if errors.Is(err, ErrBadSignature) {
			return http.StatusBadRequest, err
		}
		return http.StatusBadRequest, err
	}
	if ev.Reference == "" {
		return http.StatusOK, nil // not about one of our payments
	}
	orderID, err := s.woo.FindOrderByMeta(ctx, MetaReference, ev.Reference)
	if err != nil {
		if ev.OrderID == 0 {
			if res, serr := a.Status(ctx, cfg, ev.Reference); serr == nil {
				ev.OrderID = res.OrderID
			}
		}
		if ev.OrderID == 0 {
			slog.WarnContext(ctx, "payment webhook for an unknown payment", "provider", id, "reference", ev.Reference)
			return http.StatusOK, nil
		}
		orderID = ev.OrderID
		if s.woo.OrderMeta(ctx, orderID, MetaReference) != ev.Reference {
			// the order exists but this is not the payment we started for it
			slog.WarnContext(ctx, "payment webhook reference does not match the order", "provider", id, "order", orderID)
			return http.StatusOK, nil
		}
	}
	status, _, err := s.Sync(ctx, orderID)
	if err != nil {
		return http.StatusBadGateway, err
	}
	slog.InfoContext(ctx, "payment webhook handled", "provider", id, "order", orderID, "status", status, "event", ev.Type)
	return http.StatusOK, nil
}

// ParseOrderID reads an order id from text.
func ParseOrderID(s string) uint64 { n, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 64); return n }
