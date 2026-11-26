package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"goocommerce-api/internal/middleware"
	"goocommerce-api/internal/store"
	"goocommerce-api/internal/woo"
)

// Audit actions of the review moderation.
const (
	AuditReviewUpdated = "review.updated"
	AuditReviewDeleted = "review.deleted"
	AuditReviewReplied = "review.replied"
	auditSubjectReview = "review"
)

// ReviewsHandler serves product reviews: the shop's (public, sign-in
// optional) and their moderation (manager or superuser).
type ReviewsHandler struct {
	auditor
	woo *woo.Store
}

func NewReviewsHandler(w *woo.Store, audit *store.AuditStore) *ReviewsHandler {
	return &ReviewsHandler{auditor: newAuditor(audit), woo: w}
}

// reviewReplyPayload is POST /v1/admin/reviews/{id}/reply.
type reviewReplyPayload struct {
	Review string `json:"review"`
}

func rcPathID(r *http.Request, name string) uint64 { return woo.ParseID(chi.URLParam(r, name)) }

// rcCallerID is the signed-in caller's WordPress user id (0 when anonymous).
func rcCallerID(r *http.Request) uint64 {
	id, _ := middleware.UserIDFromContext(r.Context())
	return woo.ParseID(id)
}

// rcClientIP is the caller's address as the reverse proxy reports it.
func rcClientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		first, _, _ := strings.Cut(fwd, ",")
		return strings.TrimSpace(first)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func rcDecode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", CodeInvalidRequestBody)
		return false
	}
	return true
}

// ShopList is GET /v1/shop/products/{id}/reviews: approved reviews and replies.
func (h *ReviewsHandler) ShopList(w http.ResponseWriter, r *http.Request) {
	id := rcPathID(r, "id")
	if id == 0 {
		writeError(w, http.StatusNotFound, "Resource not found", CodeNotFound)
		return
	}
	items, err := h.woo.ProductReviews(r.Context(), id)
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// ShopCreate is POST /v1/shop/products/{id}/reviews (sign-in optional).
// Not audited: anyone can post one, so it would let anyone fill the trail.
func (h *ReviewsHandler) ShopCreate(w http.ResponseWriter, r *http.Request) {
	id := rcPathID(r, "id")
	if id == 0 {
		writeError(w, http.StatusNotFound, "Resource not found", CodeNotFound)
		return
	}
	var in woo.ReviewInput
	if !rcDecode(w, r, &in) {
		return
	}
	review, err := h.woo.CreateReview(r.Context(), id, in, rcCallerID(r), rcClientIP(r), r.UserAgent())
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, review)
}

// AdminList is GET /v1/admin/reviews?status&product&search&page&perPage.
func (h *ReviewsHandler) AdminList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := h.woo.ListReviews(r.Context(), woo.ReviewFilter{
		Status:    strings.TrimSpace(q.Get("status")),
		ProductID: woo.ParseID(q.Get("product")),
		Search:    q.Get("search"),
		Page:      rcInt(r, "page", 1),
		PerPage:   rcInt(r, "perPage", 20),
	})
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// AdminGet is GET /v1/admin/reviews/{id}.
func (h *ReviewsHandler) AdminGet(w http.ResponseWriter, r *http.Request) {
	review, err := h.woo.GetReview(r.Context(), rcPathID(r, "id"))
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writeJSON(w, http.StatusOK, review)
}

// AdminUpdate is PUT /v1/admin/reviews/{id} {status?, review?, rating?}.
func (h *ReviewsHandler) AdminUpdate(w http.ResponseWriter, r *http.Request) {
	id := rcPathID(r, "id")
	var u woo.ReviewUpdate
	if !rcDecode(w, r, &u) {
		return
	}
	review, err := h.woo.UpdateReview(r.Context(), id, u)
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	detail := map[string]any{"productId": review.ProductID, "status": review.Status}
	if u.Rating != nil {
		detail["rating"] = *u.Rating
	}
	if u.Review != nil {
		detail["contentChanged"] = true
	}
	h.record(r, AuditReviewUpdated, auditSubjectReview, strconv.FormatUint(id, 10), detail)
	writeJSON(w, http.StatusOK, review)
}

// AdminDelete is DELETE /v1/admin/reviews/{id} (permanent).
func (h *ReviewsHandler) AdminDelete(w http.ResponseWriter, r *http.Request) {
	id := rcPathID(r, "id")
	cur, err := h.woo.GetReview(r.Context(), id)
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	if err := h.woo.DeleteReview(r.Context(), id); err != nil {
		writeWooErrorRC(w, err)
		return
	}
	h.record(r, AuditReviewDeleted, auditSubjectReview, strconv.FormatUint(id, 10),
		map[string]any{"productId": cur.ProductID, "reviewer": cur.ReviewerEmail})
	w.WriteHeader(http.StatusNoContent)
}

// AdminReply is POST /v1/admin/reviews/{id}/reply {review}: the shop's
// answer, posted (approved) as the signed-in staff member.
func (h *ReviewsHandler) AdminReply(w http.ResponseWriter, r *http.Request) {
	id := rcPathID(r, "id")
	var p reviewReplyPayload
	if !rcDecode(w, r, &p) {
		return
	}
	reply, err := h.woo.ReplyToReview(r.Context(), id, p.Review, rcCallerID(r), rcClientIP(r), r.UserAgent())
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	h.record(r, AuditReviewReplied, auditSubjectReview, strconv.FormatUint(id, 10),
		map[string]any{"productId": reply.ProductID, "replyId": reply.ID})
	writeJSON(w, http.StatusCreated, reply)
}
