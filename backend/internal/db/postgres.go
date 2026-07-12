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
	if err := database.AutoMigrate(&models.Stock{}, &models.StockMetric{}, &models.ThresholdPreset{}, &models.TradeJournalEntry{}, &models.TradeJournalReflection{}, &models.TradePlan{}, &models.BrokerConnection{}, &models.BrokerAuthorizationSession{}); err != nil {
		return nil, err
	}
	if err := migrateTradeJournalSchema(database); err != nil {
		return nil, err
	}
	if err := normalizeStockMetricSufficiency(database); err != nil {
		return nil, err
	}
	if err := seedDefaultPreset(database); err != nil {
		return nil, err
	}
	return database, nil
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
