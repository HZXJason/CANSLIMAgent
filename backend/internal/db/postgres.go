package db

import (
	"errors"
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
		if err := tx.AutoMigrate(&models.Stock{}, &models.StockMetric{}, &models.ThresholdPreset{}, &models.TradeJournalEntry{}, &models.TradeJournalReflection{}, &models.TradePlan{}, &models.BrokerConnection{}, &models.BrokerAuthorizationSession{}, &models.LLMCredential{}, &models.AICalibration{}); err != nil {
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
		return tx.Exec(`INSERT INTO app_schema_migrations (version)
			VALUES ('2026-07-13-enterprise-baseline')
			ON CONFLICT (version) DO NOTHING`).Error
	})
}

func enforceBrokerExecutionUniqueness(database *gorm.DB) error {
	return database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM trade_journal_entries duplicate
			USING trade_journal_entries canonical
			WHERE duplicate.source = canonical.source
			  AND duplicate.order_id = canonical.order_id
			  AND duplicate.order_id IS NOT NULL
			  AND duplicate.id > canonical.id`).Error; err != nil {
			return err
		}
		return tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_trade_journal_broker_execution
			ON trade_journal_entries (source, order_id)
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
