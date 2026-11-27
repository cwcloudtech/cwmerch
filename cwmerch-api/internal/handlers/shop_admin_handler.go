package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"cwmerch-api/internal/models"
	"cwmerch-api/internal/store"
	"cwmerch-api/internal/woo"
	"cwmerch-api/internal/wooemail"
)

// ShopAdminHandler is the shop's back office (shop managers and administrators).
type ShopAdminHandler struct {
	auditor
	woo            *woo.Store
	users          *store.UserStore
	mailer         *wooemail.Mailer
	reports        *ReportsHandler
	maxUploadBytes int64
}

func NewShopAdminHandler(w *woo.Store, users *store.UserStore, mailer *wooemail.Mailer, reports *ReportsHandler, audit *store.AuditStore, maxUploadBytes int64) *ShopAdminHandler {
	if maxUploadBytes <= 0 {
		maxUploadBytes = 10 << 20
	}
	return &ShopAdminHandler{auditor: newAuditor(audit), woo: w, users: users, mailer: mailer, reports: reports, maxUploadBytes: maxUploadBytes}
}

func sid(id uint64) string { return strconv.FormatUint(id, 10) }

// Dashboard is GET /v1/admin/dashboard: the reports' dashboard plus recent orders.
func (h *ShopAdminHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	resp, ok := h.reports.BuildDashboard(w, r)
	if !ok {
		return
	}
	recent, err := h.woo.Orders(r.Context(), woo.OrderQuery{Page: 1, PerPage: 8})
	if err == nil {
		resp.RecentOrders = make([]any, 0, len(recent.Items))
		for _, o := range recent.Items {
			resp.RecentOrders = append(resp.RecentOrders, map[string]any{"id": o.ID, "number": o.Number, "status": o.Status,
				"total": o.Total, "currency": o.Currency, "createdAt": o.CreatedAt,
				"customer": strings.TrimSpace(o.Billing.FirstName + " " + o.Billing.LastName), "email": o.Billing.Email})
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// ------------------------------------------------------------------ products

func (h *ShopAdminHandler) Products(w http.ResponseWriter, r *http.Request) {
	res, err := h.woo.Products(r.Context(), productQuery(r, false))
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *ShopAdminHandler) Product(w http.ResponseWriter, r *http.Request) {
	p, err := h.woo.Product(r.Context(), idParam(r, "id"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *ShopAdminHandler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var in woo.ProductInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p, err := h.woo.SaveProduct(r.Context(), 0, in, callerID(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditProductCreated, models.AuditSubjectProduct, sid(p.ID), map[string]any{"name": p.Name, "sku": p.SKU})
	writeJSON(w, http.StatusCreated, p)
}

func (h *ShopAdminHandler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	var in woo.ProductInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p, err := h.woo.SaveProduct(r.Context(), idParam(r, "id"), in, callerID(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditProductUpdated, models.AuditSubjectProduct, sid(p.ID), map[string]any{"name": p.Name})
	writeJSON(w, http.StatusOK, p)
}

func (h *ShopAdminHandler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")
	force := queryFlag(r, "force")
	if err := h.woo.DeleteProduct(r.Context(), id, force); err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditProductDeleted, models.AuditSubjectProduct, sid(id), map[string]any{"force": force})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "deleted": force, "trashed": !force})
}

func (h *ShopAdminHandler) RestoreProduct(w http.ResponseWriter, r *http.Request) {
	p, err := h.woo.RestoreProduct(r.Context(), idParam(r, "id"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditProductUpdated, models.AuditSubjectProduct, sid(p.ID), map[string]any{"restored": true})
	writeJSON(w, http.StatusOK, p)
}

func (h *ShopAdminHandler) DuplicateProduct(w http.ResponseWriter, r *http.Request) {
	p, err := h.woo.DuplicateProduct(r.Context(), idParam(r, "id"), callerID(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditProductDuplicated, models.AuditSubjectProduct, sid(p.ID), map[string]any{"from": idParam(r, "id")})
	writeJSON(w, http.StatusCreated, p)
}

func (h *ShopAdminHandler) Variations(w http.ResponseWriter, r *http.Request) {
	vars, err := h.woo.Variations(r.Context(), idParam(r, "id"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

func (h *ShopAdminHandler) saveVariation(w http.ResponseWriter, r *http.Request, id uint64) {
	var in woo.VariationInput
	if !decodeJSON(w, r, &in) {
		return
	}
	v, err := h.woo.SaveVariation(r.Context(), idParam(r, "id"), id, in, callerID(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditVariationSaved, models.AuditSubjectProduct, sid(v.ID), map[string]any{"parent": v.ParentID})
	status := http.StatusOK
	if id == 0 {
		status = http.StatusCreated
	}
	writeJSON(w, status, v)
}

func (h *ShopAdminHandler) CreateVariation(w http.ResponseWriter, r *http.Request) {
	h.saveVariation(w, r, 0)
}

func (h *ShopAdminHandler) UpdateVariation(w http.ResponseWriter, r *http.Request) {
	h.saveVariation(w, r, idParam(r, "variationId"))
}

func (h *ShopAdminHandler) DeleteVariation(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "variationId")
	if err := h.woo.DeleteVariation(r.Context(), idParam(r, "id"), id); err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditVariationDeleted, models.AuditSubjectProduct, sid(id), nil)
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (h *ShopAdminHandler) GenerateVariations(w http.ResponseWriter, r *http.Request) {
	created, err := h.woo.GenerateVariations(r.Context(), idParam(r, "id"), callerID(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditVariationSaved, models.AuditSubjectProduct, sid(idParam(r, "id")), map[string]any{"generated": len(created)})
	writeJSON(w, http.StatusCreated, created)
}

// ------------------------------------------------------------------ terms

// Terms returns the handlers for one taxonomy (categories, tags, shipping classes).
func (h *ShopAdminHandler) Terms(taxonomy string) (list, create, update, del http.HandlerFunc) {
	list = func(w http.ResponseWriter, r *http.Request) {
		terms, err := h.woo.Terms(r.Context(), taxonomy)
		if err != nil {
			writeWooError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, terms)
	}
	create = func(w http.ResponseWriter, r *http.Request) {
		var in woo.TermInput
		if !decodeJSON(w, r, &in) {
			return
		}
		t, err := h.woo.CreateTerm(r.Context(), taxonomy, in)
		if err != nil {
			writeWooError(w, err)
			return
		}
		h.record(r, models.AuditTermSaved, models.AuditSubjectTerm, sid(t.ID), map[string]any{"taxonomy": taxonomy, "name": t.Name})
		writeJSON(w, http.StatusCreated, t)
	}
	update = func(w http.ResponseWriter, r *http.Request) {
		var in woo.TermInput
		if !decodeJSON(w, r, &in) {
			return
		}
		t, err := h.woo.UpdateTerm(r.Context(), taxonomy, idParam(r, "id"), in)
		if err != nil {
			writeWooError(w, err)
			return
		}
		h.record(r, models.AuditTermSaved, models.AuditSubjectTerm, sid(t.ID), map[string]any{"taxonomy": taxonomy, "name": t.Name})
		writeJSON(w, http.StatusOK, t)
	}
	del = func(w http.ResponseWriter, r *http.Request) {
		id := idParam(r, "id")
		if err := h.woo.DeleteTerm(r.Context(), taxonomy, id); err != nil {
			writeWooError(w, err)
			return
		}
		h.record(r, models.AuditTermDeleted, models.AuditSubjectTerm, sid(id), map[string]any{"taxonomy": taxonomy})
		writeJSON(w, http.StatusOK, map[string]any{"id": id})
	}
	return
}

func (h *ShopAdminHandler) Attributes(w http.ResponseWriter, r *http.Request) {
	attrs, err := h.woo.Attributes(r.Context())
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, attrs)
}

func (h *ShopAdminHandler) CreateAttribute(w http.ResponseWriter, r *http.Request) {
	var in woo.AttributeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	a, err := h.woo.CreateAttribute(r.Context(), in)
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditAttributeSaved, models.AuditSubjectAttribute, sid(a.ID), map[string]any{"name": a.Name})
	writeJSON(w, http.StatusCreated, a)
}

func (h *ShopAdminHandler) UpdateAttribute(w http.ResponseWriter, r *http.Request) {
	var in woo.AttributeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	a, err := h.woo.UpdateAttribute(r.Context(), idParam(r, "id"), in)
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditAttributeSaved, models.AuditSubjectAttribute, sid(a.ID), map[string]any{"name": a.Name})
	writeJSON(w, http.StatusOK, a)
}

func (h *ShopAdminHandler) DeleteAttribute(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")
	if err := h.woo.DeleteAttribute(r.Context(), id); err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditAttributeDeleted, models.AuditSubjectAttribute, sid(id), nil)
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (h *ShopAdminHandler) attributeTaxonomy(w http.ResponseWriter, r *http.Request) (string, bool) {
	a, err := h.woo.Attribute(r.Context(), idParam(r, "id"))
	if err != nil {
		writeWooError(w, err)
		return "", false
	}
	return a.Taxonomy(), true
}

// AttributeTerms routes /v1/admin/attributes/{id}/terms[/{termId}].
func (h *ShopAdminHandler) AttributeTerms(w http.ResponseWriter, r *http.Request) {
	tax, ok := h.attributeTaxonomy(w, r)
	if !ok {
		return
	}
	list, create, update, del := h.Terms(tax)
	termID := chi.URLParam(r, "termId")
	if termID != "" {
		rctx := chi.RouteContext(r.Context())
		// the term id replaces {id} for the generic term handlers
		for i, k := range rctx.URLParams.Keys {
			if k == "id" {
				rctx.URLParams.Values[i] = termID
			}
		}
	}
	switch {
	case r.Method == http.MethodGet:
		list(w, r)
	case r.Method == http.MethodPost:
		create(w, r)
	case r.Method == http.MethodPut:
		update(w, r)
	case r.Method == http.MethodDelete:
		del(w, r)
	}
}

// ------------------------------------------------------------------ media

func (h *ShopAdminHandler) Media(w http.ResponseWriter, r *http.Request) {
	res, err := h.woo.MediaList(r.Context(), queryInt(r, "page", 1), queryInt(r, "perPage", 40))
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *ShopAdminHandler) UploadMedia(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid upload (multipart field \"file\")", CodeInvalidRequestBody)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Missing file", CodeInvalidRequestBody)
		return
	}
	defer file.Close()
	img, err := h.woo.UploadMedia(r.Context(), header.Filename, header.Header.Get("Content-Type"), file, callerID(r), h.maxUploadBytes)
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditMediaUploaded, models.AuditSubjectMedia, sid(img.ID), map[string]any{"name": img.Name})
	writeJSON(w, http.StatusCreated, img)
}

// ------------------------------------------------------------------ orders

func (h *ShopAdminHandler) Orders(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.woo.Orders(r.Context(), woo.OrderQuery{Status: q.Get("status"), Search: strings.TrimSpace(q.Get("search")),
		CustomerID: woo.ParseID(q.Get("customer")), From: queryDate(r, "from"), To: queryDate(r, "to"),
		Page: queryInt(r, "page", 1), PerPage: queryInt(r, "perPage", 20)})
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *ShopAdminHandler) Order(w http.ResponseWriter, r *http.Request) {
	o, err := h.woo.Order(r.Context(), idParam(r, "id"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

type adminOrderPayload struct {
	checkoutPayload
	Status     string `json:"status"`
	CustomerID uint64 `json:"customerId"`
}

// CreateOrder is POST /v1/admin/orders: a manual order priced like a checkout.
func (h *ShopAdminHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var p adminOrderPayload
	if !decodeJSON(w, r, &p) {
		return
	}
	if !p.ShipToDifferentAddress && p.Shipping.Address1 == "" {
		p.Shipping = p.Billing
		p.Shipping.Email = ""
	}
	cart, err := h.woo.ComputeCart(r.Context(), woo.CartRequest{Items: p.Items, Coupons: p.Coupons, ShippingMethod: p.ShippingMethod,
		Customer: woo.Location{Country: p.Shipping.Country, State: p.Shipping.State, Postcode: p.Shipping.Postcode, City: p.Shipping.City},
		Email:    p.Billing.Email, UserID: p.CustomerID})
	if err != nil {
		writeWooError(w, err)
		return
	}
	if cart.HasErrors() {
		writeJSON(w, http.StatusConflict, map[string]any{"message": cart.Notices[0].Message, "i18n_code": "errors.cartInvalid", "cart": cart})
		return
	}
	o, err := h.woo.CreateOrder(r.Context(), woo.NewOrderInput{Cart: cart, Billing: p.Billing, Shipping: p.Shipping, PaymentMethod: p.PaymentMethod,
		CustomerID: p.CustomerID, CustomerNote: p.CustomerNote, CreatedVia: "admin", IPAddress: clientIP(r), UserAgent: r.UserAgent()})
	if err != nil {
		writeWooError(w, err)
		return
	}
	if p.Status != "" && p.Status != "pending" {
		if o, err = h.woo.SetStatus(r.Context(), o.ID, p.Status, "", true); err != nil {
			writeWooError(w, err)
			return
		}
	}
	h.record(r, models.AuditOrderCreated, models.AuditSubjectOrder, sid(o.ID), map[string]any{"total": o.Total, "status": o.Status})
	writeJSON(w, http.StatusCreated, o)
}

func (h *ShopAdminHandler) UpdateOrder(w http.ResponseWriter, r *http.Request) {
	var u woo.OrderUpdate
	if !decodeJSON(w, r, &u) {
		return
	}
	id := idParam(r, "id")
	before, err := h.woo.Order(r.Context(), id)
	if err != nil {
		writeWooError(w, err)
		return
	}
	o, err := h.woo.UpdateOrder(r.Context(), id, u)
	if err != nil {
		writeWooError(w, err)
		return
	}
	if before.Status != o.Status {
		h.record(r, models.AuditOrderStatusChanged, models.AuditSubjectOrder, sid(id), map[string]any{"from": before.Status, "to": o.Status})
	} else {
		h.record(r, models.AuditOrderUpdated, models.AuditSubjectOrder, sid(id), nil)
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *ShopAdminHandler) DeleteOrder(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")
	force := queryFlag(r, "force")
	if err := h.woo.DeleteOrder(r.Context(), id, force); err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditOrderDeleted, models.AuditSubjectOrder, sid(id), map[string]any{"force": force})
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (h *ShopAdminHandler) Notes(w http.ResponseWriter, r *http.Request) {
	notes, err := h.woo.Notes(r.Context(), idParam(r, "id"), false)
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, notes)
}

type notePayload struct {
	Note         string `json:"note"`
	CustomerNote bool   `json:"customerNote"`
}

func (h *ShopAdminHandler) AddNote(w http.ResponseWriter, r *http.Request) {
	var p notePayload
	if !decodeJSON(w, r, &p) || strings.TrimSpace(p.Note) == "" {
		if strings.TrimSpace(p.Note) == "" {
			writeError(w, http.StatusBadRequest, "A note is required", "errors.noteRequired")
		}
		return
	}
	id := idParam(r, "id")
	o, err := h.woo.Order(r.Context(), id)
	if err != nil {
		writeWooError(w, err)
		return
	}
	author, email := "", ""
	if h.users != nil {
		if u, err := h.users.FindByID(r.Context(), sid(callerID(r))); err == nil {
			author, email = u.DisplayName, u.Email
		}
	}
	note, err := h.woo.AddNote(r.Context(), id, p.Note, p.CustomerNote, author, email, callerID(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	if p.CustomerNote && h.mailer != nil {
		h.mailer.OrderCustomerNote(r.Context(), o, p.Note)
	}
	h.record(r, models.AuditOrderNoteAdded, models.AuditSubjectOrder, sid(id), map[string]any{"customerNote": p.CustomerNote})
	writeJSON(w, http.StatusCreated, note)
}

func (h *ShopAdminHandler) DeleteNote(w http.ResponseWriter, r *http.Request) {
	if err := h.woo.DeleteNote(r.Context(), idParam(r, "id"), idParam(r, "noteId")); err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditOrderNoteDeleted, models.AuditSubjectOrder, sid(idParam(r, "id")), nil)
	writeJSON(w, http.StatusOK, map[string]any{"id": idParam(r, "noteId")})
}

func (h *ShopAdminHandler) Refund(w http.ResponseWriter, r *http.Request) {
	var in woo.RefundInput
	if !decodeJSON(w, r, &in) {
		return
	}
	o, refund, err := h.woo.CreateRefund(r.Context(), idParam(r, "id"), in, callerID(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditOrderRefunded, models.AuditSubjectOrder, sid(o.ID), map[string]any{"amount": refund.Amount, "reason": refund.Reason})
	writeJSON(w, http.StatusCreated, map[string]any{"order": o, "refund": refund})
}

type orderEmailPayload struct {
	Type string `json:"type"`
}

// SendOrderEmail is POST /v1/admin/orders/{id}/emails: resend an email by hand.
func (h *ShopAdminHandler) SendOrderEmail(w http.ResponseWriter, r *http.Request) {
	var p orderEmailPayload
	if !decodeJSON(w, r, &p) {
		return
	}
	if _, ok := woo.EmailDefByID(p.Type); !ok || p.Type == "customer_new_account" {
		writeError(w, http.StatusBadRequest, "Unknown email", "errors.unknownEmail")
		return
	}
	o, err := h.woo.Order(r.Context(), idParam(r, "id"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	sent := h.mailer != nil && h.mailer.Send(r.Context(), p.Type, o, "")
	if sent {
		_, _ = h.woo.AddNote(r.Context(), o.ID, "Order details manually sent to customer ("+p.Type+").", false, "", "", 0)
	}
	h.record(r, models.AuditOrderEmailSent, models.AuditSubjectOrder, sid(o.ID), map[string]any{"email": p.Type, "sent": sent})
	writeJSON(w, http.StatusOK, map[string]any{"sent": sent})
}

// ------------------------------------------------------------------ coupons

func (h *ShopAdminHandler) Coupons(w http.ResponseWriter, r *http.Request) {
	res, err := h.woo.Coupons(r.Context(), strings.TrimSpace(r.URL.Query().Get("search")), queryInt(r, "page", 1), queryInt(r, "perPage", 20))
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *ShopAdminHandler) Coupon(w http.ResponseWriter, r *http.Request) {
	c, err := h.woo.Coupon(r.Context(), idParam(r, "id"))
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *ShopAdminHandler) saveCoupon(w http.ResponseWriter, r *http.Request, id uint64) {
	var c woo.Coupon
	if !decodeJSON(w, r, &c) {
		return
	}
	saved, err := h.woo.SaveCoupon(r.Context(), id, c, callerID(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditCouponSaved, models.AuditSubjectCoupon, sid(saved.ID), map[string]any{"code": saved.Code})
	status := http.StatusOK
	if id == 0 {
		status = http.StatusCreated
	}
	writeJSON(w, status, saved)
}

func (h *ShopAdminHandler) CreateCoupon(w http.ResponseWriter, r *http.Request) {
	h.saveCoupon(w, r, 0)
}
func (h *ShopAdminHandler) UpdateCoupon(w http.ResponseWriter, r *http.Request) {
	h.saveCoupon(w, r, idParam(r, "id"))
}

func (h *ShopAdminHandler) DeleteCoupon(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")
	if err := h.woo.DeleteCoupon(r.Context(), id, queryFlag(r, "force")); err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditCouponDeleted, models.AuditSubjectCoupon, sid(id), nil)
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

// ------------------------------------------------------------------ settings

func (h *ShopAdminHandler) Settings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"options": h.woo.AdminSettings(r.Context()), "fields": woo.SettingFields()})
}

type settingsPayload struct {
	Options map[string]any `json:"options"`
}

func (h *ShopAdminHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var p settingsPayload
	if !decodeJSON(w, r, &p) {
		return
	}
	changed, err := h.woo.SaveAdminSettings(r.Context(), p.Options)
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditSettingsUpdated, models.AuditSubjectSetting, "options", map[string]any{"changed": changed})
	h.Settings(w, r)
}

func (h *ShopAdminHandler) Taxes(w http.ResponseWriter, r *http.Request) {
	class := r.URL.Query().Get("class")
	if class == "" {
		class = "*"
	}
	rates, err := h.woo.TaxRates(r.Context(), class)
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rates)
}

func (h *ShopAdminHandler) saveTax(w http.ResponseWriter, r *http.Request, id uint64) {
	var t woo.TaxRate
	if !decodeJSON(w, r, &t) {
		return
	}
	saved, err := h.woo.SaveTaxRate(r.Context(), id, t)
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditTaxSaved, models.AuditSubjectSetting, "tax:"+sid(saved.ID), map[string]any{"rate": saved.Rate, "country": saved.Country})
	writeJSON(w, http.StatusOK, saved)
}

func (h *ShopAdminHandler) CreateTax(w http.ResponseWriter, r *http.Request) { h.saveTax(w, r, 0) }
func (h *ShopAdminHandler) UpdateTax(w http.ResponseWriter, r *http.Request) {
	h.saveTax(w, r, idParam(r, "id"))
}

func (h *ShopAdminHandler) DeleteTax(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")
	if err := h.woo.DeleteTaxRate(r.Context(), id); err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditTaxDeleted, models.AuditSubjectSetting, "tax:"+sid(id), nil)
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (h *ShopAdminHandler) TaxClasses(w http.ResponseWriter, r *http.Request) {
	classes, err := h.woo.TaxClasses(r.Context())
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, classes)
}

type namePayload struct {
	Name string `json:"name"`
}

func (h *ShopAdminHandler) CreateTaxClass(w http.ResponseWriter, r *http.Request) {
	var p namePayload
	if !decodeJSON(w, r, &p) {
		return
	}
	c, err := h.woo.CreateTaxClass(r.Context(), p.Name)
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditTaxSaved, models.AuditSubjectSetting, "taxClass:"+c.Slug, nil)
	writeJSON(w, http.StatusCreated, c)
}

func (h *ShopAdminHandler) DeleteTaxClass(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := h.woo.DeleteTaxClass(r.Context(), slug); err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditTaxDeleted, models.AuditSubjectSetting, "taxClass:"+slug, nil)
	writeJSON(w, http.StatusOK, map[string]any{"slug": slug})
}

func (h *ShopAdminHandler) Zones(w http.ResponseWriter, r *http.Request) {
	zones, err := h.woo.ShippingZones(r.Context())
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, zones)
}

func (h *ShopAdminHandler) saveZone(w http.ResponseWriter, r *http.Request, id uint64) {
	var in woo.ZoneInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if id == 0 && chi.URLParam(r, "id") == "0" {
		writeError(w, http.StatusBadRequest, "The \"rest of the world\" zone has no name or locations to edit", "errors.restOfWorldZone")
		return
	}
	z, err := h.woo.SaveShippingZone(r.Context(), id, in)
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditShippingSaved, models.AuditSubjectSetting, "zone:"+sid(z.ID), map[string]any{"name": z.Name})
	writeJSON(w, http.StatusOK, z)
}

func (h *ShopAdminHandler) CreateZone(w http.ResponseWriter, r *http.Request) { h.saveZone(w, r, 0) }
func (h *ShopAdminHandler) UpdateZone(w http.ResponseWriter, r *http.Request) {
	h.saveZone(w, r, idParam(r, "id"))
}

func (h *ShopAdminHandler) DeleteZone(w http.ResponseWriter, r *http.Request) {
	id := idParam(r, "id")
	if err := h.woo.DeleteShippingZone(r.Context(), id); err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditShippingDeleted, models.AuditSubjectSetting, "zone:"+sid(id), nil)
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

type methodPayload struct {
	MethodID string `json:"methodId"`
}

func (h *ShopAdminHandler) AddMethod(w http.ResponseWriter, r *http.Request) {
	var p methodPayload
	if !decodeJSON(w, r, &p) {
		return
	}
	m, err := h.woo.AddShippingMethod(r.Context(), idParam(r, "id"), p.MethodID)
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditShippingSaved, models.AuditSubjectSetting, "method:"+sid(m.InstanceID), map[string]any{"method": m.MethodID})
	writeJSON(w, http.StatusCreated, m)
}

func (h *ShopAdminHandler) UpdateMethod(w http.ResponseWriter, r *http.Request) {
	var in woo.MethodInput
	if !decodeJSON(w, r, &in) {
		return
	}
	m, err := h.woo.UpdateShippingMethod(r.Context(), idParam(r, "id"), idParam(r, "instanceId"), in)
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditShippingSaved, models.AuditSubjectSetting, "method:"+sid(m.InstanceID), nil)
	writeJSON(w, http.StatusOK, m)
}

