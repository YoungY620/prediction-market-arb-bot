package execution

import (
	"fmt"
	"math"

	"prediction-market-arb-bot/internal/config"
	"prediction-market-arb-bot/internal/types"
)

func BuildPreview(proposal types.Proposal, portfolio types.PortfolioSnapshot, books map[string]types.OrderBook, cfg config.Config) (types.ExecutionPreview, []types.ValidationError) {
	preview := types.ExecutionPreview{CycleID: proposal.CycleID}
	var errors []types.ValidationError
	for i, item := range proposal.NewOpportunities {
		if item.Action != "open" {
			continue
		}
		path := fmt.Sprintf("$.new_opportunities[%d]", i)
		book := books[item.TokenID]
		if book.BestAsk == nil || *book.BestAsk <= 0 {
			errors = append(errors, vErr("execution_preview_gate", "missing_best_ask", path+".token_id", "open requires best ask"))
			continue
		}
		kelly := quarterKelly(item.AIProbability, item.MarketProbability, portfolio.NavUSD)
		riskCap := math.Min(item.Order.ProposedNotionalUSD, kelly)
		riskCap = math.Min(riskCap, portfolio.NavUSD*cfg.MaxSingleTradePct)
		riskCap = math.Min(riskCap, portfolio.CashUSD)
		executable := riskCap
		var exchangeMin *float64
		if book.MinOrderSize != nil && *book.MinOrderSize > 0 {
			minNotional := *book.MinOrderSize * *book.BestAsk
			exchangeMin = &minNotional
			if executable < minNotional {
				executable = 0
			}
		}
		would := executable >= cfg.MinTradeUSD
		if !would {
			executable = 0
		}
		fill := *book.BestAsk
		preview.Items = append(preview.Items, types.ExecutionPreviewItem{
			DecisionID:             item.DecisionID,
			Action:                 item.Action,
			MarketSlug:             item.MarketSlug,
			TokenID:                item.TokenID,
			Outcome:                item.Outcome,
			ProposedNotionalUSD:    round4(item.Order.ProposedNotionalUSD),
			KellyNotionalUSD:       round4(kelly),
			RiskCappedNotionalUSD:  round4(riskCap),
			ExecutableNotionalUSD:  round4(executable),
			ExchangeMinNotionalUSD: exchangeMin,
			WouldExecute:           would,
			FillPrice:              &fill,
		})
	}
	for i, item := range proposal.PositionReviews {
		if item.Action != "reduce" && item.Action != "close" {
			continue
		}
		path := fmt.Sprintf("$.position_reviews[%d]", i)
		book := books[item.TokenID]
		if book.BestBid == nil || *book.BestBid <= 0 {
			errors = append(errors, vErr("execution_preview_gate", "missing_best_bid", path+".token_id", "reduce/close requires best bid"))
			continue
		}
		position := findPosition(portfolio.Positions, item.TokenID)
		if position == nil || position.Size <= 0 {
			errors = append(errors, vErr("execution_preview_gate", "missing_position", path+".token_id", "reduce/close requires existing paper position"))
			continue
		}
		shares := position.Size
		if item.Action == "reduce" {
			shares = position.Size / 2
		}
		notional := shares * *book.BestBid
		would := notional >= cfg.MinTradeUSD
		fill := *book.BestBid
		if !would {
			notional = 0
		}
		preview.Items = append(preview.Items, types.ExecutionPreviewItem{
			DecisionID:            item.DecisionID,
			Action:                item.Action,
			MarketSlug:            item.MarketSlug,
			TokenID:               item.TokenID,
			Outcome:               item.Outcome,
			ProposedNotionalUSD:   round4(position.CurrentValueUSD),
			KellyNotionalUSD:      0,
			RiskCappedNotionalUSD: round4(notional),
			ExecutableNotionalUSD: round4(notional),
			WouldExecute:          would,
			FillPrice:             &fill,
		})
	}
	errors = append(errors, consistencyGate(preview, cfg)...)
	return preview, errors
}

func consistencyGate(preview types.ExecutionPreview, cfg config.Config) []types.ValidationError {
	var errors []types.ValidationError
	for i, item := range preview.Items {
		path := fmt.Sprintf("$.execution_preview.items[%d]", i)
		if !item.WouldExecute {
			errors = append(errors, vErr("strategy_consistency_gate", "required_order_not_executable", path, "proposal item is not executable after preview"))
			continue
		}
		if item.ProposedNotionalUSD > 0 && item.ExecutableNotionalUSD/item.ProposedNotionalUSD < cfg.StrategyClipMinRatio {
			errors = append(errors, vErr("strategy_consistency_gate", "material_notional_clip", path+".executable_notional_usd", "executable notional materially differs from proposed notional"))
		}
	}
	return errors
}

func quarterKelly(aiProb, marketProb, bankroll float64) float64 {
	if marketProb <= 0 || marketProb >= 1 || aiProb <= marketProb {
		return 0
	}
	full := (aiProb - marketProb) / (1 - marketProb)
	return bankroll * full / 4
}

func findPosition(positions []types.PaperPosition, tokenID string) *types.PaperPosition {
	for i := range positions {
		if positions[i].TokenID == tokenID {
			return &positions[i]
		}
	}
	return nil
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func vErr(gate, code, path, message string) types.ValidationError {
	return types.ValidationError{Gate: gate, Code: code, Path: path, Message: message}
}
