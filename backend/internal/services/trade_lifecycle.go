package services

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TradesSummary struct {
	TotalTrades          int     `json:"total_trades"`
	OpenTrades           int     `json:"open_trades"`
	ClosedTrades         int     `json:"closed_trades"`
	ModelWins            int     `json:"model_wins"`
	GoodLosses           int     `json:"good_losses"`
	UnforcedErrors       int     `json:"unforced_errors"`
	ToxicWins            int     `json:"toxic_wins"`
	ExpectancyR          float64 `json:"expectancy_r"`
	AvgWinR              float64 `json:"avg_win_r"`
	AvgLossR             float64 `json:"avg_loss_r"`
	WinRate              float64 `json:"win_rate"`
	CostOfIndisciplineR  float64 `json:"cost_of_indiscipline_r"`
	DisciplinedTrades    int     `json:"disciplined_trades"`
	DisciplinedRLoopRate float64 `json:"disciplined_r_loop_rate"`
	OpenPortfolioRisk    float64 `json:"open_portfolio_risk"`
	OpenPortfolioRiskPct float64 `json:"open_portfolio_risk_percent"`
	AccountEquity        float64 `json:"account_equity"`
	OneRAmount           float64 `json:"one_r_amount"`
	MaxPortfolioHeatPct  float64 `json:"max_portfolio_heat_percent"`
	UnlinkedFillsCount   int     `json:"unlinked_fills_count"`
	PendingReviewTrades  int     `json:"pending_review_trades"`
}

