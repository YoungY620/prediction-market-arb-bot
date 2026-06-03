package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"prediction-market-arb-bot/internal/config"
	"prediction-market-arb-bot/internal/execution"
	"prediction-market-arb-bot/internal/judge"
	"prediction-market-arb-bot/internal/marketdata"
	"prediction-market-arb-bot/internal/portfolio"
	"prediction-market-arb-bot/internal/types"
	"prediction-market-arb-bot/internal/validation"

	"golang.org/x/sync/errgroup"
)

type Agent interface {
	Run(ctx context.Context, prompt string) (string, error)
}

type AgentRun = types.AgentRun

type sessionAgent interface {
	RunSession(ctx context.Context, prompt string) (types.AgentRun, error)
	ResumeSession(ctx context.Context, sessionID, prompt string) (types.AgentRun, error)
}

type CycleResult struct {
	CycleID  string
	CycleDir string
}

type coordinatorPreScreenReport struct {
	InputCount    int                        `json:"input_count"`
	SelectedCount int                        `json:"selected_count"`
	SkippedCount  int                        `json:"skipped_count"`
	MaxCandidates int                        `json:"max_candidates,omitempty"`
	MinCandidates int                        `json:"min_candidates,omitempty"`
	Items         []coordinatorPreScreenItem `json:"items"`
}

type coordinatorPreScreenItem struct {
	EventSlug  string   `json:"event_slug"`
	MarketSlug string   `json:"market_slug"`
	Selected   bool     `json:"selected"`
	Score      float64  `json:"score"`
	Reasons    []string `json:"reasons"`
}

func RunPaperCycle(ctx context.Context, cfg config.Config, agent Agent) (CycleResult, error) {
	cycleID := time.Now().UTC().Format("20060102T150405Z")
	cycleDir := filepath.Join(cfg.RuntimeDir, "cycles", cycleID)
	if err := os.MkdirAll(cycleDir, 0o755); err != nil {
		return CycleResult{}, err
	}

	market := marketdata.New(cfg)
	candidates, discoveryReport, err := market.FetchCandidateSet(ctx)
	if err != nil {
		writeJSON(filepath.Join(cycleDir, "error.json"), map[string]any{"code": "market_data_failed", "message": err.Error(), "partial_discovery_report": discoveryReport})
		_ = writeJSON(filepath.Join(cycleDir, "market-data-health.json"), marketDataHealth(candidates, discoveryReport, cfg, len(candidates)))
		return CycleResult{}, err
	}
	discoveredCandidateCount := len(candidates)
	candidates = limitCandidatesForSmoke(candidates, cfg)
	preScreenedCandidates, preScreenReport := preScreenCoordinatorCandidates(candidates, cfg)
	if err := writeJSON(filepath.Join(cycleDir, "coordinator-prescreen.json"), preScreenReport); err != nil {
		return CycleResult{}, err
	}
	candidates = preScreenedCandidates
	candidateFile := filepath.Join(cycleDir, "market-candidates.json")
	if err := writeJSON(candidateFile, compactCandidates(candidates)); err != nil {
		return CycleResult{}, err
	}
	if err := writeJSON(filepath.Join(cycleDir, "market-data-health.json"), marketDataHealth(candidates, discoveryReport, cfg, discoveredCandidateCount)); err != nil {
		return CycleResult{}, err
	}
	dataSourceHealth := checkDataSources(ctx, cfg, market, candidates)
	if err := writeJSON(filepath.Join(cycleDir, "data-source-health.json"), dataSourceHealth); err != nil {
		return CycleResult{}, err
	}
	absCandidateFile, err := filepath.Abs(candidateFile)
	if err != nil {
		return CycleResult{}, err
	}
	current := loadOrInitPortfolio(cfg, cycleID)
	contextPayload := map[string]any{
		"cycle_id":                    cycleID,
		"portfolio":                   current,
		"market_candidate_count":      len(candidates),
		"discovered_candidate_count":  discoveredCandidateCount,
		"market_candidates_file":      absCandidateFile,
		"market_candidate_sample":     firstN(compactCandidates(candidates), 20),
		"discovery_report":            discoveryReport,
		"data_source_health":          dataSourceHealth,
		"polymarket_public_api_guide": polymarketAPIGuide(cfg),
	}
	if err := writeJSON(filepath.Join(cycleDir, "agent-context.json"), contextPayload); err != nil {
		return CycleResult{}, err
	}

	plan, coordinatorResult, err := runCoordinator(ctx, agent, cfg, cycleID, cycleDir, contextPayload, candidates)
	if err != nil {
		return CycleResult{}, err
	}
	if !coordinatorResult.Passed {
		return failCycle(cycleDir, cycleID, coordinatorResult.Errors)
	}
	index, err := runResearchWorkers(ctx, agent, cfg, cycleID, cycleDir, plan)
	if err != nil {
		return CycleResult{}, err
	}
	proposal, result, preview, books, err := runReducer(ctx, agent, cfg, cycleID, cycleDir, contextPayload, candidates, current, market, index)
	if err != nil {
		return CycleResult{}, err
	}
	if !result.Passed {
		_ = writeJSON(filepath.Join(cycleDir, "proposal.json"), proposal)
		_ = writeJSON(filepath.Join(cycleDir, "validation-result.json"), result)
		return failCycle(cycleDir, cycleID, result.Errors)
	}

	if err := writeJSON(filepath.Join(cycleDir, "proposal.json"), proposal); err != nil {
		return CycleResult{}, err
	}
	if err := writeJSON(filepath.Join(cycleDir, "validation-result.json"), result); err != nil {
		return CycleResult{}, err
	}
	if err := writeJSON(filepath.Join(cycleDir, "execution-preview.json"), preview); err != nil {
		return CycleResult{}, err
	}
	post, execResult := portfolio.ApplyPaperFills(cycleID, current, proposal, preview, books)
	if err := writeJSON(filepath.Join(cycleDir, "execution-result.json"), execResult); err != nil {
		return CycleResult{}, err
	}
	if err := writeJSON(filepath.Join(cycleDir, "post-portfolio.json"), post); err != nil {
		return CycleResult{}, err
	}
	if err := writeJSON(filepath.Join(cfg.RuntimeDir, "paper", "portfolio.json"), post); err != nil {
		return CycleResult{}, err
	}
	return CycleResult{CycleID: cycleID, CycleDir: cycleDir}, nil
}

