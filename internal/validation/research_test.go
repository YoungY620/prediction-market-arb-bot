package validation

import (
	"testing"

	"prediction-market-arb-bot/internal/types"
)

func TestValidateResearchPlanRequiresEveryMarketAssigned(t *testing.T) {
	candidates := []types.MarketCandidate{
		researchCandidate("event-1", "market-1"),
		researchCandidate("event-1", "market-2"),
	}
	plan := types.ResearchPlan{
		CycleID: "cycle",
		Tasks: []types.ResearchTask{{
			TaskID:          "task-1",
			EventSlug:       "event-1",
			Domain:          "politics",
			ResearchProfile: "official and reputable news",
			MarketSlugs:     []string{"market-1"},
			Markets:         []types.MarketCandidate{researchCandidate("event-1", "market-1")},
		}},
	}
	result := ValidateResearchPlan(plan, "cycle", candidates)
	if result.Passed || !hasCode(result.Errors, "market_not_assigned") {
		t.Fatalf("expected market_not_assigned, got passed=%v errors=%#v", result.Passed, result.Errors)
	}
}

func TestParseResearchPlanToleratesCoordinatorFocusAndPriorityStrings(t *testing.T) {
	raw := `{"cycle_id":"cycle","created_at":"now","tasks":[{"task_id":"task-1","event_slug":"event-1","event_title":"Event","domain":"politics","research_profile":"official sources","priority":"high","research_focus":"check polling","market_slugs":["market-1"],"markets":[{"event_slug":"event-1","market_slug":"market-1","question":"Will it happen?","outcomes":["Yes","No"],"outcome_prices":[0.6,0.4],"token_ids":["yes","no"]}]}]}`
	plan, errors := ParseResearchPlan(raw)
	if len(errors) > 0 {
		t.Fatalf("expected flexible parse, got %#v", errors)
	}
	if plan.Tasks[0].Priority != 3 || len(plan.Tasks[0].ResearchFocus) != 1 || plan.Tasks[0].ResearchFocus[0] != "check polling" {
		t.Fatalf("unexpected parsed task: %#v", plan.Tasks[0])
	}
}

func TestValidateResearchPlanRequiresFullMarketDetails(t *testing.T) {
	candidates := []types.MarketCandidate{researchCandidate("event-1", "market-1")}
	plan := types.ResearchPlan{
		CycleID: "cycle",
		Tasks: []types.ResearchTask{{
			TaskID:          "task-1",
			EventSlug:       "event-1",
			Domain:          "politics",
			ResearchProfile: "official and reputable news",
			MarketSlugs:     []string{"market-1"},
			Markets:         []types.MarketCandidate{{MarketSlug: "market-1", Question: "Will it happen?"}},
		}},
	}
	result := ValidateResearchPlan(plan, "cycle", candidates)
	if result.Passed || !hasCode(result.Errors, "missing_market_token_ids") || !hasCode(result.Errors, "missing_market_outcomes") || !hasCode(result.Errors, "missing_market_prices") {
		t.Fatalf("expected missing market detail errors, got passed=%v errors=%#v", result.Passed, result.Errors)
	}
}

func TestValidateResearchResultAppliesDomainSpecificGate(t *testing.T) {
	task := types.ResearchTask{
		TaskID:      "task-1",
		EventSlug:   "event-1",
		Domain:      "sports",
		MarketSlugs: []string{"market-1"},
	}
	result := validResearchResult()
	result.SummarySignal.Decision = "open_candidate"
	result.Evidence = []types.Evidence{
		researchEvidence("reputable_news", "A generic preview says the team is strong."),
		researchEvidence("official", "The event exists."),
	}
	out := ValidateResearchResult(result, task)
	if out.Passed || !hasCode(out.Errors, "missing_domain_specific_evidence") {
		t.Fatalf("expected sports domain evidence failure, got passed=%v errors=%#v", out.Passed, out.Errors)
	}
	result.Evidence[0] = researchEvidence("bookmaker_odds", "Consensus odds moved after injury news.")
	out = ValidateResearchResult(result, task)
	if !out.Passed {
		t.Fatalf("expected sports evidence to pass, got errors=%#v", out.Errors)
	}
}

func TestValidateResearchResultAcceptsPoliticsNewsEvidence(t *testing.T) {
	task := types.ResearchTask{
		TaskID:      "task-1",
		EventSlug:   "event-1",
		Domain:      "geopolitics",
		MarketSlugs: []string{"market-1"},
		Markets:     []types.MarketCandidate{researchCandidate("event-1", "market-1")},
	}
	result := validResearchResult()
	result.SummarySignal.Decision = "open_candidate"
	result.Evidence = []types.Evidence{
		researchEvidence("news", "A wire report says maritime traffic remains below the market threshold."),
		researchEvidence("news", "Independent reporting says shipping lanes remain restricted."),
	}
	out := ValidateResearchResult(result, task)
	if !out.Passed {
		t.Fatalf("expected politics news evidence to pass, got errors=%#v", out.Errors)
	}
}

