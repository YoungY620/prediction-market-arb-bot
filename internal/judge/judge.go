package judge

import (
	"context"
	"encoding/json"
	"strings"

	"prediction-market-arb-bot/internal/types"
)

type Agent interface {
	Run(ctx context.Context, prompt string) (string, error)
}

func Run(ctx context.Context, agent Agent, proposal types.Proposal, retries int) (types.JudgeResult, bool, []types.ValidationError) {
	var formatErrors []types.ValidationError
	for attempt := 0; attempt <= retries; attempt++ {
		raw, err := agent.Run(ctx, buildPrompt(proposal, formatErrors))
		if err != nil {
			formatErrors = []types.ValidationError{vErr("llm_judge", "judge_command_failed", "$", err.Error())}
			continue
		}
		result, errors := parse(raw)
		if len(errors) == 0 {
			return result, false, nil
		}
		formatErrors = errors
	}
	return types.JudgeResult{}, true, formatErrors
}

func parse(raw string) (types.JudgeResult, []types.ValidationError) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return types.JudgeResult{}, []types.ValidationError{vErr("llm_judge", "invalid_judge_json", "$", "judge output must be a single JSON object")}
	}
	var result types.JudgeResult
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		return types.JudgeResult{}, []types.ValidationError{vErr("llm_judge", "invalid_judge_json", "$", err.Error())}
	}
	if result.Passed && len(result.HardFailures) > 0 {
		return types.JudgeResult{}, []types.ValidationError{vErr("llm_judge", "inconsistent_judge_result", "$.passed", "passed cannot be true when hard_failures is non-empty")}
	}
	if !result.Passed && len(result.HardFailures) == 0 {
		return types.JudgeResult{}, []types.ValidationError{vErr("llm_judge", "missing_hard_failures", "$.hard_failures", "failed judge result must include hard_failures")}
	}
	return result, nil
}

func buildPrompt(proposal types.Proposal, previous []types.ValidationError) string {
	data, _ := json.MarshalIndent(proposal, "", "  ")
	errors, _ := json.MarshalIndent(previous, "", "  ")
	return "Judge this paper-simulation proposal for research quality and probability reasoning only. Return only JSON with fields cycle_id, passed, hard_failures, warnings. Do not modify the proposal.\nPrevious format errors:\n" + string(errors) + "\nProposal:\n" + string(data)
}

func vErr(gate, code, path, message string) types.ValidationError {
	return types.ValidationError{Gate: gate, Code: code, Path: path, Message: message}
}
