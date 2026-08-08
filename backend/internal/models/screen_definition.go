package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ScreenCondition struct {
	Field         string  `json:"field"`
	Operator      string  `json:"operator"`
	Value         float64 `json:"value"`
	UnknownPolicy string  `json:"unknown_policy"`
}

type ScreenDefinition struct {
	ID          uuid.UUID         `json:"id" gorm:"type:uuid;primaryKey"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Name        string            `json:"name" gorm:"type:text;not null"`
	Description string            `json:"description" gorm:"type:text;not null;default:''"`
	AssetMarket string            `json:"asset_market" gorm:"type:text;not null"`
	StockPool   string            `json:"stock_pool" gorm:"type:text;not null"`
	Conditions  []ScreenCondition `json:"conditions" gorm:"type:jsonb;serializer:json;not null"`
	IsSystem    bool              `json:"is_system" gorm:"not null;default:false"`
}

func (definition *ScreenDefinition) BeforeCreate(_ *gorm.DB) error {
	if definition.ID == uuid.Nil {
		definition.ID = uuid.New()
	}
	return nil
}

func DefaultCANSLIMScreenDefinition() ScreenDefinition {
	return ScreenDefinition{Name: "CANSLIM 基础模板", Description: "可修改的成长与相对强度条件，不是唯一选股逻辑。", AssetMarket: "US", StockPool: MarketSP500, IsSystem: true, Conditions: []ScreenCondition{
		{Field: "quarterly_eps_growth", Operator: "gte", Value: 25, UnknownPolicy: "unknown"},
		{Field: "annual_eps_growth", Operator: "gte", Value: 25, UnknownPolicy: "unknown"},
		{Field: "volume_ratio", Operator: "gte", Value: 150, UnknownPolicy: "unknown"},
		{Field: "relative_strength", Operator: "gte", Value: 80, UnknownPolicy: "unknown"},
		{Field: "institution_delta", Operator: "gte", Value: 1, UnknownPolicy: "unknown"},
	}}
}
