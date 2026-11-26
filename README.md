# goocommerce

![goocommerce](img/goocommerce-logo-512.png)

**goocommerce** is an open-source, lighter remake of [WooCommerce](https://woocommerce.com) written in
Go (API) and React (UI). It works **directly on a WordPress/WooCommerce MySQL or MariaDB database** —
the same tables, the same users, the same orders — so it can:

* run **alone** (a fresh install creates the WooCommerce schema with Flyway), or
* run **alongside an existing WooCommerce shop**, sharing its database: what goocommerce changes,
  WooCommerce sees, and the other way round. There is no other link to WordPress (no plugin, no REST
  call), only the database;
* or be used **only as an AI agent for WooCommerce** (plus API keys, MFA and accounting exports),
  without replacing anything: then only the API is needed — see
  [API-only mode](#api-only-mode-an-ai-agent-for-woocommerce).

Public repository: https://gitlab.cwcloud.tech/oss/cwcloud/goocommerce.git — documentation:
https://www.cwcloud.tech/docs/tutorials/goocommerce

## Features

The WooCommerce features, on WooCommerce's own storage:

* **Storefront**: home page (hero banner slider, categories, featured / new / on-sale products), catalogue
  with filters (category, tag, price, on sale) and sorting, product pages (gallery, variations, attributes,
  stock, reviews, related products), cart (coupons, shipping estimate), checkout (billing/shipping, guest or
  account, shipping and payment methods), "order received" page, customer account (orders, downloads,
  addresses, account details).
* **Products**: simple, variable (attributes, variation generation), grouped, external; sale prices and
  schedules, stock management and backorders, downloadable/virtual products, categories, tags, global
  attributes, shipping classes, images (shared `wp-content/uploads`), catalogue visibility, featured,
  upsells and cross-sells, duplicate, trash/restore. `wc_product_meta_lookup` is kept in sync.
* **Orders**: HPOS tables (`wc_orders`…) or legacy `shop_order` posts (and both when WooCommerce's
  compatibility sync is on), statuses with WooCommerce's side effects (stock reduction/restoration, total
  sales, coupon usage, download permissions, paid/completed dates), notes (private or to the customer),
  refunds (with restock), manual orders. The analytics tables (`wc_order_stats`, `wc_order_product_lookup`,
  `wc_customer_lookup`, coupon and tax lookups) are updated so WooCommerce's own reports include them.
* **Pricing**: taxes (classes, rates by country/state/postcode/city, compound, prices entered with or
  without tax, display incl./excl.), shipping zones and methods (flat rate with cost formulas and shipping
  classes, free shipping, local pickup), coupons (percent, fixed cart, fixed product, every restriction),
  offline payment gateways (bank transfer, cheque, cash on delivery).
* **Customer emails**: WooCommerce's transactional emails (new order, processing, on-hold, completed,
  refunded, cancelled, failed, customer note, invoice, new account) honouring the shop's email settings,
  sent through the **CWCloud email API**. WordPress never runs its hooks for orders goocommerce changes,
  so goocommerce replicates them.
* **Back office**: dashboard, orders, products, categories, tags, attributes, coupons, customers, reviews
  moderation, reports (sales, top products/categories, stock, customers), settings (general, products,
  tax, shipping, payments, accounts, emails, advanced), users, audit trail, IP bans.

Plus:

* **Long-term API keys** (`X-Api-Key`).
* **Sign in with Google** (OIDC; GitHub and Keycloak are available too).
* **MFA** with a TOTP app or a **YubiKey** / security key (WebAuthn).
* **AI assistant** with an agentic loop: the model calls goocommerce's own API with the signed-in user's
  JWT (so it can only do what the user may do), guided by embedded Markdown skills, and only writes after
  an explicit confirmation. AI subscriptions (OpenAI, Anthropic, Mistral, Gemini, OpenRouter, DeepSeek,
  CWCloud) are per user and **shareable with other users**.
* **CWCloud AI adapter**: `POST /v1/ai/cwcloud/adapter` lets CWCloud use goocommerce's assistant as a CWAI
  external adapter.
* **Accounting exports** for the accounting department: *chiffre d'affaires* (turnover), *balance* and
  *grand livre* (general ledger) as JSON or PDF — also available to the AI assistant as tools.
* **Discord notification on every user action**, plus errors and emails.
* **OpenTelemetry** logs, metrics and traces (OTLP gRPC/HTTP) and **Prometheus** metrics (`/metrics`).
* **Swagger UI** at the API root `/`, generated from the router (`/openapi.json`) with request payloads
  and query parameters.
* **SEO**: server-rendered `<head>` (title, description, canonical, Open Graph, Twitter cards, JSON-LD
  Product/Breadcrumb/Organization) for the home, shop, category and product pages, `sitemap.xml`,
  `robots.txt`.
