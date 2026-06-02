package validation

import (
	"testing"

	"prediction-market-arb-bot/internal/config"
	"prediction-market-arb-bot/internal/portfolio"
	"prediction-market-arb-bot/internal/types"
)

func TestParseProposalRejectsMarkdown(t *testing.T) {
	_, errors := ParseProposal("```json\n{}\n```")
	if len(errors) == 0 {
		t.Fatal("expected markdown wrapped JSON to fail")
	}
}

func TestValidateProposalRejectsBadEdgeAndMissingResolution(t *testing.T) {
	cfg := config.Load()
	cfg.EdgeThresholdPct = 0.05
	p := types.Proposal{
		SchemaVersion: "1",
		CycleID:       "cy",
		CreatedAt:     "now",
		NewOpportunities: []types.NewOpportunity{{
			DecisionID:        "d1",
			EventSlug:         "ev",
			MarketSlug:        "m",
			TokenID:           "tok_yes",
			Outcome:           "Yes",
			Action:            "open",
			AIProbability:     0.56,
			MarketProbability: 0.54,
			Edge:              0.10,
			Evidence: []types.Evidence{
				validEvidence("official", "direct"),
				validEvidence("news", "direct"),
			},
			CounterEvidence: []types.Evidence{validEvidence("news", "direct")},
			Order:           types.OrderIntent{Side: "BUY", OrderType: "FOK", ProposedNotionalUSD: 10},
		}},
	}
	candidates := []types.MarketCandidate{{
		EventSlug:     "ev",
		MarketSlug:    "m",
		Outcomes:      []string{"Yes", "No"},
		OutcomePrices: []float64{0.54, 0.46},
		TokenIDs:      []string{"tok_yes", "tok_no"},
	}}
	result := ValidateProposal(p, cfg, candidates, portfolio.NewPaperPortfolio("cy", 500))
	if result.Passed {
		t.Fatal("expected validation to fail")
	}
	if !hasCode(result.Errors, "incorrect_edge") || !hasCode(result.Errors, "missing_resolution_source") {
		t.Fatalf("expected edge and resolution failures, got %#v", result.Errors)
	}
}

func TestValidateProposalRejectsUnknownToken(t *testing.T) {
	cfg := config.Load()
	p := validProposal()
	result := ValidateProposal(p, cfg, nil, portfolio.NewPaperPortfolio("cy", 500))
	if result.Passed || !hasCode(result.Errors, "unknown_token_id") {
		t.Fatalf("expected unknown token failure, got %#v", result.Errors)
	}
}

func TestValidateProposalRejectsUnsupportedGTC(t *testing.T) {
	cfg := config.Load()
	p := validProposal()
	p.NewOpportunities[0].Order.OrderType = "GTC"
	candidates := []types.MarketCandidate{{
		EventSlug:     "ev",
		MarketSlug:    "m",
		Outcomes:      []string{"Yes", "No"},
		OutcomePrices: []float64{0.55, 0.45},
		TokenIDs:      []string{"tok_yes", "tok_no"},
	}}
	result := ValidateProposal(p, cfg, candidates, portfolio.NewPaperPortfolio("cy", 500))
	if result.Passed || !hasCode(result.Errors, "unsupported_gtc") {
		t.Fatalf("expected unsupported GTC failure, got %#v", result.Errors)
	}
}

func validProposal() types.Proposal {
	return types.Proposal{
		SchemaVersion: "1",
		CycleID:       "cy",
		CreatedAt:     "now",
		NewOpportunities: []types.NewOpportunity{{
			DecisionID:        "d1",
			EventSlug:         "ev",
			MarketSlug:        "m",
			TokenID:           "tok_yes",
			Outcome:           "Yes",
			Action:            "open",
			AIProbability:     0.65,
			MarketProbability: 0.55,
			Edge:              0.10,
			Resolution:        types.Resolution{SourceURL: "https://example.com", RuleExcerpt: "rule", TimeWindow: "2026", UnresolvedAmbiguities: []string{}},
			Evidence: []types.Evidence{
				validEvidence("official", "direct"),
				validEvidence("news", "direct"),
			},
			CounterEvidence: []types.Evidence{validEvidence("news", "direct")},
			Order:           types.OrderIntent{Side: "BUY", OrderType: "FOK", ProposedNotionalUSD: 10},
		}},
	}
}

func validEvidence(sourceType, relevance string) types.Evidence {
	return types.Evidence{
		SourceURL:         "https://example.com",
		SourceType:        sourceType,
		RetrievedAt:       "2026-06-03T00:00:00Z",
		Claim:             "claim",
		ContractRelevance: relevance,
		Supports:          "position",
	}
}

func hasCode(errors []types.ValidationError, code string) bool {
	for _, item := range errors {
		if item.Code == code {
			return true
		}
	}
	return false
}
