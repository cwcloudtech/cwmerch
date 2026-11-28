# cwmerch API (tool `cwmerch_api`)

All paths start with `/v1/`. Lists answer `{items, total, page, perPage, totalPages}` and take
`?page=1&perPage=20` (max 100). Dates are `YYYY-MM-DD`. Order statuses: `pending`, `processing`,
`on-hold`, `completed`, `cancelled`, `refunded`, `failed`.

Resolve today's date from the account context for relative periods ("this month" = from the
1st of the month to today; "last month"; "this year" = January 1st to today).

## Anyone (customers too)

| Need | Call |
|---|---|
| Shop settings (currency, tax display, payment methods) | `GET /v1/shop/settings` |
| Find products | `GET /v1/shop/products?search=rose&category=bouquets&orderby=popularity|price|price-desc|date|rating` |
| One product (variations, stock, price) | `GET /v1/shop/products/{id or slug}` |
| Categories / tags | `GET /v1/shop/categories`, `GET /v1/shop/tags` |
| Reviews of a product | `GET /v1/shop/products/{id}/reviews` |
| Price a cart / shipping options | `POST /v1/shop/cart` `{"items":[{"productId":12,"variationId":0,"quantity":2}],"coupons":[],"customer":{"country":"FR","postcode":"75001"}}` |
| My orders | `GET /v1/users/me/orders/`, `GET /v1/users/me/orders/{id}` |
| My addresses / downloads | `GET /v1/users/me/addresses`, `GET /v1/users/me/downloads` |

## Shop managers and administrators

**Dashboard & reports**

| Need | Call |
|---|---|
| Overview (sales, top products, low stock, recent orders) | `GET /v1/admin/dashboard?from=&to=` |
| Sales summary (gross, net, orders, items, refunds, taxes, shipping) | `GET /v1/admin/reports/summary?from=&to=` |
| Sales over time | `GET /v1/admin/reports/sales?from=&to=&interval=day|week|month|year` |
| Best sellers / categories | `GET /v1/admin/reports/top-products?from=&to=&limit=10`, `GET /v1/admin/reports/top-categories?...` |
| Stock (low / out of stock) | `GET /v1/admin/reports/stock?type=low|out|all` |
| Best customers | `GET /v1/admin/reports/customers?from=&to=` |

**Accounting (France)** — `?year=2026`, add `&format=pdf` for the PDF (returned as a download link):

| Need | Call |
|---|---|
| Chiffre d'affaires (turnover per month: HT, TVA, shipping, TTC, refunds) | `GET /v1/admin/accounting/turnover?year=` |
| Balance des comptes | `GET /v1/admin/accounting/balance?year=` |
| Grand livre (general ledger, entries per account) | `GET /v1/admin/accounting/ledger?year=` |

**Orders**

| Need | Call |
|---|---|
| List / search | `GET /v1/admin/orders/?status=processing&search=dupont&from=&to=&customer={userId}` |
| Detail | `GET /v1/admin/orders/{id}` · notes `GET /v1/admin/orders/{id}/notes/` |
| Change status (sends the WooCommerce emails, moves stock) | `PUT /v1/admin/orders/{id}` `{"status":"completed"}` — confirm first |
| Edit addresses / transaction id | `PUT /v1/admin/orders/{id}` `{"billing":{...},"shipping":{...},"transactionId":"..."}` — confirm first |
| Add a note (private, or to the customer with an email) | `POST /v1/admin/orders/{id}/notes/` `{"note":"...","customerNote":true}` — confirm first |
| Refund | `POST /v1/admin/orders/{id}/refunds` `{"amount":10,"reason":"...","restock":true,"lineItems":[{"id":lineItemId,"quantity":1,"refundTotal":8.33,"refundTax":1.67}]}` — confirm first |
| Resend an email | `POST /v1/admin/orders/{id}/emails` `{"type":"customer_invoice|customer_processing_order|customer_completed_order|new_order"}` — confirm first |

**Catalogue**

| Need | Call |
|---|---|
| Products (any status) | `GET /v1/admin/products/?search=&status=publish|draft&stockStatus=outofstock&category=` |
| One product | `GET /v1/admin/products/{id}` (variations included) |
| Update price / stock / status | `PUT /v1/admin/products/{id}` `{"regularPrice":"29.90","salePrice":"24.90","manageStock":true,"stockQuantity":12,"status":"publish"}` — confirm first. Prices are strings; an empty `salePrice` ends a sale |
| Create a product | `POST /v1/admin/products/` `{"name":"...","type":"simple","regularPrice":"19","categories":[{"id":3}]}` — confirm first |
| Variation | `PUT /v1/admin/products/{id}/variations/{variationId}` `{"regularPrice":"...","stockQuantity":5}` — confirm first |
| Categories, tags, attributes | `GET /v1/admin/categories/`, `/v1/admin/tags/`, `/v1/admin/attributes/` |

**Customers, coupons, reviews, settings**

