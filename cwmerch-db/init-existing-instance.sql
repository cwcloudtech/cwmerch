-- ============================================================================
-- cwmerch - adopt an EXISTING WordPress/WooCommerce database
-- ============================================================================
--
-- Run this ONCE, by hand, on a database that already holds the WordPress and
-- WooCommerce tables (BACK IT UP FIRST), connected to that database:
--
--   mysqldump --single-transaction <db> > backup.sql
--   mysql <db> < init-existing-instance.sql
--
-- then let Flyway apply V2+ normally (V1 is skipped as already applied):
--
--   flyway -placeholders.table_prefix=<your WP $table_prefix> migrate
--
-- It (a) creates Flyway's history table exactly as Flyway 10 creates it on
-- MySQL/MariaDB (DDL captured with SHOW CREATE TABLE after a real migration;
-- like Flyway, no explicit charset so it uses the database default), and
-- (b) records V1__woocommerce_schema.sql as successfully applied, with the
-- checksum Flyway itself computed for that file.
--
-- The checksum does NOT depend on the table_prefix placeholder value (Flyway
-- checksums the file before placeholder replacement; verified with wp_ and
-- shop_ on MySQL 8.4 and MariaDB 11.8). It only changes if
-- V1__woocommerce_schema.sql is edited - which must never happen.
--
-- Idempotent: re-running it does nothing if the table / V1 row already exist.
-- No table prefix here: Flyway's history table is never prefixed.
-- ============================================================================

CREATE TABLE IF NOT EXISTS `flyway_schema_history` (
  `installed_rank` int NOT NULL,
  `version` varchar(50) DEFAULT NULL,
  `description` varchar(200) NOT NULL,
  `type` varchar(20) NOT NULL,
  `script` varchar(1000) NOT NULL,
  `checksum` int DEFAULT NULL,
  `installed_by` varchar(100) NOT NULL,
  `installed_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `execution_time` int NOT NULL,
  `success` tinyint(1) NOT NULL,
  PRIMARY KEY (`installed_rank`),
  KEY `flyway_schema_history_s_idx` (`success`)
) ENGINE=InnoDB;

INSERT INTO `flyway_schema_history`
  (`installed_rank`, `version`, `description`, `type`, `script`, `checksum`,
   `installed_by`, `execution_time`, `success`)
SELECT 1, '1', 'woocommerce schema', 'SQL', 'V1__woocommerce_schema.sql', 1318371643,
       SUBSTRING_INDEX(USER(), '@', 1), 0, 1
FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `flyway_schema_history` WHERE `version` = '1');
