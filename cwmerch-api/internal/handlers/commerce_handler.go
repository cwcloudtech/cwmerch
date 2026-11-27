package handlers

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"cwmerch-api/internal/email"
	"cwmerch-api/internal/invoicing"
	"cwmerch-api/internal/middleware"
	"cwmerch-api/internal/models"
	"cwmerch-api/internal/payments"
	"cwmerch-api/internal/store"
	"cwmerch-api/internal/woo"
	"cwmerch-api/internal/wooemail"
)

// CommerceHandler serves the shop switch, online payments and invoices.
type CommerceHandler struct {
	auditor
	woo      *woo.Store
	payments *payments.Service
	invoices *invoicing.Service
	mailer   *wooemail.Mailer
	users    *store.UserStore
	sender   *email.Sender
}

// WithSender wires the raw email sender (test emails).
func (h *CommerceHandler) WithSender(s *email.Sender) *CommerceHandler {
	h.sender = s
	return h
}

func NewCommerceHandler(w *woo.Store, p *payments.Service, inv *invoicing.Service, mailer *wooemail.Mailer, users *store.UserStore, audit *store.AuditStore) *CommerceHandler {
	return &CommerceHandler{auditor: newAuditor(audit), woo: w, payments: p, invoices: inv, mailer: mailer, users: users}
}

// ------------------------------------------------------------------ shop switch

// RequireShopOpen refuses the public storefront when the shop is switched off.
func (h *CommerceHandler) RequireShopOpen(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.woo.ShopConfig(r.Context()).ShopEnabled {
			writeError(w, http.StatusForbidden, "The shop is closed", "errors.shopDisabled")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RegistrationAllowed tells the user handler whether sign-up is open.
func (h *CommerceHandler) RegistrationAllowed(r *http.Request) bool {
	c := h.woo.ShopConfig(r.Context())
	return c.RegistrationEnabled
}

func (h *CommerceHandler) ShopConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.woo.ShopConfig(r.Context()))
}

func (h *CommerceHandler) UpdateShopConfig(w http.ResponseWriter, r *http.Request) {
	var c woo.ShopConfig
	if !decodeJSON(w, r, &c) {
		return
	}
	saved, err := h.woo.SaveShopConfig(r.Context(), c, callerID(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditSettingsUpdated, models.AuditSubjectSetting, "shop", map[string]any{"shopEnabled": saved.ShopEnabled,
		"registrationEnabled": saved.RegistrationEnabled})
	writeJSON(w, http.StatusOK, saved)
}

// ------------------------------------------------------------------ payment providers (admin)

func (h *CommerceHandler) Providers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.payments.Providers(r.Context()))
}

