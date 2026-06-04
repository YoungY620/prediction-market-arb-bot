package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"prediction-market-arb-bot/internal/types"
	"prediction-market-arb-bot/internal/validation"
)

type auditReport struct {
	CycleDir string       `json:"cycle_dir"`
	Passed   bool         `json:"passed"`
	Checks   []auditCheck `json:"checks"`
}

type auditCheck struct {
	Name    string   `json:"name"`
	Passed  bool     `json:"passed"`
	Message string   `json:"message,omitempty"`
	Details []string `json:"details,omitempty"`
}

type auditOptions struct {
	Strict                  bool
	MinCandidates           int
	MinDiscoveredCandidates int
	MinRawEvents            int
	MinRawMarkets           int
	MinPassedWorkers        int
}

func main() {
	var opts auditOptions
	flag.BoolVar(&opts.Strict, "strict", false, "require full-discovery short pages and reachable data sources")
	flag.IntVar(&opts.MinCandidates, "min-candidates", 0, "minimum effective candidates required in market-candidates.json")
	flag.IntVar(&opts.MinDiscoveredCandidates, "min-discovered-candidates", 0, "minimum full discovery accepted candidates required before any smoke truncation")
	flag.IntVar(&opts.MinRawEvents, "min-raw-events", 0, "minimum raw Polymarket events required in discovery report")
	flag.IntVar(&opts.MinRawMarkets, "min-raw-markets", 0, "minimum raw Polymarket markets required in discovery report")
	flag.IntVar(&opts.MinPassedWorkers, "min-passed-workers", 0, "minimum passed worker results required in research index")
	flag.Parse()
	if flag.NArg() != 1 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: go run ./cmd/cycle-audit [--strict] [--min-*] runtime/cycles/<cycle_id>")
		os.Exit(2)
	}
	report := auditCycle(flag.Arg(0), opts)
	report.Passed = true
	for _, check := range report.Checks {
		if !check.Passed {
			report.Passed = false
			break
		}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(report)
	if !report.Passed {
		os.Exit(1)
	}
}

func auditCycle(cycleDir string, opts auditOptions) auditReport {
	report := auditReport{CycleDir: cycleDir}
	candidates := readArray[types.MarketCandidate](filepath.Join(cycleDir, "market-candidates.json"))
	health := readObject[map[string]any](filepath.Join(cycleDir, "market-data-health.json"))
	dataHealth := readObject[types.DataSourceHealth](filepath.Join(cycleDir, "data-source-health.json"))
	preScreen := readObject[coordinatorPreScreenReport](filepath.Join(cycleDir, "coordinator-prescreen.json"))
	plan := readObject[types.ResearchPlan](filepath.Join(cycleDir, "research-plan.json"))
	index := readObject[types.ResearchIndex](filepath.Join(cycleDir, "research-index.json"))
	proposal := readObject[types.Proposal](filepath.Join(cycleDir, "proposal.json"))
	validationResult := readObject[types.ValidationResult](filepath.Join(cycleDir, "validation-result.json"))
	preview := readObject[types.ExecutionPreview](filepath.Join(cycleDir, "execution-preview.json"))
	portfolio := readObject[types.PortfolioSnapshot](filepath.Join(cycleDir, "post-portfolio.json"))

	report.add(fileExistsCheck(cycleDir, "market-candidates.json", "market-data-health.json", "data-source-health.json", "coordinator-prescreen.json"))
	report.add(discoveryCheck(candidates.Value, health.Value, preScreen.Value, preScreen.OK, opts))
	report.add(preScreenCheck(preScreen.Value, preScreen.OK, len(candidates.Value)))
	report.add(dataSourceCheck(dataHealth.Value, dataHealth.OK, opts.Strict, len(candidates.Value)))
	report.add(researchPlanCheck(plan.Value, plan.OK, candidates.Value))
	report.add(researchIndexCheck(index.Value, index.OK, plan.Value, opts))
	report.add(workerResultCheck(cycleDir, plan.Value, index.Value))
	report.add(proposalCheck(proposal.Value, proposal.OK, validationResult.Value, validationResult.OK, candidates.Value))
	report.add(executionArtifactsCheck(preview.Value, preview.OK, portfolio.Value, portfolio.OK))
	return report
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

func (r *auditReport) add(check auditCheck) {
	r.Checks = append(r.Checks, check)
}

type readResult[T any] struct {
	Value T
	OK    bool
	Err   error
}

func readArray[T any](path string) readResult[[]T] {
	var value []T
	err := readJSON(path, &value)
	return readResult[[]T]{Value: value, OK: err == nil, Err: err}
}

func readObject[T any](path string) readResult[T] {
	var value T
	err := readJSON(path, &value)
	return readResult[T]{Value: value, OK: err == nil, Err: err}
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func fileExistsCheck(cycleDir string, names ...string) auditCheck {
	var missing []string
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(cycleDir, name)); err != nil {
			missing = append(missing, name)
		}
	}
	return auditCheck{Name: "required_discovery_artifacts", Passed: len(missing) == 0, Details: missing}
}