func runCoordinator(ctx context.Context, agent Agent, cfg config.Config, cycleID, cycleDir string, contextPayload map[string]any, candidates []types.MarketCandidate) (types.ResearchPlan, types.ValidationResult, error) {
	var plan types.ResearchPlan
	var result types.ValidationResult
	var lastErrors []types.ValidationError
	dir := filepath.Join(cycleDir, "coordinator")
	for attempt := 0; attempt <= cfg.CoordinatorRepairAttempts; attempt++ {
		raw, err := agent.Run(ctx, buildCoordinatorPrompt(contextPayload, lastErrors))
		if err != nil {
			writeJSON(filepath.Join(cycleDir, "error.json"), map[string]any{"code": "coordinator_agent_failed", "message": err.Error()})
			return types.ResearchPlan{}, types.ValidationResult{}, err
		}
		parsed, parseErrors := validation.ParseResearchPlan(raw)
		_ = appendJSONLine(filepath.Join(dir, "attempts.jsonl"), map[string]any{
			"attempt":      attempt + 1,
			"raw_output":   raw,
			"parse_errors": parseErrors,
		})
		if len(parseErrors) > 0 {
			lastErrors = parseErrors
			result = types.ValidationResult{CycleID: cycleID, Passed: false, Errors: parseErrors}
		} else {
			plan = parsed
			plan.CycleID = cycleID
			if plan.CreatedAt == "" {
				plan.CreatedAt = time.Now().UTC().Format(time.RFC3339)
			}
			result = validation.ValidateResearchPlan(plan, cycleID, candidates)
			lastErrors = result.Errors
		}
		_ = writeJSON(filepath.Join(dir, "validation-result.json"), result)
		if result.Passed {
			_ = writeJSON(filepath.Join(dir, "research-plan.json"), plan)
			_ = writeJSON(filepath.Join(cycleDir, "research-plan.json"), plan)
			return plan, result, nil
		}
	}
	return plan, result, nil
}

func runResearchWorkers(ctx context.Context, agent Agent, cfg config.Config, cycleID, cycleDir string, plan types.ResearchPlan) (types.ResearchIndex, error) {
	dir := filepath.Join(cycleDir, "deep-research")
	records, results := runWorkerBatch(ctx, agent, cfg, cycleID, dir, plan.Tasks, 0)
	followUpTasks := buildFollowUpTasks(cfg, plan, results)
	var followUpRecords []types.WorkerRecord
	if cfg.MaxFollowUpDepth > 0 && len(followUpTasks) > 0 {
		followUpRecords, _ = runWorkerBatch(ctx, agent, cfg, cycleID, dir, followUpTasks, 1)
		records = append(records, followUpRecords...)
	}
	index := buildResearchIndex(cycleID, records, len(followUpTasks))
	if err := writeJSON(filepath.Join(dir, "index.json"), index); err != nil {
		return types.ResearchIndex{}, err
	}
	if err := writeJSON(filepath.Join(cycleDir, "research-index.json"), index); err != nil {
		return types.ResearchIndex{}, err
	}
	return index, nil
}

func runWorkerBatch(ctx context.Context, agent Agent, cfg config.Config, cycleID, researchDir string, tasks []types.ResearchTask, depth int) ([]types.WorkerRecord, []types.ResearchResult) {
	var mu sync.Mutex
	records := make([]types.WorkerRecord, 0, len(tasks))
	results := make([]types.ResearchResult, 0, len(tasks))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(resolveWorkerConcurrency(cfg))
	for _, task := range tasks {
		task := task
		group.Go(func() error {
			record, result := runOneWorker(groupCtx, agent, cfg, cycleID, researchDir, task, depth)
			mu.Lock()
			records = append(records, record)
			if result.TaskID != "" {
				results = append(results, result)
			}
			mu.Unlock()
			return nil
		})
	}
	_ = group.Wait()
	return records, results
}