func TestValidateResearchResultRejectsOpenSummaryWithoutExecutableBinding(t *testing.T) {
	task := types.ResearchTask{
		TaskID:      "task-1",
		EventSlug:   "event-1",
		Domain:      "politics",
		MarketSlugs: []string{"market-1"},
		Markets:     []types.MarketCandidate{researchCandidate("event-1", "market-1")},
	}
	result := validResearchResult()
	result.SummarySignal.Decision = "open_candidate"
	result.SummarySignal.MaxEdge = 0
	result.SummarySignal.Confidence = 0
	result.SummarySignal.MarketSlugsOfInterest = nil
	result.MarketAssessments[0].TokenID = ""
	out := ValidateResearchResult(result, task)
	for _, code := range []string{"missing_positive_max_edge", "missing_positive_confidence", "missing_market_slugs_of_interest", "missing_token_id"} {
		if !hasCode(out.Errors, code) {
			t.Fatalf("expected %s in errors, got %#v", code, out.Errors)
		}
	}
}

func TestValidateResearchResultRejectsOpenMarketBindingMismatch(t *testing.T) {
	task := types.ResearchTask{
		TaskID:      "task-1",
		EventSlug:   "event-1",
		Domain:      "politics",
		MarketSlugs: []string{"market-1"},
		Markets:     []types.MarketCandidate{researchCandidate("event-1", "market-1")},
	}
	result := validResearchResult()
	result.SummarySignal.Decision = "open_candidate"
	result.MarketAssessments[0].TokenID = "no-token"
	result.MarketAssessments[0].Outcome = "Yes"
	result.MarketAssessments[0].MarketProbability = 0.45
	result.MarketAssessments[0].AIProbability = 0.53
	result.MarketAssessments[0].Edge = 0.08
	out := ValidateResearchResult(result, task)
	for _, code := range []string{"token_outcome_mismatch", "market_probability_mismatch"} {
		if !hasCode(out.Errors, code) {
			t.Fatalf("expected %s in errors, got %#v", code, out.Errors)
		}
	}
}

func TestParseResearchResultNormalizesCommonKimiAliases(t *testing.T) {
	raw := `{
		"task_id":"task-1",
		"event_slug":"event-1",
		"status":"completed",
		"summary_signal":{"decision":"hold","confidence":"0.72","rationale":"close to fair value"},
		"market_assessments":[{"market_slug":"market-1","token_id":"token-1","outcome":"Yes","recommendation":"no_trade","ai_probability":"0.6","market_probability":"0.6","edge":"0","thesis":"fair"}],
		"evidence":[
			{"source_url":"https://example.com/a","source_type":"official","retrieved_at":"2026-06-03T00:00:00Z","claim":"official context checked","contract_relevance":"direct","supports":true,"probability_effect_pp":"0"},
			{"source_url":"https://example.com/b","source_type":"reputable_news","retrieved_at":"2026-06-03T00:00:00Z","claim":"news context checked","contract_relevance":"direct","supports":"market-1","probability_effect_pp":0}
		],
		"counter_evidence":[{"source_url":"https://example.com/c","source_type":"reputable_news","retrieved_at":"2026-06-03T00:00:00Z","claim":"counter checked","contract_relevance":"direct","supports":"market-1","probability_effect_pp":0}],
		"unresolved_ambiguities":[],
		"follow_up_requests":[]
	}`
	result, errors := ParseResearchResult(raw)
	if len(errors) > 0 {
		t.Fatalf("expected parse success, got %#v", errors)
	}
	if result.Status != "passed" || result.SummarySignal.Decision != "watchlist" || result.SummarySignal.ReasonShort != "close to fair value" || result.SummarySignal.Confidence != 0.72 || result.Evidence[0].Supports != "true" || result.MarketAssessments[0].Recommendation != "skip" {
		t.Fatalf("unexpected normalized result: %#v", result)
	}
}

func validResearchResult() types.ResearchResult {
	return types.ResearchResult{
		TaskID:    "task-1",
		EventSlug: "event-1",
		Status:    "passed",
		SummarySignal: types.SummarySignal{
			Decision:              "watchlist",
			MaxEdge:               0.08,
			Confidence:            0.7,
			MarketSlugsOfInterest: []string{"market-1"},
			ReasonShort:           "interesting but not yet decisive",
		},
		MarketAssessments: []types.MarketAssessment{{
			MarketSlug:        "market-1",
			TokenID:           "token-1",
			Outcome:           "Yes",
			Recommendation:    "open_candidate",
			AIProbability:     0.68,
			MarketProbability: 0.60,
			Edge:              0.08,
			Thesis:            "evidence supports a mispriced yes outcome",
		}},
		Evidence:        []types.Evidence{researchEvidence("official", "Official source supports the contract-relevant claim."), researchEvidence("reputable_news", "Independent reporting supports the same claim.")},
		CounterEvidence: []types.Evidence{researchEvidence("reputable_news", "A plausible counter scenario remains possible.")},
	}
}

func researchCandidate(eventSlug, marketSlug string) types.MarketCandidate {
	return types.MarketCandidate{
		EventSlug:       eventSlug,
		EventTitle:      "Event",
		MarketSlug:      marketSlug,
		Question:        "Will it happen?",
		Outcomes:        []string{"Yes", "No"},
		OutcomePrices:   []float64{0.6, 0.4},
		TokenIDs:        []string{"token-1", "no-token"},
		LiquidityUSD:    10000,
		EnableOrderBook: true,
	}
}

func researchEvidence(sourceType, claim string) types.Evidence {
	return types.Evidence{
		SourceURL:         "https://example.com/source",
		SourceType:        sourceType,
		RetrievedAt:       "2026-06-03T00:00:00Z",
		Claim:             claim,
		ContractRelevance: "direct",
		Supports:          "market-1",
		ProbabilityEffect: 0.02,
	}
}
