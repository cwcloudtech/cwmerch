package handlers

import (
	"reflect"

	"cwmerch-api/internal/openapi"
	"cwmerch-api/internal/woo"
)

// reportsCustomersReviewsDocs documents the reports, accounting, reviews and
// customers routes (merged into EndpointDocs).
func reportsCustomersReviewsDocs() map[string]openapi.EndpointDoc {
	t := reflect.TypeOf
	body := func(v any) openapi.EndpointDoc { return openapi.EndpointDoc{Body: t(v)} }
	q := func(query ...openapi.Param) openapi.EndpointDoc { return openapi.EndpointDoc{Query: query} }
	p := func(name string) openapi.Param { return openapi.Param{Name: name} }
	ints := func(name string) openapi.Param { return openapi.Param{Name: name, Type: "integer"} }

	return map[string]openapi.EndpointDoc{
		// --- dashboard / reports ---
		"GET /v1/admin/dashboard":              q(p("from"), p("to"), p("interval")),
		"GET /v1/admin/reports/summary":        q(p("from"), p("to")),
		"GET /v1/admin/reports/sales":          q(p("from"), p("to"), p("interval")),
		"GET /v1/admin/reports/top-products":   q(p("from"), p("to"), ints("limit")),
		"GET /v1/admin/reports/top-categories": q(p("from"), p("to"), ints("limit")),
		"GET /v1/admin/reports/stock":          q(p("type"), ints("page"), ints("perPage")),
		"GET /v1/admin/reports/customers":      q(p("from"), p("to"), ints("page"), ints("perPage")),

		// --- accounting (JSON, or ?format=pdf) ---
		"GET /v1/admin/accounting/turnover": q(ints("year"), p("format")),
		"GET /v1/admin/accounting/balance":  q(ints("year"), p("format")),
		"GET /v1/admin/accounting/ledger":   q(ints("year"), p("format")),

		// --- reviews ---
		"POST /v1/shop/products/{id}/reviews": body(woo.ReviewInput{}),
		"GET /v1/admin/reviews":               q(p("status"), ints("product"), p("search"), ints("page"), ints("perPage")),
		"PUT /v1/admin/reviews/{id}":          body(woo.ReviewUpdate{}),
		"POST /v1/admin/reviews/{id}/reply":   body(reviewReplyPayload{}),

		// --- customers ---
		"GET /v1/admin/customers":             q(p("search"), ints("page"), ints("perPage")),
		"PUT /v1/admin/customers/{id}":        body(woo.CustomerUpdate{}),
		"GET /v1/admin/customers/{id}/orders": q(ints("page"), ints("perPage")),
	}
}
