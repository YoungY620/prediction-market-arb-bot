package portfolio

import (
	"testing"

	"prediction-market-arb-bot/internal/types"
)

func TestApplyPaperFillsUpdatesCashPositionAndPnL(t *testing.T) {
	before := NewPaperPortfolio("cy", 100)
	fill := 0.5
	bid := 0.6
	proposal := types.Proposal{
		CycleID: "cy",
		NewOpportunities: []types.NewOpportunity{{
			DecisionID: "d1",
			EventSlug:  "ev",
			MarketSlug: "m",
			TokenID:    "tok",
			Outcome:    "Yes",
		}},
	}
	preview := types.ExecutionPreview{CycleID: "cy", Items: []types.ExecutionPreviewItem{{
		DecisionID:            "d1",
		Action:                "open",
		TokenID:               "tok",
		ExecutableNotionalUSD: 10,
		WouldExecute:          true,
		FillPrice:             &fill,
	}}}
	after, result := ApplyPaperFills("cy", before, proposal, preview, map[string]types.OrderBook{
		"tok": {TokenID: "tok", BestBid: &bid},
	})
	if len(result.Trades) != 1 {
		t.Fatal("expected one paper trade")
	}
	if after.CashUSD != 90 {
		t.Fatalf("expected cash 90, got %f", after.CashUSD)
	}
	if len(after.Positions) != 1 || after.Positions[0].AvgCost != 0.5 {
		t.Fatalf("unexpected position: %#v", after.Positions)
	}
	if after.Positions[0].UnrealizedPnLUSD != 2 {
		t.Fatalf("expected unrealized pnl 2, got %f", after.Positions[0].UnrealizedPnLUSD)
	}
}

func TestApplyPaperFillsClosesPositionAndRealizesPnL(t *testing.T) {
	before := NewPaperPortfolio("cy", 90)
	before.Positions = []types.PaperPosition{{
		PositionID:      "pos_tok",
		MarketSlug:      "m",
		EventSlug:       "ev",
		TokenID:         "tok",
		Outcome:         "Yes",
		Size:            20,
		AvgCost:         0.5,
		MarkPrice:       0.5,
		CostBasisUSD:    10,
		CurrentValueUSD: 10,
	}}
	before.TotalPositionValueUSD = 10
	before.NavUSD = 100
	fill := 0.6
	proposal := types.Proposal{
		CycleID: "cy",
		PositionReviews: []types.PositionReview{{
			DecisionID: "r1",
			EventSlug:  "ev",
			MarketSlug: "m",
			TokenID:    "tok",
			Outcome:    "Yes",
			Action:     "close",
		}},
	}
	preview := types.ExecutionPreview{CycleID: "cy", Items: []types.ExecutionPreviewItem{{
		DecisionID:            "r1",
		Action:                "close",
		TokenID:               "tok",
		ExecutableNotionalUSD: 12,
		WouldExecute:          true,
		FillPrice:             &fill,
	}}}
	after, result := ApplyPaperFills("cy", before, proposal, preview, map[string]types.OrderBook{})
	if len(result.Trades) != 1 || result.Trades[0].Side != "SELL" {
		t.Fatalf("expected one sell trade, got %#v", result.Trades)
	}
	if after.CashUSD != 102 {
		t.Fatalf("expected cash 102, got %f", after.CashUSD)
	}
	if len(after.Positions) != 0 {
		t.Fatalf("expected closed position, got %#v", after.Positions)
	}
}
