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

The API base URL is not baked into the build: `.env.dist` sets
`VITE_API_URL="\${CWMERCH_API_URL}"`, so the bundle keeps the `${CWMERCH_API_URL}`
placeholder. At container start, `.docker/nginx/docker-entrypoint.sh` runs `envsubst`
on the built `.js`, `.css` and `.html` files with every defined environment variable
(like comwork-cloud-ui), and `.docker/nginx/default.conf` serves them.

SEO: the storefront pages (`/`, `/shop`, products, categories, tags, any path that is
neither a file nor an app route), `/sitemap.xml` and `/robots.txt` are loaded from the
API's **public** URL (the same `CWMERCH_API_URL`: `/v1/seo/html?path=…`) and served
under the UI's own address, falling back to the plain SPA shell (`/index.html`, never
proxied) when the API is unreachable. The entrypoint splits `CWMERCH_API_URL` into
origin and base path, and nginx resolves the API's name per request (resolver from
`/etc/resolv.conf`), so it starts even when the API is down.

| Variable | Default | Used for |
|---|---|---|
| `CWMERCH_API_URL` | `http://localhost:8080` | public API URL the browser calls |

Not defined (placeholder left as is) or in development (`pnpm run dev`, no `.env`):
`http://localhost:8080`.

The storefront look (colours, fonts, logo, banner, home sections) comes from
`GET /v1/shop/appearance` and the API's `/v1/shop/custom.css`; edit it under
Admin › Appearance.