* **Appearance**: shop name, logo, favicon, home banner slides, 8 colour themes (classic, garden, rose,
  ocean, sunset, midnight, minimal, luxury), fonts, corner radius, announcement bar, home sections, social
  links and **custom CSS**.
* **Forgotten password** and **contact form** (CWCloud `contactreq`).

## Architecture

| Path | What it is |
|---|---|
| `goocommerce-api/` | Go API (chi, database/sql + MySQL driver). `internal/woo` is WooCommerce's storage, `internal/store` goocommerce's own tables, `internal/handlers` the HTTP layer. |
| `goocommerce-ui/` | React (Vite, plain JavaScript): storefront, customer account, back office and AI assistant. |
| `goocommerce-db/` | Flyway migrations: `V1` the WooCommerce schema, `V2` goocommerce's `goo_*` tables, `V3` WooCommerce's default options and terms. |
| `.docker/nginx/` | The UI's nginx config (SPA, SEO proxying) and entrypoint. |
| `docs/API.md` | Overview of the API (the exhaustive reference is the generated OpenAPI). |
| `ci/` | Delivery: builds and pushes the `goocommerce-api` and `goocommerce-ui` images to the registry. |

## Quick start (Docker Compose, fresh install)

```bash
git clone https://gitlab.cwcloud.tech/oss/cwcloud/goocommerce.git && cd goocommerce
cp .env.example .env        # set JWT_SECRET (openssl rand -base64 48) and the passwords
docker compose up -d --build
```

* Shop: http://localhost:3000 — the **first account registered becomes the administrator**.
* API and Swagger UI: http://localhost:8080 (`/openapi.json`, `/metrics`).

The compose file starts MySQL 8.4, runs the Flyway migrations, then the API and the UI.

## Database

goocommerce uses WordPress' table prefix (`WP_TABLE_PREFIX`, default `wp_`). The migrations use the Flyway
placeholder `${table_prefix}` (`-placeholders.table_prefix=wp_`).

### Fresh install

Nothing to do: the `flyway` service applies `V1__woocommerce_schema.sql` (the WordPress 7.1 + WooCommerce
11.3 schema), `V2__goocommerce_tables.sql` and `V3__woocommerce_defaults.sql`.

### Alongside an existing WooCommerce instance

The WooCommerce tables already exist, so V1 must be **marked as applied** instead of being run:

1. **Back up the database.**
2. Create Flyway's history table with V1 recorded (checksum `1657541756`, the same whatever the prefix):

   ```bash
   mysql -h <host> -u <user> -p <database> < goocommerce-db/init-existing-instance.sql
   ```

   The script creates `flyway_schema_history` exactly as Flyway 10 does, then:

   ```sql
   INSERT INTO `flyway_schema_history`
     (`installed_rank`, `version`, `description`, `type`, `script`, `checksum`,
      `installed_by`, `execution_time`, `success`)
   SELECT 1, '1', 'woocommerce schema', 'SQL', 'V1__woocommerce_schema.sql', 1657541756,
          SUBSTRING_INDEX(USER(), '@', 1), 0, 1
   FROM DUAL
   WHERE NOT EXISTS (SELECT 1 FROM `flyway_schema_history` WHERE `version` = '1');
   ```

3. Point `.env` at the WordPress database (`MYSQL_HOST`, `MYSQL_DATABASE`, `MYSQL_USER`,
   `MYSQL_PASSWORD`, `WP_TABLE_PREFIX`) and run the migrations: V2 only **adds** goocommerce's `goo_*`
   tables, V3 only adds the default options and terms that are **missing** (it never overwrites a value).

   ```bash
   docker compose up flyway
   ```

4. Start goocommerce without the bundled database: `docker compose up -d api ui`.

The MySQL user needs `SELECT, INSERT, UPDATE, DELETE` on the WordPress tables, plus `CREATE, ALTER,
INDEX, REFERENCES` for the migrations. MariaDB works with the same JDBC URL
(`jdbc:mysql://host:3306/db?allowPublicKeyRetrieval=true&useSSL=false`).

**Sharing images**: mount WordPress' `wp-content/uploads` on the API (`GOOCOMMERCE_UPLOADS_DIR`) and set
`GOOCOMMERCE_UPLOADS_URL` to its public URL (e.g. `https://myshop.example/wp-content/uploads`): images
uploaded in goocommerce then appear in WordPress' media library and vice versa.

**Passwords**: customers keep one password for both. goocommerce reads every WordPress hash format and
writes WordPress 6.8+'s (`GOOCOMMERCE_PASSWORD_HASH=bcrypt`); set `phpass` if an older WordPress shares
the database.

See [goocommerce-db/README.md](goocommerce-db/README.md) for the details.

## API-only mode: an AI agent for WooCommerce

