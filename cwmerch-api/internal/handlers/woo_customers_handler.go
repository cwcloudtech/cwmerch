package handlers

import (
	"net/http"
	"strconv"

	"cwmerch-api/internal/store"
	"cwmerch-api/internal/woo"
)

// Audit actions of customer administration.
const (
	AuditCustomerUpdated = "customer.updated"
	auditSubjectCustomer = "customer"
)

// CustomersHandler is the admin's customers (manager or superuser).
type CustomersHandler struct {
	auditor
	woo *woo.Store
}

func NewCustomersHandler(w *woo.Store, audit *store.AuditStore) *CustomersHandler {
	return &CustomersHandler{auditor: newAuditor(audit), woo: w}
}

// List is GET /v1/admin/customers?search&page&perPage.
func (h *CustomersHandler) List(w http.ResponseWriter, r *http.Request) {
	page, err := h.woo.ListCustomers(r.Context(), r.URL.Query().Get("search"), rcInt(r, "page", 1), rcInt(r, "perPage", 20))
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// Get is GET /v1/admin/customers/{id}.
func (h *CustomersHandler) Get(w http.ResponseWriter, r *http.Request) {
	c, err := h.woo.GetCustomer(r.Context(), rcPathID(r, "id"))
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// Update is PUT /v1/admin/customers/{id} {firstName?, lastName?, email?, billing?, shipping?}.
func (h *CustomersHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := rcPathID(r, "id")
	var u woo.CustomerUpdate
	if !rcDecode(w, r, &u) {
		return
	}
	c, err := h.woo.UpdateCustomer(r.Context(), id, u)
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	changed := []string{}
	if u.FirstName != nil || u.LastName != nil {
		changed = append(changed, "name")
	}
	if u.Email != nil {
		changed = append(changed, "email")
	}
	if u.Billing != nil {
		changed = append(changed, "billing")
	}
	if u.Shipping != nil {
		changed = append(changed, "shipping")
	}
	h.record(r, AuditCustomerUpdated, auditSubjectCustomer, strconv.FormatUint(id, 10),
		map[string]any{"email": c.Email, "changed": changed})
	writeJSON(w, http.StatusOK, c)
}

// Orders is GET /v1/admin/customers/{id}/orders?page&perPage: order summaries.
func (h *CustomersHandler) Orders(w http.ResponseWriter, r *http.Request) {
	id := rcPathID(r, "id")
	if _, err := h.woo.GetCustomer(r.Context(), id); err != nil {
		writeWooErrorRC(w, err)
		return
	}
	page, err := h.woo.CustomerOrders(r.Context(), id, rcInt(r, "page", 1), rcInt(r, "perPage", 20))
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}
