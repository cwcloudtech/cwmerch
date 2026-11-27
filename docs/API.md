# cwmerch API contract

The authoritative, always-in-sync description is the OpenAPI document generated
from the router: `GET /openapi.json`, rendered by Swagger UI at `GET /` on the
API. This file is the human overview of the same surface.

## Conventions

* Base path `/v1`. JSON in and out (`Content-Type: application/json`), except
  uploads (`multipart/form-data`) and PDF exports (`application/pdf`).
* Authentication: `Authorization: Bearer <jwt>` (from login) **or**
  `X-Api-Key: <long-term API key>` (created under `/v1/users/me/api-keys`).
* Errors: `{"message": "...", "i18n_code": "errors.xxx"}`. A 5xx never carries
  its cause; the `X-Cid` response header is the correlation id to give support.
* Lists: `?page=1&perPage=20` (perPage max 100) answer
  `{"items": [...], "total": 123, "page": 1, "perPage": 20, "totalPages": 7}`.
* Money is a JSON number in the shop currency, rounded to the shop's decimals.
* Dates are RFC 3339 strings (UTC); date filters accept `YYYY-MM-DD`.
* Order statuses are WooCommerce's without the `wc-` prefix:
  `pending, processing, on-hold, completed, cancelled, refunded, failed, checkout-draft`.
* Roles (`role` on a user): `superuser` (WordPress *administrator*), `manager`
  (WooCommerce *shop_manager*), `customer` (any other WordPress role),
  `disabled`, `ban`. The admin area is for `superuser` and `manager`; user
  administration, audit and IP bans are `superuser` only.

## Public / infrastructure

| Method & path | Notes |
|---|---|
| `GET /` | Swagger UI |
| `GET /openapi.json` | OpenAPI 3 document |
| `GET /metrics` | Prometheus |
| `GET /v1/health`, `GET /v1/manifest` | liveness, build manifest `{version, sha, details}` |
| `POST /v1/contact` | `{email, subject, message, name, firstname}` → forwarded to CWCloud `/v1/contactreq` |
| `GET /v1/media/{path}` | uploaded images (when cwmerch owns the uploads folder) |

## Authentication & account

| Method & path | Body / answer |
|---|---|
| `POST /v1/users/` | register a customer `{email, password, name, surname}` → `UserResponse` |
| `POST /v1/users/login` | `{email, password}` (email **or** WordPress username) → `UserResponse` or `{mfaRequired:true, challengeToken, hasTotp, hasWebAuthn}` |
| `POST /v1/users/login/mfa/totp` | `{challengeToken, code}` → `UserResponse` |
| `POST /v1/users/login/mfa/webauthn/begin` / `finish` | WebAuthn (YubiKey) login ceremony |
| `POST /v1/users/forgot-password` | `{email}` → always 200 |
| `POST /v1/users/reset-password` | `{token, password, confirmPassword}` |
| `GET /v1/oidc/` | `[{name:"google", label:"Google"}]` enabled providers |
| `GET /v1/oidc/google/login?redirect=/my-account` | 302 to Google; back to UI `/oidc/callback#token=...` |
| `GET /v1/users/me` / `PUT /v1/users/me` | `{name, surname, displayName, password?, confirmPassword?}` |
| `POST /v1/users/logout` | audit only |
| `GET/PUT /v1/users/me/addresses` | `{billing: Address, shipping: Address}` |
| `GET /v1/users/me/orders` | paged `Order` list of the caller |
| `GET /v1/users/me/orders/{id}` | one of the caller's orders |
| `GET /v1/users/me/downloads` | `[{productId, productName, downloadId, name, url, remaining, expiresAt}]` |
| `GET/POST /v1/users/me/api-keys`, `DELETE /{id}` | long-term API keys (`{description, expiresAt?}` → plaintext `token` once) |
| `GET /v1/users/me/mfa` | `{totpEnabled, webauthn:[{id,name,createdAt}]}` |
| `POST /v1/users/me/mfa/totp/setup` | `{secret, otpauthUrl, qrCode(data URI)}` |
| `POST /v1/users/me/mfa/totp/confirm` `{code}`, `DELETE /v1/users/me/mfa/totp` | |
| `POST /v1/users/me/mfa/webauthn/register/begin` / `finish`, `DELETE /webauthn/{credentialId}` | YubiKey / security keys |

`UserResponse` = `{id, email, name, surname, displayName, username, role, token, mfaEnabled, i18nCode?}`.

`Address` = `{firstName, lastName, company, address1, address2, city, state, postcode, country, email, phone}`
(`email` only on billing).

## Public shop