func runOneWorker(ctx context.Context, agent Agent, cfg config.Config, cycleID, researchDir string, task types.ResearchTask, depth int) (types.WorkerRecord, types.ResearchResult) {
	taskID := safeFileName(task.TaskID)
	base := filepath.Join(researchDir, "depth-"+fmt.Sprint(depth))
	taskPath := filepath.Join(base, "tasks", taskID+".json")
	resultPath := filepath.Join(base, "results", taskID+".json")
	validationPath := filepath.Join(base, "validation", taskID+".json")
	_ = writeJSON(taskPath, task)
	var lastErrors []types.ValidationError
	var finalResult types.ResearchResult
	var validationResult types.ValidationResult
	var sessionID string
	for attempt := 0; attempt <= cfg.WorkerRepairAttempts; attempt++ {
		attemptCtx := ctx
		cancel := func() {}
		if cfg.WorkerAgentTimeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, cfg.WorkerAgentTimeout)
		}
		run, resumed, err := runAgentAttempt(attemptCtx, agent, sessionID, buildWorkerPrompt(task, cfg, lastErrors))
		cancel()
		if err != nil {
			lastErrors = []types.ValidationError{{Gate: "worker_agent", Code: "agent_failed", Path: "$", Message: err.Error()}}
			validationResult = types.ValidationResult{CycleID: task.TaskID, Passed: false, Errors: lastErrors}
			failedSessionID := sessionID
			_ = appendJSONLine(filepath.Join(base, "attempts", taskID+".jsonl"), map[string]any{
				"attempt":     attempt + 1,
				"task_id":     task.TaskID,
				"status":      "agent_error",
				"session_id":  failedSessionID,
				"resumed":     resumed,
				"agent_error": err.Error(),
			})
			sessionID = ""
			continue
		}
		if run.SessionID != "" {
			sessionID = run.SessionID
		}
		raw := run.Output
		parsed, parseErrors := validation.ParseResearchResult(raw)
		attemptStatus := "parsed"
		if len(parseErrors) > 0 {
			attemptStatus = "parse_failed"
		}
		_ = appendJSONLine(filepath.Join(base, "attempts", taskID+".jsonl"), map[string]any{
			"attempt":      attempt + 1,
			"task_id":      task.TaskID,
			"status":       attemptStatus,
			"session_id":   sessionID,
			"resumed":      resumed,
			"raw_output":   raw,
			"parse_errors": parseErrors,
		})
		if len(parseErrors) > 0 {
			lastErrors = parseErrors
			validationResult = types.ValidationResult{CycleID: task.TaskID, Passed: false, Errors: parseErrors}
			continue
		}
		finalResult = parsed
		validationResult = validation.ValidateResearchResult(finalResult, task)
		lastErrors = validationResult.Errors
		if !validationResult.Passed {
			_ = appendJSONLine(filepath.Join(base, "attempts", taskID+".jsonl"), map[string]any{
				"attempt":           attempt + 1,
				"task_id":           task.TaskID,
				"status":            "validation_failed",
				"session_id":        sessionID,
				"resumed":           resumed,
				"validation_errors": validationResult.Errors,
			})
		}
		if validationResult.Passed {
			break
		}
	}
	_ = writeJSON(resultPath, finalResult)
	_ = writeJSON(validationPath, validationResult)
	status := "failed"
	if validationResult.Passed {
		status = finalResult.Status
	}
	return types.WorkerRecord{
		TaskID:         task.TaskID,
		EventSlug:      task.EventSlug,
		Domain:         task.Domain,
		Status:         status,
		ResultPath:     resultPath,
		ValidationPath: validationPath,
		SummarySignal:  finalResult.SummarySignal,
		Errors:         validationResult.Errors,
	}, finalResult
}

func runAgentAttempt(ctx context.Context, agent Agent, sessionID, prompt string) (types.AgentRun, bool, error) {
	if sessionID != "" {
		if runner, ok := agent.(sessionAgent); ok {
			run, err := runner.ResumeSession(ctx, sessionID, prompt)
			return run, true, err
		}
	}
	if runner, ok := agent.(sessionAgent); ok {
		run, err := runner.RunSession(ctx, prompt)
		return run, false, err
	}
	output, err := agent.Run(ctx, prompt)
	return types.AgentRun{Output: output}, false, err
}

func reducerAttemptStatus(parseErrors []types.ValidationError) string {
	if len(parseErrors) > 0 {
		return "parse_failed"
	}
	return "parsed"
}

