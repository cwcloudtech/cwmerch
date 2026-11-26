package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"goocommerce-api/internal/accountingpdf"
	"goocommerce-api/internal/woo"
)

// ReportsHandler serves the admin dashboard, the WooCommerce-Analytics-like
// reports and the French accounting exports (manager or superuser).
type ReportsHandler struct {
	woo *woo.Store
}

func NewReportsHandler(w *woo.Store) *ReportsHandler {
	return &ReportsHandler{woo: w}
}

// writeWooErrorRC maps a woo error: a business refusal keeps its status and
// i18n code, a missing row is a 404, anything else goes through
// writeStoreError (499 on a canceled request, else a logged 500).
func writeWooErrorRC(w http.ResponseWriter, err error) {
	var we *woo.Error
	switch {
	case errors.As(err, &we):
		writeError(w, we.Status, we.Message, we.Code)
	case errors.Is(err, woo.ErrNotFound):
		writeError(w, http.StatusNotFound, "Resource not found", CodeNotFound)
	default:
		writeStoreError(w, err)
	}
}

// rcInt reads an integer query parameter (def when absent or invalid).
func rcInt(r *http.Request, name string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(name))); err == nil {
		return n
	}
	return def
}

// reportRange reads ?from=YYYY-MM-DD&to=YYYY-MM-DD (both inclusive, shop
// timezone; default the last 30 days) as the half-open [from, to+1day).
func (h *ReportsHandler) reportRange(ctx context.Context, r *http.Request) (time.Time, time.Time, bool) {
	loc := h.woo.ReportLocation(ctx)
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	from, to := today.AddDate(0, 0, -29), today
	if v := strings.TrimSpace(r.URL.Query().Get("from")); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, loc)
		if err != nil {
			return time.Time{}, time.Time{}, false
		}
		from = t
	}
	if v := strings.TrimSpace(r.URL.Query().Get("to")); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, loc)
		if err != nil {
			return time.Time{}, time.Time{}, false
		}
		to = t
	}
	if to.Before(from) || to.Sub(from) > 20*366*24*time.Hour {
		return time.Time{}, time.Time{}, false
	}
	return from, to.AddDate(0, 0, 1), true
}

func rcWriteInvalidRange(w http.ResponseWriter) {
	writeError(w, http.StatusBadRequest, "Invalid date range (from/to are YYYY-MM-DD, from <= to)", CodeInvalidDateRange)
}

// Summary is GET /v1/admin/reports/summary?from&to.
func (h *ReportsHandler) Summary(w http.ResponseWriter, r *http.Request) {
	from, to, ok := h.reportRange(r.Context(), r)
	if !ok {
		rcWriteInvalidRange(w)
		return
	}
	sum, err := h.woo.ReportSummary(r.Context(), from, to)
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

// Sales is GET /v1/admin/reports/sales?from&to&interval=day|week|month|year.
func (h *ReportsHandler) Sales(w http.ResponseWriter, r *http.Request) {
	from, to, ok := h.reportRange(r.Context(), r)
	if !ok {
		rcWriteInvalidRange(w)
		return
	}
	sales, err := h.woo.SalesIntervals(r.Context(), from, to, r.URL.Query().Get("interval"))
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sales)
}

