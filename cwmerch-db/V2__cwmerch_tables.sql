-- cwmerch's own tables, next to WordPress' and WooCommerce's.
--
-- Everything cwmerch adds lives in `${table_prefix}cwmerch_*` tables, so the
-- WordPress/WooCommerce tables keep exactly the shape WooCommerce expects and a
-- WordPress install sharing the database never sees a column it does not know.
-- Users are WordPress users (`${table_prefix}users`); these tables only hang
-- cwmerch's extras off their ID (MFA, account status, API keys, AI...).
--
-- Free-form payloads are LONGTEXT holding JSON (not the JSON type) so the same
-- file runs on MySQL 5.7+/8.x and MariaDB 10.3+ alike.
--
-- Applied migrations are immutable: Flyway checksums this file.

-- cwmerch's extras on a WordPress user: account status (disabled / ban),
-- MFA (TOTP secret, enabled flag) and the OIDC subjects linked to the account.
CREATE TABLE IF NOT EXISTS ${table_prefix}cwmerch_users (
    user_id    BIGINT UNSIGNED NOT NULL,
    data       LONGTEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

-- Long-term API keys (X-Api-Key). Only the SHA-256 of the token is stored.
CREATE TABLE IF NOT EXISTS ${table_prefix}cwmerch_api_keys (
    id          CHAR(36) NOT NULL,
    user_id     BIGINT UNSIGNED NOT NULL,
    key_hash    CHAR(64) NOT NULL,
    description VARCHAR(255) NOT NULL DEFAULT '',
    expires_at  DATETIME NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY cwmerch_api_keys_hash (key_hash),
    KEY cwmerch_api_keys_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

-- WebAuthn credentials (YubiKey and other security keys) used as a second factor.
CREATE TABLE IF NOT EXISTS ${table_prefix}cwmerch_webauthn_credentials (
    id            CHAR(36) NOT NULL,
    user_id       BIGINT UNSIGNED NOT NULL,
    credential_id VARBINARY(1023) NOT NULL,
    public_key    BLOB NOT NULL,
    sign_count    BIGINT UNSIGNED NOT NULL DEFAULT 0,
    transports    TEXT NOT NULL,
    name          VARCHAR(255) NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY cwmerch_webauthn_credential (credential_id),
    KEY cwmerch_webauthn_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

-- Append-only audit trail of what users did (also logged, counted and sent to Discord).
CREATE TABLE IF NOT EXISTS ${table_prefix}cwmerch_audit_log (
    id           CHAR(36) NOT NULL,
    user_id      BIGINT UNSIGNED NULL,
    action       VARCHAR(64) NOT NULL,
    subject_type VARCHAR(32) NOT NULL,
    subject_id   VARCHAR(255) NOT NULL DEFAULT '',
    data         LONGTEXT NOT NULL,
    created_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY cwmerch_audit_created (created_at),
    KEY cwmerch_audit_subject (subject_type, subject_id),
    KEY cwmerch_audit_user (user_id),
    KEY cwmerch_audit_action (action)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

-- Addresses seen per user (user_id 0 = anonymous visitor), with their geo lookup.
CREATE TABLE IF NOT EXISTS ${table_prefix}cwmerch_sessions (
    id         CHAR(36) NOT NULL,
    user_id    BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ip         VARCHAR(64) NOT NULL,
    data       LONGTEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY cwmerch_sessions_ip_user (ip, user_id),
    KEY cwmerch_sessions_user (user_id, updated_at),
    KEY cwmerch_sessions_ip (ip)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

CREATE TABLE IF NOT EXISTS ${table_prefix}cwmerch_ip_bans (
    id         CHAR(36) NOT NULL,
    ip         VARCHAR(64) NOT NULL,
    data       LONGTEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY cwmerch_ip_bans_ip (ip)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

-- The AI assistant: the provider catalogue (data, not code), a user's
-- subscriptions (connections), who they are shared with, and conversations.
CREATE TABLE IF NOT EXISTS ${table_prefix}cwmerch_ai_providers (
    id            CHAR(36) NOT NULL,
    name          VARCHAR(64) NOT NULL,
    format        VARCHAR(32) NOT NULL,
    base_url      VARCHAR(512) NOT NULL,
    default_model VARCHAR(255) NOT NULL,
    data          LONGTEXT NOT NULL,
    position      INT NOT NULL DEFAULT 0,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY cwmerch_ai_providers_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

INSERT IGNORE INTO ${table_prefix}cwmerch_ai_providers (id, name, format, base_url, default_model, position, data) VALUES
    ('7c1d0e4e-2f7b-4b44-9f0a-000000000001', 'openai', 'openai', 'https://api.openai.com/v1', 'gpt-4o-mini', 1,
        '{"label":"OpenAI","keyHint":"sk-...","models":["gpt-4.1","gpt-4.1-mini","gpt-4o","gpt-4o-mini","o4-mini","o3"]}'),
    ('7c1d0e4e-2f7b-4b44-9f0a-000000000002', 'mistral', 'openai', 'https://api.mistral.ai/v1', 'mistral-small-latest', 2,
        '{"label":"Mistral","models":["mistral-large-latest","mistral-medium-latest","mistral-small-latest","magistral-medium-latest","open-mistral-nemo","codestral-latest"]}'),
    ('7c1d0e4e-2f7b-4b44-9f0a-000000000003', 'anthropic', 'anthropic', 'https://api.anthropic.com/v1', 'claude-sonnet-5-5', 3,
        '{"label":"Anthropic (Claude)","keyHint":"sk-ant-...","models":["claude-opus-5-5","claude-sonnet-5-5","claude-haiku-5-5","claude-sonnet-4-5","claude-haiku-4-5"]}'),
    ('7c1d0e4e-2f7b-4b44-9f0a-000000000004', 'gemini', 'gemini', 'https://generativelanguage.googleapis.com/v1beta', 'gemini-2.5-flash', 4,
        '{"label":"Google Gemini","keyHint":"AIza...","models":["gemini-2.5-pro","gemini-2.5-flash","gemini-2.0-flash"]}'),
    ('7c1d0e4e-2f7b-4b44-9f0a-000000000005', 'openrouter', 'openai', 'https://openrouter.ai/api/v1', 'meta-llama/llama-3.3-70b-instruct', 5,
        '{"label":"OpenRouter","keyHint":"sk-or-...","models":["meta-llama/llama-3.3-70b-instruct","anthropic/claude-sonnet-4.5","openai/gpt-4o-mini","google/gemini-2.5-flash","deepseek/deepseek-chat","qwen/qwen-2.5-72b-instruct"]}'),
    ('7c1d0e4e-2f7b-4b44-9f0a-000000000006', 'deepseek', 'openai', 'https://api.deepseek.com/v1', 'deepseek-chat', 6,
        '{"label":"DeepSeek","models":["deepseek-chat","deepseek-reasoner"]}'),
    ('7c1d0e4e-2f7b-4b44-9f0a-000000000007', 'cwcloud', 'cwcloud', 'https://api.cwcloud.tech/v1/ai/prompt', 'claude-sonnet', 7,
        '{"label":"CWCloud (CWAI)","keyHint":"the CWCloud API token","models":["mistral-medium","claude-sonnet","claude-haiku","gemini-flash"]}');

CREATE TABLE IF NOT EXISTS ${table_prefix}cwmerch_ai_connections (
    id         CHAR(36) NOT NULL,
    owner_id   BIGINT UNSIGNED NOT NULL,
    data       LONGTEXT NOT NULL,
    position   INT NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY cwmerch_ai_connections_owner (owner_id, position)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

CREATE TABLE IF NOT EXISTS ${table_prefix}cwmerch_ai_connection_shares (
    id            CHAR(36) NOT NULL,
    connection_id CHAR(36) NOT NULL,
    user_id       BIGINT UNSIGNED NOT NULL,
    role          VARCHAR(16) NOT NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY cwmerch_ai_shares_connection_user (connection_id, user_id),
    KEY cwmerch_ai_shares_user (user_id),
    CONSTRAINT cwmerch_ai_shares_connection FOREIGN KEY (connection_id)
        REFERENCES ${table_prefix}cwmerch_ai_connections (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

CREATE TABLE IF NOT EXISTS ${table_prefix}cwmerch_ai_conversations (
    id         CHAR(36) NOT NULL,
    user_id    BIGINT UNSIGNED NOT NULL,
    data       LONGTEXT NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY cwmerch_ai_conversations_user (user_id, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

CREATE TABLE IF NOT EXISTS ${table_prefix}cwmerch_ai_prompts (
    id              CHAR(36) NOT NULL,
    conversation_id CHAR(36) NOT NULL,
    question        LONGTEXT NOT NULL,
    answer          LONGTEXT NOT NULL,
    data            LONGTEXT NOT NULL,
    created_at      DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY cwmerch_ai_prompts_conversation (conversation_id, created_at),
    CONSTRAINT cwmerch_ai_prompts_conversation FOREIGN KEY (conversation_id)
        REFERENCES ${table_prefix}cwmerch_ai_conversations (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

-- cwmerch's own settings, one JSON document per name: `appearance` (logo, home
-- banner slides, theme colours, fonts, custom CSS...), `shop` (public shop on/off),
-- `payments` (online payment providers; secrets sealed), `invoicing`.
-- WooCommerce's settings stay in `${table_prefix}options`, where WooCommerce reads them.
CREATE TABLE IF NOT EXISTS ${table_prefix}cwmerch_settings (
    name       VARCHAR(191) NOT NULL,
    data       JSON NOT NULL,
    updated_by BIGINT UNSIGNED NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

-- Invoices: one per order, numbered without gaps per (year) sequence. `data` is
-- the frozen snapshot the PDF is rendered from (seller, buyer, lines, taxes,
-- stamp), so a later change of the order or the settings never alters an
-- issued invoice.
CREATE TABLE IF NOT EXISTS ${table_prefix}cwmerch_invoices (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    order_id   BIGINT UNSIGNED NOT NULL,
    number     VARCHAR(64) NOT NULL,
    data       JSON NOT NULL,
    issued_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY cwmerch_invoices_order (order_id),
    UNIQUE KEY cwmerch_invoices_number (number),
    KEY cwmerch_invoices_issued (issued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

-- Invoice/tax defaults per country (decision table keyed by ISO 3166-1 alpha-2,
-- '*' = fallback): what "apply country defaults" fills the invoicing settings
-- with. Data, not code: add a country with a row.
CREATE TABLE IF NOT EXISTS ${table_prefix}cwmerch_country_defaults (
    country    CHAR(2) NOT NULL,
    data       JSON NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (country)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

INSERT IGNORE INTO ${table_prefix}cwmerch_country_defaults (country, data) VALUES
    ('*',  '{"name":"Other countries","taxMode":"vat","vatLabel":"VAT","defaultVatRate":0,"stampAmount":0,"stampLabel":"","currency":"","registrationLabel":"Company number","vatNumberLabel":"VAT number","numberPrefix":"INV-","locale":"en","vatExemptionMention":"","legalMentions":""}'),
    ('FR', '{"name":"France","taxMode":"vat","vatLabel":"TVA","defaultVatRate":20,"stampAmount":0,"stampLabel":"","currency":"EUR","registrationLabel":"SIRET","vatNumberLabel":"N° TVA intracommunautaire","numberPrefix":"FAC-","locale":"fr","vatExemptionMention":"TVA non applicable, art. 293 B du CGI","legalMentions":"En cas de retard de paiement, une pénalité égale à 3 fois le taux d''intérêt légal et une indemnité forfaitaire pour frais de recouvrement de 40 € sont exigibles (art. L441-10 du Code de commerce). Pas d''escompte pour paiement anticipé."}'),
    ('BE', '{"name":"Belgique","taxMode":"vat","vatLabel":"TVA","defaultVatRate":21,"stampAmount":0,"stampLabel":"","currency":"EUR","registrationLabel":"N° d''entreprise (BCE)","vatNumberLabel":"N° TVA","numberPrefix":"FAC-","locale":"fr","vatExemptionMention":"Régime de la franchise de taxe - TVA non applicable","legalMentions":""}'),
    ('LU', '{"name":"Luxembourg","taxMode":"vat","vatLabel":"TVA","defaultVatRate":17,"stampAmount":0,"stampLabel":"","currency":"EUR","registrationLabel":"RCS","vatNumberLabel":"N° TVA","numberPrefix":"FAC-","locale":"fr","vatExemptionMention":"","legalMentions":""}'),
    ('CH', '{"name":"Suisse","taxMode":"vat","vatLabel":"TVA","defaultVatRate":8.1,"stampAmount":0,"stampLabel":"","currency":"CHF","registrationLabel":"IDE","vatNumberLabel":"N° TVA","numberPrefix":"FAC-","locale":"fr","vatExemptionMention":"","legalMentions":""}'),
    ('DE', '{"name":"Deutschland","taxMode":"vat","vatLabel":"MwSt.","defaultVatRate":19,"stampAmount":0,"stampLabel":"","currency":"EUR","registrationLabel":"Handelsregister","vatNumberLabel":"USt-IdNr.","numberPrefix":"RE-","locale":"en","vatExemptionMention":"Gemäß § 19 UStG wird keine Umsatzsteuer berechnet.","legalMentions":""}'),
    ('ES', '{"name":"España","taxMode":"vat","vatLabel":"IVA","defaultVatRate":21,"stampAmount":0,"stampLabel":"","currency":"EUR","registrationLabel":"NIF","vatNumberLabel":"NIF-IVA","numberPrefix":"FAC-","locale":"en","vatExemptionMention":"","legalMentions":""}'),
    ('IT', '{"name":"Italia","taxMode":"vat_stamp","vatLabel":"IVA","defaultVatRate":22,"stampAmount":2,"stampLabel":"Imposta di bollo","currency":"EUR","registrationLabel":"Codice fiscale","vatNumberLabel":"Partita IVA","numberPrefix":"FT-","locale":"en","vatExemptionMention":"","legalMentions":"Imposta di bollo assolta sulle fatture esenti di importo superiore a 77,47 euro."}'),
    ('NL', '{"name":"Nederland","taxMode":"vat","vatLabel":"BTW","defaultVatRate":21,"stampAmount":0,"stampLabel":"","currency":"EUR","registrationLabel":"KvK","vatNumberLabel":"BTW-nummer","numberPrefix":"F-","locale":"en","vatExemptionMention":"","legalMentions":""}'),
    ('PT', '{"name":"Portugal","taxMode":"vat","vatLabel":"IVA","defaultVatRate":23,"stampAmount":0,"stampLabel":"","currency":"EUR","registrationLabel":"NIPC","vatNumberLabel":"NIF","numberPrefix":"FT-","locale":"en","vatExemptionMention":"","legalMentions":""}'),
    ('GB', '{"name":"United Kingdom","taxMode":"vat","vatLabel":"VAT","defaultVatRate":20,"stampAmount":0,"stampLabel":"","currency":"GBP","registrationLabel":"Company number","vatNumberLabel":"VAT number","numberPrefix":"INV-","locale":"en","vatExemptionMention":"","legalMentions":""}'),
    ('US', '{"name":"United States","taxMode":"none","vatLabel":"Sales tax","defaultVatRate":0,"stampAmount":0,"stampLabel":"","currency":"USD","registrationLabel":"EIN","vatNumberLabel":"Sales tax permit","numberPrefix":"INV-","locale":"en","vatExemptionMention":"","legalMentions":""}'),
    ('CA', '{"name":"Canada","taxMode":"vat","vatLabel":"GST/HST","defaultVatRate":5,"stampAmount":0,"stampLabel":"","currency":"CAD","registrationLabel":"Business number","vatNumberLabel":"GST/HST number","numberPrefix":"INV-","locale":"en","vatExemptionMention":"","legalMentions":""}'),
    ('TN', '{"name":"Tunisie","taxMode":"vat_stamp","vatLabel":"TVA","defaultVatRate":19,"stampAmount":1,"stampLabel":"Timbre fiscal","currency":"TND","registrationLabel":"Matricule fiscal","vatNumberLabel":"Code TVA","numberPrefix":"FAC-","locale":"fr","vatExemptionMention":"","legalMentions":"Timbre fiscal de 1,000 TND (loi de finances)."}'),
    ('MA', '{"name":"Maroc","taxMode":"vat","vatLabel":"TVA","defaultVatRate":20,"stampAmount":0,"stampLabel":"Droit de timbre","currency":"MAD","registrationLabel":"ICE","vatNumberLabel":"IF","numberPrefix":"FAC-","locale":"fr","vatExemptionMention":"","legalMentions":""}'),
    ('DZ', '{"name":"Algérie","taxMode":"vat_stamp","vatLabel":"TVA","defaultVatRate":19,"stampAmount":0,"stampLabel":"Droit de timbre","currency":"DZD","registrationLabel":"RC","vatNumberLabel":"NIF","numberPrefix":"FAC-","locale":"fr","vatExemptionMention":"","legalMentions":"Droit de timbre de 1 % applicable aux règlements en espèces."}'),
    ('SN', '{"name":"Sénégal","taxMode":"vat","vatLabel":"TVA","defaultVatRate":18,"stampAmount":0,"stampLabel":"","currency":"XOF","registrationLabel":"NINEA","vatNumberLabel":"N° TVA","numberPrefix":"FAC-","locale":"fr","vatExemptionMention":"","legalMentions":""}'),
    ('CI', '{"name":"Côte d''Ivoire","taxMode":"vat_stamp","vatLabel":"TVA","defaultVatRate":18,"stampAmount":100,"stampLabel":"Timbre","currency":"XOF","registrationLabel":"RCCM","vatNumberLabel":"N° CC","numberPrefix":"FAC-","locale":"fr","vatExemptionMention":"","legalMentions":""}'),
    ('AE', '{"name":"United Arab Emirates","taxMode":"vat","vatLabel":"VAT","defaultVatRate":5,"stampAmount":0,"stampLabel":"","currency":"AED","registrationLabel":"Trade licence","vatNumberLabel":"TRN","numberPrefix":"INV-","locale":"en","vatExemptionMention":"","legalMentions":""}'),
    ('AU', '{"name":"Australia","taxMode":"vat","vatLabel":"GST","defaultVatRate":10,"stampAmount":0,"stampLabel":"","currency":"AUD","registrationLabel":"ACN","vatNumberLabel":"ABN","numberPrefix":"INV-","locale":"en","vatExemptionMention":"","legalMentions":"Tax invoice."}'),
    ('EG', '{"name":"Egypt","taxMode":"vat","vatLabel":"VAT","defaultVatRate":14,"stampAmount":0,"stampLabel":"Stamp duty","currency":"EGP","registrationLabel":"Commercial register","vatNumberLabel":"Tax registration number","numberPrefix":"INV-","locale":"en","vatExemptionMention":"","legalMentions":""}'),
    ('SA', '{"name":"Saudi Arabia","taxMode":"vat","vatLabel":"VAT","defaultVatRate":15,"stampAmount":0,"stampLabel":"","currency":"SAR","registrationLabel":"CR number","vatNumberLabel":"VAT registration number","numberPrefix":"INV-","locale":"en","vatExemptionMention":"","legalMentions":"Tax invoice."}'),
    ('CM', '{"name":"Cameroun","taxMode":"vat","vatLabel":"TVA","defaultVatRate":19.25,"stampAmount":0,"stampLabel":"","currency":"XAF","registrationLabel":"RCCM","vatNumberLabel":"NIU","numberPrefix":"FAC-","locale":"fr","vatExemptionMention":"","legalMentions":""}');
