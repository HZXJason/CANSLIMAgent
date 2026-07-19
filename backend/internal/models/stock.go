package models

import "time"

const (
	MarketSP500       = "SP500"
	MarketNASDAQ100   = "NASDAQ100"
	MarketRUSSELL2000 = "RUSSELL2000"
	MarketSSEMain     = "SSE_MAIN"
	MarketSZSEMain    = "SZSE_MAIN"
	MarketSTAR        = "STAR"
	MarketChiNext     = "CHINEXT"
	MarketBSE         = "BSE"
)

type Stock struct {
	ID        uint        `gorm:"primaryKey" json:"id"`
	Symbol    string      `gorm:"size:10;not null;uniqueIndex:idx_symbol_market" json:"symbol"`
	Name      string      `gorm:"size:100;not null" json:"name"`
	Market    string      `gorm:"size:20;not null;uniqueIndex:idx_symbol_market;index" json:"market"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	Metric    StockMetric `gorm:"foreignKey:StockID" json:"-"`
}

type StockMetric struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	StockID           uint       `gorm:"not null;uniqueIndex" json:"stock_id"`
	Stock             *Stock     `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	CEPSGrowthPct     *float64   `gorm:"column:c_eps_growth_pct;type:decimal(10,2)" json:"c_eps_growth_pct"`
	CDataSufficient   bool       `gorm:"column:c_data_sufficient;default:false" json:"c_data_sufficient"`
	AAnnualEPSGrowth  []float64  `gorm:"column:a_annual_eps_growth;type:jsonb;serializer:json" json:"a_annual_eps_growth"`
	AYearsAvailable   int        `gorm:"column:a_years_available" json:"a_years_available"`
	ADataSufficient   bool       `gorm:"column:a_data_sufficient;default:false" json:"a_data_sufficient"`
	SVolumeRatio      *float64   `gorm:"column:s_volume_ratio;type:decimal(10,2)" json:"s_volume_ratio"`
	SDataSufficient   bool       `gorm:"column:s_data_sufficient;default:false" json:"s_data_sufficient"`
	LRSRating         *int       `gorm:"column:l_rs_rating" json:"l_rs_rating"`
	LDataSufficient   bool       `gorm:"column:l_data_sufficient;default:false" json:"l_data_sufficient"`
	IInstitutionDelta *int       `gorm:"column:i_institution_delta" json:"i_institution_delta"`
	IDataSufficient   bool       `gorm:"column:i_data_sufficient;default:false" json:"i_data_sufficient"`
	AIAnalysisAt      *time.Time `gorm:"column:ai_analysis_at" json:"ai_analysis_at"`
	NLabel            *bool      `gorm:"column:n_label" json:"n_label"`
	NAnalysisAt       *time.Time `gorm:"column:n_analysis_at" json:"n_analysis_at"`
	NAnalysisSummary  string     `gorm:"column:n_analysis_summary;type:text" json:"n_analysis_summary"`
	DataAsOf          time.Time  `gorm:"column:data_as_of;type:date;not null" json:"data_as_of"`
	FetchedAt         time.Time  `gorm:"column:fetched_at;autoCreateTime" json:"fetched_at"`
}

type ThresholdPreset struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"size:50;not null;uniqueIndex" json:"name"`
	IsDefault    bool      `gorm:"default:false" json:"is_default"`
	CMinGrowth   float64   `gorm:"column:c_min_growth;type:decimal(5,2);default:25.0" json:"c_min_growth"`
	AMinGrowth   float64   `gorm:"column:a_min_growth;type:decimal(5,2);default:25.0" json:"a_min_growth"`
	AMinYears    int       `gorm:"column:a_min_years;default:3" json:"a_min_years"`
	SVolumeRatio float64   `gorm:"column:s_volume_ratio;type:decimal(5,2);default:150.0" json:"s_volume_ratio"`
	SMAPeriod    int       `gorm:"column:s_ma_period;default:50" json:"s_ma_period"`
	LMinRS       int       `gorm:"column:l_min_rs;default:80" json:"l_min_rs"`
	IMinDelta    int       `gorm:"column:i_min_delta;default:1" json:"i_min_delta"`
	CreatedAt    time.Time `json:"created_at"`
}

func ValidMarket(market string) bool {
	switch market {
	case MarketSP500, MarketNASDAQ100, MarketRUSSELL2000,
		MarketSSEMain, MarketSZSEMain, MarketSTAR, MarketChiNext, MarketBSE:
		return true
	default:
		return false
	}
}

func IsAshareMarket(market string) bool {
	switch market {
	case MarketSSEMain, MarketSZSEMain, MarketSTAR, MarketChiNext, MarketBSE:
		return true
	default:
		return false
	}
}

func MarketDisabledReason(market string) string {
	return "不支持的市场：" + market + "，目前支持 SP500、NASDAQ100、RUSSELL2000、SSE_MAIN、SZSE_MAIN、STAR、CHINEXT、BSE"
}

func DefaultThresholdPreset() ThresholdPreset {
	return ThresholdPreset{
		Name:         "欧奈尔默认",
		IsDefault:    true,
		CMinGrowth:   25,
		AMinGrowth:   25,
		AMinYears:    3,
		SVolumeRatio: 150,
		SMAPeriod:    50,
		LMinRS:       80,
		IMinDelta:    1,
	}
}