func runReducer(ctx context.Context, agent Agent, cfg config.Config, cycleID, cycleDir string, contextPayload map[string]any, candidates []types.MarketCandidate, current types.PortfolioSnapshot, market marketdata.Client, index types.ResearchIndex) (types.Proposal, types.ValidationResult, types.ExecutionPreview, map[string]types.OrderBook, error) {
	dir := filepath.Join(cycleDir, "reducer")
	var proposal types.Proposal
	var result types.ValidationResult
	var preview types.ExecutionPreview
	var books map[string]types.OrderBook
	var lastErrors []types.ValidationError
	var sessionID string
	if researchHealthErrors := reducerResearchHealthGate(index, cfg); len(researchHealthErrors) > 0 {
		result = types.ValidationResult{CycleID: cycleID, Passed: false, Errors: researchHealthErrors}
		_ = writeJSON(filepath.Join(dir, "validation-result.json"), result)
		return proposal, result, preview, books, nil
	}
	for attempt := 0; attempt <= cfg.ReducerRepairAttempts; attempt++ {
		run, resumed, err := runAgentAttempt(ctx, agent, sessionID, buildReducerPrompt(contextPayload, filepath.Join(cycleDir, "deep-research"), index, lastErrors))
		if err != nil {
			failedSessionID := sessionID
			_ = appendJSONLine(filepath.Join(dir, "attempts.jsonl"), map[string]any{
				"attempt":     attempt + 1,
				"status":      "agent_error",
				"session_id":  failedSessionID,
				"resumed":     resumed,
				"agent_error": err.Error(),
			})
			sessionID = ""
			writeJSON(filepath.Join(cycleDir, "error.json"), map[string]any{"code": "reducer_agent_failed", "message": err.Error()})
			return types.Proposal{}, types.ValidationResult{}, types.ExecutionPreview{}, nil, err
		}
		if run.SessionID != "" {
			sessionID = run.SessionID
		}
		raw := run.Output
		parsed, parseErrors := validation.ParseProposal(raw)
		_ = appendJSONLine(filepath.Join(dir, "attempts.jsonl"), map[string]any{
			"attempt":      attempt + 1,
			"status":       reducerAttemptStatus(parseErrors),
			"session_id":   sessionID,
			"resumed":      resumed,
			"raw_output":   raw,
			"parse_errors": parseErrors,
		})
		if len(parseErrors) > 0 {
			lastErrors = parseErrors
			result = types.ValidationResult{CycleID: cycleID, Passed: false, Errors: parseErrors}
			continue
		}
		proposal = parsed
		proposal.CycleID = cycleID
		if proposal.CreatedAt == "" {
			proposal.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		}
		books = readBooksForProposal(ctx, market, proposal)
		result = validation.ValidateProposal(proposal, cfg, candidates, current)
		result.Errors = append(result.Errors, reducerResearchGate(proposal, index)...)
		if len(result.Errors) > 0 {
			result.Passed = false
			_ = appendJSONLine(filepath.Join(dir, "attempts.jsonl"), map[string]any{
				"attempt":           attempt + 1,
				"status":            "validation_failed",
				"session_id":        sessionID,
				"resumed":           resumed,
				"validation_errors": result.Errors,
			})
		}
		if result.Passed && cfg.LLMJudgeEnabled {
			judgeResult, skipped, judgeErrors := judge.Run(ctx, agent, proposal, cfg.LLMJudgeFormatRetryCount)
			if skipped {
				result.Warnings = append(result.Warnings, types.ValidationError{
					Gate:    "llm_judge",
					Code:    "llm_judge_skipped_invalid_output",
					Path:    "$",
					Message: "LLM judge output was invalid after retry; deterministic gates continue",
				})
				result.Warnings = append(result.Warnings, judgeErrors...)
				if !cfg.AllowExecutionWhenJudgeSkipped {
					result.Passed = false
					result.Errors = append(result.Errors, types.ValidationError{
						Gate:    "llm_judge",
						Code:    "judge_skipped_not_allowed",
						Path:    "$",
						Message: "configuration disallows execution when judge is skipped",
					})
				}
			} else if !judgeResult.Passed {
				result.Passed = false
				result.Errors = append(result.Errors, judgeResult.HardFailures...)
			} else {
				result.Warnings = append(result.Warnings, judgeResult.Warnings...)
			}
		}
		if result.Passed {
			previewErrors := []types.ValidationError{}
			preview, previewErrors = execution.BuildPreview(proposal, current, books, cfg)
			if len(previewErrors) > 0 {
				result.Passed = false
				result.Errors = append(result.Errors, previewErrors...)
			}
		}
		_ = writeJSON(filepath.Join(dir, "validation-result.json"), result)
		if result.Passed {
			return proposal, result, preview, books, nil
		}
		lastErrors = result.Errors
	}
	return proposal, result, preview, books, nil
}

func reducerResearchHealthGate(index types.ResearchIndex, cfg config.Config) []types.ValidationError {
	if index.TaskCount == 0 {
		return nil
	}
	maxFailureRate := cfg.MaxWorkerFailureRate
	if maxFailureRate < 0 {
		maxFailureRate = 0
	}
	if maxFailureRate > 1 {
		maxFailureRate = 1
	}
	failureRate := float64(index.FailedCount) / float64(index.TaskCount)
	if failureRate > maxFailureRate {
		return []types.ValidationError{{
			Gate:    "reducer_research_health_gate",
			Code:    "worker_failure_rate_exceeded",
			Path:    "$.research_index",
			Message: fmt.Sprintf("worker failure rate %.2f exceeds max %.2f; cycle research is invalid and must be replanned", failureRate, maxFailureRate),
		}}
	}
	return nil
}

func buildFollowUpTasks(cfg config.Config, plan types.ResearchPlan, results []types.ResearchResult) []types.ResearchTask {
	if cfg.MaxFollowUpTasksPerCycle <= 0 {
		return nil
	}
	byTask := map[string]types.ResearchTask{}
	for _, task := range plan.Tasks {
		byTask[task.TaskID] = task
	}
	var tasks []types.ResearchTask
	for _, result := range results {
		if result.Status != "needs_followup" || len(result.FollowUpRequests) == 0 {
			continue
		}
		parent, ok := byTask[result.TaskID]
		if !ok {
			continue
		}
		for i, request := range result.FollowUpRequests {
			if len(tasks) >= cfg.MaxFollowUpTasksPerCycle {
				return tasks
			}
			marketSet := map[string]bool{}
			for _, slug := range request.MarketSlugs {
				marketSet[slug] = true
			}
			task := parent
			task.TaskID = parent.TaskID + "-followup-" + fmt.Sprint(i+1)
			task.ResearchFocus = append([]string{request.Reason}, request.NeededData...)
			if len(marketSet) > 0 {
				task.MarketSlugs = nil
				task.Markets = nil
				for _, market := range parent.Markets {
					if marketSet[market.MarketSlug] {
						task.MarketSlugs = append(task.MarketSlugs, market.MarketSlug)
						task.Markets = append(task.Markets, market)
					}
				}
			}
			if len(task.MarketSlugs) > 0 {
				tasks = append(tasks, task)
			}
		}
	}
	return tasks
}