func (h *CommerceHandler) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	var u payments.ProviderUpdate
	if !decodeJSON(w, r, &u) {
		return
	}
	p, err := h.payments.Update(r.Context(), chi.URLParam(r, "id"), u, callerID(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	changed := []string{}
	for k, v := range u.Secrets {
		if strings.TrimSpace(v) != "" {
			changed = append(changed, k)
		}
	}
	h.record(r, models.AuditGatewayUpdated, models.AuditSubjectSetting, "payment:"+p.ID,
		map[string]any{"enabled": p.Enabled, "mode": p.Mode, "secretsChanged": changed})
	writeJSON(w, http.StatusOK, p)
}

func (h *CommerceHandler) TestProvider(w http.ResponseWriter, r *http.Request) {
	if err := h.payments.Test(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Connection OK"})
}

// ------------------------------------------------------------------ payments (shop)

func (h *CommerceHandler) orderByKey(w http.ResponseWriter, r *http.Request) (woo.Order, bool) {
	o, err := h.woo.OrderByKey(r.Context(), idParam(r, "id"), r.URL.Query().Get("key"))
	if err != nil {
		writeWooError(w, err)
		return woo.Order{}, false
	}
	return o, true
}

func publicOrder(o woo.Order) woo.Order {
	o.CustomerIPAddress, o.CustomerUserAgent = "", ""
	return o
}

// PaymentStatus is GET /v1/shop/orders/{id}/payment?key=: asks the provider.
func (h *CommerceHandler) PaymentStatus(w http.ResponseWriter, r *http.Request) {
	o, ok := h.orderByKey(w, r)
	if !ok {
		return
	}
	status := payments.StatusPending
	if h.payments.IsOnline(o.PaymentMethod) {
		var err error
		status, o, err = h.payments.Sync(r.Context(), o.ID)
		if err != nil {
			writeWooError(w, err)
			return
		}
	} else if o.PaidAt != nil || o.Status == "processing" || o.Status == "completed" {
		status = payments.StatusPaid
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status, "order": publicOrder(o)})
}

// Pay is POST /v1/shop/orders/{id}/pay?key=: a new payment for a pending order.
func (h *CommerceHandler) Pay(w http.ResponseWriter, r *http.Request) {
	o, ok := h.orderByKey(w, r)
	if !ok {
		return
	}
	if status, synced, err := h.payments.Sync(r.Context(), o.ID); err == nil && status == payments.StatusPaid {
		writeJSON(w, http.StatusConflict, map[string]any{"message": "This order is already paid", "i18n_code": "errors.orderAlreadyPaid", "order": publicOrder(synced)})
		return
	}
	if o.Status == "failed" {
		var err error
		if o, err = h.woo.SetStatus(r.Context(), o.ID, "pending", "New payment attempt.", false); err != nil {
			writeWooError(w, err)
			return
		}
	}
	sess, err := h.payments.Start(r.Context(), o)
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"payment": sess})
}

// Webhook is the providers' callback: /v1/payments/{provider}/webhook.
func (h *CommerceHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	status, err := h.payments.Webhook(r.Context(), chi.URLParam(r, "provider"), r, body)
	if err != nil {
		writeError(w, status, err.Error(), "errors.paymentWebhook")
		return
	}
	writeJSON(w, status, map[string]string{"status": "ok"})
}

// ------------------------------------------------------------------ invoicing (admin)

func (h *CommerceHandler) InvoicingSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.invoices.Settings(r.Context()))
}

func (h *CommerceHandler) UpdateInvoicingSettings(w http.ResponseWriter, r *http.Request) {
	var s invoicing.Settings
	if !decodeJSON(w, r, &s) {
		return
	}
	saved, err := h.invoices.SaveSettings(r.Context(), s, callerID(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditSettingsUpdated, models.AuditSubjectSetting, "invoicing", map[string]any{"taxMode": saved.TaxMode, "country": saved.Country})
	writeJSON(w, http.StatusOK, saved)
}

func (h *CommerceHandler) CountryDefaults(w http.ResponseWriter, r *http.Request) {
	rows, err := h.invoices.CountryDefaults(r.Context())
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *CommerceHandler) CountryDefault(w http.ResponseWriter, r *http.Request) {
	row, err := h.invoices.CountryDefault(r.Context(), chi.URLParam(r, "iso"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *CommerceHandler) SaveCountryDefault(w http.ResponseWriter, r *http.Request) {
	var row invoicing.CountryDefaults
	if !decodeJSON(w, r, &row) {
		return
	}
	saved, err := h.invoices.SaveCountryDefault(r.Context(), chi.URLParam(r, "iso"), row)
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditSettingsUpdated, models.AuditSubjectSetting, "countryDefaults:"+strings.ToUpper(chi.URLParam(r, "iso")),
		map[string]any{"taxMode": saved.TaxMode, "vatRate": saved.DefaultVatRate, "stamp": saved.StampAmount})
	writeJSON(w, http.StatusOK, saved)
}

func (h *CommerceHandler) DeleteCountryDefault(w http.ResponseWriter, r *http.Request) {
	iso := strings.ToUpper(chi.URLParam(r, "iso"))
	if err := h.invoices.DeleteCountryDefault(r.Context(), iso); err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditSettingsUpdated, models.AuditSubjectSetting, "countryDefaults:"+iso, map[string]any{"deleted": true})
	writeJSON(w, http.StatusOK, map[string]string{"country": iso})
}

type applyDefaultsPayload struct {
	Country       string `json:"country"`
	CreateTaxRate bool   `json:"createTaxRate"`
}

func (h *CommerceHandler) ApplyDefaults(w http.ResponseWriter, r *http.Request) {
	var p applyDefaultsPayload
	if !decodeJSON(w, r, &p) {
		return
	}
	set, err := h.invoices.ApplyDefaults(r.Context(), p.Country, p.CreateTaxRate, callerID(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditSettingsUpdated, models.AuditSubjectSetting, "invoicing", map[string]any{"appliedCountry": p.Country, "taxRate": p.CreateTaxRate})
	writeJSON(w, http.StatusOK, set)
}

func (h *CommerceHandler) Template(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"default": invoicing.DefaultTemplate, "variables": invoicing.Variables})
}