func discoveryCheck(candidates []types.MarketCandidate, raw map[string]any, preScreen coordinatorPreScreenReport, preScreenOK bool, opts auditOptions) auditCheck {
	report, ok := decodeDiscoveryReport(raw["discovery_report"])
	if !ok {
		return auditCheck{Name: "discovery_scope", Passed: false, Message: "market-data-health.json is missing discovery_report"}
	}
	var details []string
	if report.RawEventCount <= 0 {
		details = append(details, "raw_event_count must be positive")
	}
	if report.RawMarketCount <= 0 {
		details = append(details, "raw_market_count must be positive")
	}
	smokeLimitApplied := boolField(raw, "smoke_candidate_limit_applied")
	if !smokeLimitApplied && preScreenOK && report.AcceptedMarketCount != preScreen.InputCount {
		details = append(details, "accepted_market_count must match coordinator prescreen input_count")
	}
	if !smokeLimitApplied && !preScreenOK && report.AcceptedMarketCount != len(candidates) {
		details = append(details, "accepted_market_count must match market-candidates.json when prescreen artifact is missing")
	}
	if smokeLimitApplied && report.AcceptedMarketCount < len(candidates) {
		details = append(details, "smoke-limited market-candidates.json cannot exceed discovery accepted count")
	}
	if report.AcceptedEventCount <= 0 && len(candidates) > 0 {
		details = append(details, "accepted_event_count must be positive when candidates exist")
	}
	if len(report.SortDimensions) < 4 && !report.Partial && !report.CacheHit {
		details = append(details, "all four sort dimensions must be attempted")
	}
	if len(report.FilteredByReason) == 0 {
		details = append(details, "filtered_by_reason must be populated")
	}
	if opts.MinCandidates > 0 && len(candidates) < opts.MinCandidates {
		details = append(details, fmt.Sprintf("market-candidates.json has %d candidates, below required minimum %d", len(candidates), opts.MinCandidates))
	}
	if opts.MinDiscoveredCandidates > 0 && report.AcceptedMarketCount < opts.MinDiscoveredCandidates {
		details = append(details, fmt.Sprintf("discovery accepted %d markets, below required minimum %d", report.AcceptedMarketCount, opts.MinDiscoveredCandidates))
	}
	if opts.MinRawEvents > 0 && report.RawEventCount < opts.MinRawEvents {
		details = append(details, fmt.Sprintf("discovery saw %d raw events, below required minimum %d", report.RawEventCount, opts.MinRawEvents))
	}
	if opts.MinRawMarkets > 0 && report.RawMarketCount < opts.MinRawMarkets {
		details = append(details, fmt.Sprintf("discovery saw %d raw markets, below required minimum %d", report.RawMarketCount, opts.MinRawMarkets))
	}
	if opts.Strict {
		for _, dim := range report.SortDimensions {
			if !dim.ShortPage {
				details = append(details, "strict discovery requires short_page=true for "+dim.Order)
			}
		}
	}
	return auditCheck{Name: "discovery_scope", Passed: len(details) == 0, Details: details}
}