func buildResearchIndex(cycleID string, records []types.WorkerRecord, followUpCount int) types.ResearchIndex {
	index := types.ResearchIndex{
		CycleID:       cycleID,
		TaskCount:     len(records),
		FollowUpCount: followUpCount,
		Records:       records,
	}
	for _, record := range records {
		if record.Status == "failed" {
			index.FailedCount++
			continue
		}
		index.PassedCount++
		switch record.SummarySignal.Decision {
		case "open_candidate":
			index.OpenCandidates = append(index.OpenCandidates, record)
		case "watchlist":
			index.Watchlist = append(index.Watchlist, record)
		}
		if len(record.SummarySignal.PositionActionsOfInterest) > 0 {
			index.PositionRelated = append(index.PositionRelated, record)
		}
	}
	return index
}

func reducerResearchGate(proposal types.Proposal, index types.ResearchIndex) []types.ValidationError {
	allowedMarkets := map[string]bool{}
	for _, record := range index.OpenCandidates {
		if record.Status == "failed" {
			continue
		}
		for _, slug := range record.SummarySignal.MarketSlugsOfInterest {
			allowedMarkets[slug] = true
		}
	}
	var errors []types.ValidationError
	for i, item := range proposal.NewOpportunities {
		if item.Action != "open" {
			continue
		}
		if !allowedMarkets[item.MarketSlug] {
			errors = append(errors, types.ValidationError{
				Gate:    "reducer_research_gate",
				Code:    "open_not_backed_by_passed_worker",
				Path:    fmt.Sprintf("$.new_opportunities[%d].market_slug", i),
				Message: "new open action must be backed by a passed worker open_candidate summary signal",
			})
		}
	}
	return errors
}

func resolveWorkerConcurrency(cfg config.Config) int {
	if cfg.ResearchWorkerConcurrency > 0 {
		return cfg.ResearchWorkerConcurrency
	}
	n := runtime.NumCPU()
	if n < 1 {
		return 1
	}
	return n
}

func safeFileName(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		return "task"
	}
	return name
}

func limitCandidatesForSmoke(candidates []types.MarketCandidate, cfg config.Config) []types.MarketCandidate {
	if cfg.SmokeMaxCandidatesPerCycle <= 0 || len(candidates) <= cfg.SmokeMaxCandidatesPerCycle {
		return candidates
	}
	return candidates[:cfg.SmokeMaxCandidatesPerCycle]
}

func preScreenCoordinatorCandidates(candidates []types.MarketCandidate, cfg config.Config) ([]types.MarketCandidate, coordinatorPreScreenReport) {
	report := coordinatorPreScreenReport{
		InputCount:    len(candidates),
		MaxCandidates: cfg.CoordinatorPreScreenMaxCandidates,
		MinCandidates: cfg.CoordinatorPreScreenMinCandidates,
		Items:         []coordinatorPreScreenItem{},
	}
	selected := make([]types.MarketCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		reasons := coordinatorPreScreenReasons(candidate)
		isSelected := len(reasons) == 0
		if isSelected {
			selected = append(selected, candidate)
		}
		report.Items = append(report.Items, coordinatorPreScreenItem{
			EventSlug:  candidate.EventSlug,
			MarketSlug: candidate.MarketSlug,
			Selected:   isSelected,
			Score:      candidate.Score,
			Reasons:    reasons,
		})
	}
	if len(selected) == 0 && len(candidates) > 0 {
		selected = append(selected, candidates[0])
		report.Items[0].Selected = true
		report.Items[0].Reasons = append(report.Items[0].Reasons, "fallback_kept_broad_coverage")
	}
	selected = applyCoordinatorPreScreenCap(selected, cfg)
	selectedSet := map[string]bool{}
	for _, candidate := range selected {
		selectedSet[candidate.MarketSlug] = true
	}
	for i := range report.Items {
		if report.Items[i].Selected && !selectedSet[report.Items[i].MarketSlug] {
			report.Items[i].Selected = false
			report.Items[i].Reasons = append(report.Items[i].Reasons, "deferred_after_broad_cap")
		}
	}
	report.SelectedCount = len(selected)
	report.SkippedCount = report.InputCount - report.SelectedCount
	return selected, report
}

func coordinatorPreScreenReasons(candidate types.MarketCandidate) []string {
	var reasons []string
	if !candidate.EnableOrderBook {
		reasons = append(reasons, "order_book_disabled")
	}
	if len(candidate.TokenIDs) == 0 {
		reasons = append(reasons, "missing_token_ids")
	}
	if len(candidate.Outcomes) == 0 {
		reasons = append(reasons, "missing_outcomes")
	}
	if len(candidate.OutcomePrices) == 0 {
		reasons = append(reasons, "missing_outcome_prices")
	}
	if candidate.LiquidityUSD <= 0 {
		reasons = append(reasons, "missing_liquidity")
	}
	return reasons
}

