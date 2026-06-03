package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"prediction-market-arb-bot/internal/types"
)

type summaryEvent struct {
	Event         string `json:"event"`
	Run           int    `json:"run"`
	ExitCode      int    `json:"exit_code"`
	AuditExitCode *int   `json:"audit_exit_code"`
	CycleDir      string `json:"cycle_dir"`
	Log           string `json:"log"`
	AuditLog      string `json:"audit_log"`
}

type aggregate struct {
	SummaryPath          string   `json:"summary_path"`
	Runs                 int      `json:"runs"`
	PaperCyclePassed     int      `json:"paper_cycle_passed"`
	AuditedRuns          int      `json:"audited_runs"`
	AuditPassed          int      `json:"audit_passed"`
	CandidateCountTotal  int      `json:"candidate_count_total"`
	DiscoveredCountTotal int      `json:"discovered_candidate_count_total"`
	WorkerTaskTotal      int      `json:"worker_task_total"`
	WorkerPassedTotal    int      `json:"worker_passed_total"`
	WorkerFailedTotal    int      `json:"worker_failed_total"`
	NewOpportunityTotal  int      `json:"new_opportunity_total"`
	PositionReviewTotal  int      `json:"position_review_total"`
	FailedRuns           []string `json:"failed_runs"`
}

func main() {
	flag.Parse()
	if flag.NArg() != 1 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: go run ./cmd/smoke-summary runtime/hourly-smoke/summary.jsonl")
		os.Exit(2)
	}
	out, err := summarize(flag.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(out)
}

func summarize(path string) (aggregate, error) {
	file, err := os.Open(path)
	if err != nil {
		return aggregate{}, err
	}
	defer file.Close()
	out := aggregate{SummaryPath: path, FailedRuns: []string{}}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event summaryEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil || event.Event != "end" {
			continue
		}
		out.Runs++
		if event.ExitCode == 0 {
			out.PaperCyclePassed++
		}
		if event.AuditExitCode != nil {
			out.AuditedRuns++
			if *event.AuditExitCode == 0 {
				out.AuditPassed++
			} else {
				out.FailedRuns = append(out.FailedRuns, event.CycleDir)
			}
		} else {
			out.FailedRuns = append(out.FailedRuns, event.CycleDir)
		}
		addCycleMetrics(&out, event.CycleDir)
	}
	return out, scanner.Err()
}

func addCycleMetrics(out *aggregate, cycleDir string) {
	var health map[string]any
	if readJSON(filepath.Join(cycleDir, "market-data-health.json"), &health) == nil {
		out.CandidateCountTotal += intNumber(health["candidate_count"])
		out.DiscoveredCountTotal += intNumber(health["discovered_candidate_count"])
	}
	var index types.ResearchIndex
	if readJSON(filepath.Join(cycleDir, "research-index.json"), &index) == nil {
		out.WorkerTaskTotal += index.TaskCount
		out.WorkerPassedTotal += index.PassedCount
		out.WorkerFailedTotal += index.FailedCount
	}
	var proposal types.Proposal
	if readJSON(filepath.Join(cycleDir, "proposal.json"), &proposal) == nil {
		out.NewOpportunityTotal += len(proposal.NewOpportunities)
		out.PositionReviewTotal += len(proposal.PositionReviews)
	}
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func intNumber(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}
