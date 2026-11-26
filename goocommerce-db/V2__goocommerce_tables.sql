-- goocommerce's own tables, next to WordPress' and WooCommerce's.
--
-- Everything goocommerce adds lives in `${table_prefix}goo_*` tables, so the
-- WordPress/WooCommerce tables keep exactly the shape WooCommerce expects and a
-- WordPress install sharing the database never sees a column it does not know.
-- Users are WordPress users (`${table_prefix}users`); these tables only hang
-- goocommerce's extras off their ID (MFA, account status, API keys, AI...).
--
-- Free-form payloads are LONGTEXT holding JSON (not the JSON type) so the same
-- file runs on MySQL 5.7+/8.x and MariaDB 10.3+ alike.
--
-- Applied migrations are immutable: Flyway checksums this file.

-- goocommerce's extras on a WordPress user: account status (disabled / ban),
-- MFA (TOTP secret, enabled flag) and the OIDC subjects linked to the account.
CREATE TABLE IF NOT EXISTS ${table_prefix}goo_users (
    user_id    BIGINT UNSIGNED NOT NULL,
    data       LONGTEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

-- Long-term API keys (X-Api-Key). Only the SHA-256 of the token is stored.
CREATE TABLE IF NOT EXISTS ${table_prefix}goo_api_keys (
    id          CHAR(36) NOT NULL,
    user_id     BIGINT UNSIGNED NOT NULL,
    key_hash    CHAR(64) NOT NULL,
    description VARCHAR(255) NOT NULL DEFAULT '',
    expires_at  DATETIME NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY goo_api_keys_hash (key_hash),
    KEY goo_api_keys_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

-- WebAuthn credentials (YubiKey and other security keys) used as a second factor.
CREATE TABLE IF NOT EXISTS ${table_prefix}goo_webauthn_credentials (
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
    UNIQUE KEY goo_webauthn_credential (credential_id),
    KEY goo_webauthn_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

-- Append-only audit trail of what users did (also logged, counted and sent to Discord).
CREATE TABLE IF NOT EXISTS ${table_prefix}goo_audit_log (
    id           CHAR(36) NOT NULL,
    user_id      BIGINT UNSIGNED NULL,
    action       VARCHAR(64) NOT NULL,
    subject_type VARCHAR(32) NOT NULL,
    subject_id   VARCHAR(255) NOT NULL DEFAULT '',
    data         LONGTEXT NOT NULL,
    created_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY goo_audit_created (created_at),
    KEY goo_audit_subject (subject_type, subject_id),
    KEY goo_audit_user (user_id),
    KEY goo_audit_action (action)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

-- Addresses seen per user (user_id 0 = anonymous visitor), with their geo lookup.
CREATE TABLE IF NOT EXISTS ${table_prefix}goo_sessions (
    id         CHAR(36) NOT NULL,
    user_id    BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ip         VARCHAR(64) NOT NULL,
    data       LONGTEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY goo_sessions_ip_user (ip, user_id),
    KEY goo_sessions_user (user_id, updated_at),
    KEY goo_sessions_ip (ip)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

CREATE TABLE IF NOT EXISTS ${table_prefix}goo_ip_bans (
    id         CHAR(36) NOT NULL,
    ip         VARCHAR(64) NOT NULL,
    data       LONGTEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY goo_ip_bans_ip (ip)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

-- The AI assistant: the provider catalogue (data, not code), a user's
-- subscriptions (connections), who they are shared with, and conversations.
CREATE TABLE IF NOT EXISTS ${table_prefix}goo_ai_providers (
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
    UNIQUE KEY goo_ai_providers_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

INSERT IGNORE INTO ${table_prefix}goo_ai_providers (id, name, format, base_url, default_model, position, data) VALUES
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

CREATE TABLE IF NOT EXISTS ${table_prefix}goo_ai_connections (
    id         CHAR(36) NOT NULL,
    owner_id   BIGINT UNSIGNED NOT NULL,
    data       LONGTEXT NOT NULL,
    position   INT NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY goo_ai_connections_owner (owner_id, position)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

CREATE TABLE IF NOT EXISTS ${table_prefix}goo_ai_connection_shares (
    id            CHAR(36) NOT NULL,
    connection_id CHAR(36) NOT NULL,
    user_id       BIGINT UNSIGNED NOT NULL,
    role          VARCHAR(16) NOT NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY goo_ai_shares_connection_user (connection_id, user_id),
    KEY goo_ai_shares_user (user_id),
    CONSTRAINT goo_ai_shares_connection FOREIGN KEY (connection_id)
        REFERENCES ${table_prefix}goo_ai_connections (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

CREATE TABLE IF NOT EXISTS ${table_prefix}goo_ai_conversations (
    id         CHAR(36) NOT NULL,
    user_id    BIGINT UNSIGNED NOT NULL,
    data       LONGTEXT NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY goo_ai_conversations_user (user_id, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

CREATE TABLE IF NOT EXISTS ${table_prefix}goo_ai_prompts (
    id              CHAR(36) NOT NULL,
    conversation_id CHAR(36) NOT NULL,
    question        LONGTEXT NOT NULL,
    answer          LONGTEXT NOT NULL,
    data            LONGTEXT NOT NULL,
    created_at      DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    KEY goo_ai_prompts_conversation (conversation_id, created_at),
    CONSTRAINT goo_ai_prompts_conversation FOREIGN KEY (conversation_id)
        REFERENCES ${table_prefix}goo_ai_conversations (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;

-- goocommerce's own settings (one JSON document per name): the shop's
-- appearance (logo, home banner slides, theme colours, fonts, custom CSS...).
-- WooCommerce's settings stay in `${table_prefix}options`, where WooCommerce reads them.
CREATE TABLE IF NOT EXISTS ${table_prefix}goo_settings (
    name       VARCHAR(191) NOT NULL,
    data       LONGTEXT NOT NULL,
    updated_by BIGINT UNSIGNED NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;
