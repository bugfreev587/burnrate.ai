-- postgres-init.sql
-- burnrate-ai database initialisation
\echo 'Initialising burnrate database...'

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================
-- users
-- Central identity table, synced from Clerk on first sign-in.
-- burnrate_api_key stores the key_id of the user's default
-- gateway API key so the dashboard can display it quickly.
-- ============================================================
CREATE TABLE IF NOT EXISTS users (
  id                TEXT        PRIMARY KEY,           -- Clerk user ID  e.g. user_2lXYZ…
  email             TEXT        NOT NULL UNIQUE,
  name              TEXT,
  role              TEXT        NOT NULL DEFAULT 'viewer',
  status            TEXT        NOT NULL DEFAULT 'active',
  burnrate_api_key  TEXT,                              -- key_id of the user's primary gateway key
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT users_role_check   CHECK (role   IN ('owner','admin','editor','viewer')),
  CONSTRAINT users_status_check CHECK (status IN ('active','suspended','pending'))
);

CREATE INDEX IF NOT EXISTS idx_users_email  ON users (email);
CREATE INDEX IF NOT EXISTS idx_users_status ON users (status);

-- ============================================================
-- api_keys
-- Machine-to-machine keys used by the claude-code agent (or CI)
-- to authenticate with the burnrate gateway.
-- Pattern mirrors kubernetes-cost-monitor/api-server api_keys.
-- ============================================================
CREATE TABLE IF NOT EXISTS api_keys (
  id          BIGSERIAL   PRIMARY KEY,
  user_id     TEXT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  key_id      UUID        NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  label       TEXT        NOT NULL DEFAULT '',
  salt        BYTEA       NOT NULL,
  secret_hash BYTEA       NOT NULL,        -- HMAC-SHA256(pepper ‖ salt ‖ secret)
  scopes      TEXT[]      NOT NULL DEFAULT ARRAY[]::TEXT[],
  revoked     BOOLEAN     NOT NULL DEFAULT FALSE,
  expires_at  TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys (user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_revoked  ON api_keys (key_id) WHERE revoked = FALSE;

-- ============================================================
-- provider_keys
-- Upstream LLM provider API keys (Anthropic, OpenAI …).
-- encrypted_api_key is AES-256-GCM ciphertext produced at the
-- application layer before INSERT; the column stores raw bytes.
-- ============================================================
CREATE TABLE IF NOT EXISTS provider_keys (
  id                BIGSERIAL   PRIMARY KEY,
  user_id           TEXT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  provider          TEXT        NOT NULL,               -- 'anthropic' | 'openai'
  encrypted_api_key BYTEA       NOT NULL,
  label             TEXT        NOT NULL DEFAULT '',
  revoked           BOOLEAN     NOT NULL DEFAULT FALSE,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT provider_keys_provider_check CHECK (provider IN ('anthropic','openai'))
);

CREATE INDEX IF NOT EXISTS idx_provider_keys_user_id ON provider_keys (user_id);

-- ============================================================
-- usage_logs
-- One row per LLM request reported by the claude-code agent.
-- request_id is the unique identifier sent by the agent so
-- duplicate submissions can be detected (UNIQUE constraint).
-- ============================================================
CREATE TABLE IF NOT EXISTS usage_logs (
  id                BIGSERIAL   PRIMARY KEY,
  user_id           TEXT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  provider          TEXT        NOT NULL,               -- 'anthropic' | 'openai'
  model             TEXT        NOT NULL,               -- e.g. 'claude-sonnet-4-6'
  prompt_tokens     BIGINT      NOT NULL DEFAULT 0,
  completion_tokens BIGINT      NOT NULL DEFAULT 0,
  cost              NUMERIC(14,8) NOT NULL DEFAULT 0,   -- USD
  request_id        TEXT        UNIQUE,                 -- idempotency key from agent
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_usage_logs_user_id    ON usage_logs (user_id);
CREATE INDEX IF NOT EXISTS idx_usage_logs_created_at ON usage_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_logs_provider   ON usage_logs (provider, model);

\echo 'burnrate database initialised.'