func preScreenCheck(report coordinatorPreScreenReport, ok bool, candidateCount int) auditCheck {
	if !ok {
		return auditCheck{Name: "coordinator_prescreen", Passed: false, Message: "coordinator-prescreen.json is missing or invalid"}
	}
	var details []string
	if report.InputCount < report.SelectedCount {
		details = append(details, "selected_count cannot exceed input_count")
	}
	if report.InputCount != report.SelectedCount+report.SkippedCount {
		details = append(details, "input_count must equal selected_count + skipped_count")
	}
	if report.SelectedCount != candidateCount {
		details = append(details, "selected_count must match market-candidates.json length")
	}
	if len(report.Items) != report.InputCount {
		details = append(details, "items length must equal input_count")
	}
	if report.MaxCandidates <= 0 && report.SelectedCount == 0 && report.InputCount > 0 {
		details = append(details, "default broad prescreen must keep at least one candidate when input exists")
	}
	if report.MaxCandidates > 0 && report.InputCount >= report.MinCandidates && report.SelectedCount < report.MinCandidates {
		details = append(details, fmt.Sprintf("prescreen selected %d candidates, below broad minimum %d", report.SelectedCount, report.MinCandidates))
	}
	return auditCheck{Name: "coordinator_prescreen", Passed: len(details) == 0, Details: details}
}

func boolField(record map[string]any, key string) bool {
	value, ok := record[key].(bool)
	return ok && value
}

func decodeDiscoveryReport(value any) (types.DiscoveryReport, bool) {
	data, err := json.Marshal(value)
	if err != nil {
		return types.DiscoveryReport{}, false
	}
	var report types.DiscoveryReport
	if err := json.Unmarshal(data, &report); err != nil {
		return types.DiscoveryReport{}, false
	}
	return report, true
}

func dataSourceCheck(health types.DataSourceHealth, ok bool, strict bool, candidateCount int) auditCheck {
	if !ok {
		return auditCheck{Name: "data_source_health", Passed: false, Message: "data-source-health.json is missing or invalid"}
	}
	seen := map[string]types.DataSourceCheck{}
	for _, check := range health.Checks {
		seen[check.Name] = check
	}
	var details []string
	required := []string{"polymarket_gamma_events", "web_research_http"}
	if candidateCount > 0 {
		required = append(required, "polymarket_clob_book")
	}
	for _, name := range required {
		check, exists := seen[name]
		if !exists {
			details = append(details, name+" check is missing")
			continue
		}
		if strict && !check.Reachable {
			details = append(details, name+" is not reachable: "+check.Message)
		}
	}
	return auditCheck{Name: "data_source_health", Passed: len(details) == 0, Details: details}
}

func researchPlanCheck(plan types.ResearchPlan, ok bool, candidates []types.MarketCandidate) auditCheck {
	if !ok {
		return auditCheck{Name: "research_plan_coverage", Passed: false, Message: "research-plan.json is missing or invalid"}
	}
	result := validation.ValidateResearchPlan(plan, plan.CycleID, candidates)
	return auditCheck{Name: "research_plan_coverage", Passed: result.Passed, Details: validationDetails(result.Errors)}
}

func researchIndexCheck(index types.ResearchIndex, ok bool, plan types.ResearchPlan, opts auditOptions) auditCheck {
	if !ok {
		return auditCheck{Name: "research_index_coverage", Passed: false, Message: "research-index.json is missing or invalid"}
	}
	var details []string
	if index.TaskCount < len(plan.Tasks) {
		details = append(details, "research index task_count is below initial plan task count")
	}
	if index.PassedCount+index.FailedCount != index.TaskCount {
		details = append(details, "passed_count + failed_count must equal task_count")
	}
	if len(index.Records) != index.TaskCount {
		details = append(details, "records length must equal task_count")
	}
	if index.TaskCount > 0 && index.PassedCount == 0 {
		details = append(details, "at least one worker must pass when research tasks exist")
	}
	if opts.MinPassedWorkers > 0 && index.PassedCount < opts.MinPassedWorkers {
		details = append(details, fmt.Sprintf("research index has %d passed workers, below required minimum %d", index.PassedCount, opts.MinPassedWorkers))
	}
	return auditCheck{Name: "research_index_coverage", Passed: len(details) == 0, Details: details}
}

