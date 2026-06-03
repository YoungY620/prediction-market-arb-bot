package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"prediction-market-arb-bot/internal/types"
)

func TestAuditCycleAcceptsBroadPrescreenedDiscovery(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, filepath.Join(dir, "market-candidates.json"), []types.MarketCandidate{
		testCandidate("ev1", "m1"),
		testCandidate("ev2", "m2"),
	})
	writeTestJSON(t, filepath.Join(dir, "coordinator-prescreen.json"), coordinatorPreScreenReport{
		InputCount:    3,
		SelectedCount: 2,
		SkippedCount:  1,
		MaxCandidates: 0,
		MinCandidates: 24,
		Items: []coordinatorPreScreenItem{
			{EventSlug: "ev1", MarketSlug: "m1", Selected: true},
			{EventSlug: "ev2", MarketSlug: "m2", Selected: true},
			{EventSlug: "ev3", MarketSlug: "m3", Selected: false, Reasons: []string{"order_book_disabled"}},
		},
	})
	writeTestJSON(t, filepath.Join(dir, "market-data-health.json"), map[string]any{
		"candidate_count":               2,
		"discovered_candidate_count":    3,
		"smoke_candidate_limit_applied": false,
		"discovery_report": types.DiscoveryReport{
			RawEventCount:       12,
			RawMarketCount:      24,
			AcceptedMarketCount: 3,
			AcceptedEventCount:  3,
			FilteredByReason:    map[string]int{"order_book_disabled": 1},
			SortDimensions: []types.DiscoverySortReport{
				{Order: "volume24hr", ShortPage: true},
				{Order: "liquidity", ShortPage: true},
				{Order: "startDate", ShortPage: true},
				{Order: "competitive", ShortPage: true},
			},
		},
	})
	writeTestJSON(t, filepath.Join(dir, "data-source-health.json"), types.DataSourceHealth{Checks: []types.DataSourceCheck{
		{Name: "polymarket_gamma_events", Reachable: true},
		{Name: "polymarket_clob_book", Reachable: true},
		{Name: "web_research_http", Reachable: true},
	}})
	writeTestJSON(t, filepath.Join(dir, "research-plan.json"), types.ResearchPlan{
		CycleID: "cycle",
		Tasks: []types.ResearchTask{{
			TaskID:          "task-1",
			EventSlug:       "ev1",
			Domain:          "politics",
			ResearchProfile: "official and news",
			MarketSlugs:     []string{"m1"},
			Markets:         []types.MarketCandidate{testCandidate("ev1", "m1")},
		}, {
			TaskID:          "task-2",
			EventSlug:       "ev2",
			Domain:          "politics",
			ResearchProfile: "official and news",
			MarketSlugs:     []string{"m2"},
			Markets:         []types.MarketCandidate{testCandidate("ev2", "m2")},
		}},
	})
	writeTestJSON(t, filepath.Join(dir, "research-index.json"), types.ResearchIndex{
		CycleID:     "cycle",
		TaskCount:   2,
		PassedCount: 1,
		FailedCount: 1,
		Records: []types.WorkerRecord{
			{TaskID: "task-1", EventSlug: "ev1", Status: "passed", ResultPath: filepath.Join(dir, "worker-result.json")},
			{TaskID: "task-2", EventSlug: "ev2", Status: "failed", ResultPath: filepath.Join(dir, "worker-failed-result.json"), ValidationPath: filepath.Join(dir, "worker-validation.json")},
		},
	})
	writeTestJSON(t, filepath.Join(dir, "worker-result.json"), testResearchResult("task-1", "ev1", "m1"))
	writeTestJSON(t, filepath.Join(dir, "worker-failed-result.json"), types.ResearchResult{})
	writeTestJSON(t, filepath.Join(dir, "worker-validation.json"), types.ValidationResult{
		CycleID: "task-2",
		Passed:  false,
		Errors:  []types.ValidationError{{Gate: "worker_agent", Code: "agent_failed", Path: "$", Message: "timeout"}},
	})
	writeTestJSON(t, filepath.Join(dir, "proposal.json"), types.Proposal{
		SchemaVersion: "1",
		CycleID:       "cycle",
		CreatedAt:     "2026-06-03T00:00:00Z",
		NoTrade: &types.NoTradeReason{
			Reason:              "no executable edge",
			CandidateCount:      2,
			CheckedMarketSlugs:  []string{"m1", "m2"},
			MissingRequirements: []string{"positive edge"},
			DataHealth:          []string{"ok"},
		},
	})
	writeTestJSON(t, filepath.Join(dir, "validation-result.json"), types.ValidationResult{CycleID: "cycle", Passed: true})
	writeTestJSON(t, filepath.Join(dir, "execution-preview.json"), types.ExecutionPreview{})
	writeTestJSON(t, filepath.Join(dir, "post-portfolio.json"), types.PortfolioSnapshot{NavUSD: 100})

	report := auditCycle(dir, auditOptions{})
	if !allPassed(report.Checks) {
		t.Fatalf("expected audit checks to pass, got %#v", report.Checks)
	}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func allPassed(checks []auditCheck) bool {
	for _, check := range checks {
		if !check.Passed {
			return false
		}
	}
	return true
}

func testCandidate(eventSlug, marketSlug string) types.MarketCandidate {
	return types.MarketCandidate{
		EventSlug:       eventSlug,
		EventTitle:      "Event",
		MarketSlug:      marketSlug,
		Question:        "Will it happen?",
		Outcomes:        []string{"Yes", "No"},
		OutcomePrices:   []float64{0.6, 0.4},
		TokenIDs:        []string{"yes", "no"},
		LiquidityUSD:    10000,
		EnableOrderBook: true,
	}
}

func testResearchResult(taskID, eventSlug, marketSlug string) types.ResearchResult {
	return types.ResearchResult{
		TaskID:    taskID,
		EventSlug: eventSlug,
		Status:    "passed",
		SummarySignal: types.SummarySignal{
			Decision:    "skip",
			Confidence:  0.7,
			ReasonShort: "no edge",
		},
		MarketAssessments: []types.MarketAssessment{{
			MarketSlug:        marketSlug,
			TokenID:           "yes",
			Outcome:           "Yes",
			Recommendation:    "skip",
			AIProbability:     0.6,
			MarketProbability: 0.6,
			Edge:              0,
			Thesis:            "fair",
		}},
		Evidence: []types.Evidence{
			{SourceURL: "https://example.com/a", SourceType: "official", RetrievedAt: "2026-06-03T00:00:00Z", Claim: "official checked", ContractRelevance: "direct"},
			{SourceURL: "https://example.com/b", SourceType: "news", RetrievedAt: "2026-06-03T00:00:00Z", Claim: "news checked", ContractRelevance: "direct"},
		},
		CounterEvidence: []types.Evidence{
			{SourceURL: "https://example.com/c", SourceType: "news", RetrievedAt: "2026-06-03T00:00:00Z", Claim: "counter checked", ContractRelevance: "direct"},
		},
	}
}
