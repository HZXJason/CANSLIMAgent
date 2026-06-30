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
	if err := database.AutoMigrate(&models.Stock{}, &models.StockMetric{}, &models.ThresholdPreset{}, &models.TradeJournalEntry{}); err != nil {
		return nil, err
	}
	if err := seedDefaultPreset(database); err != nil {
		return nil, err
	}
	return database, nil
}

func seedDefaultPreset(database *gorm.DB) error {
	preset := models.DefaultThresholdPreset()
	return database.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"is_default"}),
	}).Create(&preset).Error
}
