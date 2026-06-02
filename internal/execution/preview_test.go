package execution

import (
	"testing"

	"prediction-market-arb-bot/internal/config"
	"prediction-market-arb-bot/internal/portfolio"
	"prediction-market-arb-bot/internal/types"
)

func TestBuildPreviewRejectsMaterialClip(t *testing.T) {
	cfg := config.Load()
	cfg.StrategyClipMinRatio = 0.5
	p := types.Proposal{
		CycleID: "cy",
		NewOpportunities: []types.NewOpportunity{{
			DecisionID:        "d1",
			MarketSlug:        "m",
			TokenID:           "tok",
			Outcome:           "Yes",
			Action:            "open",
			AIProbability:     0.56,
			MarketProbability: 0.55,
			Order:             types.OrderIntent{ProposedNotionalUSD: 70},
		}},
	}
	ask := 0.55
	preview, errors := BuildPreview(p, portfolio.NewPaperPortfolio("cy", 500), map[string]types.OrderBook{
		"tok": {TokenID: "tok", BestAsk: &ask},
	}, cfg)
	if len(preview.Items) != 1 {
		t.Fatal("expected one preview item")
	}
	if len(errors) == 0 {
		t.Fatal("expected strategy consistency error")
	}
}