| Method & path | Notes |
|---|---|
| `GET /v1/shop/settings` | `ShopSettings` (below) |
| `GET /v1/shop/countries` | `[{code, name, states:[{code,name}]}]` restricted to the selling countries |
| `GET /v1/shop/products` | `?search&category(slug or id)&tag&minPrice&maxPrice&onSale&featured&orderby=menu_order|date|price|price-desc|popularity|rating|title&page&perPage` → paged `Product` (no `variations`) |
| `GET /v1/shop/products/{idOrSlug}` | `Product` with `variations` and `related` (ids) |
| `GET /v1/shop/products/{id}/reviews` | `[Review]` approved only |
| `POST /v1/shop/products/{id}/reviews` | `{rating, review, author?, email?}` (auth optional; obeys "verified owners only") |
| `GET /v1/shop/categories` | `[Category]` (flat, `parent` gives the tree) |
| `GET /v1/shop/tags` | `[Tag]` |
| `POST /v1/shop/cart` | price a cart: `CartRequest` → `Cart` |
| `POST /v1/shop/checkout` | `CheckoutRequest` → 201 `{order: Order, paymentInstructions, token?}` |
| `GET /v1/shop/orders/{id}?key=wc_order_xxx` | "order received" page |

`ShopSettings`:
```json
{"name":"My shop","description":"","currency":"EUR","currencySymbol":"€","currencyPosition":"right_space",
 "thousandSeparator":" ","decimalSeparator":",","decimals":2,"weightUnit":"kg","dimensionUnit":"cm",
 "calcTaxes":true,"pricesIncludeTax":true,"taxDisplayShop":"incl","taxDisplayCart":"incl",
 "enableCoupons":true,"enableReviews":true,"enableReviewRating":true,"reviewRatingRequired":true,
 "verifiedOwnersOnly":false,"guestCheckout":true,"signupAtCheckout":true,
 "defaultCountry":"FR","allowedCountries":["FR","BE"],"shipToCountries":["FR","BE"],
 "storeAddress":{"address1":"","address2":"","city":"","postcode":"","country":"FR","state":""},
 "paymentGateways":[{"id":"bacs","title":"Direct bank transfer","description":"..."}],
 "contactEnabled":true,"oidcProviders":["google"],"version":"1.0.0"}
```

`Product`:
```json
{"id":12,"name":"Hoodie","slug":"hoodie","type":"simple|variable|grouped|external","status":"publish|draft|pending|private",
 "featured":false,"catalogVisibility":"visible|catalog|search|hidden","description":"<p>html</p>","shortDescription":"",
 "sku":"H-1","price":42,"regularPrice":45,"salePrice":42,"dateOnSaleFrom":null,"dateOnSaleTo":null,"onSale":true,
 "priceMin":42,"priceMax":42,
 "taxStatus":"taxable|shipping|none","taxClass":"","manageStock":true,"stockQuantity":10,
 "stockStatus":"instock|outofstock|onbackorder","backorders":"no|notify|yes","lowStockAmount":null,
 "soldIndividually":false,"weight":"0.5","length":"","width":"","height":"","shippingClass":"",
 "virtual":false,"downloadable":false,"downloads":[{"id":"uuid","name":"file","file":"https://..."}],
 "downloadLimit":-1,"downloadExpiry":-1,"externalUrl":"","buttonText":"","purchaseNote":"","menuOrder":0,
 "reviewsAllowed":true,"averageRating":4.5,"ratingCount":2,"totalSales":7,
 "categories":[{"id":3,"name":"Clothing","slug":"clothing"}],"tags":[{"id":5,"name":"winter","slug":"winter"}],
 "images":[{"id":40,"src":"https://.../hoodie.jpg","name":"hoodie","alt":""}],
 "attributes":[{"id":1,"name":"Color","slug":"pa_color","position":0,"visible":true,"variation":true,"options":["Blue","Red"]}],
 "defaultAttributes":[{"slug":"pa_color","option":"Blue"}],
 "variations":[Variation],"upsellIds":[],"crossSellIds":[],"groupedProducts":[],"related":[13,14],
 "createdAt":"...","updatedAt":"..."}
```
`Variation` = `{id, parentId, sku, description, price, regularPrice, salePrice, onSale, dateOnSaleFrom, dateOnSaleTo,
status, virtual, downloadable, manageStock, stockQuantity, stockStatus, backorders, weight, length, width, height,
shippingClass, taxClass, image:{id,src}|null, attributes:[{slug, name, option}], menuOrder}`
(an empty `option` = "any").

`Category` = `{id, name, slug, parent, description, count, image:{id,src}|null, menuOrder}`; `Tag` = `{id, name, slug, description, count}`.
`Review` = `{id, productId, productName, reviewer, reviewerEmail, review, rating, verified, status:"approved|hold|spam|trash", createdAt}`.

