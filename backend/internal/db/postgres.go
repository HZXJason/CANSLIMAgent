package db

import (
	"errors"
	"fmt"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func OpenPostgres(databaseURL string) (*gorm.DB, error) {
	database, err := ConnectPostgres(databaseURL)
	if err != nil {
		return nil, err
	}
	if err := Migrate(database); err != nil {
		return nil, err
	}
	return database, nil
}

func ConnectPostgres(databaseURL string) (*gorm.DB, error) {
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL 未配置")
	}
	database, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	sqlDB, err := database.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	return database, nil
}

func Migrate(database *gorm.DB) error {
	return database.Transaction(func(tx *gorm.DB) error {
		// Serialize startup schema work across replicas. The transaction-scoped
		// advisory lock is released automatically on commit or rollback.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(0x43414e534c494d)).Error; err != nil {
			return err
		}
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS app_schema_migrations (
			version text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`).Error; err != nil {
			return err
		}
		if err := tx.AutoMigrate(&models.User{}); err != nil {
			return err
		}
		defaultUser := models.DefaultUser()
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&defaultUser).Error; err != nil {
			return err
		}
		if err := prepareDefaultUserOwnership(tx); err != nil {
			return err
		}
		if err := tx.AutoMigrate(
			&models.Stock{},
			&models.StockMetric{},
			&models.ThresholdPreset{},
			&models.CandidateSnapshot{},
			&models.ResearchCase{},
			&models.UserThesis{},
			&models.PreTradeRiskReview{},
			&models.RiskAcknowledgement{},
			&models.TradeReview{},
			&models.BehaviorObservation{},
			&models.TradingProfile{},
			&models.TradingPrinciple{},
			&models.TradingPlaybook{},
			&models.TradingPolicy{},
			&models.AIMemoryPreference{},
			&models.ScreenDefinition{},
			&models.Trade{},
			&models.TradeJournalEntry{},
			&models.TradeJournalReflection{},
			&models.TradePlan{},
			&models.PromptContextSnapshot{},
			&models.BrokerConnection{},
			&models.BrokerAuthorizationSession{},
			&models.LLMCredential{},
		); err != nil {
			return err
		}
		if err := finalizeDefaultUserOwnership(tx); err != nil {
			return err
		}
		if err := migrateTradeJournalSchema(tx); err != nil {
			return err
		}
		if err := enforceBrokerExecutionUniqueness(tx); err != nil {
			return err
		}
		if err := normalizeStockMetricSufficiency(tx); err != nil {
			return err
		}
		if err := seedDefaultPreset(tx); err != nil {
			return err
		}
		if err := seedDefaultScreenDefinition(tx); err != nil {
			return err
		}
		if err := seedDefaultPlaybooks(tx); err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO app_schema_migrations (version)
			VALUES ('2026-07-13-enterprise-baseline')
			ON CONFLICT (version) DO NOTHING`).Error
	})
}

var defaultUserOwnedTables = []string{
	"candidate_snapshots", "research_cases", "user_theses", "pre_trade_risk_reviews", "risk_acknowledgements",
	"trade_reviews", "behavior_observations", "trading_profiles", "trading_principles", "trading_playbooks",
	"ai_memory_preferences", "trade_journal_entries", "trade_journal_reflections", "trade_plans",
	"broker_connections", "broker_authorization_sessions", "llm_credentials",
}

