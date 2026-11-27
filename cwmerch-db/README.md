# cwmerch-db

Database migrations for cwmerch, run by standalone
[Flyway](https://documentation.red-gate.com/fd) (`flyway/flyway:10-alpine`), in two
equivalent sets:

| Folder | Database |
|--------|----------|
| `mysql/` | MySQL/MariaDB: the database cwmerch shares with WordPress/WooCommerce (the reference) |
| `postgresql/` | PostgreSQL (new projects): **generated** from `mysql/` by `ci/tools/mysql2pg.py`, never edited by hand |

`.docker/flyway/migrate.sh` (used by `docker-compose.yml`) picks the folder and the
JDBC URL from `DB_TYPE` (`mysql`, `mariadb`, `my` / `postgres`, `postgresql`, `pgsql`,
`pg`, `psql`) and the other `DB_*` variables, like the API.

## PostgreSQL

The PostgreSQL migrations create the same tables, columns, keys and default rows,
lowercase. The generator maps:

* text columns to `citext` (the `citext` extension, trusted since PostgreSQL 13), so
  comparisons and `LIKE` stay case-insensitive like WordPress' `*_ci` collations;
* `datetime` to `timestamp`, `AUTO_INCREMENT` to identity columns, `JSON` to `jsonb`,
  `BLOB`/`VARBINARY` to `bytea`, unsigned integers to the next wider type;
* MySQL's zero date `'0000-00-00 00:00:00'` to `'0001-01-01 00:00:00'`;
* prefix indexes on long text to `left(col, n)` expression indexes;
* `ON UPDATE CURRENT_TIMESTAMP` to a trigger (`cwm_touch_updated_at()`);
* `INSERT IGNORE` to `ON CONFLICT DO NOTHING`.

V1 also creates `cwm_int()` / `cwm_num()`: MySQL's lenient text-to-number casts,
used by the API (whose queries are written in MySQL's dialect and translated at the
driver level on PostgreSQL). After changing a file in `mysql/`, regenerate:

```sh
python3 ci/tools/mysql2pg.py
```

Tested on PostgreSQL 17.

## Files

| File | What it is |
|------|------------|
| `mysql/V1__woocommerce_schema.sql` | Schema only: the 52 tables of WordPress core + WooCommerce (core, Admin/Analytics, HPOS order tables, Action Scheduler), captured with `mysqldump --no-data` from a fresh WordPress 7.1.5 + WooCommerce 11.3.1 install with HPOS on. `CREATE TABLE IF NOT EXISTS`, InnoDB, `utf8mb4_unicode_520_ci`. |
| `mysql/V2__cwmerch_tables.sql` | cwmerch's own `${table_prefix}cwmerch_*` tables. |
| `mysql/V3__woocommerce_defaults.sql` | Idempotent seed of a fresh install's defaults: core options, the `<prefix>user_roles` roles/capabilities, WooCommerce settings (EUR, HPOS on, BACS/cheque/COD disabled), `product_type` / `product_visibility` / `product_cat` "Uncategorized" terms, and `default_product_cat`. It only inserts missing rows and never overwrites, so it is safe on a populated shop. |
| `mysql/V4__default_vat_rates.sql` | Default VAT rates in WooCommerce's `woocommerce_tax_rates` (Standard class): Tunisia TVA 19 %, France TVA 20 %. Only for a country with no rate yet (never duplicates nor overrides an existing shop's rates); the shop edits them under Settings → Taxes. Applied only when taxes are enabled (`woocommerce_calc_taxes`). |
| `mysql/init-existing-instance.sql` | Run by hand once, to adopt an existing WooCommerce database: creates `flyway_schema_history` and marks V1 as applied. |

Tested on MySQL 8.4 and MariaDB 11.8.

## The `${table_prefix}` placeholder

Every WordPress/WooCommerce/cwmerch table name is written
`${table_prefix}<name>`. Set it to WordPress' `$table_prefix` from `wp-config.php`
(usually `wp_`):

```sh
flyway -placeholders.table_prefix=wp_ ...
```

It also names the roles option (`wp_user_roles` for `wp_`, `shop_user_roles` for
`shop_`), like WordPress. Flyway's own `flyway_schema_history` table is never
prefixed. Flyway replaces `${...}` everywhere in a script, comments included, so
never write a literal dollar-brace in a migration. The PHP-serialized values in
V3 contain none.

## Fresh install (empty database)

Flyway runs V1, V2, V3... in order. With docker compose:

