-- ============================================================================
-- cwmerch - V4: default VAT rates (WooCommerce standard tax rates)
-- ============================================================================
--
-- Tunisia (TVA 19 %) and France (TVA 20 %), in WooCommerce's own table
-- (woocommerce_tax_rates, "Standard" class, all states/postcodes/cities, on
-- shipping too), the way WooCommerce stores a rate entered in
-- WooCommerce > Settings > Tax > Standard rates ("19.0000").
--
-- They are defaults only: the shop changes or removes them from the back office
-- (Settings > Taxes) depending on what it sells (reduced rates, exemptions...).
-- Taxes are only applied when they are enabled (woocommerce_calc_taxes).
--
-- A country that already has a rate (any class) is left untouched, so this
-- never duplicates nor overrides what an existing WooCommerce shop configured.
--
-- !!! NEVER EDIT THIS FILE ONCE RELEASED !!! (Flyway checksums it)
-- ============================================================================

INSERT INTO `${table_prefix}woocommerce_tax_rates`
  (`tax_rate_country`, `tax_rate_state`, `tax_rate`, `tax_rate_name`, `tax_rate_priority`,
   `tax_rate_compound`, `tax_rate_shipping`, `tax_rate_order`, `tax_rate_class`)
SELECT 'TN', '', '19.0000', 'TVA', 1, 0, 1, 0, ''
FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `${table_prefix}woocommerce_tax_rates` WHERE `tax_rate_country` = 'TN');

INSERT INTO `${table_prefix}woocommerce_tax_rates`
  (`tax_rate_country`, `tax_rate_state`, `tax_rate`, `tax_rate_name`, `tax_rate_priority`,
   `tax_rate_compound`, `tax_rate_shipping`, `tax_rate_order`, `tax_rate_class`)
SELECT 'FR', '', '20.0000', 'TVA', 1, 0, 1, 1, ''
FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `${table_prefix}woocommerce_tax_rates` WHERE `tax_rate_country` = 'FR');

-- WooCommerce caches its tax rate lookups under a "taxes" cache version:
-- dropping it makes a WordPress running alongside see the new rates at once.
DELETE FROM `${table_prefix}options`
WHERE `option_name` IN ('_transient_taxes-transient-version', '_transient_timeout_taxes-transient-version');