You don't have to replace WooCommerce to use goocommerce. With `GOOCOMMERCE_API_ONLY=true`, goocommerce
only serves its API: the **AI assistant** (and the CWCloud adapter), API keys, MFA, Google sign-in,
reports and the **accounting exports**, on your WooCommerce data. WordPress stays the shop and no UI
container is needed.

```bash
# after initialising Flyway's history as above
echo "GOOCOMMERCE_API_ONLY=true" >> .env
docker compose up -d flyway api
```

Sign in with your WordPress administrator or shop manager account (same email or username, same
password), add an AI subscription (`POST /v1/ai/connections/`) and chat (`POST /v1/ai/chat`) — or plug it
into CWCloud as a CWAI external adapter (`POST /v1/ai/cwcloud/adapter` with an `X-Api-Key`).

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_DATABASE`, `MYSQL_USER`, `MYSQL_PASSWORD` | `localhost`, `3306`, `wordpress`, `wordpress`, — | The (WordPress) database |
| `WP_TABLE_PREFIX` | `wp_` | WordPress table prefix |
| `MYSQL_POOL_SIZE` | `10` | Connection pool size |
| `JWT_SECRET` | — (required) | Session token signing key |
| `GOOCOMMERCE_API_URL` / `GOOCOMMERCE_UI_URL` | `http://localhost:8080` / `http://localhost:3000` | Public URLs |
| `GOOCOMMERCE_SHOP_URL` | UI URL | Storefront URL for permalinks, canonical links and the sitemap |
| `GOOCOMMERCE_UI_INTERNAL_URL` | `http://ui` | Where the API fetches the UI's `index.html` for SEO rendering |
| `GOOCOMMERCE_CORS_ENABLED`, `GOOCOMMERCE_CORS_ALLOWED_ORIGINS` | off, `*` | CORS |
| `GOOCOMMERCE_ACTIVATION_MODE` | `auto` | New customer accounts: `auto`, `email` (confirmation link), `admin` (approval) |
| `GOOCOMMERCE_PASSWORD_HASH` | `bcrypt` | `bcrypt` (WordPress ≥ 6.8) or `phpass` |
| `GOOCOMMERCE_UPLOADS_DIR`, `GOOCOMMERCE_UPLOADS_URL` | —, API `/v1/media` | `wp-content/uploads` folder and its public URL |
| `GOOCOMMERCE_MAX_UPLOAD_SIZE` | `10485760` | Max image upload (bytes) |
| `GOOCOMMERCE_API_ONLY` | `false` | API-only mode (no storefront/back-office routes) |
| `GOOCOMMERCE_OIDC_GOOGLE_CLIENT_ID`, `GOOCOMMERCE_OIDC_GOOGLE_CLIENT_SECRET` | — | Google sign-in (redirect URI `<API URL>/v1/oidc/google/callback`) |
| `CWCLOUD_API_URL`, `CWCLOUD_API_KEY` | `https://api.cwcloud.tech`, — | CWCloud email API |
| `CWCLOUD_CONTACT_FORM_ID` | — | CWCloud contact form (contact page) |
| `GOOCOMMERCE_MAIL_FROM` | `noreply@cwcloud.tech` | Default sender (WooCommerce's "From" address wins when set) |
| `DISCORD_TOKEN` | — | Discord webhook `{id}/{token}` (notification on every user action) |
| `GOOCOMMERCE_OTEL_ENDPOINT`, `GOOCOMMERCE_OTEL_PROTO` | —, `otlp/grpc` | OpenTelemetry collector (`otlp/grpc` or `otlp/http`) |
| `GOOCOMMERCE_AI_DOWNLOAD_DIR`, `GOOCOMMERCE_AI_DOWNLOAD_TTL` | `storage`, `30m` | Files generated by the assistant (PDF exports) |

UI container: `GOOCOMMERCE_API_URL` (public API URL for the browser) and `GOOCOMMERCE_API_INTERNAL_URL`
(API URL nginx proxies SEO pages to, default `http://api:8080`).

## Development

```bash
docker compose up -d db flyway
```

```bash
cd goocommerce-api && MYSQL_HOST=127.0.0.1 JWT_SECRET=dev GOOCOMMERCE_CORS_ENABLED=true go run .
```

```bash
cd goocommerce-ui && npx pnpm@latest install && npx pnpm@latest run dev
```

Checks: `go vet ./... && go test ./...` in `goocommerce-api`, `pnpm run lint && pnpm run build` in
`goocommerce-ui`.

## Delivery

`ci/app/deliver.sh` (GitLab CI on `main`) builds the `goocommerce-api` and `goocommerce-ui` images and
pushes them to the registry with the version from `VERSION`, plus `version-sha` and `latest` tags. There
is no deployment step.

## License

[MIT](LICENSE) © 2026 CWCLOUD SASU.
