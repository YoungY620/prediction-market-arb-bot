package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"prediction-market-arb-bot/internal/config"
	"prediction-market-arb-bot/internal/marketdata"
	"prediction-market-arb-bot/internal/types"
)

type testAgent struct {
	response  string
	responses []string
	errors    []error
	delay     time.Duration
	mu        sync.Mutex
	i         int
}

func (a *testAgent) Run(ctx context.Context, _ string) (string, error) {
	if a.delay > 0 {
		select {
		case <-time.After(a.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.errors) > 0 && a.i < len(a.errors) && a.errors[a.i] != nil {
		err := a.errors[a.i]
		a.i++
		return "", err
	}
	if len(a.responses) > 0 {
		if a.i >= len(a.responses) {
			return a.responses[len(a.responses)-1], nil
		}
		response := a.responses[a.i]
		a.i++
		return response, nil
	}
	return a.response, nil
}

type testSessionAgent struct {
	responses []string
	calls     []string
	i         int
}

func (a *testSessionAgent) Run(ctx context.Context, prompt string) (string, error) {
	run, err := a.RunSession(ctx, prompt)
	return run.Output, err
}

func (a *testSessionAgent) RunSession(_ context.Context, _ string) (AgentRun, error) {
	a.calls = append(a.calls, "fresh")
	response := a.responses[min(a.i, len(a.responses)-1)]
	a.i++
	return AgentRun{Output: response, SessionID: "session-1"}, nil
}

func (a *testSessionAgent) ResumeSession(_ context.Context, sessionID, _ string) (AgentRun, error) {
	a.calls = append(a.calls, "resume:"+sessionID)
	response := a.responses[min(a.i, len(a.responses)-1)]
	a.i++
	return AgentRun{Output: response, SessionID: sessionID}, nil
}

func TestValidationFailureDoesNotWriteExecutionArtifacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path != "/events" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	dir := t.TempDir()
	cfg := config.Load()
	cfg.RuntimeDir = dir
	cfg.GammaBaseURL = server.URL
	cfg.WebHealthURL = server.URL + "/health"
	cfg.MaxRepairAttempts = 0
	cfg.LLMJudgeEnabled = false
	_, err := RunPaperCycle(context.Background(), cfg, &testAgent{response: "not json"})
	if err == nil {
		t.Fatal("expected failed cycle")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "paper", "portfolio.json")); !os.IsNotExist(statErr) {
		t.Fatalf("paper portfolio should not be written after validation failure, stat err=%v", statErr)
	}
	matches, globErr := filepath.Glob(filepath.Join(dir, "cycles", "*", "error.json"))
	if globErr != nil || len(matches) != 1 {
		t.Fatalf("expected one error artifact, matches=%v err=%v", matches, globErr)
	}
	healthMatches, healthGlobErr := filepath.Glob(filepath.Join(dir, "cycles", "*", "data-source-health.json"))
	if healthGlobErr != nil || len(healthMatches) != 1 {
		t.Fatalf("expected data source health artifact, matches=%v err=%v", healthMatches, healthGlobErr)
	}
}

