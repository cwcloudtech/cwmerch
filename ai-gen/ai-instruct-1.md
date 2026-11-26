# ai-instruct-1

Goocommerce is a remake of Woocommerce lighter in go.
Provide exactly the same features plus:
* Long term API keys
* OIDC with Google
* MFA with TOPT app or Yubikey
* AI assistant with agentic loop, tools with the API and the jwt of the connected user and AI subscription shareable with other users + embedded skills in markdown
* Discord notification on every user actions
* OTEL logs, metrics and traces
* Prometheus metrics
* Swagger/OAS page at root endpoint with payload's and query param documentation like the project references below
* Endpoint for CWCloud AI adapters `/v1/ai/cwcloud/adapter` like the project references below

A goocommerce as to be able to work alongside an existing woocommerce instance.

No direct link with woocommerce, only the database, replicate the hooks to send emails to customer like `~/jsa-api`.

goocommerce has to present a public shop like woocommerce and admin part.

## Email sender

Use the CWCloud's email API

## Project references

* `~/jsa-api`: connect to the MySQL database and woocommerce tables
* `~/cwclock`, `~/strong-fish` and `~/shadai`: for all features described earlier

## Technologies

* Golang for the API (`goocommerce-api`)
* React for the UI (`goocommerce-ui`)
* Flywaydb for the db migrations (first migration: reproduce the Woocommerce database schema and in the README provide a SQL script to init the migration table with the first migration with checksum for existing instances)
* Document the product in `cwcloud-website` (public repo: https://gitlab.cwcloud.tech/oss/cwcloud/goocommerce.git) with the installation procedure (especially the database part) in a tutorial and use docker compose in the tutorial. Same in `README.md`

## Licence

Opensource MIT (use same licence as `~/cwclock`).
