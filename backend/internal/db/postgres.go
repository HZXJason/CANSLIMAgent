package db

import (
	"errors"

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
	if err := database.AutoMigrate(&models.Stock{}, &models.StockMetric{}, &models.ThresholdPreset{}, &models.TradeJournalEntry{}, &models.TradePlan{}); err != nil {
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
			"name":       preset.Name,
			"is_default": true,
		}).Error
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return database.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"is_default"}),
	}).Create(&preset).Error
}