```yaml
services:
  db-migrate:
    image: flyway/flyway:10-alpine
    command: >
      -url=jdbc:mysql://db:3306/wordpress?allowPublicKeyRetrieval=true&useSSL=false
      -user=${DB_USER} -password=${DB_PASSWORD}
      -placeholders.table_prefix=${TABLE_PREFIX:-wp_}
      -locations=filesystem:/flyway/sql
      -connectRetries=30
      migrate
    volumes:
      - ./cwmerch-db/mysql:/flyway/sql:ro
    depends_on: [db]
```

Or without compose:

```sh
docker run --rm -v "$PWD/cwmerch-db/mysql:/flyway/sql:ro" flyway/flyway:10-alpine \
  '-url=jdbc:mysql://HOST:3306/DB?allowPublicKeyRetrieval=true&useSSL=false' \
  -user=USER -password=PASS -placeholders.table_prefix=wp_ \
  -locations=filesystem:/flyway/sql migrate
```

PostgreSQL without compose:

```sh
docker run --rm -v "$PWD/cwmerch-db/postgresql:/flyway/sql:ro" flyway/flyway:10-alpine \
  '-url=jdbc:postgresql://HOST:5432/DB' -user=USER -password=PASS \
  -placeholders.table_prefix=wp_ -locations=filesystem:/flyway/sql migrate
```

* MySQL 8.x: `allowPublicKeyRetrieval=true` is required when the user uses
  `caching_sha2_password` over a non-TLS connection. Add `useSSL=false` if the
  server has no TLS.
* MariaDB: `jdbc:mariadb://HOST:3306/DB` works with no extra flags. The
  `jdbc:mysql://` URL above works too.

After a fresh install, edit the placeholder values V3 seeds (`siteurl`, `home`,
`blogname`, `admin_email`, `woocommerce_email_from_*`, store address). V3
deliberately differs from a stock WooCommerce install in a few values, and only
when they are missing: `woocommerce_currency=EUR`,
`woocommerce_default_country=FR`, `kg`/`cm` units, `timezone_string=UTC`,
`woocommerce_custom_orders_table_data_sync_enabled=no`.

V1 relaxes the session `sql_mode` (removes `NO_ZERO_DATE`, `NO_ZERO_IN_DATE` and
strict mode) for its own statements, like WordPress' `wpdb` does. Action
Scheduler columns default to `'0000-00-00 00:00:00'`, which MySQL 8's default
mode rejects.

## Existing WordPress/WooCommerce instance

The WordPress/WooCommerce tables already exist, so V1 must **not** run. Record it
as applied instead:

1. **Back up the database first**:
   `mysqldump --single-transaction DB > backup.sql`.
2. Check that the shop runs WooCommerce with HPOS (`wc_orders` etc.) and that
   the tables match V1, or at least that every V1 table exists. V1 is never
   executed here, so cwmerch will not create a missing table for you.
3. Connect to the WordPress database and run the adoption script:
   `mysql DB < cwmerch-db/mysql/init-existing-instance.sql`.
   It creates `flyway_schema_history` exactly as Flyway 10 does and inserts row 1:
   version `1`, `V1__woocommerce_schema.sql`, `checksum = 1318371643`,
   `success = 1`. You can safely run it again.
4. Run `flyway ... -placeholders.table_prefix=<your prefix> migrate`. Flyway
   reports `Current version of schema: 1`, then applies V2 and V3. V3 only adds
   what is missing and keeps all your existing settings, roles and terms.
   Expect harmless `Duplicate entry ... (1062)` warnings from its `INSERT IGNORE`s.
5. Run `flyway ... validate`. It must say `Successfully validated N migrations`.

### The V1 checksum

`1318371643` is the CRC32 that Flyway 10 computed for `V1__woocommerce_schema.sql`.
It was read from `flyway_schema_history` after a real migration, not computed by
hand. Flyway checksums the file **before** placeholder replacement, so the value
is the same for every `table_prefix`. It was verified with `wp_` and `shop_` on
MySQL 8.4 and MariaDB 11.8.

## Applied migrations are immutable

Flyway stores each applied script's checksum, and `validate`/`migrate` fail on a
mismatch. Once a `V<n>__*.sql` file is released, never edit it, not even a
comment or whitespace. Put any change in a new, higher-numbered file. This
matters most for V1: its checksum is also hard-coded in
`init-existing-instance.sql`. Keep migrations in version order. With Flyway's
default `outOfOrder=false`, a version lower than the database's current version
(for example a late V2 on a database already at V3) is not applied, and `validate`
reports it as a resolved migration that was never applied.