// TopProducts is GET /v1/admin/reports/top-products?from&to&limit.
func (h *ReportsHandler) TopProducts(w http.ResponseWriter, r *http.Request) {
	from, to, ok := h.reportRange(r.Context(), r)
	if !ok {
		rcWriteInvalidRange(w)
		return
	}
	items, err := h.woo.TopProducts(r.Context(), from, to, rcInt(r, "limit", 10))
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// TopCategories is GET /v1/admin/reports/top-categories?from&to&limit.
func (h *ReportsHandler) TopCategories(w http.ResponseWriter, r *http.Request) {
	from, to, ok := h.reportRange(r.Context(), r)
	if !ok {
		rcWriteInvalidRange(w)
		return
	}
	items, err := h.woo.TopCategories(r.Context(), from, to, rcInt(r, "limit", 10))
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// Stock is GET /v1/admin/reports/stock?type=low|out|all&page&perPage.
func (h *ReportsHandler) Stock(w http.ResponseWriter, r *http.Request) {
	page, err := h.woo.StockReport(r.Context(), r.URL.Query().Get("type"), rcInt(r, "page", 1), rcInt(r, "perPage", 20))
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// Customers is GET /v1/admin/reports/customers?from&to&page&perPage.
func (h *ReportsHandler) Customers(w http.ResponseWriter, r *http.Request) {
	from, to, ok := h.reportRange(r.Context(), r)
	if !ok {
		rcWriteInvalidRange(w)
		return
	}
	page, err := h.woo.CustomerReport(r.Context(), from, to, rcInt(r, "page", 1), rcInt(r, "perPage", 20))
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// DashboardResponse is GET /v1/admin/dashboard.
type DashboardResponse struct {
	Summary      woo.ReportSummary   `json:"summary"`
	Sales        []woo.SalesInterval `json:"sales"`
	Interval     string              `json:"interval"`
	TopProducts  []woo.TopProduct    `json:"topProducts"`
	LowStock     []woo.StockItem     `json:"lowStock"`
	RecentOrders []any               `json:"recentOrders"`
}

// rcDashboardInterval picks the chart's bucket from the period's length.
func rcDashboardInterval(from, to time.Time) string {
	switch days := to.Sub(from).Hours() / 24; {
	case days > 3*366:
		return "year"
	case days > 120:
		return "month"
	case days > 62:
		return "week"
	}
	return "day"
}

// Dashboard is GET /v1/admin/dashboard?from&to[&interval]. recentOrders is
// left empty here (filled by the orders side).
func (h *ReportsHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	resp, ok := h.BuildDashboard(w, r)
	if ok {
		writeJSON(w, http.StatusOK, resp)
	}
}

// BuildDashboard computes the dashboard (writing the error response and
// returning false on failure), so a caller can add recentOrders.
func (h *ReportsHandler) BuildDashboard(w http.ResponseWriter, r *http.Request) (DashboardResponse, bool) {
	ctx := r.Context()
	from, to, ok := h.reportRange(ctx, r)
	if !ok {
		rcWriteInvalidRange(w)
		return DashboardResponse{}, false
	}
	interval := r.URL.Query().Get("interval")
	if interval == "" {
		interval = rcDashboardInterval(from, to)
	}
	sum, err := h.woo.ReportSummary(ctx, from, to)
	if err != nil {
		writeWooErrorRC(w, err)
		return DashboardResponse{}, false
	}
	sales, err := h.woo.SalesIntervals(ctx, from, to, interval)
	if err != nil {
		writeWooErrorRC(w, err)
		return DashboardResponse{}, false
	}
	top, err := h.woo.TopProducts(ctx, from, to, 5)
	if err != nil {
		writeWooErrorRC(w, err)
		return DashboardResponse{}, false
	}
	low, err := h.woo.StockReport(ctx, "low", 1, 10)
	if err != nil {
		writeWooErrorRC(w, err)
		return DashboardResponse{}, false
	}
	return DashboardResponse{Summary: sum, Sales: sales.Intervals, Interval: sales.Interval, TopProducts: top,
		LowStock: low.Items, RecentOrders: []any{}}, true
}

// accountingYear reads ?year= (default: the current year, shop timezone).
func (h *ReportsHandler) accountingYear(w http.ResponseWriter, r *http.Request) (int, bool) {
	year := time.Now().In(h.woo.ReportLocation(r.Context())).Year()
	if v := strings.TrimSpace(r.URL.Query().Get("year")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1970 || n > 2200 {
			writeError(w, http.StatusBadRequest, "Invalid year", CodeInvalidDateRange)
			return 0, false
		}
		year = n
	}
	return year, true
}

func rcWantsPDF(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("format")), "pdf")
}

func writePDFRC(w http.ResponseWriter, filename string, data []byte) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// Turnover is GET /v1/admin/accounting/turnover?year&format=json|pdf.
func (h *ReportsHandler) Turnover(w http.ResponseWriter, r *http.Request) {
	year, ok := h.accountingYear(w, r)
	if !ok {
		return
	}
	t, err := h.woo.Turnover(r.Context(), year)
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	if !rcWantsPDF(r) {
		writeJSON(w, http.StatusOK, t)
		return
	}
	data, err := accountingpdf.Turnover(h.woo.AccountingShop(r.Context()), t)
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writePDFRC(w, fmt.Sprintf("chiffre-affaires-%d.pdf", year), data)
}

// Balance is GET /v1/admin/accounting/balance?year&format=json|pdf.
func (h *ReportsHandler) Balance(w http.ResponseWriter, r *http.Request) {
	year, ok := h.accountingYear(w, r)
	if !ok {
		return
	}
	b, err := h.woo.Balance(r.Context(), year)
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	if !rcWantsPDF(r) {
		writeJSON(w, http.StatusOK, b)
		return
	}
	data, err := accountingpdf.Balance(h.woo.AccountingShop(r.Context()), b)
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writePDFRC(w, fmt.Sprintf("balance-%d.pdf", year), data)
}

// Ledger is GET /v1/admin/accounting/ledger?year&format=json|pdf.
func (h *ReportsHandler) Ledger(w http.ResponseWriter, r *http.Request) {
	year, ok := h.accountingYear(w, r)
	if !ok {
		return
	}
	l, err := h.woo.Ledger(r.Context(), year)
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	if !rcWantsPDF(r) {
		writeJSON(w, http.StatusOK, l)
		return
	}
	data, err := accountingpdf.Ledger(h.woo.AccountingShop(r.Context()), l)
	if err != nil {
		writeWooErrorRC(w, err)
		return
	}
	writePDFRC(w, fmt.Sprintf("grand-livre-%d.pdf", year), data)
}