| Need | Call |
|---|---|
| Customers | `GET /v1/admin/customers/?search=`, `GET /v1/admin/customers/{id}`, `GET /v1/admin/customers/{id}/orders` |
| Coupons | `GET /v1/admin/coupons/?search=`; create `POST /v1/admin/coupons/` `{"code":"SUMMER10","discountType":"percent","amount":10,"dateExpires":"2026-08-31T23:59:59Z"}` — confirm first |
| Announcements (shop banners) | `GET /v1/admin/announcements/?search=&status=active` ; create `POST /v1/admin/announcements/` `{"title":"Green Friday","content":"<h2>GREEN FRIDAY</h2><p>-15 % with <code>GREEN15</code></p>","placement":"home","expiresAt":"2026-11-29T22:59:00Z"}` ; edit `PUT /v1/admin/announcements/{id}` (full object) ; `DELETE` — confirm first |
| SEO overview (scores per page) | `GET /v1/admin/seo/overview?type=product|product_cat|product_tag|pages&search=` → rows with `seoRating`/`readabilityRating` (good, ok, bad, na), focus keyphrase, SEO title, meta description |
| SEO of one page | `GET /v1/admin/seo/products/{id}` or `GET /v1/admin/seo/terms/{product_cat|product_tag}/{id}` → `meta` (the SEO data), `head` (as shown in Google), `analysis`; or the `cwmerch_seo_analyze` tool |
| Change the SEO of a page | `PUT /v1/admin/seo/products/{id}` (or `/v1/admin/seo/terms/{taxonomy}/{id}`) with the FULL `meta` object read before and your changes: `{"focusKeyphrase":"oak chair","title":"%%title%% %%sep%% %%sitename%%","description":"...","noIndex":"","noFollow":false,"advanced":[],"canonical":"","breadcrumbTitle":"","ogTitle":"","ogDescription":"","ogImage":"","twitterTitle":"","twitterDescription":"","twitterImage":"","cornerstone":false,"primaryCategory":0}` — empty title/description = the content type's template; confirm first |
| SEO settings (templates, separator, social) | `GET /v1/admin/seo/settings`; `PUT /v1/admin/seo/settings` (full object) — confirm first. Template variables: `%%title%%`, `%%sitename%%`, `%%sep%%`, `%%sitedesc%%`, `%%excerpt%%`, `%%primary_category%%`, `%%term_title%%`, `%%pt_plural%%`, `%%currentyear%%`, `%%wc_price%%`, `%%wc_sku%%` |
| Reviews to moderate | `GET /v1/admin/reviews/?status=hold`; `PUT /v1/admin/reviews/{id}` `{"status":"approved"}` — confirm first |
| Shop settings | `GET /v1/admin/settings/` (options + field list); taxes `GET /v1/admin/taxes/`; shipping `GET /v1/admin/shipping/zones/`; payments `GET /v1/admin/payment-gateways/` |

## Tips

* To find an order by a customer's name, search `GET /v1/admin/orders/?search=<name>`.
* "Orders to ship" are usually `status=processing`; "awaiting payment" are `on-hold` (bank
  transfer, cheque) or `pending`.
* A product's `stockQuantity` is null when stock is not managed; read `stockStatus` then.
* For a variable product, the price shown is the cheapest variation (`priceMin`..`priceMax`).

## Payments, invoices, shop switch (managers)

| Need | Call |
|---|---|
| Is the public shop open? | `GET /v1/admin/shop-config` (`{shopEnabled, closedMessage, registrationEnabled}`); change with `PUT` — confirm first |
| Online payment providers (Stripe, Konnect, Flouci) | `GET /v1/admin/payment-providers` (never shows secrets); `POST /v1/admin/payment-providers/{id}/test` to check credentials |
| Re-check an online payment of an order | `GET /v1/shop/orders/{id}/payment?key={orderKey}` (marks the order paid when the provider confirms) |
| Invoices | `GET /v1/admin/invoices/?search=`; one order's: `GET /v1/admin/orders/{id}/invoice`; issue it: `POST /v1/admin/orders/{id}/invoice` — confirm first; PDF: `GET /v1/admin/invoices/{id}/pdf` (download link) |
| Invoicing settings (VAT / timbre fiscal, numbering, seller) | `GET /v1/admin/invoicing/settings`; per-country defaults: `GET /v1/admin/invoicing/country-defaults` |

## Database: PostgreSQL / MySQL, clone data (administrators)

cwmerch runs on MySQL/MariaDB (next to WordPress) or PostgreSQL. "Clone data" copies the whole shop
from one database to another, between the same or different engines.

| Need | Call |
|---|---|
| Which database the shop runs on (engine, version, migration, products/orders/users counts) + the running/last clone | `GET /v1/admin/clone` |
| Check a remote database (connects, compares migrations, lists issues) — read-only | `POST /v1/admin/clone/test` `{"direction":"export|import","remote":{"type":"postgres|mysql|mariadb","host":"...","port":"5432","name":"...","user":"...","password":"...","prefix":"wp_","sslMode":"disable"}}` |
| Start a clone — confirm first | `POST /v1/admin/clone` same body + `"conflict":"overwrite|skip","confirm":true` (202) |
| Follow it | `GET /v1/admin/clone/job` (`status` running → done/failed, per-table `copied`/`replaced`) |

* `export` copies the main database INTO the remote one; `import` copies the remote one INTO the main
  database (the shop runs on the result right away).
* `conflict`: `overwrite` (default: rows with the same id, or clashing on a unique key, are replaced by
  the source's; the metadata of the posts/orders/users the source has are replaced) or `skip` (keep the
  destination's rows, only add the missing ones).
* Always: 1) `POST /v1/admin/clone/test`; 2) summarize it for the user — direction, source → destination
  (engine, host/database, counts), `ready` and the `issues` (the destination must be migrated with
  cwmerch's Flyway migrations first), the conflict mode — and remind them to back up the destination
  and that images (wp-content/uploads) are not in the database; 3) only after their explicit yes, start
  it; 4) poll the job every few seconds and report the result (tables, rows copied, failures).
* Database passwords: suggest entering them in *Administration → Clone data* rather than in the chat.
  If the user gives one here, use it only in the tool call and never repeat it in your answers.