func applyCoordinatorPreScreenCap(candidates []types.MarketCandidate, cfg config.Config) []types.MarketCandidate {
	if cfg.CoordinatorPreScreenMaxCandidates <= 0 || len(candidates) <= cfg.CoordinatorPreScreenMaxCandidates {
		return candidates
	}
	minCandidates := cfg.CoordinatorPreScreenMinCandidates
	if minCandidates < 1 {
		minCandidates = 1
	}
	limit := cfg.CoordinatorPreScreenMaxCandidates
	if limit < minCandidates {
		limit = minCandidates
	}
	if limit >= len(candidates) {
		return candidates
	}
	return candidates[:limit]
}

func checkDataSources(ctx context.Context, cfg config.Config, client marketdata.Client, candidates []types.MarketCandidate) types.DataSourceHealth {
	health := types.DataSourceHealth{
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Checks:    []types.DataSourceCheck{},
		Warnings:  []types.ValidationError{},
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}
	health.Checks = append(health.Checks, httpHealthCheck(ctx, httpClient, "polymarket_gamma_events", cfg.GammaBaseURL+"/events?active=true&closed=false&limit=1&offset=0"))
	if tokenID := firstCandidateToken(candidates); tokenID != "" {
		book, err := client.ReadBook(ctx, tokenID)
		check := types.DataSourceCheck{Name: "polymarket_clob_book", URL: cfg.CLOBBaseURL + "/book?token_id=" + tokenID, Reachable: err == nil}
		if err != nil {
			check.Message = err.Error()
		} else if len(book.Bids) == 0 && len(book.Asks) == 0 {
			check.Message = "book endpoint reachable but returned empty bids and asks"
		}
		health.Checks = append(health.Checks, check)
	} else {
		health.Warnings = append(health.Warnings, types.ValidationError{
			Gate:    "data_source_health",
			Code:    "no_candidate_token_for_clob_check",
			Path:    "$.checks.polymarket_clob_book",
			Message: "CLOB book health was skipped because discovery produced no candidate token",
		})
	}
	if strings.TrimSpace(cfg.WebHealthURL) != "" {
		health.Checks = append(health.Checks, httpHealthCheck(ctx, httpClient, "web_research_http", cfg.WebHealthURL))
	}
	for _, check := range health.Checks {
		if !check.Reachable {
			health.Warnings = append(health.Warnings, types.ValidationError{
				Gate:    "data_source_health",
				Code:    "source_unreachable",
				Path:    "$.checks." + check.Name,
				Message: check.Message,
			})
		}
	}
	return health
}

func httpHealthCheck(ctx context.Context, client *http.Client, name, url string) types.DataSourceCheck {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return types.DataSourceCheck{Name: name, URL: url, Reachable: false, Message: err.Error()}
	}
	resp, err := client.Do(req)
	if err != nil {
		return types.DataSourceCheck{Name: name, URL: url, Reachable: false, Message: err.Error()}
	}
	defer resp.Body.Close()
	return types.DataSourceCheck{
		Name:       name,
		URL:        url,
		Reachable:  resp.StatusCode < 500,
		StatusCode: resp.StatusCode,
		Message:    resp.Status,
	}
}

func firstCandidateToken(candidates []types.MarketCandidate) string {
	for _, candidate := range candidates {
		for _, tokenID := range candidate.TokenIDs {
			if strings.TrimSpace(tokenID) != "" {
				return tokenID
			}
		}
	}
	return ""
}

func failCycle(cycleDir, cycleID string, errors []types.ValidationError) (CycleResult, error) {
	payload := map[string]any{"cycle_id": cycleID, "status": "validation_failed", "errors": errors}
	_ = writeJSON(filepath.Join(cycleDir, "error.json"), payload)
	return CycleResult{}, fmt.Errorf("cycle validation failed: %d error(s)", len(errors))
}

func loadOrInitPortfolio(cfg config.Config, cycleID string) types.PortfolioSnapshot {
	path := filepath.Join(cfg.RuntimeDir, "paper", "portfolio.json")
	var snapshot types.PortfolioSnapshot
	if readJSON(path, &snapshot) == nil && snapshot.StartingNavUSD > 0 {
		snapshot.CycleID = cycleID
		return snapshot
	}
	return portfolio.NewPaperPortfolio(cycleID, cfg.PaperStartingCashUSD)
}

func readBooksForProposal(ctx context.Context, client marketdata.Client, proposal types.Proposal) map[string]types.OrderBook {
	books := map[string]types.OrderBook{}
	for _, item := range proposal.NewOpportunities {
		if item.TokenID == "" {
			continue
		}
		book, _ := client.ReadBook(ctx, item.TokenID)
		books[item.TokenID] = book
	}
	for _, item := range proposal.PositionReviews {
		if item.TokenID == "" {
			continue
		}
		book, _ := client.ReadBook(ctx, item.TokenID)
		books[item.TokenID] = book
	}
	return books
}