type previewPayload struct {
	Template string `json:"template"`
	OrderID  uint64 `json:"orderId"`
}

func writePDF(w http.ResponseWriter, filename string, pdf []byte, inline bool) {
	w.Header().Set("Content-Type", "application/pdf")
	disp := "attachment"
	if inline {
		disp = "inline"
	}
	w.Header().Set("Content-Disposition", disp+`; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	_, _ = w.Write(pdf)
}

func (h *CommerceHandler) Preview(w http.ResponseWriter, r *http.Request) {
	var p previewPayload
	if !decodeJSON(w, r, &p) {
		return
	}
	pdf, err := h.invoices.Preview(r.Context(), p.Template, p.OrderID)
	if err != nil {
		writeWooError(w, err)
		return
	}
	writePDF(w, "invoice-preview.pdf", pdf, true)
}

func (h *CommerceHandler) Invoices(w http.ResponseWriter, r *http.Request) {
	res, err := h.invoices.List(r.Context(), r.URL.Query().Get("search"), queryInt(r, "page", 1), queryInt(r, "perPage", 20))
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *CommerceHandler) Invoice(w http.ResponseWriter, r *http.Request) {
	inv, err := h.invoices.Get(r.Context(), idParam(r, "id"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

func (h *CommerceHandler) InvoicePDF(w http.ResponseWriter, r *http.Request) {
	inv, err := h.invoices.Get(r.Context(), idParam(r, "id"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	pdf, err := h.invoices.PDF(r.Context(), inv)
	if err != nil {
		writeWooError(w, err)
		return
	}
	writePDF(w, invoicing.Filename(inv), pdf, false)
}

func (h *CommerceHandler) InvoiceMarkdown(w http.ResponseWriter, r *http.Request) {
	inv, err := h.invoices.Get(r.Context(), idParam(r, "id"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.TrimSuffix(invoicing.Filename(inv), ".pdf")+`.md"`)
	_, _ = w.Write([]byte(inv.Markdown))
}

func (h *CommerceHandler) SendInvoice(w http.ResponseWriter, r *http.Request) {
	inv, err := h.invoices.Get(r.Context(), idParam(r, "id"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	sent := h.sendInvoice(r, inv)
	h.record(r, models.AuditOrderEmailSent, models.AuditSubjectOrder, sid(inv.OrderID), map[string]any{"invoice": inv.Number, "sent": sent})
	writeJSON(w, http.StatusOK, map[string]any{"sent": sent})
}

func (h *CommerceHandler) sendInvoice(r *http.Request, inv invoicing.Invoice) bool {
	o, err := h.woo.Order(r.Context(), inv.OrderID)
	if err != nil || h.mailer == nil {
		return false
	}
	pdf, err := h.invoices.PDF(r.Context(), inv)
	if err != nil {
		return false
	}
	return h.mailer.SendInvoice(r.Context(), o, inv.Number, invoicing.Filename(inv), pdf)
}

func (h *CommerceHandler) OrderInvoice(w http.ResponseWriter, r *http.Request) {
	inv, err := h.invoices.ByOrder(r.Context(), idParam(r, "id"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

func (h *CommerceHandler) IssueOrderInvoice(w http.ResponseWriter, r *http.Request) {
	inv, created, err := h.invoices.Issue(r.Context(), idParam(r, "id"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
		h.record(r, models.AuditOrderUpdated, models.AuditSubjectOrder, sid(inv.OrderID), map[string]any{"invoice": inv.Number})
	}
	writeJSON(w, status, inv)
}

// MyInvoice is GET /v1/users/me/orders/{id}/invoice: the customer's PDF.
func (h *CommerceHandler) MyInvoice(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserIDFromContext(r.Context())
	o, err := h.woo.Order(r.Context(), idParam(r, "id"))
	if err != nil || strconv.FormatUint(o.CustomerID, 10) != uid {
		writeError(w, http.StatusNotFound, "Invoice not found", CodeNotFound)
		return
	}
	inv, err := h.invoices.ByOrder(r.Context(), o.ID)
	if err != nil {
		writeWooError(w, err)
		return
	}
	pdf, err := h.invoices.PDF(r.Context(), inv)
	if err != nil {
		writeWooError(w, err)
		return
	}
	writePDF(w, invoicing.Filename(inv), pdf, false)
}

// Permalinks is GET /v1/admin/permalinks (WooCommerce's URL bases).
func (h *CommerceHandler) Permalinks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.woo.Permalinks(r.Context()))
}

// UpdatePermalinks is PUT /v1/admin/permalinks: writes woocommerce_permalinks
// (WordPress serves the same URLs after rebuilding its rewrite rules).
func (h *CommerceHandler) UpdatePermalinks(w http.ResponseWriter, r *http.Request) {
	var in woo.PermalinksInput
	if !decodeJSON(w, r, &in) {
		return
	}
	pl, err := h.woo.SavePermalinks(r.Context(), in)
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditSettingsUpdated, models.AuditSubjectSetting, "permalinks",
		map[string]any{"productBase": pl.ProductBase, "categoryBase": pl.CategoryBase, "tagBase": pl.TagBase})
	writeJSON(w, http.StatusOK, pl)
}

// CWCloudSettings is GET /v1/admin/cwcloud-settings: the email API key
// (shared with the CWCloud WordPress plugin) and the contact form settings,
// all read from WordPress' tables.
func (h *CommerceHandler) CWCloudSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.woo.CWCloudSettings(r.Context()))
}

func (h *CommerceHandler) UpdateCWCloudSettings(w http.ResponseWriter, r *http.Request) {
	var in woo.CWCloudSettingsInput
	if !decodeJSON(w, r, &in) {
		return
	}
	set, err := h.woo.SaveCWCloudSettings(r.Context(), in)
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditSettingsUpdated, models.AuditSubjectSetting, "cwcloud",
		map[string]any{"apiKeyChanged": in.ClearAPIKey || (in.APIKey != nil && strings.TrimSpace(*in.APIKey) != ""), "contactMode": set.ContactMode})
	writeJSON(w, http.StatusOK, set)
}

type testEmailPayload struct {
	To string `json:"to"`
}

// TestEmail sends a test email with the configured CWCloud key.
func (h *CommerceHandler) TestEmail(w http.ResponseWriter, r *http.Request) {
	var p testEmailPayload
	if !decodeJSON(w, r, &p) || !strings.Contains(p.To, "@") {
		if !strings.Contains(p.To, "@") {
			writeError(w, http.StatusBadRequest, "Please add a valid email", CodeInvalidEmail)
		}
		return
	}
	if h.mailer == nil || h.sender == nil {
		writeJSON(w, http.StatusOK, map[string]any{"sent": false})
		return
	}
	html, _ := h.sender.RenderLayout("cwmerch", "<p>This is a test email sent with your CWCloud API key.</p>")
	sent := h.sender.Send(r.Context(), h.woo.Option(r.Context(), "woocommerce_email_from_address"), p.To, "", "", "cwmerch test email", html, nil)
	writeJSON(w, http.StatusOK, map[string]any{"sent": sent})
}
