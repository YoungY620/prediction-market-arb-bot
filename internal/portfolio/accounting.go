package portfolio

import (
	"fmt"
	"math"
	"time"

	"prediction-market-arb-bot/internal/types"
)

func NewPaperPortfolio(cycleID string, startingCash float64) types.PortfolioSnapshot {
	now := time.Now().UTC().Format(time.RFC3339)
	return types.PortfolioSnapshot{
		CycleID:        cycleID,
		Mode:           "paper",
		SnapshotAt:     now,
		CashUSD:        startingCash,
		NavUSD:         startingCash,
		StartingNavUSD: startingCash,
		ReturnPct:      0,
		Positions:      []types.PaperPosition{},
	}
}

func ApplyPaperFills(cycleID string, before types.PortfolioSnapshot, proposal types.Proposal, preview types.ExecutionPreview, books map[string]types.OrderBook) (types.PortfolioSnapshot, types.ExecutionResult) {
	after := before
	after.CycleID = cycleID
	after.Mode = "paper"
	after.SnapshotAt = time.Now().UTC().Format(time.RFC3339)
	result := types.ExecutionResult{CycleID: cycleID, Mode: "paper"}
	opportunities := map[string]types.NewOpportunity{}
	for _, opp := range proposal.NewOpportunities {
		opportunities[opp.DecisionID] = opp
	}
	reviews := map[string]types.PositionReview{}
	for _, review := range proposal.PositionReviews {
		reviews[review.DecisionID] = review
	}
	for _, item := range preview.Items {
		if !item.WouldExecute || item.ExecutableNotionalUSD <= 0 || item.FillPrice == nil {
			continue
		}
		trade := buildTrade(cycleID, after.SnapshotAt, item, opportunities, reviews)
		result.Trades = append(result.Trades, trade)
		if trade.Side == "BUY" {
			after.CashUSD = round4(after.CashUSD - trade.NotionalUSD)
			after.Positions = upsertBuy(after.Positions, trade)
		} else if trade.Side == "SELL" {
			after.CashUSD = round4(after.CashUSD + trade.NotionalUSD)
			after.Positions = applySell(after.Positions, trade)
		}
	}
	after = MarkToMarket(after, books)
	return after, result
}

func buildTrade(cycleID, timestamp string, item types.ExecutionPreviewItem, opportunities map[string]types.NewOpportunity, reviews map[string]types.PositionReview) types.PaperTrade {
	shares := item.ExecutableNotionalUSD / *item.FillPrice
	trade := types.PaperTrade{
		TradeID:     fmt.Sprintf("tr_%s_%s", cycleID, item.DecisionID),
		CycleID:     cycleID,
		DecisionID:  item.DecisionID,
		Action:      item.Action,
		TokenID:     item.TokenID,
		Outcome:     item.Outcome,
		Shares:      round6(shares),
		Price:       round6(*item.FillPrice),
		NotionalUSD: round4(item.ExecutableNotionalUSD),
		Timestamp:   timestamp,
	}
	if item.Action == "open" {
		opp := opportunities[item.DecisionID]
		trade.Side = "BUY"
		trade.MarketSlug = opp.MarketSlug
		trade.EventSlug = opp.EventSlug
		return trade
	}
	review := reviews[item.DecisionID]
	trade.Side = "SELL"
	trade.MarketSlug = review.MarketSlug
	trade.EventSlug = review.EventSlug
	return trade
}

func MarkToMarket(snapshot types.PortfolioSnapshot, books map[string]types.OrderBook) types.PortfolioSnapshot {
	total := 0.0
	for i, pos := range snapshot.Positions {
		mark := pos.MarkPrice
		if book, ok := books[pos.TokenID]; ok && book.BestBid != nil && *book.BestBid > 0 {
			mark = *book.BestBid
		}
		pos.MarkPrice = round6(mark)
		pos.CostBasisUSD = round4(pos.Size * pos.AvgCost)
		pos.CurrentValueUSD = round4(pos.Size * pos.MarkPrice)
		pos.UnrealizedPnLUSD = round4(pos.CurrentValueUSD - pos.CostBasisUSD)
		if pos.CostBasisUSD > 0 {
			pos.UnrealizedPnLPct = round6(pos.UnrealizedPnLUSD / pos.CostBasisUSD)
		}
		pos.UpdatedAt = snapshot.SnapshotAt
		snapshot.Positions[i] = pos
		total += pos.CurrentValueUSD
	}
	snapshot.TotalPositionValueUSD = round4(total)
	snapshot.NavUSD = round4(snapshot.CashUSD + total)
	if snapshot.StartingNavUSD > 0 {
		snapshot.ReturnPct = round6((snapshot.NavUSD - snapshot.StartingNavUSD) / snapshot.StartingNavUSD)
	}
	return snapshot
}

func upsertBuy(positions []types.PaperPosition, trade types.PaperTrade) []types.PaperPosition {
	for i, pos := range positions {
		if pos.TokenID != trade.TokenID {
			continue
		}
		oldCost := pos.Size * pos.AvgCost
		newCost := trade.Shares * trade.Price
		newSize := pos.Size + trade.Shares
		pos.Size = round6(newSize)
		pos.AvgCost = round6((oldCost + newCost) / newSize)
		pos.UpdatedAt = trade.Timestamp
		positions[i] = pos
		return positions
	}
	return append(positions, types.PaperPosition{
		PositionID: fmt.Sprintf("pos_%s", trade.TokenID),
		MarketSlug: trade.MarketSlug,
		EventSlug:  trade.EventSlug,
		TokenID:    trade.TokenID,
		Outcome:    trade.Outcome,
		Size:       trade.Shares,
		AvgCost:    trade.Price,
		MarkPrice:  trade.Price,
		OpenedAt:   trade.Timestamp,
		UpdatedAt:  trade.Timestamp,
	})
}

func applySell(positions []types.PaperPosition, trade types.PaperTrade) []types.PaperPosition {
	for i, pos := range positions {
		if pos.TokenID != trade.TokenID {
			continue
		}
		sold := math.Min(pos.Size, trade.Shares)
		pos.Size = round6(pos.Size - sold)
		pos.RealizedPnLUSD = round4(pos.RealizedPnLUSD + sold*(trade.Price-pos.AvgCost))
		pos.UpdatedAt = trade.Timestamp
		if pos.Size <= 0.000001 {
			return append(positions[:i], positions[i+1:]...)
		}
		positions[i] = pos
		return positions
	}
	return positions
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func round6(value float64) float64 {
	return math.Round(value*1000000) / 1000000
}