func workerResultCheck(cycleDir string, plan types.ResearchPlan, index types.ResearchIndex) auditCheck {
	tasks := map[string]types.ResearchTask{}
	for _, task := range plan.Tasks {
		tasks[task.TaskID] = task
	}
	var details []string
	for _, record := range index.Records {
		if record.ResultPath == "" {
			details = append(details, record.TaskID+": missing result_path")
			continue
		}
		resultPath := resolvePath(cycleDir, record.ResultPath)
		validationPath := resolvePath(cycleDir, record.ValidationPath)
		if record.Status == "failed" {
			var validationResult types.ValidationResult
			if err := readJSON(validationPath, &validationResult); err != nil {
				details = append(details, record.TaskID+": cannot read failed worker validation JSON: "+err.Error())
			} else if len(validationResult.Errors) == 0 {
				details = append(details, record.TaskID+": failed worker must include validation errors")
			}
			continue
		}
		var result types.ResearchResult
		if err := readJSON(resultPath, &result); err != nil {
			details = append(details, record.TaskID+": cannot read result JSON: "+err.Error())
			continue
		}
		if task, ok := tasks[record.TaskID]; ok {
			validationResult := validation.ValidateResearchResult(result, task)
			if !validationResult.Passed {
				details = append(details, record.TaskID+": "+strings.Join(validationDetails(validationResult.Errors), "; "))
			}
		}
	}
	return auditCheck{Name: "worker_result_quality", Passed: len(details) == 0, Details: details}
}

func resolvePath(cycleDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return filepath.Join(cycleDir, path)
}

func proposalCheck(proposal types.Proposal, proposalOK bool, result types.ValidationResult, resultOK bool, candidates []types.MarketCandidate) auditCheck {
	if !proposalOK || !resultOK {
		return auditCheck{Name: "final_proposal_gate", Passed: false, Message: "proposal.json or validation-result.json is missing or invalid"}
	}
	var details []string
	if !result.Passed {
		details = append(details, "validation-result.json is not passed")
		details = append(details, validationDetails(result.Errors)...)
	}
	if len(proposal.NewOpportunities) == 0 && len(proposal.PositionReviews) == 0 {
		if proposal.NoTrade == nil {
			details = append(details, "empty proposal must include no_trade")
		} else {
			if proposal.NoTrade.CandidateCount != len(candidates) {
				details = append(details, "no_trade.candidate_count must match market-candidates.json")
			}
			if len(proposal.NoTrade.CheckedMarketSlugs) == 0 && len(candidates) > 0 {
				details = append(details, "no_trade.checked_market_slugs must not be empty when candidates exist")
			}
		}
	}
	return auditCheck{Name: "final_proposal_gate", Passed: len(details) == 0, Details: details}
}

func executionArtifactsCheck(preview types.ExecutionPreview, previewOK bool, portfolio types.PortfolioSnapshot, portfolioOK bool) auditCheck {
	var details []string
	if !previewOK {
		details = append(details, "execution-preview.json is missing or invalid")
	}
	if !portfolioOK {
		details = append(details, "post-portfolio.json is missing or invalid")
	}
	if portfolioOK && portfolio.NavUSD <= 0 {
		details = append(details, "portfolio NAV must be positive")
	}
	_ = preview
	return auditCheck{Name: "paper_execution_artifacts", Passed: len(details) == 0, Details: details}
}

func validationDetails(errors []types.ValidationError) []string {
	out := make([]string, 0, len(errors))
	for _, item := range errors {
		out = append(out, item.Gate+":"+item.Code+":"+item.Path)
	}
	return out
}