func buildCoordinatorPrompt(contextPayload map[string]any, errors []types.ValidationError) string {
	contextJSON, _ := json.MarshalIndent(contextPayload, "", "  ")
	var b strings.Builder
	b.WriteString("You are the coordinator for a local Polymarket paper-trading research cycle. No real money, live orders, wallet calls, or trading API calls will be used.\n")
	b.WriteString("Return only a single valid JSON object. No markdown. No commentary. No code fence.\n")
	b.WriteString("You may use Bash and built-in web/data-source tools for read-only research: curl public Polymarket endpoints, jq local JSON artifacts, inspect order books, search the public web, and fetch public source pages. Do not edit files, write files, call wallets, submit orders, cancel orders, or use private trading APIs.\n")
	b.WriteString("Create one event-level worker task for every event represented in the full market_candidates_file. Do not cap the number of tasks. Do not skip previously checked events.\n")
	b.WriteString("Use a todo list and sub-agents if your runtime supports them. Group only markets from the same event into the same task.\n")
	b.WriteString("Use the full market_candidates_file in context. The sample is only for orientation and must not be treated as the full universe.\n")
	b.WriteString("Output schema: cycle_id, created_at, tasks. Each task must include task_id, event_slug, event_title, domain, research_profile, priority, research_focus, market_slugs, markets.\n")
	b.WriteString("priority must be numeric. research_focus must be an array of strings. markets must copy the full candidate objects from market_candidates_file, including question, outcomes, outcome_prices, token_ids, resolution, liquidity_usd, best_bid, best_ask, tags, and score. Do not summarize market objects.\n")
	b.WriteString("Domain should be a compact routing label such as sports, politics, geopolitics, macro, finance, tech, culture, or general. Research profile should describe what kind of sources the worker must seek.\n")
	if len(errors) > 0 {
		errorsJSON, _ := json.MarshalIndent(errors, "", "  ")
		b.WriteString("\nPrevious validation errors. Repair the full JSON research plan, not a patch:\n")
		b.Write(errorsJSON)
		b.WriteString("\n")
	}
	b.WriteString("\nContext:\n")
	b.Write(contextJSON)
	return b.String()
}

func buildWorkerPrompt(task types.ResearchTask, cfg config.Config, errors []types.ValidationError) string {
	taskJSON, _ := json.MarshalIndent(task, "", "  ")
	var b strings.Builder
	b.WriteString("You are a bounded research worker for one Polymarket event. Return only a single JSON object. No markdown. No commentary. No code fence.\n")
	b.WriteString("You have a hard per-attempt timeout of about ")
	b.WriteString(fmt.Sprint(int(cfg.WorkerAgentTimeout.Seconds())))
	b.WriteString(" seconds. Finish early with valid JSON rather than doing open-ended research.\n")
	b.WriteString("Use read-only Bash/web tools only if needed. Do not edit files, write files, call wallets, submit orders, cancel orders, or use private trading APIs.\n")
	b.WriteString("Do not use sub-agents. Do not run broad deep-research loops. Use at most 3 external fetch/search/curl operations total for this task; prefer official/primary pages and one reputable news/data source.\n")
	b.WriteString("Assess every market in the assigned task using candidate price/order-book data first, then only minimal external evidence. The task domain determines evidence standard: sports needs odds/injury/lineup/rating style evidence; politics/geopolitics needs official, polling, legal/policy, news/reputable_news, or primary evidence; macro/finance needs official data, central-bank, market-data, or news/reputable_news evidence.\n")
	b.WriteString("For the whole task, include exactly 2-4 evidence records and 1-2 counter_evidence records. Every factual claim used for probability assessment must include source_url, source_type, retrieved_at, claim, contract_relevance, supports, and probability_effect_pp.\n")
	b.WriteString("Output schema: task_id, event_slug, status, summary_signal, market_assessments, evidence, counter_evidence, unresolved_ambiguities, follow_up_requests, error.\n")
	b.WriteString("status must be passed, failed, or needs_followup. summary_signal.decision must be open_candidate, watchlist, skip, failed, or needs_followup. summary_signal.reason_short is required. market_assessments[].recommendation must be open_candidate, watchlist, or skip. edge must equal ai_probability - market_probability.\n")
	b.WriteString("If summary_signal.decision is open_candidate, summary_signal.max_edge and confidence must be positive, market_slugs_of_interest must name the open_candidate market slugs, and max_edge should match the strongest open market_assessments[].edge.\n")
	b.WriteString("Every open_candidate market assessment must include market_slug, token_id, outcome, recommendation, ai_probability, market_probability, edge, and thesis. token_id and outcome must correspond to the same index in the assigned market candidate, and market_probability must match the assigned candidate outcome price.\n")
	b.WriteString("If you cannot finish bounded research in time, return status failed with error and still include market_assessments if possible. If more research is needed but the event is promising, use status needs_followup and add follow_up_requests. Follow-up is for missing data only, not schema repair.\n")
	b.WriteString("Keep the JSON compact: short claims, short thesis strings, no long prose, no citations outside evidence arrays.\n")
	if len(errors) > 0 {
		errorsJSON, _ := json.MarshalIndent(errors, "", "  ")
		b.WriteString("\nPrevious validation errors. Repair the full JSON research result, not a patch:\n")
		b.Write(errorsJSON)
		b.WriteString("\nReuse findings already gathered in this session. Do not repeat broad search. Only run a targeted fetch/search if the validation errors explicitly require missing evidence or a missing candidate binding.\n")
	}
	b.WriteString("\nAssigned task:\n")
	b.Write(taskJSON)
	return b.String()
}

