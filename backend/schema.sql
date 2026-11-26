-- 股票基础信息
CREATE TABLE IF NOT EXISTS stocks (
  id          SERIAL PRIMARY KEY,
  symbol      VARCHAR(10) NOT NULL,
  name        VARCHAR(100) NOT NULL,
  market      VARCHAR(20) NOT NULL,
  created_at  TIMESTAMP DEFAULT NOW(),
  updated_at  TIMESTAMP DEFAULT NOW(),
  UNIQUE(symbol, market)
);

-- 各维度原始数值（每次刷新覆盖写入）
CREATE TABLE IF NOT EXISTS stock_metrics (
  id                    SERIAL PRIMARY KEY,
  stock_id              INTEGER REFERENCES stocks(id) ON DELETE CASCADE,
  c_eps_growth_pct      DECIMAL(10,2),
  c_data_sufficient     BOOLEAN DEFAULT FALSE,
  a_annual_eps_growth   JSONB,
  a_years_available     INTEGER,
  a_data_sufficient     BOOLEAN DEFAULT FALSE,
  s_volume_ratio        DECIMAL(10,2),
  s_data_sufficient     BOOLEAN DEFAULT FALSE,
  l_rs_rating           INTEGER,
  l_data_sufficient     BOOLEAN DEFAULT FALSE,
  i_institution_delta   INTEGER,
  i_data_sufficient     BOOLEAN DEFAULT FALSE,
  n_label               BOOLEAN,
  n_analysis_at         TIMESTAMP,
  n_analysis_summary    TEXT,
  data_as_of            DATE NOT NULL,
  fetched_at            TIMESTAMP DEFAULT NOW(),
  UNIQUE(stock_id)
);

-- 用户阈值方案
CREATE TABLE IF NOT EXISTS threshold_presets (
  id              SERIAL PRIMARY KEY,
  name            VARCHAR(50) NOT NULL UNIQUE,
  is_default      BOOLEAN DEFAULT FALSE,
  c_min_growth    DECIMAL(5,2) DEFAULT 25.0,
  a_min_growth    DECIMAL(5,2) DEFAULT 25.0,
  a_min_years     INTEGER DEFAULT 3,
  s_volume_ratio  DECIMAL(5,2) DEFAULT 150.0,
  s_ma_period     INTEGER DEFAULT 50,
  l_min_rs        INTEGER DEFAULT 80,
  i_min_delta     INTEGER DEFAULT 1,
  created_at      TIMESTAMP DEFAULT NOW()
);

INSERT INTO threshold_presets (name, is_default)
VALUES ('欧奈尔默认', TRUE)
ON CONFLICT (name) DO UPDATE SET is_default = TRUE;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Runtime migrations in internal/db/postgres.go are canonical. This file is a
-- readable baseline for the user-owned trading context introduced in v2.
CREATE TABLE IF NOT EXISTS users (
  id            UUID PRIMARY KEY,
  created_at    TIMESTAMPTZ DEFAULT NOW(),
  updated_at    TIMESTAMPTZ DEFAULT NOW(),
  display_name  TEXT NOT NULL,
  timezone      TEXT NOT NULL,
  base_currency TEXT NOT NULL,
  status        TEXT NOT NULL
);

INSERT INTO users (id, display_name, timezone, base_currency, status)
VALUES ('00000000-0000-4000-8000-000000000001', 'AlphaLens User', 'Asia/Shanghai', 'USD', 'active')
ON CONFLICT (id) DO NOTHING;

-- 交易日志：逐笔成交，字段与 Vibe-Trading TradeRecord 对齐
CREATE TABLE IF NOT EXISTS trade_journal_entries (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  executed_at TIMESTAMPTZ NOT NULL,
  symbol      TEXT NOT NULL,
  name        TEXT NOT NULL DEFAULT '',
  side        TEXT NOT NULL CHECK (side IN ('buy', 'sell')),
  quantity    NUMERIC(20, 8) NOT NULL,
  price       NUMERIC(20, 8) NOT NULL,
  amount      NUMERIC(24, 8) NOT NULL,
  fee         NUMERIC(20, 8) NOT NULL DEFAULT 0,
  market      TEXT NOT NULL DEFAULT 'other',
  currency    TEXT NOT NULL DEFAULT '',
  order_id    TEXT,
  source      TEXT NOT NULL DEFAULT 'manual',
  notes       TEXT,
  reflection  TEXT
);