func RebuildUserTrades(ctx context.Context, database *gorm.DB, userID uuid.UUID) ([]models.Trade, error) {
	if database == nil || !database.Migrator().HasTable(&models.Trade{}) || !database.Migrator().HasTable(&models.TradeJournalEntry{}) || !database.Migrator().HasTable(&models.TradePlan{}) {
		return []models.Trade{}, nil
	}
	hasReviewsTable := database.Migrator().HasTable(&models.TradeReview{})

	var fills []models.TradeJournalEntry
	if err := database.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("executed_at asc, created_at asc, id asc").
		Find(&fills).Error; err != nil {
		return nil, err
	}

	var plans []models.TradePlan
	if err := database.WithContext(ctx).Where("user_id = ?", userID).Find(&plans).Error; err != nil {
		return nil, err
	}
	planByID := make(map[uuid.UUID]models.TradePlan, len(plans))
	for _, plan := range plans {
		planByID[plan.ID] = plan
	}

	var existingTrades []models.Trade
	if err := database.WithContext(ctx).Where("user_id = ?", userID).Find(&existingTrades).Error; err != nil {
		return nil, err
	}
	type preservedTradeState struct {
		id              uuid.UUID
		currentStopLoss *float64
		tradePlanID     *uuid.UUID
	}
	preservedByPlan := make(map[uuid.UUID]preservedTradeState)
	preservedByOpenKey := make(map[string]preservedTradeState)
	for _, tr := range existingTrades {
		state := preservedTradeState{id: tr.ID, currentStopLoss: tr.CurrentStopLoss, tradePlanID: tr.TradePlanID}
		if tr.TradePlanID != nil {
			preservedByPlan[*tr.TradePlanID] = state
		}
		key := tr.Market + "|" + tr.Symbol + "|" + tr.OpenedAt.UTC().Format(time.RFC3339Nano)
		preservedByOpenKey[key] = state
	}

	var reviews []models.TradeReview
	if hasReviewsTable {
		if err := database.WithContext(ctx).Where("user_id = ?", userID).Order("version desc").Find(&reviews).Error; err != nil {
			return nil, err
		}
	}
	latestReviewByEntry := make(map[uuid.UUID]models.TradeReview)
	for _, rev := range reviews {
		if _, exists := latestReviewByEntry[rev.JournalEntryID]; !exists {
			latestReviewByEntry[rev.JournalEntryID] = rev
		}
	}

	type positionAccumulator struct {
		trade           models.Trade
		fillIDs         []uuid.UUID
		buyCost         float64
		sellProceeds    float64
		hasStopLoss     bool
		processDeviated bool
	}

	activeBySymbol := make(map[string]*positionAccumulator)
	built := make([]*positionAccumulator, 0)

	finalizeAccumulator := func(acc *positionAccumulator) {
		tr := &acc.trade
		if acc.trade.TotalBuyQuantity > 0 {
			tr.AvgEntryPrice = round4(acc.buyCost / tr.TotalBuyQuantity)
		}
		if acc.trade.TotalSellQuantity > 0 {
			avgExit := round4(acc.sellProceeds / tr.TotalSellQuantity)
			tr.AvgExitPrice = &avgExit
		}
		if tr.TradePlanID != nil {
			if plan, ok := planByID[*tr.TradePlanID]; ok {
				tr.PlaybookID = plan.PlaybookID
				tr.PlannedEntryPrice = plan.EntryPrice
				tr.PlannedStopLoss = plan.StopLoss
				if tr.CurrentStopLoss == nil && plan.StopLoss != nil {
					stop := *plan.StopLoss
					tr.CurrentStopLoss = &stop
				}
				if plan.EntryPrice != nil && plan.StopLoss != nil && tr.TotalBuyQuantity > 0 {
					perShare := math.Abs(*plan.EntryPrice - *plan.StopLoss)
					if perShare > 0 {
						initRisk := round4(perShare * tr.TotalBuyQuantity)
						tr.InitialRiskAmount = &initRisk
						slipR := (tr.AvgEntryPrice - *plan.EntryPrice) / perShare
						if tr.Direction == models.TradeDirectionShort {
							slipR = (*plan.EntryPrice - tr.AvgEntryPrice) / perShare
						}
						slipR = round4(slipR)
						tr.EntrySlippageR = &slipR
						if slipR > 0.25 {
							tr.AutoViolations = appendUniqueViolation(tr.AutoViolations, "entry_chase_over_0_25r")
						}
					}
				} else if plan.MaxRiskAmount != nil && *plan.MaxRiskAmount > 0 {
					initRisk := round4(*plan.MaxRiskAmount)
					tr.InitialRiskAmount = &initRisk
				}
			}
		} else {
			tr.AutoViolations = appendUniqueViolation(tr.AutoViolations, "off_plan_unlinked")
		}

		if tr.OpenQuantity > 1e-6 {
			tr.Status = models.TradeStatusOpen
			tr.Quadrant = models.TradeQuadrantOpen
			activeStop := tr.CurrentStopLoss
			if activeStop == nil {
				activeStop = tr.PlannedStopLoss
			}
			if activeStop != nil && *activeStop > 0 {
				riskPerShare := tr.AvgEntryPrice - *activeStop
				if tr.Direction == models.TradeDirectionShort {
					riskPerShare = *activeStop - tr.AvgEntryPrice
				}
				if riskPerShare > 0 {
					tr.OpenRiskAmount = round4(riskPerShare * tr.OpenQuantity)
				} else {
					tr.OpenRiskAmount = 0
				}
			} else {
				tr.OpenRiskAmount = round4(tr.AvgEntryPrice * tr.OpenQuantity * 0.08)
				tr.AutoViolations = appendUniqueViolation(tr.AutoViolations, "missing_stop_loss")
			}
		} else {
			tr.Status = models.TradeStatusClosed
			tr.OpenQuantity = 0
			tr.OpenRiskAmount = 0
			netPnL := round4(acc.sellProceeds - acc.buyCost - tr.TotalFees)
			tr.RealizedPnL = &netPnL
			if acc.buyCost > 0 {
				pct := round4(netPnL / acc.buyCost * 100.0)
				tr.RealizedPnLPercent = &pct
			}
			if tr.InitialRiskAmount != nil && *tr.InitialRiskAmount > 0 {
				rVal := round4(netPnL / *tr.InitialRiskAmount)
				tr.RealizedR = &rVal
				if rVal < -1.15 {
					tr.AutoViolations = appendUniqueViolation(tr.AutoViolations, "stop_loss_violated")
				}
			}
			if tr.ClosedAt != nil {
				days := round2(tr.ClosedAt.Sub(tr.OpenedAt).Hours() / 24.0)
				if days < 0 {
					days = 0
				}
				tr.HoldingDays = &days
			}
			goodProcess := tr.TradePlanID != nil &&
				!acc.processDeviated &&
				!hasViolationCode(tr.AutoViolations, "stop_loss_violated") &&
				!hasViolationCode(tr.AutoViolations, "entry_chase_over_0_25r")
			switch {
			case goodProcess && netPnL >= 0:
				tr.Quadrant = models.TradeQuadrantModelWin
			case goodProcess && netPnL < 0:
				tr.Quadrant = models.TradeQuadrantGoodLoss
			case !goodProcess && netPnL < 0:
				tr.Quadrant = models.TradeQuadrantUnforcedError
			default:
				tr.Quadrant = models.TradeQuadrantToxicWin
			}
		}
	}

	for _, fill := range fills {
		if fill.Quantity <= 0 || fill.Price <= 0 {
			continue
		}
		symbol := strings.ToUpper(strings.TrimSpace(fill.Symbol))
		market := strings.ToLower(strings.TrimSpace(fill.Market))
		if market == "" {
			market = models.TradeMarketOther
		}
		key := market + "|" + symbol

		if fill.Side == models.TradeSideBuy {
			acc := activeBySymbol[key]
			if acc == nil {
				currency := fill.Currency
				if currency == "" {
					currency = "USD"
				}
				tr := models.Trade{
					ID:             uuid.New(),
					UserID:         userID,
					TradePlanID:    fill.TradePlanID,
					Symbol:         symbol,
					Name:           fill.Name,
					Market:         market,
					Currency:       currency,
					Direction:      models.TradeDirectionLong,
					Status:         models.TradeStatusOpen,
					OpenedAt:       fill.ExecutedAt,
					Quadrant:       models.TradeQuadrantOpen,
					AutoViolations: []string{},
				}
				openKey := market + "|" + symbol + "|" + fill.ExecutedAt.UTC().Format(time.RFC3339Nano)
				if preserved, ok := preservedByOpenKey[openKey]; ok {
					tr.ID = preserved.id
					tr.CurrentStopLoss = preserved.currentStopLoss
					if tr.TradePlanID == nil {
						tr.TradePlanID = preserved.tradePlanID
					}
				} else if fill.TradePlanID != nil {
					if preserved, ok := preservedByPlan[*fill.TradePlanID]; ok {
						tr.ID = preserved.id
						tr.CurrentStopLoss = preserved.currentStopLoss
					}
				}
				acc = &positionAccumulator{trade: tr}
				activeBySymbol[key] = acc
				built = append(built, acc)
			}
			if acc.trade.TradePlanID == nil && fill.TradePlanID != nil {
				acc.trade.TradePlanID = fill.TradePlanID
			}
			if acc.trade.Name == "" && fill.Name != "" {
				acc.trade.Name = fill.Name
			}
			acc.trade.TotalBuyQuantity += fill.Quantity
			acc.trade.OpenQuantity += fill.Quantity
			acc.trade.TotalFees += fill.Fee
			acc.buyCost += fill.Quantity * fill.Price
			acc.fillIDs = append(acc.fillIDs, fill.ID)
			if rev, ok := latestReviewByEntry[fill.ID]; ok && rev.ProcessQuality == "deviated" {
				acc.processDeviated = true
			}
			continue
		}

		// Sell fill
		acc := activeBySymbol[key]
		if acc == nil {
			continue
		}
		if acc.trade.TradePlanID == nil && fill.TradePlanID != nil {
			acc.trade.TradePlanID = fill.TradePlanID
		}
		matched := math.Min(fill.Quantity, acc.trade.OpenQuantity)
		acc.trade.TotalSellQuantity += matched
		acc.trade.OpenQuantity -= matched
		acc.trade.TotalFees += fill.Fee
		acc.sellProceeds += matched * fill.Price
		acc.fillIDs = append(acc.fillIDs, fill.ID)
		if rev, ok := latestReviewByEntry[fill.ID]; ok && rev.ProcessQuality == "deviated" {
			acc.processDeviated = true
		}
		if acc.trade.OpenQuantity <= 1e-6 {
			closedAt := fill.ExecutedAt
			acc.trade.ClosedAt = &closedAt
			finalizeAccumulator(acc)
			delete(activeBySymbol, key)
		}
	}

	for _, acc := range activeBySymbol {
		finalizeAccumulator(acc)
	}

	trades := make([]models.Trade, 0, len(built))
	for _, acc := range built {
		trades = append(trades, acc.trade)
	}
	sort.Slice(trades, func(i, j int) bool {
		return trades[i].OpenedAt.After(trades[j].OpenedAt)
	})

	err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.TradeJournalEntry{}).Where("user_id = ?", userID).Update("trade_id", nil).Error; err != nil {
			return err
		}
		if hasReviewsTable {
			if err := tx.Model(&models.TradeReview{}).Where("user_id = ?", userID).Update("trade_id", nil).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("user_id = ?", userID).Delete(&models.Trade{}).Error; err != nil {
			return err
		}
		for index := range built {
			acc := built[index]
			if err := tx.Create(&acc.trade).Error; err != nil {
				return err
			}
			if len(acc.fillIDs) > 0 {
				if err := tx.Model(&models.TradeJournalEntry{}).
					Where("user_id = ? AND id IN ?", userID, acc.fillIDs).
					Update("trade_id", acc.trade.ID).Error; err != nil {
					return err
				}
				if hasReviewsTable {
					if err := tx.Model(&models.TradeReview{}).
						Where("user_id = ? AND journal_entry_id IN ?", userID, acc.fillIDs).
						Update("trade_id", acc.trade.ID).Error; err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return trades, nil
}

func ComputeTradesSummary(
	trades []models.Trade,
	profile models.TradingProfile,
	fills []models.TradeJournalEntry,
	reviews []models.TradeReview,
	preTradeReviews []models.PreTradeRiskReview,
) TradesSummary {
	summary := TradesSummary{
		TotalTrades: len(trades),
	}
	if profile.AccountEquity != nil && *profile.AccountEquity > 0 {
		summary.AccountEquity = round2(*profile.AccountEquity)
		stdRisk := 1.0
		if profile.StandardRiskPercent != nil && *profile.StandardRiskPercent > 0 {
			stdRisk = *profile.StandardRiskPercent
		}
		summary.OneRAmount = round2(summary.AccountEquity * stdRisk / 100.0)
	}
	if profile.MaxPortfolioHeatPercent != nil && *profile.MaxPortfolioHeatPercent > 0 {
		summary.MaxPortfolioHeatPct = *profile.MaxPortfolioHeatPercent
	} else {
		summary.MaxPortfolioHeatPct = 5.0
	}

	pretradeReviewedPlan := make(map[uuid.UUID]bool)
	for _, ptr := range preTradeReviews {
		pretradeReviewedPlan[ptr.TradePlanID] = true
	}

	completedReviewByTrade := make(map[uuid.UUID]models.TradeReview)
	completedReviewByEntry := make(map[uuid.UUID]models.TradeReview)
	for _, rev := range reviews {
		if rev.CompletedAt == nil {
			continue
		}
		if rev.TradeID != nil {
			if _, ok := completedReviewByTrade[*rev.TradeID]; !ok {
				completedReviewByTrade[*rev.TradeID] = rev
			}
		}
		if _, ok := completedReviewByEntry[rev.JournalEntryID]; !ok {
			completedReviewByEntry[rev.JournalEntryID] = rev
		}
	}

	fillTradeReviewed := make(map[uuid.UUID]bool)
	for _, fill := range fills {
		if fill.TradePlanID == nil {
			summary.UnlinkedFillsCount++
		}
		if fill.TradeID != nil {
			if _, ok := completedReviewByEntry[fill.ID]; ok {
				fillTradeReviewed[*fill.TradeID] = true
			}
		}
	}

	var sumR, winRSum, lossRSum, indisciplineLossR float64
	var rCount, winCount, lossCount int
	for _, tr := range trades {
		if tr.Status == models.TradeStatusOpen {
			summary.OpenTrades++
			summary.OpenPortfolioRisk += tr.OpenRiskAmount
			continue
		}
		if tr.Status != models.TradeStatusClosed {
			continue
		}
		summary.ClosedTrades++
		switch tr.Quadrant {
		case models.TradeQuadrantModelWin:
			summary.ModelWins++
		case models.TradeQuadrantGoodLoss:
			summary.GoodLosses++
		case models.TradeQuadrantUnforcedError:
			summary.UnforcedErrors++
		case models.TradeQuadrantToxicWin:
			summary.ToxicWins++
		}

		_, hasTradeReview := completedReviewByTrade[tr.ID]
		isReviewed := hasTradeReview || fillTradeReviewed[tr.ID]
		if !isReviewed {
			summary.PendingReviewTrades++
		}

		hasPretrade := tr.TradePlanID != nil && pretradeReviewedPlan[*tr.TradePlanID]
		withinStop := tr.RealizedR == nil || *tr.RealizedR >= -1.15
		if tr.TradePlanID != nil && hasPretrade && withinStop && isReviewed {
			summary.DisciplinedTrades++
		}

		if tr.RealizedR != nil {
			r := *tr.RealizedR
			sumR += r
			rCount++
			if r >= 0 {
				winRSum += r
				winCount++
			} else {
				lossRSum += r
				lossCount++
				if tr.Quadrant == models.TradeQuadrantUnforcedError {
					indisciplineLossR += math.Abs(r)
				}
			}
		}
	}

	summary.OpenPortfolioRisk = round2(summary.OpenPortfolioRisk)
	if summary.AccountEquity > 0 {
		summary.OpenPortfolioRiskPct = round2(summary.OpenPortfolioRisk / summary.AccountEquity * 100.0)
	}
	if rCount > 0 {
		summary.ExpectancyR = round2(sumR / float64(rCount))
	}
	if winCount > 0 {
		summary.AvgWinR = round2(winRSum / float64(winCount))
	}
	if lossCount > 0 {
		summary.AvgLossR = round2(lossRSum / float64(lossCount))
	}
	if summary.ClosedTrades > 0 {
		summary.WinRate = round2(float64(summary.ModelWins+summary.ToxicWins) / float64(summary.ClosedTrades) * 100.0)
		summary.DisciplinedRLoopRate = round2(float64(summary.DisciplinedTrades) / float64(summary.ClosedTrades) * 100.0)
	}
	summary.CostOfIndisciplineR = round2(indisciplineLossR)
	return summary
}

func appendUniqueViolation(list []string, code string) []string {
	for _, item := range list {
		if item == code {
			return list
		}
	}
	return append(list, code)
}

func hasViolationCode(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}
