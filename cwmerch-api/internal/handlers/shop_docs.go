package handlers

import (
	"reflect"

	"cwmerch-api/internal/invoicing"
	"cwmerch-api/internal/openapi"
	"cwmerch-api/internal/payments"
	"cwmerch-api/internal/woo"
)

// shopDocs documents the storefront and back-office routes: the JSON body
// (reflected from the struct the handler decodes) and the query parameters.
func shopDocs() map[string]openapi.EndpointDoc {
	t := reflect.TypeOf
	body := func(v any) openapi.EndpointDoc { return openapi.EndpointDoc{Body: t(v)} }
	p := func(name string) openapi.Param { return openapi.Param{Name: name} }
	n := func(name string) openapi.Param { return openapi.Param{Name: name, Type: "integer"} }
	num := func(name string) openapi.Param { return openapi.Param{Name: name, Type: "number"} }
	b := func(name string) openapi.Param { return openapi.Param{Name: name, Type: "boolean"} }
	q := func(params ...openapi.Param) openapi.EndpointDoc { return openapi.EndpointDoc{Query: params} }
	paging := []openapi.Param{n("page"), n("perPage")}
	with := func(extra ...openapi.Param) openapi.EndpointDoc { return q(append(extra, paging...)...) }

	return map[string]openapi.EndpointDoc{
		// --- public shop ---
		"GET /v1/shop/countries": q(p("lang")),
		"GET /v1/countries":      q(p("lang")),
		"GET /v1/shop/products": with(p("search"), p("category"), p("tag"), num("minPrice"), num("maxPrice"), b("onSale"), b("featured"),
			p("orderby"), p("order"), p("include"), p("exclude")),
		"GET /v1/shop/categories":    q(b("hideEmpty")),
		"POST /v1/shop/cart":         body(woo.CartRequest{}),
		"POST /v1/shop/checkout":     body(checkoutPayload{}),
		"GET /v1/shop/orders/{id}":   q(openapi.Param{Name: "key", Required: true}),
		"GET /v1/seo/html":           q(openapi.Param{Name: "path", Required: true}),
		"PUT /v1/users/me/addresses": body(addressesPayload{}),
		"GET /v1/users/me/orders/":   with(p("status")),

		// --- catalogue ---
		"GET /v1/admin/products/": with(p("search"), p("status"), p("type"), p("category"), p("tag"), p("stockStatus"), b("onSale"),
			b("featured"), p("orderby"), p("order"), p("include"), p("exclude")),
		"POST /v1/admin/products/":                             body(woo.ProductInput{}),
		"PUT /v1/admin/products/{id}":                          body(woo.ProductInput{}),
		"DELETE /v1/admin/products/{id}":                       q(b("force")),
		"POST /v1/admin/products/{id}/variations/":             body(woo.VariationInput{}),
		"PUT /v1/admin/products/{id}/variations/{variationId}": body(woo.VariationInput{}),
		"POST /v1/admin/categories/":                           body(woo.TermInput{}),
		"PUT /v1/admin/categories/{id}":                        body(woo.TermInput{}),
		"POST /v1/admin/tags/":                                 body(woo.TermInput{}),
		"PUT /v1/admin/tags/{id}":                              body(woo.TermInput{}),
		"POST /v1/admin/shipping/classes/":                     body(woo.TermInput{}),
		"PUT /v1/admin/shipping/classes/{id}":                  body(woo.TermInput{}),
		"POST /v1/admin/attributes/":                           body(woo.AttributeInput{}),
		"PUT /v1/admin/attributes/{id}":                        body(woo.AttributeInput{}),
		"POST /v1/admin/attributes/{id}/terms/":                body(woo.TermInput{}),
		"PUT /v1/admin/attributes/{id}/terms/{termId}":         body(woo.TermInput{}),
		"GET /v1/admin/media/":                                 with(),

		// --- orders ---
		"GET /v1/admin/orders/":              with(p("status"), p("search"), n("customer"), p("from"), p("to")),
		"POST /v1/admin/orders/":             body(adminOrderPayload{}),
		"PUT /v1/admin/orders/{id}":          body(woo.OrderUpdate{}),
		"DELETE /v1/admin/orders/{id}":       q(b("force")),
		"POST /v1/admin/orders/{id}/notes/":  body(notePayload{}),
		"POST /v1/admin/orders/{id}/refunds": body(woo.RefundInput{}),
		"POST /v1/admin/orders/{id}/emails":  body(orderEmailPayload{}),

		// --- coupons / settings ---
		"GET /v1/admin/coupons/":                                 with(p("search")),
		"POST /v1/admin/coupons/":                                body(woo.Coupon{}),
		"PUT /v1/admin/coupons/{id}":                             body(woo.Coupon{}),
		"DELETE /v1/admin/coupons/{id}":                          q(b("force")),
		"PUT /v1/admin/settings/":                                body(settingsPayload{}),
		"GET /v1/admin/taxes/":                                   q(p("class")),
		"POST /v1/admin/taxes/":                                  body(woo.TaxRate{}),
		"PUT /v1/admin/taxes/{id}":                               body(woo.TaxRate{}),
		"POST /v1/admin/tax-classes/":                            body(namePayload{}),
		"POST /v1/admin/shipping/zones/":                         body(woo.ZoneInput{}),
		"PUT /v1/admin/shipping/zones/{id}":                      body(woo.ZoneInput{}),
		"POST /v1/admin/shipping/zones/{id}/methods/":            body(methodPayload{}),
		"PUT /v1/admin/shipping/zones/{id}/methods/{instanceId}": body(woo.MethodInput{}),
		"PUT /v1/admin/payment-gateways/{id}":                    body(woo.GatewayUpdate{}),
		"PUT /v1/admin/emails/{id}":                              body(woo.EmailSettings{}),
		"PUT /v1/admin/appearance/":                              body(woo.Appearance{}),

		// --- shop switch, online payments, invoices ---
		"POST /v1/admin/clone":                           body(clonePayload{}),
		"POST /v1/admin/clone/test":                      body(clonePayload{}),
		"PUT /v1/admin/cwcloud-settings":                 body(woo.CWCloudSettingsInput{}),
		"POST /v1/admin/cwcloud-settings/test-email":     body(testEmailPayload{}),
		"PUT /v1/admin/permalinks":                       body(woo.PermalinksInput{}),
		"PUT /v1/admin/shop-config":                      body(woo.ShopConfig{}),
		"PUT /v1/admin/payment-providers/{id}":           body(payments.ProviderUpdate{}),
		"GET /v1/shop/orders/{id}/payment":               q(openapi.Param{Name: "key", Required: true}),
		"POST /v1/shop/orders/{id}/pay":                  q(openapi.Param{Name: "key", Required: true}),
		"GET /v1/payments/{provider}/webhook":            q(p("payment_ref"), p("payment_id"), p("success")),
		"PUT /v1/admin/invoicing/settings":               body(invoicing.Settings{}),
		"PUT /v1/admin/invoicing/country-defaults/{iso}": body(invoicing.CountryDefaults{}),
		"POST /v1/admin/invoicing/apply-defaults":        body(applyDefaultsPayload{}),
		"POST /v1/admin/invoicing/preview":               body(previewPayload{}),
		"GET /v1/admin/invoices/":                        with(p("search")),
	}
}