func TestCoordinatorMustAssignEveryCandidateMarket(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/events":
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("offset") != "0" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[
				{"slug":"ev1","title":"event 1","tags":[{"slug":"politics","label":"Politics"}],"markets":[
					{"slug":"m1","question":"q1","enableOrderBook":true,"liquidityNum":10000,"volume24hr":100,"outcomes":"[\"Yes\",\"No\"]","outcomePrices":"[\"0.6\",\"0.4\"]","clobTokenIds":"[\"tok1\",\"tok2\"]"},
					{"slug":"m2","question":"q2","enableOrderBook":true,"liquidityNum":10000,"volume24hr":100,"outcomes":"[\"Yes\",\"No\"]","outcomePrices":"[\"0.7\",\"0.3\"]","clobTokenIds":"[\"tok3\",\"tok4\"]"}
				]}
			]`))
		case "/book":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"bids":[{"price":"0.5","size":"100"}],"asks":[{"price":"0.6","size":"100"}]}`))
		case "/health":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	cfg := config.Load()
	cfg.RuntimeDir = dir
	cfg.GammaBaseURL = server.URL
	cfg.CLOBBaseURL = server.URL
	cfg.WebHealthURL = server.URL + "/health"
	cfg.EventPagesPerSort = 1
	cfg.CoordinatorRepairAttempts = 0
	cfg.LLMJudgeEnabled = false
	agent := &testAgent{response: fmt.Sprintf(`{
		"cycle_id":"cycle",
		"created_at":"2026-06-03T00:00:00Z",
		"tasks":[{
			"task_id":"task-1",
			"event_slug":"ev1",
			"event_title":"event 1",
			"domain":"politics",
			"research_profile":"official and reputable news",
			"priority":1,
			"research_focus":["full event"],
			"market_slugs":["m1"],
			"markets":[%s]
		}]
	}`, `{"event_slug":"ev1","event_title":"event 1","market_slug":"m1","question":"q1","outcomes":["Yes","No"],"outcome_prices":[0.6,0.4],"token_ids":["tok1","tok2"],"liquidity_usd":10000,"enable_order_book":true}`)}
	_, err := RunPaperCycle(context.Background(), cfg, agent)
	if err == nil {
		t.Fatal("expected coordinator gate failure")
	}
	matches, globErr := filepath.Glob(filepath.Join(dir, "cycles", "*", "coordinator", "validation-result.json"))
	if globErr != nil || len(matches) != 1 {
		t.Fatalf("expected coordinator validation artifact, matches=%v err=%v", matches, globErr)
	}
	data, readErr := os.ReadFile(matches[0])
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(data), "market_not_assigned") {
		t.Fatalf("expected market_not_assigned in validation artifact: %s", data)
	}
}

func TestPaperCycleWritesArtifactsForNoTradeResearchPath(t *testing.T) {
	server := eventServer()
	defer server.Close()

	dir := t.TempDir()
	cfg := config.Load()
	cfg.RuntimeDir = dir
	cfg.GammaBaseURL = server.URL
	cfg.CLOBBaseURL = server.URL
	cfg.WebHealthURL = server.URL + "/health"
	cfg.EventPagesPerSort = 1
	cfg.CoordinatorRepairAttempts = 0
	cfg.WorkerRepairAttempts = 0
	cfg.ReducerRepairAttempts = 0
	cfg.LLMJudgeEnabled = false
	agent := &testAgent{responses: []string{
		`{
			"cycle_id":"cycle",
			"created_at":"2026-06-03T00:00:00Z",
			"tasks":[{
				"task_id":"task-ev1",
				"event_slug":"ev1",
				"event_title":"event 1",
				"domain":"politics",
				"research_profile":"official and reputable news",
				"priority":1,
				"research_focus":["full event"],
				"market_slugs":["m1"],
				"markets":[{"event_slug":"ev1","event_title":"event 1","market_slug":"m1","question":"q1","outcomes":["Yes","No"],"outcome_prices":[0.6,0.4],"token_ids":["tok1","tok2"],"liquidity_usd":10000,"enable_order_book":true}]
			}]
		}`,
		`{
			"task_id":"task-ev1",
			"event_slug":"ev1",
			"status":"passed",
			"summary_signal":{"decision":"skip","max_edge":0,"confidence":0.7,"market_slugs_of_interest":[],"position_actions_of_interest":[],"reason_short":"no durable edge"},
			"market_assessments":[{"market_slug":"m1","token_id":"tok1","outcome":"Yes","recommendation":"skip","ai_probability":0.6,"market_probability":0.6,"edge":0,"thesis":"market appears fairly priced"}],
			"evidence":[
				{"source_url":"https://example.com/a","source_type":"official","retrieved_at":"2026-06-03T00:00:00Z","claim":"official context checked","contract_relevance":"direct","supports":"m1","probability_effect_pp":0},
				{"source_url":"https://example.com/b","source_type":"reputable_news","retrieved_at":"2026-06-03T00:00:00Z","claim":"news context checked","contract_relevance":"direct","supports":"m1","probability_effect_pp":0}
			],
			"counter_evidence":[{"source_url":"https://example.com/c","source_type":"reputable_news","retrieved_at":"2026-06-03T00:00:00Z","claim":"counter scenario checked","contract_relevance":"direct","supports":"m1","probability_effect_pp":0}],
			"unresolved_ambiguities":[],
			"follow_up_requests":[]
		}`,
		`{
			"schema_version":"1",
			"cycle_id":"cycle",
			"created_at":"2026-06-03T00:00:00Z",
			"position_reviews":[],
			"new_opportunities":[],
			"no_trade":{"reason":"No passed worker result found a positive edge above threshold.","candidate_count":1,"checked_market_slugs":["m1"],"missing_requirements":["positive edge above threshold"],"data_health":["gamma events reachable","web health reachable"]}
		}`,
	}}
	result, err := RunPaperCycle(context.Background(), cfg, agent)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(result.CycleDir, "market-data-health.json"),
		filepath.Join(result.CycleDir, "data-source-health.json"),
		filepath.Join(result.CycleDir, "research-plan.json"),
		filepath.Join(result.CycleDir, "research-index.json"),
		filepath.Join(result.CycleDir, "proposal.json"),
		filepath.Join(result.CycleDir, "execution-preview.json"),
		filepath.Join(result.CycleDir, "post-portfolio.json"),
		filepath.Join(dir, "paper", "portfolio.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s: %v", path, err)
		}
	}
}

