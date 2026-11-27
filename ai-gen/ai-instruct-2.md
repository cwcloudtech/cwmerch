# ai-instruct-2

## Project identity

Rename everything (no migration has been played for now) to `cwmerch` new project's name.

The new logo is [here](./assets/logo.jpeg) : do a transparent background png or svg with clean borders please.

## Settings / online payment

The public shop for user can be enabled/disabled through administration (goocommerce can be used only as an internal information system).

Also I want you to allow Stripe configuration to trigger payment intent and receive payment's response from Stripe (optional: configurable through administration with credential). Use JSON columns for shop's configurations. You can check on `ee` branch of `~/comwork-cloud-api` how Stripe is used (I'd like to keep adapter pattern because they'll be other payment systems).

I want the same thing with `konnect`, here's their API: https://docs.konnect.network/docs/fr/api-integration/intro

Or flouci: https://docs.flouci.com/introduction (check also on `~/comwork-cloud-api`, I think there's an adapter).

Do invoice generation based on markdown/go template with VAT or timbre fiscal configurable (shop's settings, like `~/cwclock` propose default settings value based on countries with a decision table with iso country's code).
