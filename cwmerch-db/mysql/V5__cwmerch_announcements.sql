-- ============================================================================
-- cwmerch - V5: storefront announcements
-- ============================================================================
--
-- Banners shown at the top of the storefront ("GREEN FRIDAY -15 %..."), managed
-- in the back office (Announcements). `data` holds the editable content:
--   {"title": "admin label", "content": "<p>HTML (WYSIWYG)</p>",
--    "placement": "home" | "all", "background": "" (theme colour) | "#rrggbb",
--    "dismissible": true}
-- An announcement is shown while it is enabled and not expired (expires_at,
-- UTC, NULL = no end), ordered by position then newest.
--
-- !!! NEVER EDIT THIS FILE ONCE RELEASED !!! (Flyway checksums it)
-- ============================================================================

CREATE TABLE IF NOT EXISTS ${table_prefix}cwmerch_announcements (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    enabled    TINYINT(1) NOT NULL DEFAULT 1,
    position   INT NOT NULL DEFAULT 0,
    expires_at DATETIME NULL,
    data       JSON NOT NULL,
    created_by BIGINT UNSIGNED NULL,
    updated_by BIGINT UNSIGNED NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY cwmerch_announcements_active (enabled, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;