func TestWorkerAgentTimeoutRecordsFailedWorker(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load()
	cfg.WorkerAgentTimeout = 10 * time.Millisecond
	task := types.ResearchTask{
		TaskID:          "slow-task",
		EventSlug:       "event-1",
		Domain:          "politics",
		ResearchProfile: "official",
		MarketSlugs:     []string{"market-1"},
		Markets:         []types.MarketCandidate{researchRuntimeCandidate("event-1", "market-1")},
	}
	record, _ := runOneWorker(context.Background(), &testAgent{delay: time.Second, response: `{}`}, cfg, "cycle", dir, task, 0)
	if record.Status != "failed" || len(record.Errors) == 0 || record.Errors[0].Code != "agent_failed" {
		t.Fatalf("expected failed timeout record, got %#v", record)
	}
	if _, err := os.Stat(record.ValidationPath); err != nil {
		t.Fatalf("expected validation artifact: %v", err)
	}
}

func TestWorkerRetriesAfterAgentFailure(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load()
	cfg.WorkerRepairAttempts = 1
	task := types.ResearchTask{
		TaskID:          "retry-task",
		EventSlug:       "event-1",
		Domain:          "politics",
		ResearchProfile: "official",
		MarketSlugs:     []string{"market-1"},
		Markets:         []types.MarketCandidate{researchRuntimeCandidate("event-1", "market-1")},
	}
	agent := &testAgent{
		errors: []error{errors.New("signal: killed"), nil},
		responses: []string{`{
			"task_id":"retry-task",
			"event_slug":"event-1",
			"status":"passed",
			"summary_signal":{"decision":"skip","max_edge":0,"confidence":0.7,"market_slugs_of_interest":[],"position_actions_of_interest":[],"reason_short":"no durable edge"},
			"market_assessments":[{"market_slug":"market-1","token_id":"yes","outcome":"Yes","recommendation":"skip","ai_probability":0.6,"market_probability":0.6,"edge":0,"thesis":"fair"}],
			"evidence":[
				{"source_url":"https://example.com/a","source_type":"official","retrieved_at":"2026-06-03T00:00:00Z","claim":"official context checked","contract_relevance":"direct","supports":"market-1","probability_effect_pp":0},
				{"source_url":"https://example.com/b","source_type":"reputable_news","retrieved_at":"2026-06-03T00:00:00Z","claim":"news context checked","contract_relevance":"direct","supports":"market-1","probability_effect_pp":0}
			],
			"counter_evidence":[{"source_url":"https://example.com/c","source_type":"reputable_news","retrieved_at":"2026-06-03T00:00:00Z","claim":"counter checked","contract_relevance":"direct","supports":"market-1","probability_effect_pp":0}],
			"unresolved_ambiguities":[],
			"follow_up_requests":[]
		}`},
	}
	record, _ := runOneWorker(context.Background(), agent, cfg, "cycle", dir, task, 0)
	if record.Status != "passed" || len(record.Errors) != 0 {
		t.Fatalf("expected worker to pass after retry, got %#v", record)
	}
	attempts, err := os.ReadFile(filepath.Join(dir, "depth-0", "attempts", "retry-task.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(attempts), "\n") != 2 {
		t.Fatalf("expected two attempts, got %s", attempts)
	}
}

func TestWorkerResumesSessionForValidationRepair(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Load()
	cfg.WorkerRepairAttempts = 1
	task := types.ResearchTask{
		TaskID:          "repair-task",
		EventSlug:       "event-1",
		Domain:          "politics",
		ResearchProfile: "official",
		MarketSlugs:     []string{"market-1"},
		Markets:         []types.MarketCandidate{researchRuntimeCandidate("event-1", "market-1")},
	}
	agent := &testSessionAgent{responses: []string{
		`{
			"task_id":"repair-task",
			"event_slug":"event-1",
			"status":"passed",
			"summary_signal":{"decision":"open_candidate","max_edge":0,"confidence":0,"market_slugs_of_interest":[],"reason_short":"broken open"},
			"market_assessments":[{"market_slug":"market-1","token_id":"","outcome":"Yes","recommendation":"open_candidate","ai_probability":0.6,"market_probability":0.6,"edge":0,"thesis":"missing token"}],
			"evidence":[
				{"source_url":"https://example.com/a","source_type":"official","retrieved_at":"2026-06-03T00:00:00Z","claim":"official context checked","contract_relevance":"direct","supports":"market-1","probability_effect_pp":0},
				{"source_url":"https://example.com/b","source_type":"news","retrieved_at":"2026-06-03T00:00:00Z","claim":"news context checked","contract_relevance":"direct","supports":"market-1","probability_effect_pp":0}
			],
			"counter_evidence":[{"source_url":"https://example.com/c","source_type":"news","retrieved_at":"2026-06-03T00:00:00Z","claim":"counter checked","contract_relevance":"direct","supports":"market-1","probability_effect_pp":0}],
			"unresolved_ambiguities":[],
			"follow_up_requests":[]
		}`,
		`{
			"task_id":"repair-task",
			"event_slug":"event-1",
			"status":"passed",
			"summary_signal":{"decision":"skip","max_edge":0,"confidence":0.7,"market_slugs_of_interest":[],"reason_short":"no executable edge after repair"},
			"market_assessments":[{"market_slug":"market-1","token_id":"yes","outcome":"Yes","recommendation":"skip","ai_probability":0.6,"market_probability":0.6,"edge":0,"thesis":"fair"}],
			"evidence":[
				{"source_url":"https://example.com/a","source_type":"official","retrieved_at":"2026-06-03T00:00:00Z","claim":"official context checked","contract_relevance":"direct","supports":"market-1","probability_effect_pp":0},
				{"source_url":"https://example.com/b","source_type":"news","retrieved_at":"2026-06-03T00:00:00Z","claim":"news context checked","contract_relevance":"direct","supports":"market-1","probability_effect_pp":0}
			],
			"counter_evidence":[{"source_url":"https://example.com/c","source_type":"news","retrieved_at":"2026-06-03T00:00:00Z","claim":"counter checked","contract_relevance":"direct","supports":"market-1","probability_effect_pp":0}],
			"unresolved_ambiguities":[],
			"follow_up_requests":[]
		}`,
	}}
	record, _ := runOneWorker(context.Background(), agent, cfg, "cycle", dir, task, 0)
	if record.Status != "passed" {
		t.Fatalf("expected repaired worker to pass, got %#v", record)
	}
	if got, want := strings.Join(agent.calls, ","), "fresh,resume:session-1"; got != want {
		t.Fatalf("expected session resume repair, got %s", got)
	}
}

func TestCoordinatorPrescreenKeepsBroadCoverage(t *testing.T) {
	cfg := config.Load()
	selected, report := preScreenCoordinatorCandidates([]types.MarketCandidate{
		researchRuntimeCandidate("ev1", "high-a"),
		{EventSlug: "ev2", MarketSlug: "not-executable", Question: "Will it happen?", Outcomes: []string{"Yes", "No"}, OutcomePrices: []float64{0.5, 0.5}, EnableOrderBook: false, LiquidityUSD: 10000, Score: 100},
		researchRuntimeCandidate("ev3", "high-b"),
	}, cfg)
	if len(selected) != 2 {
		t.Fatalf("expected broad selected set minus non-executable candidate, got %#v report=%#v", selected, report)
	}
	for _, item := range selected {
		if item.MarketSlug == "not-executable" {
			t.Fatalf("non-executable candidate should not be sent to coordinator: %#v", report)
		}
	}
	if report.InputCount != 3 || report.SelectedCount != 2 || report.SkippedCount != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestReducerResumesSessionForValidationRepair(t *testing.T) {
	cycleDir := t.TempDir()
	cfg := config.Load()
	cfg.ReducerRepairAttempts = 1
	cfg.LLMJudgeEnabled = false
	candidate := researchRuntimeCandidate("event-1", "market-1")
	agent := &testSessionAgent{responses: []string{
		`{
			"schema_version":"1",
			"cycle_id":"cycle",
			"created_at":"2026-06-03T00:00:00Z",
			"position_reviews":[],
			"new_opportunities":[]
		}`,
		`{
			"schema_version":"1",
			"cycle_id":"cycle",
			"created_at":"2026-06-03T00:00:00Z",
			"position_reviews":[],
			"new_opportunities":[],
			"no_trade":{"reason":"No executable edge after reducer validation repair.","candidate_count":1,"checked_market_slugs":["market-1"],"missing_requirements":["risk-adjusted open decision"],"data_health":["ok"]}
		}`,
	}}
	proposal, result, _, _, err := runReducer(
		context.Background(),
		agent,
		cfg,
		"cycle",
		cycleDir,
		map[string]any{"cycle_id": "cycle"},
		[]types.MarketCandidate{candidate},
		types.PortfolioSnapshot{StartingNavUSD: 500, CashUSD: 500, NavUSD: 500},
		eventMarketClient(t),
		types.ResearchIndex{CycleID: "cycle", TaskCount: 1, PassedCount: 1, Records: []types.WorkerRecord{{TaskID: "task-1", Status: "passed"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed || proposal.NoTrade == nil {
		t.Fatalf("expected repaired reducer no_trade to pass, result=%#v proposal=%#v", result, proposal)
	}
	if got, want := strings.Join(agent.calls, ","), "fresh,resume:session-1"; got != want {
		t.Fatalf("expected reducer session resume repair, got %s", got)
	}
	attempts, err := os.ReadFile(filepath.Join(cycleDir, "reducer", "attempts.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(attempts), `"status":"validation_failed"`) || !strings.Contains(string(attempts), `"resumed":true`) {
		t.Fatalf("expected reducer attempts to record validation failure and resume: %s", attempts)
	}
}

func TestReducerResearchHealthGateRejectsHighWorkerFailureRate(t *testing.T) {
	cfg := config.Load()
	cfg.MaxWorkerFailureRate = 0.5
	errors := reducerResearchHealthGate(types.ResearchIndex{TaskCount: 3, PassedCount: 1, FailedCount: 2}, cfg)
	if len(errors) != 1 || errors[0].Code != "worker_failure_rate_exceeded" {
		t.Fatalf("expected worker_failure_rate_exceeded, got %#v", errors)
	}
	errors = reducerResearchHealthGate(types.ResearchIndex{TaskCount: 4, PassedCount: 2, FailedCount: 2}, cfg)
	if len(errors) != 0 {
		t.Fatalf("expected threshold boundary to pass, got %#v", errors)
	}
}

func eventMarketClient(t *testing.T) marketdata.Client {
	t.Helper()
	server := eventServer()
	t.Cleanup(server.Close)
	cfg := config.Load()
	cfg.GammaBaseURL = server.URL
	cfg.CLOBBaseURL = server.URL
	return marketdata.New(cfg)
}

func researchRuntimeCandidate(eventSlug, marketSlug string) types.MarketCandidate {
	return types.MarketCandidate{
		EventSlug:       eventSlug,
		MarketSlug:      marketSlug,
		Question:        "Will it happen?",
		Outcomes:        []string{"Yes", "No"},
		OutcomePrices:   []float64{0.6, 0.4},
		TokenIDs:        []string{"yes", "no"},
		EnableOrderBook: true,
		LiquidityUSD:    10000,
	}
}

func eventServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/events":
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("offset") != "0" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[
				{"slug":"ev1","title":"event 1","tags":[{"slug":"politics","label":"Politics"}],"markets":[
					{"slug":"m1","question":"q1","enableOrderBook":true,"liquidityNum":10000,"volume24hr":100,"outcomes":"[\"Yes\",\"No\"]","outcomePrices":"[\"0.6\",\"0.4\"]","clobTokenIds":"[\"tok1\",\"tok2\"]"}
				]}
			]`))
		case "/book":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"bids":[{"price":"0.5","size":"100"}],"asks":[{"price":"0.6","size":"100"}]}`))
		case "/health":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
}
