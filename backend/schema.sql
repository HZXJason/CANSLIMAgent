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

-- 交易日志：逐笔成交，字段与 Vibe-Trading TradeRecord 对齐
CREATE TABLE IF NOT EXISTS trade_journal_entries (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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
  last_synced_at        TIMESTAMPTZ
);

-- Vibe-Trading 交易画像与行为反思运行记录
CREATE TABLE IF NOT EXISTS trade_journal_reflections (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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
  rationale           TEXT,
  ai_summary          TEXT,
  notes               TEXT,
  executed_journal_id UUID REFERENCES trade_journal_entries(id) ON DELETE SET NULL,
  executed_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_trade_plans_created_at ON trade_plans (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_trade_plans_ticker_status ON trade_plans (ticker, status);