`CartRequest`:
```json
{"items":[{"productId":12,"variationId":0,"quantity":2}],"coupons":["WELCOME10"],
 "customer":{"country":"FR","state":"","postcode":"75001","city":"Paris"},"shippingMethod":"flat_rate:3"}
```
`Cart`:
```json
{"items":[{"key":"12-0","productId":12,"variationId":0,"name":"Hoodie","sku":"H-1","image":"https://...",
   "attributes":[{"name":"Color","option":"Blue"}],"quantity":2,"price":42,"subtotal":84,"subtotalTax":14,
   "total":75.6,"totalTax":12.6,"maxQuantity":10}],
 "coupons":[{"code":"welcome10","discount":8.4,"discountTax":1.4}],
 "subtotal":84,"subtotalTax":14,"discountTotal":8.4,"discountTax":1.4,"shippingTotal":5,"shippingTax":1,
 "taxTotal":13.6,"total":81.6,"taxes":[{"rateId":1,"label":"TVA","amount":13.6}],
 "shippingRates":[{"id":"flat_rate:3","methodId":"flat_rate","instanceId":3,"label":"Flat rate","cost":5,"tax":1}],
 "chosenShippingMethod":"flat_rate:3","needsShipping":true,"currency":"EUR",
 "notices":[{"type":"error|notice","code":"cart.outOfStock","message":"..."}]}
```
Displayed prices follow `taxDisplayCart`; `subtotal`/`total` are always **excluding** tax and the `*Tax` fields carry the tax (WooCommerce's own split), `total` of the cart is the grand total to pay.

`CheckoutRequest` = `CartRequest` minus `customer`, plus
`{billing: Address, shipping: Address, shipToDifferentAddress, paymentMethod:"bacs|cheque|cod", customerNote, createAccount, accountPassword}`.

`Order`:
```json
{"id":101,"number":"101","orderKey":"wc_order_abc","status":"processing","currency":"EUR",
 "createdAt":"...","updatedAt":"...","paidAt":null,"completedAt":null,"customerId":3,"customerNote":"",
 "billing":Address,"shipping":Address,"paymentMethod":"bacs","paymentMethodTitle":"Direct bank transfer","transactionId":"",
 "pricesIncludeTax":true,
 "lineItems":[{"id":1,"productId":12,"variationId":0,"name":"Hoodie","sku":"H-1","quantity":2,"subtotal":70,"subtotalTax":14,"total":63,"totalTax":12.6,"meta":[{"key":"Color","value":"Blue"}]}],
 "shippingLines":[{"id":2,"methodId":"flat_rate","instanceId":3,"methodTitle":"Flat rate","total":5,"totalTax":1}],
 "feeLines":[],"couponLines":[{"id":3,"code":"welcome10","discount":7,"discountTax":1.4}],
 "taxLines":[{"id":4,"rateId":1,"label":"TVA","compound":false,"taxTotal":12.6,"shippingTaxTotal":1,"ratePercent":20}],
 "refunds":[{"id":110,"reason":"","amount":10,"createdAt":"...","refundedBy":1}],
 "discountTotal":7,"discountTax":1.4,"shippingTotal":5,"shippingTax":1,"cartTax":12.6,"totalTax":13.6,"total":81.6,
 "totalRefunded":0,"createdVia":"checkout|admin|cwmerch","customerIpAddress":"","customerUserAgent":""}
```

## Admin (role `manager` or `superuser`)

| Area | Endpoints |
|---|---|
| Dashboard | `GET /v1/admin/dashboard?from&to` → `{summary: ReportSummary, sales: [Interval], topProducts, lowStock, recentOrders}` |
| Products | `GET /v1/admin/products?search&status&type&category&stockStatus&orderby&order&page&perPage`, `POST /v1/admin/products`, `GET/PUT/DELETE /v1/admin/products/{id}` (`?force=true` deletes, else trash), `POST /{id}/duplicate` |
| Variations | `GET/POST /v1/admin/products/{id}/variations`, `PUT/DELETE /v1/admin/products/{id}/variations/{variationId}`, `POST /v1/admin/products/{id}/variations/generate` (all combinations) |
| Categories | `GET/POST /v1/admin/categories`, `PUT/DELETE /v1/admin/categories/{id}` |
| Tags | `GET/POST /v1/admin/tags`, `PUT/DELETE /v1/admin/tags/{id}` |
| Attributes | `GET/POST /v1/admin/attributes`, `PUT/DELETE /{id}`, `GET/POST /v1/admin/attributes/{id}/terms`, `PUT/DELETE /v1/admin/attributes/{id}/terms/{termId}` — `Attribute` = `{id, name, slug, type, orderBy, hasArchives}` |
| Shipping classes | `GET/POST /v1/admin/shipping/classes`, `PUT/DELETE /{id}` |
| Media | `POST /v1/admin/media` (multipart `file`) → `{id, src, name, alt}`, `GET /v1/admin/media?page` |
| Orders | `GET /v1/admin/orders?status&search&customer&from&to&page&perPage`, `POST /v1/admin/orders` (manual order, `CheckoutRequest`-like plus `status`, `customerId`), `GET/PUT/DELETE /v1/admin/orders/{id}` (PUT `{status?, billing?, shipping?, customerNote?, transactionId?}`; status changes send the WooCommerce emails), `GET/POST /v1/admin/orders/{id}/notes` (`{note, customerNote}`), `DELETE /notes/{noteId}`, `POST /v1/admin/orders/{id}/refunds` (`{amount, reason, lineItems:[{id, quantity, refundTotal}], restock}`), `POST /v1/admin/orders/{id}/emails` (`{type:"customer_invoice|new_order|customer_processing_order|customer_completed_order"}`) |
| Customers | `GET /v1/admin/customers?search&page`, `GET/PUT /v1/admin/customers/{id}` (`{firstName,lastName,email,billing,shipping}`), `GET /v1/admin/customers/{id}/orders` — `Customer` = `{id, email, username, firstName, lastName, role, ordersCount, totalSpent, billing, shipping, createdAt, lastOrderAt}` |
| Coupons | `GET/POST /v1/admin/coupons`, `GET/PUT/DELETE /v1/admin/coupons/{id}` — `Coupon` = `{id, code, amount, discountType:"percent|fixed_cart|fixed_product", description, dateExpires, usageCount, individualUse, productIds, excludedProductIds, usageLimit, usageLimitPerUser, limitUsageToXItems, freeShipping, productCategories, excludedProductCategories, excludeSaleItems, minimumAmount, maximumAmount, emailRestrictions, status}` |
| Reviews | `GET /v1/admin/reviews?status&product&page`, `PUT /v1/admin/reviews/{id}` (`{status, review?, rating?}`), `DELETE /v1/admin/reviews/{id}`, `POST /v1/admin/reviews/{id}/reply` (`{review}`) |
| Settings | `GET /v1/admin/settings` → `{options: {name: value}, fields: [{name, group, type, label, choices?}]}`, `PUT /v1/admin/settings` `{options: {...}}` (only whitelisted WooCommerce option names) |
| Taxes | `GET /v1/admin/taxes?class`, `POST /v1/admin/taxes`, `PUT/DELETE /v1/admin/taxes/{id}` — `TaxRate` = `{id, country, state, postcodes[], cities[], rate, name, priority, compound, shipping, order, class}`; `GET/POST /v1/admin/tax-classes` (`{name}`), `DELETE /v1/admin/tax-classes/{slug}` |
| Shipping zones | `GET/POST /v1/admin/shipping/zones`, `PUT/DELETE /v1/admin/shipping/zones/{id}` (`{name, order, locations:[{code, type:"country|state|postcode|continent"}]}`), `POST /v1/admin/shipping/zones/{id}/methods` (`{methodId:"flat_rate|free_shipping|local_pickup"}`), `PUT/DELETE /v1/admin/shipping/zones/{id}/methods/{instanceId}` (`{enabled, order, settings:{title, cost, tax_status, min_amount, requires, ignore_discounts, class_cost_<id>, no_class_cost, type}}`). Zone `0` is "Locations not covered by your other zones". |
| Payments | `GET /v1/admin/payment-gateways` → `[{id, title, description, enabled, order, settings}]`, `PUT /v1/admin/payment-gateways/{id}` `{enabled, settings}` (bacs `accounts` too) |
| Emails | `GET /v1/admin/emails` → `[{id, title, description, customerEmail, enabled, recipient, subject, heading, additionalContent, emailType}]`, `PUT /v1/admin/emails/{id}` |
| Reports | `GET /v1/admin/reports/summary?from&to` → `ReportSummary {grossSales, netSales, orders, itemsSold, refunds, discount, shipping, taxes, averageOrderValue, newCustomers}`; `GET /v1/admin/reports/sales?from&to&interval=day|week|month|year` → `{intervals:[{date, orders, grossSales, netSales, itemsSold, refunds, taxes, shipping}]}`; `GET /v1/admin/reports/top-products?from&to&limit`; `GET /v1/admin/reports/top-categories?from&to&limit`; `GET /v1/admin/reports/stock?type=low|out|all&page`; `GET /v1/admin/reports/customers?from&to&page` |
| Accounting | `GET /v1/admin/accounting/turnover?year&format=json|pdf` (chiffre d'affaires per month), `GET /v1/admin/accounting/balance?year&format=json|pdf` (balance des comptes), `GET /v1/admin/accounting/ledger?year&format=json|pdf` (grand livre) |

Superuser only: `GET /v1/admin/users?search&role&page`, `PUT /v1/admin/users/{id}` (`{role:"superuser|manager|customer|disabled|ban"}`), `DELETE /v1/admin/users/{id}`, `POST /v1/admin/users/{id}/disable-mfa`, `GET /v1/admin/users/{id}/ips`, `GET /v1/admin/audit?page&action&userId`, `GET /v1/admin/ip-bans/`, `GET /v1/admin/ip-bans/stats`, `PUT /v1/admin/ip-bans/` (`{ip, action:"ban|unban", motif}`).

## AI assistant (any signed-in user)

Same contract as cwclock:

* `GET /v1/ai/providers`, `GET /v1/ai/shareable-emails?q=`
* `GET/POST /v1/ai/connections/`, `POST /v1/ai/connections/order` (`{ids}`), `PUT/DELETE /v1/ai/connections/{id}`,
  `GET/POST /v1/ai/connections/{id}/shares` (`{email, role:"user"}`), `DELETE /v1/ai/connections/{id}/shares/{userId}`
  — `AIConnection` = `{id, ownerId, name, provider, baseUrl, model, maxTokens, hasToken, role:"owner|user", createdAt, updatedAt}`, the write payload adds `token`.
* `POST /v1/ai/chat` `{message, connectionId?, conversationId?, messages?:[{role, message}], settings?}` →
  `{status, message (Markdown answer), usage, model, conversationId, title, downloadTtlSeconds?}`.
  The agent loop calls cwmerch's own API **with the caller's JWT** (tool `cwmerch_api`), guided by the embedded Markdown skills;
  generated PDFs (accounting exports) come back as links to `GET /v1/ai-downloads/{id}`.
* `GET /v1/ai/conversations/`, `GET /v1/ai/conversations/{id}`, `DELETE /v1/ai/conversations/` (`{ids?, all?}`)
* `POST /v1/ai/cwcloud/adapter` — CWCloud external adapter: `{message}` or `{messages:[{role, message}]}`, optional `adapter` (subscription name) → `{status:"ok", message, usage:{prompt_tokens, completion_tokens, total_tokens}}`.

## Added since the first draft

| Method & path | Notes |
|---|---|
| `GET /v1/shop/appearance` | storefront look: `{name, tagline, logo, favicon, theme, colors{primary, primaryText, secondary, accent, background, surface, text, muted, border, header, headerText, footer, footerText, sale}, fonts{heading, body}, radius, banner{enabled, autoplay, intervalSeconds, height, slides[{image, mobileImage, title, subtitle, buttonText, buttonLink, align, overlay, textColor}]}, announcement, homeSections[{type, title, limit, content}], social, contact, footerText, updatedAt}` |
| `GET /v1/shop/themes` | colour/font presets: classic, garden, rose, ocean, sunset, midnight, minimal, luxury |
| `GET /v1/shop/custom.css` | theme CSS variables (`--cwmerch-*`) + the admin's custom CSS |
| `GET/PUT /v1/admin/appearance/` | edit the appearance (adds `customCss`) |
| `GET /v1/seo/html?path=/product/slug` | the UI's `index.html` with the page's `<head>` (title, description, canonical, Open Graph, Twitter, JSON-LD) — for a reverse proxy that routes public page navigations here (the UI's nginx does not) |
| `GET /sitemap.xml`, `GET /robots.txt` | SEO |
| `GET /v1/countries`, `GET /v1/currencies` | every country (`{iso, code, name, continent, states}`) / currency (`{code, symbol}`) |
| `POST /v1/admin/products/{id}/restore` | take a product out of the trash |
| `GET /v1/users/me/downloads/{productId}/{downloadId}` | consume a download, answers `{url}` |
| `GET /v1/admin/reviews/{id}` | one review |

Cart/checkout lines accept `attributes` as a map (`{"pa_color":"Blue"}`) or a list
(`[{"slug":"pa_color","option":"Blue"}]`) to pick "any" variation attributes. Checkout answers `409`
with the priced `cart` when the cart has error notices (out of stock, invalid coupon…).

**API-only mode** (`CWMERCH_API_ONLY=true`): the cart, checkout, order-received and customer
account shop routes are not mounted; everything else (admin, reports, accounting, AI, auth, MFA, API keys)
is. cwmerch then serves as an AI agent and back-office API next to an unchanged WooCommerce.

## ai-instruct-2: shop switch, online payments, invoices

### Shop switch (cwmerch as an internal information system)

* `GET /v1/admin/shop-config` / `PUT /v1/admin/shop-config` (manager) →
  `{"shopEnabled": true, "closedMessage": "", "registrationEnabled": true}`.
* `GET /v1/shop/settings` adds `shopEnabled`, `closedMessage`, `registrationEnabled`.
* When `shopEnabled` is false the public storefront endpoints (products, categories, tags, reviews, cart,
  checkout, sitemap) answer **403** `{"i18n_code":"errors.shopDisabled"}`; `/v1/shop/settings`,
  `/v1/shop/appearance`, sign-in and the whole back office keep working. With `registrationEnabled`
  false, `POST /v1/users/` answers 403 `errors.registrationDisabled`.

### Online payments (adapter pattern: Stripe, Konnect, Flouci)

* `GET /v1/admin/payment-providers` (manager) →
  `[{"id":"stripe","name":"Stripe","enabled":false,"configured":false,"mode":"test|live","title":"Credit card (Stripe)","description":"","settings":{...public settings},"secrets":{"secretKey":true,"webhookSecret":false},"webhookUrl":"https://api.../v1/payments/stripe/webhook","currencies":["*"]}]`
  — `secrets` only says whether each secret is stored (never its value).
  * stripe settings: `publishableKey`; secrets: `secretKey`, `webhookSecret`.
  * konnect settings: `walletId`, `acceptedPaymentMethods` (`["wallet","bank_card","e-DINAR"]`), `lifespan` (minutes), `addPaymentFeesToAmount`, `theme`; secrets: `apiKey`. `mode` test = sandbox.
  * flouci settings: `publicKey`, `acceptCard`, `sessionTimeoutSecs`; secrets: `privateKey`. TND only.
* `PUT /v1/admin/payment-providers/{id}` `{enabled, mode, title, description, settings:{...}, secrets:{...}}`
  (a blank or absent secret keeps the stored one; secrets are sealed with `CWMERCH_SECRET_KEY`).
* `POST /v1/admin/payment-providers/{id}/test` → `{"ok": true, "message": "..."}`.
* `GET /v1/shop/settings` `paymentGateways` entries gain `"type":"offline|online"`, and online ones
  `"publishableKey"` (Stripe).
* Checkout with an online `paymentMethod` (`stripe`, `konnect`, `flouci`): the order is created
  `pending`, and the 201 answer adds
  `"payment": {"provider":"stripe","status":"requires_payment","reference":"pi_...","clientSecret":"pi_..._secret_...","publishableKey":"pk_...","redirectUrl":""}`
  — Stripe: confirm in the browser with Stripe.js (Payment Element) using `clientSecret`, `return_url`
  = `<UI>/checkout/payment/{orderId}?key={orderKey}`; Konnect/Flouci: redirect the browser to
  `redirectUrl` (they come back to the same UI URL).
* `GET /v1/shop/orders/{id}/payment?key=` → `{"status":"paid|pending|failed","order":Order}`
  (asks the provider, idempotent: marks the order paid → processing if it is).
* `POST /v1/shop/orders/{id}/pay?key=` → a new `payment` object for a still-pending order (retry).
* Provider callbacks: `POST /v1/payments/stripe/webhook` (Stripe-Signature), `GET /v1/payments/konnect/webhook?payment_ref=`,
  `POST|GET /v1/payments/flouci/webhook` (X-Flouci-Signature).

### Invoices (Markdown / Go template → PDF, VAT or timbre fiscal)

* `GET /v1/admin/invoicing/settings` / `PUT` (manager) — `InvoiceSettings`:
  ```json
  {"enabled":true,"autoGenerate":"none|processing|completed","numberPrefix":"FAC-","nextNumber":1,"numberPadding":5,
   "resetYearly":true,"country":"FR","locale":"fr","taxMode":"vat|stamp|vat_stamp|none","vatLabel":"TVA",
   "defaultVatRate":20,"vatExemptionMention":"TVA non applicable, art. 293 B du CGI","stampAmount":0,
   "stampLabel":"Timbre fiscal","stampChargeCustomer":false,
   "seller":{"name":"","address":"","postcode":"","city":"","country":"FR","email":"","phone":"","website":"",
             "registrationLabel":"SIRET","registrationNumber":"","vatNumberLabel":"N° TVA intracommunautaire",
             "vatNumber":"","iban":"","bic":""},
   "legalMentions":"","paymentTerms":"","footer":"","template":""}
  ```
  `template` empty = the default template. `stampChargeCustomer` adds the stamp to the customer's total
  at checkout (a fee line).
* `GET /v1/admin/invoicing/country-defaults` → `[{"country":"TN","name":"Tunisia","taxMode":"vat_stamp","vatLabel":"TVA","defaultVatRate":19,"stampAmount":1,"stampLabel":"Timbre fiscal","currency":"TND","registrationLabel":"Matricule fiscal","vatNumberLabel":"Code TVA","numberPrefix":"FAC-","locale":"fr","vatExemptionMention":"","legalMentions":"..."}]`
  (the decision table, keyed by ISO 3166 code; `*` is the fallback).
* `POST /v1/admin/invoicing/apply-defaults` `{"country":"TN","createTaxRate":true}` → the updated settings
  (and, with `createTaxRate`, a standard WooCommerce tax rate for that country when none exists).
* `GET /v1/admin/invoicing/template` → `{"default":"<markdown template>","variables":[{"name":".Invoice.Number","description":"..."}]}`.
* `POST /v1/admin/invoicing/preview` `{"template":"...optional...","orderId":123}` → `application/pdf`
  (the last order when `orderId` is absent).
* `GET /v1/admin/invoices?search&page&perPage` → paged `Invoice`;
  `Invoice` = `{"id","number","orderId","issuedAt","customer":{"name","email"},"currency","totalHT","totalTax","stamp","totalTTC","taxes":[{"label","rate","base","amount"}],"lines":[...],"status":"issued"}`.
* `POST /v1/admin/orders/{id}/invoice` → 201 `Invoice` (200 with the existing one: one invoice per order).
* `GET /v1/admin/orders/{id}/invoice` → `Invoice` or 404.
* `GET /v1/admin/invoices/{id}/pdf` (attachment `facture-FAC-2026-00001.pdf`), `GET /v1/admin/invoices/{id}/markdown`.
* `POST /v1/admin/invoices/{id}/send` → emails the PDF to the customer `{"sent":true}`.
* `GET /v1/users/me/orders/{id}/invoice` → the customer's invoice PDF (404 when none).
* Cart and order JSON: `fees: [{"name":"Timbre fiscal","total":1,"totalTax":0}]` and `feeTotal`
  (cart) / `feeLines` (order).

**Country decision table administration** (manager): the rows behind "apply country defaults",
pre-filled with cwclock's countries (AE, AU, BE, DE, DZ, EG, FR, GB, SA, TN, US) plus CA, CH, CI, CM, ES,
IT, LU, MA, NL, PT, SN and the `*` fallback.

* `GET /v1/admin/invoicing/country-defaults` → the list (each row has `country` + the fields above).
* `GET /v1/admin/invoicing/country-defaults/{iso}` → one row (falls back to `*` with `"fallback": true`).
* `PUT /v1/admin/invoicing/country-defaults/{iso}` → create or replace a row (body = the row without
  `country`); `iso` is an ISO 3166-1 alpha-2 code or `*`.
* `DELETE /v1/admin/invoicing/country-defaults/{iso}` → remove a row (`*` cannot be deleted).

### Phone confirmation when no payment partner is configured

When no online payment partner (Stripe, Konnect, Flouci) is enabled and configured for the shop currency:

* `GET /v1/shop/settings` answers `"phoneConfirmation": true`.
* `POST /v1/shop/checkout` **requires** `billing.phone` (8 to 15 digits; `+ . - ( )` and spaces allowed) —
  else 400 `errors.phoneRequired`. `paymentMethod` may be empty (no offline gateway needed); the order
  is placed **on hold** with the note "To confirm by phone with the customer at …", and its payment
  method title is "Confirmation by phone" (French shops: "Confirmation par téléphone").
* The 201 answer adds `"confirmation": {"orderNumber":"123","phone":"+216 20 000 000","message":"Your order number is 123. We will call you at … to confirm it."}`.

### Languages

* `GET/PUT /v1/admin/shop-config` also carries `"languages": ["en","fr","ar"]` (the languages the shop
  offers, at least one; supported: `en`, `fr`, `ar`) and `"defaultLanguage": "fr"` (one of them) —
  400 `errors.unsupportedLanguage`, `errors.languageRequired`, `errors.invalidDefaultLanguage`.
* `GET /v1/shop/settings` exposes `languages` and `defaultLanguage`; the UI only offers those.
* Customer emails follow WordPress' `WPLANG` when set, else the shop's default language (Arabic emails are
  right-to-left). PDF exports are never Arabic: invoices are English or French (`locale` en/fr), the
  accounting exports French.

### Permalinks (same URLs as WordPress / WooCommerce)

Product, category, tag and shop pages use WooCommerce's permalinks (option `woocommerce_permalinks`, set in
WordPress → Settings → Permalinks, and the slug of the `woocommerce_shop_page_id` page). Unset bases fall
back to WooCommerce's localized defaults (`product`/`product-category`/`product-tag`/`shop`, French:
`produit`/`categorie-produit`/`etiquette-produit`/`boutique`).

* `GET /v1/shop/settings` carries `"permalinks": {"productBase":"produit","categoryBase":"categorie-produit","tagBase":"etiquette-produit","attributeBase":"","shopBase":"boutique","productCategoryInPath":false}`.
* Products carry `path` (`/produit/bouquet-de-roses`; with `%product_cat%` in the base, the full path of the
  product's category with the lowest id like WooCommerce, `uncategorized` when none) and `permalink`
  (site URL + path). Categories and tags carry `path` (`/categorie-produit/parent/child`).
* `GET /v1/shop/categories/{slug}` also accepts a hierarchical `parent/child` path.
* `GET /v1/seo/html?path=…` resolves any of those paths (and the sitemap lists them).
* `GET /v1/admin/permalinks` / `PUT /v1/admin/permalinks` `{"productBase":"boutique/%product_cat%","categoryBase":"rayon","tagBase":"mot-cle","attributeBase":""}`
  (manager): writes `woocommerce_permalinks` like WooCommerce and deletes WordPress' `rewrite_rules` option so
  WordPress rebuilds its rules. Bases are required and must differ (400 `errors.invalidPermalinks`).

### CWCloud emails and contact form (stored in WordPress)

* `GET /v1/admin/cwcloud-settings` (manager) →
  `{"apiUrl","apiKeySet","apiKeySource":"wordpress|env|","contactFormId","contactRecipient","recipientSource":"contact-form-7|option|admin_email","contactSubject","contactForm7Id","contactForm7","emailFrom","contactMode":"contactreq|email|disabled"}`.
  The key is never returned.
* `PUT /v1/admin/cwcloud-settings` `{"apiUrl","apiKey","clearApiKey","contactFormId","contactRecipient","contactSubject","emailFrom"}`
  (all optional; a blank `apiKey` keeps the stored one). The key goes to WordPress' `cwcloud-api-secret` option,
  the one of the CWCloud WordPress plugin (`cwcloud-api-url`, `cwcloud-contact-form-id` next to it); the
  WordPress options win over `CWCLOUD_API_*` / `CWCLOUD_CONTACT_FORM_ID`. The recipient and subject are saved in
  the Contact Form 7 form (`wpcf7_contact_form` post, `_mail` meta) when there is one, else in the options
  `cwmerch_contact_recipient` / `cwmerch_contact_subject`; default recipient: WordPress' `admin_email`.
  `emailFrom` is `woocommerce_email_from_address`.
* `POST /v1/admin/cwcloud-settings/test-email` `{"to":"me@example.com"}` → `{"sent":true|false}`.
* `POST /v1/contact`: with a CWCloud contact form id, forwarded to it (`contactreq`); without one but with
  an API key, emailed to the recipient with the visitor as reply-to (`email`); else 405
  `errors.contactFormNotConfigured`. `GET /v1/shop/settings` `contactEnabled` follows.

## ai-instruct-3: PostgreSQL and clone data

### Databases

`DB_TYPE` = `mysql` (`mariadb`, `my`) or `postgres` (`postgresql`, `pgsql`, `pg`, `psql`); `DB_HOST`, `DB_PORT`
(default 3306 / 5432), `DB_NAME`, `DB_USER`, `DB_PASSWORD`, `DB_TABLE_PREFIX`, `DB_SSLMODE`, `DB_POOL_SIZE`
(the `MYSQL_*` / `WP_TABLE_PREFIX` names remain fallbacks). The API contract is identical on both engines.

### Clone data (role `superuser`)

* `GET /v1/admin/clone` → `{"main": Info, "job": Job|null}` — `Info` =
  `{"type":"mysql|postgres","version","host","name","prefix","migration":"3","tables","rows","products","orders","customers"}`.
* `POST /v1/admin/clone/test` `{"direction":"export|import","remote":{"type","host","port","name","user","password","prefix","sslMode"}}`
  → `{"main": Info, "remote": Info, "ready": true, "issues": [...], "missing": ["table", ...]}` (read-only).
  `ready` needs a migrated destination (`cwmerch_settings` present, Flyway history at least the source's version).
* `POST /v1/admin/clone` same body + `"conflict":"overwrite|skip"` (default overwrite) + `"confirm":true` → 202 `Job`:
  `{"id","direction","conflict","status":"running|done|failed","source":Info,"target":Info,"copied","total","error",
  "startedAt","finishedAt","startedBy","tables":[{"table","sourceRows","copied","replaced","beforeRows","afterRows","status":"pending|running|done|failed","error","durationMs"}]}`.
  Audited as `database.cloned`.
* `GET /v1/admin/clone/job` → the running/last `Job` (404 `errors.cloneNotFound` if none).
* Errors: 400 `errors.cloneConfirmRequired`, `errors.cloneInvalidDirection`, `errors.cloneSameDatabase`,
  `errors.cloneConnection` (the remote cannot be reached; the message says why), 409 `errors.cloneRunning`,
  `errors.cloneNotReady` (migrations).
* `export`: main → remote; `import`: remote → main (the API drops its caches afterwards). `overwrite`: upsert by
  primary key, destination rows clashing on another unique key are deleted, child rows (postmeta, usermeta,
  commentmeta, termmeta, term_relationships, wc_orders_meta, wc_order_addresses, order items and their meta,
  tax rate locations) of the parents the source has are replaced; `skip`: only missing rows are inserted.

### AI assistant

The `cwmerch_api` tool runs read-only POSTs without a confirmation: `/v1/shop/cart`, `/v1/admin/clone/test`,
`/v1/admin/payment-providers/{id}/test`. The skill describes the clone flow (test → summary → explicit yes →
start → poll).