func (h *ShopAdminHandler) DeleteMethod(w http.ResponseWriter, r *http.Request) {
	if err := h.woo.DeleteShippingMethod(r.Context(), idParam(r, "id"), idParam(r, "instanceId")); err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditShippingDeleted, models.AuditSubjectSetting, "method:"+sid(idParam(r, "instanceId")), nil)
	writeJSON(w, http.StatusOK, map[string]any{"instanceId": idParam(r, "instanceId")})
}

func (h *ShopAdminHandler) Gateways(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.woo.Gateways(r.Context()))
}

func (h *ShopAdminHandler) UpdateGateway(w http.ResponseWriter, r *http.Request) {
	var u woo.GatewayUpdate
	if !decodeJSON(w, r, &u) {
		return
	}
	g, err := h.woo.SaveGateway(r.Context(), chi.URLParam(r, "id"), u)
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditGatewayUpdated, models.AuditSubjectSetting, "gateway:"+g.ID, map[string]any{"enabled": g.Enabled})
	writeJSON(w, http.StatusOK, g)
}

func (h *ShopAdminHandler) Emails(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.woo.EmailSettingsList(r.Context()))
}

func (h *ShopAdminHandler) UpdateEmail(w http.ResponseWriter, r *http.Request) {
	var in woo.EmailSettings
	if !decodeJSON(w, r, &in) {
		return
	}
	es, err := h.woo.SaveEmailSetting(r.Context(), chi.URLParam(r, "id"), in)
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditEmailUpdated, models.AuditSubjectSetting, "email:"+es.ID, map[string]any{"enabled": es.Enabled})
	writeJSON(w, http.StatusOK, es)
}

func (h *ShopAdminHandler) Appearance(w http.ResponseWriter, r *http.Request) {
	a, err := h.woo.AdminAppearance(r.Context())
	if err != nil {
		writeWooError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *ShopAdminHandler) UpdateAppearance(w http.ResponseWriter, r *http.Request) {
	var a woo.Appearance
	if !decodeJSON(w, r, &a) {
		return
	}
	saved, err := h.woo.SaveAppearance(r.Context(), a, callerID(r))
	if err != nil {
		writeWooError(w, err)
		return
	}
	h.record(r, models.AuditAppearanceUpdated, models.AuditSubjectSetting, "appearance", map[string]any{"theme": saved.Theme})
	writeJSON(w, http.StatusOK, saved)
}
