package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"prediction-market-arb-bot/internal/config"
	"prediction-market-arb-bot/internal/execution"
	"prediction-market-arb-bot/internal/judge"
	"prediction-market-arb-bot/internal/marketdata"
	"prediction-market-arb-bot/internal/portfolio"
	"prediction-market-arb-bot/internal/types"
	"prediction-market-arb-bot/internal/validation"
)

type Agent interface {
	Run(ctx context.Context, prompt string) (string, error)
}

type CycleResult struct {
	CycleID  string
	CycleDir string
}

func RunPaperCycle(ctx context.Context, cfg config.Config, agent Agent) (CycleResult, error) {
	cycleID := time.Now().UTC().Format("20060102T150405Z")
	cycleDir := filepath.Join(cfg.RuntimeDir, "cycles", cycleID)
	if err := os.MkdirAll(cycleDir, 0o755); err != nil {
		return CycleResult{}, err
	}

	market := marketdata.New(cfg)
	candidates, err := market.FetchCandidates(ctx)
	if err != nil {
		writeJSON(filepath.Join(cycleDir, "error.json"), map[string]any{"code": "market_data_failed", "message": err.Error()})
		return CycleResult{}, err
	}
	current := loadOrInitPortfolio(cfg, cycleID)
	contextPayload := map[string]any{
		"cycle_id":          cycleID,
		"portfolio":         current,
		"market_candidates": candidates,
	}

	var proposal types.Proposal
	var result types.ValidationResult
	var preview types.ExecutionPreview
	var books map[string]types.OrderBook
	var lastErrors []types.ValidationError

	for attempt := 0; attempt <= cfg.MaxRepairAttempts; attempt++ {
		raw, err := agent.Run(ctx, buildPrompt(contextPayload, lastErrors))
		if err != nil {
			writeJSON(filepath.Join(cycleDir, "error.json"), map[string]any{"code": "agent_failed", "message": err.Error()})
			return CycleResult{}, err
		}
		parsed, parseErrors := validation.ParseProposal(raw)
		if len(parseErrors) > 0 {
			lastErrors = parseErrors
			if attempt == cfg.MaxRepairAttempts {
				return failCycle(cycleDir, cycleID, lastErrors)
			}
			continue
		}
		proposal = parsed
		proposal.CycleID = cycleID
		if proposal.CreatedAt == "" {
			proposal.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		}
		books = readBooksForProposal(ctx, market, proposal)
		result = validation.ValidateProposal(proposal, cfg, candidates, current)
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
		if result.Passed {
			break
		}
		lastErrors = result.Errors
		if attempt == cfg.MaxRepairAttempts {
			writeJSON(filepath.Join(cycleDir, "proposal.json"), proposal)
			writeJSON(filepath.Join(cycleDir, "validation-result.json"), result)
			return failCycle(cycleDir, cycleID, lastErrors)
		}
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

func buildPrompt(contextPayload map[string]any, errors []types.ValidationError) string {
	contextJSON, _ := json.MarshalIndent(contextPayload, "", "  ")
	var b strings.Builder
	b.WriteString("You are producing a JSON-only research proposal for a local paper simulation. No real money, live orders, wallet calls, or trading API calls will be used.\n")
	b.WriteString("Return only a single valid JSON object. No markdown. No commentary. No code fence.\n")
	b.WriteString("MVP supports only FOK-style paper execution; do not output GTC or GTD.\n")
	b.WriteString("Every open item needs resolution, evidence, counter_evidence, probabilities, edge, and order intent.\n")
	b.WriteString("Schema fields: schema_version, cycle_id, created_at, position_reviews, new_opportunities.\n")
	b.WriteString("Use only market candidates present in the context for new opportunities.\n")
	if len(errors) > 0 {
		errorsJSON, _ := json.MarshalIndent(errors, "", "  ")
		b.WriteString("\nPrevious validation errors. Repair the full JSON proposal, not a patch:\n")
		b.Write(errorsJSON)
		b.WriteString("\n")
	}
	b.WriteString("\nContext:\n")
	b.Write(contextJSON)
	return b.String()
}