func buildReducerPrompt(contextPayload map[string]any, researchDir string, index types.ResearchIndex, errors []types.ValidationError) string {
	contextJSON, _ := json.MarshalIndent(contextPayload, "", "  ")
	indexJSON, _ := json.MarshalIndent(index, "", "  ")
	var b strings.Builder
	b.WriteString("You are the reducer for a local Polymarket paper simulation. Return only a single valid JSON object. No markdown. No commentary. No code fence.\n")
	b.WriteString("You may use Bash and built-in web/data-source tools for read-only research and to read JSON files under the deep research directory. Do not edit files, write files, call wallets, submit orders, cancel orders, or use private trading APIs.\n")
	b.WriteString("Read the research index first, then inspect only worker result files that are open_candidate, watchlist, or position-related. Failed worker tasks must not be used for open decisions or position operations this cycle.\n")
	b.WriteString("Produce the final executable proposal JSON for paper trading. MVP supports only FOK-style paper execution; do not output GTC or GTD. Use only market candidates present in the context and only passed worker open candidates for new open actions.\n")
	b.WriteString("If nothing is trade-ready, output empty arrays and structured no_trade with reason, candidate_count, checked_market_slugs, missing_requirements, and data_health.\n")
	b.WriteString("Open items require resolution, evidence, counter_evidence, probabilities, edge, and order intent. Every edge must equal ai_probability - market_probability.\n")
	b.WriteString("Schema fields: schema_version, cycle_id, created_at, position_reviews, new_opportunities, no_trade.\n")
	if len(errors) > 0 {
		errorsJSON, _ := json.MarshalIndent(errors, "", "  ")
		b.WriteString("\nPrevious reducer/final-gate errors. Repair the full JSON proposal, not a patch:\n")
		b.Write(errorsJSON)
		b.WriteString("\n")
	}
	b.WriteString("\nDeep research directory:\n")
	b.WriteString(researchDir)
	b.WriteString("\nResearch index:\n")
	b.Write(indexJSON)
	b.WriteString("\nCycle context:\n")
	b.Write(contextJSON)
	return b.String()
}

func compactCandidates(candidates []types.MarketCandidate) []map[string]any {
	out := make([]map[string]any, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, map[string]any{
			"event_slug":        c.EventSlug,
			"event_title":       c.EventTitle,
			"market_slug":       c.MarketSlug,
			"question":          c.Question,
			"outcomes":          c.Outcomes,
			"outcome_prices":    c.OutcomePrices,
			"token_ids":         c.TokenIDs,
			"liquidity_usd":     c.LiquidityUSD,
			"volume_24h_usd":    c.Volume24hUSD,
			"best_bid":          c.BestBid,
			"best_ask":          c.BestAsk,
			"spread":            c.Spread,
			"enable_order_book": c.EnableOrderBook,
			"category_slug":     c.CategorySlug,
			"category_label":    c.CategoryLabel,
			"tags":              c.Tags,
			"score":             c.Score,
			"resolution": map[string]any{
				"source_url":   c.Resolution.SourceURL,
				"rule_excerpt": truncate(c.Resolution.RuleExcerpt, 600),
				"time_window":  c.Resolution.TimeWindow,
			},
		})
	}
	return out
}

func firstN[T any](items []T, n int) []T {
	if len(items) <= n {
		return items
	}
	return items[:n]
}

func polymarketAPIGuide(cfg config.Config) map[string]any {
	return map[string]any{
		"gamma_events": map[string]string{
			"description": "Fetch open events with limit/offset pagination, expand each event.markets[] entry into concrete markets. Repeat across order=volume24hr, liquidity, startDate, and competitive.",
			"example":     cfg.GammaBaseURL + "/events?active=true&closed=false&limit=50&offset=0&order=volume24hr&ascending=false",
		},
		"gamma_market_by_slug": map[string]string{
			"description": "Fetch or verify a specific market by slug when deeper binding checks are needed.",
			"example":     cfg.GammaBaseURL + "/markets?slug=<market_slug>",
		},
		"clob_order_book": map[string]string{
			"description": "Fetch public CLOB order book for a token_id. Use max bid as bestBid and min ask as bestAsk; do not trust array order.",
			"example":     cfg.CLOBBaseURL + "/book?token_id=<token_id>",
		},
	}
}

func marketDataHealth(candidates []types.MarketCandidate, report types.DiscoveryReport, cfg config.Config, discoveredCandidateCount int) map[string]any {
	missingSourceURL := 0
	missingRule := 0
	missingPrices := 0
	totalTokens := 0
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Resolution.SourceURL) == "" {
			missingSourceURL++
		}
		if strings.TrimSpace(candidate.Resolution.RuleExcerpt) == "" {
			missingRule++
		}
		if len(candidate.OutcomePrices) == 0 {
			missingPrices++
		}
		totalTokens += len(candidate.TokenIDs)
	}
	return map[string]any{
		"candidate_count":                        len(candidates),
		"discovered_candidate_count":             discoveredCandidateCount,
		"smoke_candidate_limit":                  cfg.SmokeMaxCandidatesPerCycle,
		"smoke_candidate_limit_applied":          cfg.SmokeMaxCandidatesPerCycle > 0 && discoveredCandidateCount > len(candidates),
		"total_token_count":                      totalTokens,
		"missing_source_url_count":               missingSourceURL,
		"missing_rule_count":                     missingRule,
		"missing_prices_count":                   missingPrices,
		"discovery_report":                       report,
		"all_sort_dimensions_reached_short_page": allSortDimensionsReachedShortPage(report),
		"source":                                 "gamma_events_multidimensional",
	}
}

func allSortDimensionsReachedShortPage(report types.DiscoveryReport) bool {
	if len(report.SortDimensions) == 0 {
		return false
	}
	for _, dimension := range report.SortDimensions {
		if !dimension.ShortPage {
			return false
		}
	}
	return true
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
