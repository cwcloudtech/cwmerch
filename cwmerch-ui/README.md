# cwmerch-ui

React storefront + WooCommerce-like admin for **cwmerch** (MIT), a lighter Go
remake of WooCommerce that shares an existing WooCommerce MySQL database. It talks
only to `cwmerch-api` (contract: [`../docs/API.md`](../docs/API.md)).

Vite + React 18 (plain JSX), react-router v6, axios, react-toastify, react-icons,
react-markdown. English and French (`src/i18n/*.json`).

## Develop

```sh
npx pnpm@latest install
npx pnpm@latest run dev       # http://localhost:3000, API expected on http://localhost:8080
npx pnpm@latest run lint
npx pnpm@latest run build     # static files in dist/
npx pnpm@latest run preview
```

## Runtime configuration

The API base URL is **not** baked into the build. `index.html` loads `/config.js`,
which sets `window.__CWMERCH__ = { apiUrl: "..." }` (dev default in
`public/config.js`: `http://localhost:8080`).

In the nginx image, `.docker/nginx/docker-entrypoint.sh` rewrites it at start:

| Variable | Default | Used for |
|---|---|---|
| `CWMERCH_API_URL` | `http://localhost:8080` | public API URL the browser calls (written to `/config.js`) |
| `CWMERCH_API_INTERNAL_URL` | `http://api:8080` | where nginx proxies SEO-rendered pages (`/v1/seo/html`), `/sitemap.xml` and `/robots.txt` |

Copy `.docker/nginx/default.conf` and `.docker/nginx/seo-proxy.inc` to
`/etc/nginx/conf.d/` and use the entrypoint script; the public routes (`/`, `/shop`,
`/product/*`, `/product-category/*`, `/product-tag/*`) are served pre-rendered by
the API for crawlers, falling back to the plain SPA when the API is unavailable.

The storefront look (colours, fonts, logo, banner, home sections) comes from
`GET /v1/shop/appearance` and the API's `/v1/shop/custom.css`; edit it under
Admin › Appearance.