func prepareDefaultUserOwnership(database *gorm.DB) error {
	for _, table := range defaultUserOwnedTables {
		if !database.Migrator().HasTable(table) {
			continue
		}
		if !database.Migrator().HasColumn(table, "user_id") {
			if err := database.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN user_id uuid`, table)).Error; err != nil {
				return err
			}
		}
		if err := database.Exec(fmt.Sprintf(`UPDATE %s SET user_id = ? WHERE user_id IS NULL`, table), models.DefaultUserID).Error; err != nil {
			return err
		}
	}
	// Replace pre-user global uniqueness with ownership-scoped indexes created by AutoMigrate.
	for _, statement := range []string{
		`DROP INDEX IF EXISTS idx_behavior_observations_key`,
		`DROP INDEX IF EXISTS idx_broker_connections_broker`,
		`DROP INDEX IF EXISTS idx_llm_credential_scope`,
	} {
		if err := database.Exec(statement).Error; err != nil {
			return err
		}
	}
	if database.Migrator().HasTable("broker_connections") {
		if err := database.Exec(`ALTER TABLE broker_connections DROP CONSTRAINT IF EXISTS broker_connections_broker_key`).Error; err != nil {
			return err
		}
	}
	if database.Migrator().HasTable("llm_credentials") {
		if err := database.Exec(`ALTER TABLE llm_credentials DROP CONSTRAINT IF EXISTS llm_credentials_provider_backend_host_key`).Error; err != nil {
			return err
		}
	}
	return nil
}

func finalizeDefaultUserOwnership(database *gorm.DB) error {
	for _, table := range append(defaultUserOwnedTables, "trading_policies", "trades", "prompt_context_snapshots") {
		if !database.Migrator().HasTable(table) || !database.Migrator().HasColumn(table, "user_id") {
			continue
		}
		if err := database.Exec(fmt.Sprintf(`UPDATE %s SET user_id = ? WHERE user_id IS NULL`, table), models.DefaultUserID).Error; err != nil {
			return err
		}
		if err := database.Exec(fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN user_id SET NOT NULL`, table)).Error; err != nil {
			return err
		}
		constraint := "fk_" + table + "_user"
		statement := fmt.Sprintf(`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = '%s') THEN
				ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
			END IF;
		END $$`, constraint, table, constraint)
		if err := database.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedDefaultPlaybooks(database *gorm.DB) error {
	var count int64
	if err := database.Model(&models.TradingPlaybook{}).Where("user_id = ?", models.DefaultUserID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	strPtr := func(value string) *string { return &value }
	f64Ptr := func(value float64) *float64 { return &value }
	defaults := []models.TradingPlaybook{
		{
			UserID:                 models.DefaultUserID,
			Name:                   "箱体枢轴突破 (Pivot Breakout)",
			SetupType:              "pivot_breakout",
			Markets:                []string{"US", "CN"},
			Horizons:               []string{"swing"},
			MarketContext:          strPtr("大盘处于确认上升趋势或受压初期，指数站稳 50SMA"),
			EntryRules:             strPtr("放量突破 4 周以上紧凑基底枢轴点，量比 >= 1.5"),
			InvalidationRules:      strPtr("放量跌破突破日低点或收盘跌回枢轴下方 3%"),
			StopRules:              strPtr("突破日低点或 1.0 ATR 止损，最大不超过 8%"),
			TargetRules:            strPtr("至少 2.5R，达到 +2R 移动止损至保本"),
			PositionRules:          strPtr("标准 1.0R 风险仓位，单票不超过总资金 20%"),
			NoTradeRules:           strPtr("大盘下跌回调期或距财报不足 5 个交易日不开新突破仓"),
			DefaultRiskMultiple:    f64Ptr(1.0),
			DefaultStopATRMultiple: f64Ptr(1.0),
			Status:                 models.PlaybookStatusActive,
			Source:                 "system",
			EvidenceReviewIDs:      []string{},
			AIEnabled:              true,
		},
		{
			UserID:                 models.DefaultUserID,
			Name:                   "财报跳空延续 (PEAD Gap & Go)",
			SetupType:              "pead_earnings_gap",
			Markets:                []string{"US", "CN"},
			Horizons:               []string{"swing"},
			MarketContext:          strPtr("业绩超预期并伴随机构大成交量跳空缺口"),
			EntryRules:             strPtr("突破首日开盘区间高点或缩量回踩跳空缺口上沿企稳"),
			InvalidationRules:      strPtr("完全回补跳空缺口或跌破缺口日最低价"),
			StopRules:              strPtr("缺口日低点下方，宽止损控制在 1.2 ATR 左右"),
			TargetRules:            strPtr("持有至 50SMA 跌破或实现 3R 以上分批兑现"),
			PositionRules:          strPtr("波动较大，建议先用 0.5R 试错"),
			NoTradeRules:           strPtr("营收无超预期或高开低走放巨量滞涨不追"),
			DefaultRiskMultiple:    f64Ptr(0.5),
			DefaultStopATRMultiple: f64Ptr(1.2),
			Status:                 models.PlaybookStatusActive,
			Source:                 "system",
			EvidenceReviewIDs:      []string{},
			AIEnabled:              true,
		},
		{
			UserID:                 models.DefaultUserID,
			Name:                   "强势股 21EMA 缩量回踩 (Pullback)",
			SetupType:              "ema21_pullback",
			Markets:                []string{"US", "CN"},
			Horizons:               []string{"swing"},
			MarketContext:          strPtr("RS 线处于高位的主线领涨股，大盘非下跌回调期"),
			EntryRules:             strPtr("缩量回踩 21EMA 获支撑后，收复前一日高点时入场"),
			InvalidationRules:      strPtr("放量有效跌破 21EMA 且次日无法收回"),
			StopRules:              strPtr("回踩低点下方或 1.0 ATR"),
			TargetRules:            strPtr("前高附近观察量价，目标 >= 2R"),
			PositionRules:          strPtr("0.5R - 1.0R，禁止在连续阴跌放量时左侧接飞刀"),
			NoTradeRules:           strPtr("跌破 50SMA 或同期派发日 >= 5 天不开仓"),
			DefaultRiskMultiple:    f64Ptr(0.5),
			DefaultStopATRMultiple: f64Ptr(1.0),
			Status:                 models.PlaybookStatusActive,
			Source:                 "system",
			EvidenceReviewIDs:      []string{},
			AIEnabled:              true,
		},
	}
	for index := range defaults {
		if err := database.Create(&defaults[index]).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedDefaultScreenDefinition(database *gorm.DB) error {
	definitions := []models.ScreenDefinition{models.DefaultCANSLIMScreenDefinition()}
	cn := models.DefaultCANSLIMScreenDefinition()
	cn.Name, cn.AssetMarket, cn.StockPool = "CANSLIM A股基础模板", "CN", models.MarketSSEMain
	definitions = append(definitions, cn)
	for _, definition := range definitions {
		var existing models.ScreenDefinition
		if err := database.Where("is_system = ? AND name = ?", true, definition.Name).First(&existing).Error; err == nil {
			continue
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := database.Create(&definition).Error; err != nil {
			return err
		}
	}
	return nil
}

func enforceBrokerExecutionUniqueness(database *gorm.DB) error {
	return database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM trade_journal_entries duplicate
			USING trade_journal_entries canonical
			WHERE duplicate.user_id = canonical.user_id
			  AND duplicate.source = canonical.source
			  AND duplicate.order_id = canonical.order_id
			  AND duplicate.order_id IS NOT NULL
			  AND duplicate.id > canonical.id`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DROP INDEX IF EXISTS idx_trade_journal_broker_execution`).Error; err != nil {
			return err
		}
		return tx.Exec(`CREATE UNIQUE INDEX idx_trade_journal_broker_execution
			ON trade_journal_entries (user_id, source, order_id)
			WHERE order_id IS NOT NULL`).Error
	})
}

func migrateTradeJournalSchema(database *gorm.DB) error {
	if !database.Migrator().HasColumn("trade_journal_entries", "ticker") {
		return nil
	}

	// Preserve old rows without pretending the former position percentage was a
	// filled quantity. New writes use only the canonical TradeRecord columns.
	statements := []string{
		`UPDATE trade_journal_entries
		 SET symbol = COALESCE(NULLIF(symbol, ''), ticker),
		     side = COALESCE(NULLIF(side, ''), direction, 'buy'),
		     executed_at = COALESCE(trade_date::timestamptz, executed_at),
		     source = CASE WHEN COALESCE(source, '') IN ('', 'manual') THEN 'legacy_unstructured' ELSE source END
		 WHERE symbol = ''`,
		"ALTER TABLE trade_journal_entries ALTER COLUMN ticker DROP NOT NULL",
		"ALTER TABLE trade_journal_entries ALTER COLUMN direction DROP NOT NULL",
		"ALTER TABLE trade_journal_entries ALTER COLUMN trade_date DROP NOT NULL",
		"ALTER TABLE trade_journal_entries ALTER COLUMN position_size DROP NOT NULL",
		"ALTER TABLE trade_journal_entries ALTER COLUMN stop_loss DROP NOT NULL",
	}
	for _, statement := range statements {
		if err := database.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func normalizeStockMetricSufficiency(database *gorm.DB) error {
	statements := []string{
		"ALTER TABLE stock_metrics ALTER COLUMN c_data_sufficient SET DEFAULT false",
		"ALTER TABLE stock_metrics ALTER COLUMN a_data_sufficient SET DEFAULT false",
		"ALTER TABLE stock_metrics ALTER COLUMN s_data_sufficient SET DEFAULT false",
		"ALTER TABLE stock_metrics ALTER COLUMN l_data_sufficient SET DEFAULT false",
		"ALTER TABLE stock_metrics ALTER COLUMN i_data_sufficient SET DEFAULT false",
		"UPDATE stock_metrics SET c_data_sufficient = false WHERE c_eps_growth_pct IS NULL",
		"UPDATE stock_metrics SET a_data_sufficient = false WHERE a_annual_eps_growth IS NULL OR a_annual_eps_growth = '[]'::jsonb",
		"UPDATE stock_metrics SET s_data_sufficient = false WHERE s_volume_ratio IS NULL",
		"UPDATE stock_metrics SET l_data_sufficient = false WHERE l_rs_rating IS NULL",
		"UPDATE stock_metrics SET i_data_sufficient = false WHERE i_institution_delta IS NULL",
		"UPDATE stock_metrics SET ai_analysis_at = n_analysis_at WHERE ai_analysis_at IS NULL AND n_analysis_at IS NOT NULL",
	}
	for _, statement := range statements {
		if err := database.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedDefaultPreset(database *gorm.DB) error {
	preset := models.DefaultThresholdPreset()
	var existingDefault models.ThresholdPreset
	if err := database.Where("is_default = ?", true).First(&existingDefault).Error; err == nil {
		return database.Model(&existingDefault).Updates(map[string]interface{}{
			"name":           preset.Name,
			"is_default":     true,
			"c_min_growth":   preset.CMinGrowth,
			"a_min_growth":   preset.AMinGrowth,
			"a_min_years":    preset.AMinYears,
			"s_volume_ratio": preset.SVolumeRatio,
			"s_ma_period":    preset.SMAPeriod,
			"l_min_rs":       preset.LMinRS,
			"i_min_delta":    preset.IMinDelta,
		}).Error
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return database.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"is_default"}),
	}).Create(&preset).Error
}