CREATE INDEX IF NOT EXISTS idx_trade_journal_created_at ON trade_journal_entries (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_trade_journal_executed_at ON trade_journal_entries (executed_at DESC);
CREATE INDEX IF NOT EXISTS idx_trade_journal_symbol ON trade_journal_entries (symbol);

-- Broker credentials are encrypted by the backend before being written here.
CREATE TABLE IF NOT EXISTS broker_connections (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  broker                TEXT NOT NULL UNIQUE,
  label                 TEXT NOT NULL DEFAULT '',
  credential_ciphertext BYTEA NOT NULL,
  credential_nonce      BYTEA NOT NULL,
  account_name          TEXT NOT NULL DEFAULT '',
  connection_method     TEXT NOT NULL DEFAULT '',
  connection_status     TEXT NOT NULL DEFAULT 'unconnected',
  pending_authorization_session_id UUID,
  authorization_expires_at TIMESTAMPTZ,
  last_synced_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_broker_connections_connection_status ON broker_connections (connection_status);
CREATE INDEX IF NOT EXISTS idx_broker_connections_pending_authorization_session_id ON broker_connections (pending_authorization_session_id);
CREATE INDEX IF NOT EXISTS idx_broker_connections_authorization_expires_at ON broker_connections (authorization_expires_at);

-- LLM API keys are encrypted by the backend and scoped by provider plus the
-- normalized custom gateway host. The plaintext key is never stored here.
CREATE TABLE IF NOT EXISTS llm_credentials (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  provider              TEXT NOT NULL,
  backend_host          TEXT NOT NULL DEFAULT '',
  credential_ciphertext BYTEA NOT NULL,
  credential_nonce      BYTEA NOT NULL,
  UNIQUE (provider, backend_host)
);

-- OAuth state is stored only as a SHA-256 digest; the raw browser value is
-- never written to the database.
CREATE TABLE IF NOT EXISTS broker_authorization_sessions (
  id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  broker                   TEXT NOT NULL,
  state_hash               BYTEA NOT NULL UNIQUE,
  pkce_verifier_ciphertext BYTEA NOT NULL,
  pkce_verifier_nonce      BYTEA NOT NULL,
  expires_at               TIMESTAMPTZ NOT NULL,
  consumed_at              TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_broker_authorization_sessions_broker ON broker_authorization_sessions (broker);
CREATE INDEX IF NOT EXISTS idx_broker_authorization_sessions_expires_at ON broker_authorization_sessions (expires_at);
CREATE INDEX IF NOT EXISTS idx_broker_authorization_sessions_consumed_at ON broker_authorization_sessions (consumed_at);

-- Vibe-Trading 交易画像与行为反思运行记录
CREATE TABLE IF NOT EXISTS trade_journal_reflections (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  scope        TEXT NOT NULL DEFAULT 'all',
  record_count INTEGER NOT NULL,
  engine       TEXT NOT NULL,
  payload      JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_trade_journal_reflections_created_at ON trade_journal_reflections (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_trade_journal_reflections_scope ON trade_journal_reflections (scope);

-- 交易计划：发生在交易日志之前的预案，可执行后关联到日志
CREATE TABLE IF NOT EXISTS trade_plans (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ticker              TEXT NOT NULL,
  direction           TEXT NOT NULL CHECK (direction IN ('buy', 'sell', 'watch')),
  status              TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'waiting', 'executed', 'cancelled', 'expired')),
  plan_date           DATE NOT NULL,
  valid_until         DATE,
  entry_trigger       TEXT,
  entry_price         NUMERIC(12, 4),
  stop_loss           NUMERIC(12, 4),
  take_profit         NUMERIC(12, 4),
  position_size       NUMERIC(6, 2),
  planned_quantity    NUMERIC(20, 4),
  risk_multiple       NUMERIC(6, 2),
  initial_risk_per_share NUMERIC(12, 4),
  one_r_amount        NUMERIC(20, 4),
  playbook_id         UUID,
  rationale           TEXT,
  ai_summary          TEXT,
  notes               TEXT,
  executed_journal_id UUID REFERENCES trade_journal_entries(id) ON DELETE SET NULL,
  executed_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_trade_plans_created_at ON trade_plans (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_trade_plans_ticker_status ON trade_plans (ticker, status);

CREATE TABLE IF NOT EXISTS trading_profiles (
  id                                  BIGSERIAL PRIMARY KEY,
  user_id                             UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
  created_at                          TIMESTAMPTZ DEFAULT NOW(),
  updated_at                          TIMESTAMPTZ DEFAULT NOW(),
  configured                         BOOLEAN NOT NULL DEFAULT FALSE,
  primary_markets                    JSONB NOT NULL DEFAULT '[]'::jsonb,
  asset_classes                      JSONB NOT NULL DEFAULT '[]'::jsonb,
  trading_horizons                   JSONB NOT NULL DEFAULT '[]'::jsonb,
  strategy_preferences               JSONB NOT NULL DEFAULT '[]'::jsonb,
  investment_objectives              JSONB NOT NULL DEFAULT '[]'::jsonb,
  risk_tolerance                     TEXT,
  max_risk_per_trade_percent         NUMERIC(8,4),
  max_daily_loss_percent             NUMERIC(8,4),
  max_open_positions                 BIGINT,
  max_portfolio_concentration_percent NUMERIC(8,4),
  leverage_allowed                   BOOLEAN,
  account_equity                     NUMERIC(20,4),
  account_currency                   TEXT NOT NULL DEFAULT 'USD',
  standard_risk_percent              NUMERIC(8,4),
  max_portfolio_heat_percent         NUMERIC(8,4)
);

CREATE TABLE IF NOT EXISTS trading_policies (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at     TIMESTAMPTZ DEFAULT NOW(),
  updated_at     TIMESTAMPTZ DEFAULT NOW(),
  kind           TEXT NOT NULL CHECK (kind IN ('rule', 'discipline')),
  category       TEXT NOT NULL,
  title          TEXT NOT NULL,
  statement      TEXT NOT NULL,
  condition      JSONB NOT NULL DEFAULT '{}'::jsonb,
  action         JSONB NOT NULL DEFAULT '{}'::jsonb,
  scope          JSONB NOT NULL DEFAULT '{}'::jsonb,
  strength       TEXT NOT NULL CHECK (strength IN ('hard', 'soft', 'reminder')),
  origin         TEXT NOT NULL CHECK (origin IN ('manual', 'ai_suggested', 'system')),
  status         TEXT NOT NULL CHECK (status IN ('draft', 'active', 'archived')),
  prompt_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  version        INTEGER NOT NULL DEFAULT 1,
  effective_from TIMESTAMPTZ,
  effective_to   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_trading_policies_user_status ON trading_policies (user_id, status);

CREATE TABLE IF NOT EXISTS trades (
  id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id                  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at               TIMESTAMPTZ DEFAULT NOW(),
  updated_at               TIMESTAMPTZ DEFAULT NOW(),
  trade_plan_id            UUID REFERENCES trade_plans(id) ON DELETE SET NULL,
  playbook_id              UUID,
  symbol                   TEXT NOT NULL,
  name                     TEXT NOT NULL DEFAULT '',
  market                   TEXT NOT NULL,
  currency                 TEXT NOT NULL DEFAULT 'USD',
  direction                TEXT NOT NULL CHECK (direction IN ('long', 'short')),
  status                   TEXT NOT NULL CHECK (status IN ('open', 'closed', 'cancelled')),
  opened_at                TIMESTAMPTZ NOT NULL,
  closed_at                TIMESTAMPTZ,
  open_quantity            NUMERIC(20,8) NOT NULL DEFAULT 0,
  total_buy_quantity       NUMERIC(20,8) NOT NULL DEFAULT 0,
  total_sell_quantity      NUMERIC(20,8) NOT NULL DEFAULT 0,
  avg_entry_price          NUMERIC(20,8) NOT NULL DEFAULT 0,
  avg_exit_price           NUMERIC(20,8),
  current_stop_loss        NUMERIC(20,8),
  total_fees               NUMERIC(20,8) NOT NULL DEFAULT 0,
  planned_entry_price      NUMERIC(20,8),
  planned_stop_loss        NUMERIC(20,8),
  initial_risk_amount      NUMERIC(24,8),
  open_risk_amount         NUMERIC(24,8) NOT NULL DEFAULT 0,
  realized_pn_l            NUMERIC(24,8),
  realized_pn_l_percent    NUMERIC(12,6),
  realized_r               NUMERIC(12,4),
  entry_slippage_r         NUMERIC(12,4),
  holding_days             NUMERIC(10,2),
  quadrant                 TEXT NOT NULL DEFAULT 'open',
  auto_violations          JSONB NOT NULL DEFAULT '[]'::jsonb
);

ALTER TABLE trade_journal_entries ADD COLUMN IF NOT EXISTS trade_id UUID REFERENCES trades(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS prompt_context_snapshots (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at      TIMESTAMPTZ DEFAULT NOW(),
  purpose         TEXT NOT NULL,
  context_version TEXT NOT NULL,
  trade_plan_id   UUID REFERENCES trade_plans(id) ON DELETE SET NULL,
  context_hash    TEXT NOT NULL,
  payload         JSONB NOT NULL
);
