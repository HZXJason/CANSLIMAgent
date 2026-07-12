package handlers

import (
	"strconv"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/alphalens/canslimagent/backend/internal/services"
)

// normalizeBrokerExecutions is the single boundary between broker-specific
// payloads and vibe.trade_record.v1 journal entries. Invalid fills are not
// imported; the user receives the reason in the preview response instead.
func normalizeBrokerExecutions(broker string, input []services.BrokerExecution) ([]services.BrokerExecution, []string) {
	result := make([]services.BrokerExecution, 0, len(input))
	warnings := []string{}
	seen := map[string]bool{}
	for _, item := range input {
		item.ExternalID = strings.TrimSpace(item.ExternalID)
		item.Symbol = normalizeBrokerSymbol(broker, item.Symbol)
		item.Name = strings.TrimSpace(item.Name)
		item.Side = normalizeBrokerSide(item.Side)
		item.Market = normalizeBrokerMarket(item.Market, item.Symbol)
		item.Currency = normalizeBrokerCurrency(item.Currency, item.Market)
		executedAt, err := parseBrokerTime(item.ExecutedAt)
		if item.ExternalID == "" || item.Symbol == "" || item.Side == "" || item.Quantity <= 0 || item.Price <= 0 || err != nil {
			warnings = append(warnings, "已跳过缺少成交编号、标的、方向、时间、数量或价格的券商成交")
			continue
		}
		if item.Fee < 0 {
			warnings = append(warnings, item.ExternalID+" 的手续费无效，已按 0 导入")
			item.Fee = 0
		}
		if seen[item.ExternalID] {
			warnings = append(warnings, item.ExternalID+" 在本次返回中重复，已跳过")
			continue
		}
		seen[item.ExternalID] = true
		item.ExecutedAt = executedAt.UTC().Format(time.RFC3339)
		result = append(result, item)
	}
	return result, warnings
}

func normalizeBrokerSymbol(broker, symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if broker == models.BrokerFutu {
		if prefix, rest, ok := strings.Cut(symbol, "."); ok && (prefix == "US" || prefix == "HK" || prefix == "SH" || prefix == "SZ") {
			if prefix == "US" {
				return rest
			}
			return rest + "." + prefix
		}
	}
	if broker == models.BrokerLongPort {
		if ticker, region, ok := strings.Cut(symbol, "."); ok {
			switch region {
			case "US":
				return ticker
			case "HK", "SH", "SZ", "BJ":
				return ticker + "." + region
			}
		}
	}
	return symbol
}

func normalizeBrokerSide(side string) string {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "buy", "bought", "bot", "long":
		return models.TradeSideBuy
	case "sell", "sold", "sld", "short":
		return models.TradeSideSell
	default:
		return ""
	}
}

func normalizeBrokerMarket(market, symbol string) string {
	market = strings.ToLower(strings.TrimSpace(market))
	if models.ValidTradeMarket(market) {
		return market
	}
	return inferTradeMarket(symbol)
}

func normalizeBrokerCurrency(currency, market string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency != "" {
		return currency
	}
	return defaultTradeCurrency(market)
}

func parseBrokerTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if unix, err := strconv.ParseInt(value, 10, 64); err == nil {
		if unix > 10_000_000_000 {
			return time.UnixMilli(unix), nil
		}
		return time.Unix(unix, 0), nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, strconv.ErrSyntax
}
