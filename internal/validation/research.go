package validation

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"prediction-market-arb-bot/internal/types"
)

func ParseResearchPlan(raw string) (types.ResearchPlan, []types.ValidationError) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return types.ResearchPlan{}, []types.ValidationError{err("coordinator_schema_gate", "not_pure_json", "$", "coordinator output must be a single JSON object")}
	}
	var plan types.ResearchPlan
	if e := json.Unmarshal([]byte(trimmed), &plan); e != nil {
		return types.ResearchPlan{}, []types.ValidationError{err("coordinator_schema_gate", "invalid_json", "$", e.Error())}
	}
	return plan, nil
}

func ValidateResearchPlan(plan types.ResearchPlan, cycleID string, candidates []types.MarketCandidate) types.ValidationResult {
	result := types.ValidationResult{CycleID: cycleID, Passed: true}
	byEvent := candidatesByEvent(candidates)
	byMarket := candidatesByMarket(candidates)
	seenTasks := map[string]bool{}
	coveredEvents := map[string]bool{}
	coveredMarkets := map[string]bool{}
	for i, task := range plan.Tasks {
		path := fmt.Sprintf("$.tasks[%d]", i)
		if placeholder(task.TaskID) {
			result.Errors = append(result.Errors, err("coordinator_gate", "missing_task_id", path+".task_id", "task_id is required"))
		}
		if seenTasks[task.TaskID] && task.TaskID != "" {
			result.Errors = append(result.Errors, err("coordinator_gate", "duplicate_task_id", path+".task_id", "task_id must be unique"))
		}
		seenTasks[task.TaskID] = true
		if placeholder(task.EventSlug) {
			result.Errors = append(result.Errors, err("coordinator_gate", "missing_event_slug", path+".event_slug", "event_slug is required"))
		} else if len(byEvent[task.EventSlug]) == 0 {
			result.Errors = append(result.Errors, err("coordinator_gate", "unknown_event_slug", path+".event_slug", "event_slug is not present in current candidates"))
		}
		coveredEvents[task.EventSlug] = true
		if placeholder(task.Domain) {
			result.Errors = append(result.Errors, err("coordinator_gate", "missing_domain", path+".domain", "domain is required for worker routing"))
		}
		if placeholder(task.ResearchProfile) {
			result.Errors = append(result.Errors, err("coordinator_gate", "missing_research_profile", path+".research_profile", "research_profile is required"))
		}
		if len(task.MarketSlugs) == 0 {
			result.Errors = append(result.Errors, err("coordinator_gate", "missing_market_slugs", path+".market_slugs", "event task must include market_slugs"))
		}
		taskMarketDetails := map[string]bool{}
		taskMarketsBySlug := map[string]types.MarketCandidate{}
		for _, market := range task.Markets {
			taskMarketDetails[market.MarketSlug] = true
			taskMarketsBySlug[market.MarketSlug] = market
		}
		for j, slug := range task.MarketSlugs {
			candidate, ok := byMarket[slug]
			marketPath := fmt.Sprintf("%s.market_slugs[%d]", path, j)
			if !ok {
				result.Errors = append(result.Errors, err("coordinator_gate", "unknown_market_slug", marketPath, "market_slug is not present in current candidates"))
				continue
			}
			coveredMarkets[slug] = true
			if !taskMarketDetails[slug] {
				result.Errors = append(result.Errors, err("coordinator_gate", "missing_market_detail", path+".markets", "task markets must include full candidate detail for "+slug))
			} else {
				result.Errors = append(result.Errors, validateTaskMarketDetail(path+".markets", taskMarketsBySlug[slug], candidate)...)
			}
			if candidate.EventSlug != task.EventSlug {
				result.Errors = append(result.Errors, err("coordinator_gate", "market_event_mismatch", marketPath, "market_slug does not belong to task event_slug"))
			}
		}
	}
	for eventSlug := range byEvent {
		if !coveredEvents[eventSlug] {
			result.Errors = append(result.Errors, err("coordinator_gate", "event_not_assigned", "$.tasks", "every candidate event must be assigned to a worker task: "+eventSlug))
		}
	}
	for marketSlug := range byMarket {
		if !coveredMarkets[marketSlug] {
			result.Errors = append(result.Errors, err("coordinator_gate", "market_not_assigned", "$.tasks", "every candidate market must be assigned to a worker task: "+marketSlug))
		}
	}
	if len(candidates) > 0 && len(plan.Tasks) == 0 {
		result.Errors = append(result.Errors, err("coordinator_gate", "missing_tasks", "$.tasks", "non-empty candidate set requires worker tasks"))
	}
	if len(result.Errors) > 0 {
		result.Passed = false
	}
	return result
}

func validateTaskMarketDetail(path string, market types.MarketCandidate, candidate types.MarketCandidate) []types.ValidationError {
	var errors []types.ValidationError
	if placeholder(market.Question) {
		errors = append(errors, err("coordinator_gate", "missing_market_question", path, "task market detail must include question for "+candidate.MarketSlug))
	}
	if len(market.TokenIDs) != len(candidate.TokenIDs) || len(market.TokenIDs) == 0 {
		errors = append(errors, err("coordinator_gate", "missing_market_token_ids", path, "task market detail must include token_ids for "+candidate.MarketSlug))
	}
	if len(market.Outcomes) != len(candidate.Outcomes) || len(market.Outcomes) == 0 {
		errors = append(errors, err("coordinator_gate", "missing_market_outcomes", path, "task market detail must include outcomes for "+candidate.MarketSlug))
	}
	if len(market.OutcomePrices) != len(candidate.OutcomePrices) || len(market.OutcomePrices) == 0 {
		errors = append(errors, err("coordinator_gate", "missing_market_prices", path, "task market detail must include outcome_prices for "+candidate.MarketSlug))
	}
	return errors
}

func ParseResearchResult(raw string) (types.ResearchResult, []types.ValidationError) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return types.ResearchResult{}, []types.ValidationError{err("worker_schema_gate", "not_pure_json", "$", "worker output must be a single JSON object")}
	}
	var result types.ResearchResult
	if e := json.Unmarshal([]byte(trimmed), &result); e != nil {
		return types.ResearchResult{}, []types.ValidationError{err("worker_schema_gate", "invalid_json", "$", e.Error())}
	}
	return result, nil
}

func ValidateResearchResult(result types.ResearchResult, task types.ResearchTask) types.ValidationResult {
	out := types.ValidationResult{CycleID: task.TaskID, Passed: true}
	if result.TaskID != task.TaskID {
		out.Errors = append(out.Errors, err("worker_gate", "task_id_mismatch", "$.task_id", "result task_id must match assigned task"))
	}
	if result.EventSlug != task.EventSlug {
		out.Errors = append(out.Errors, err("worker_gate", "event_slug_mismatch", "$.event_slug", "result event_slug must match assigned task"))
	}
	if !oneOf(result.Status, "passed", "failed", "needs_followup") {
		out.Errors = append(out.Errors, err("worker_gate", "invalid_status", "$.status", "status must be passed, failed, or needs_followup"))
	}
	validateSummarySignal(&out, result.SummarySignal)
	if result.Status == "failed" {
		if placeholder(result.Error) {
			out.Errors = append(out.Errors, err("worker_gate", "missing_error", "$.error", "failed result must explain the failure"))
		}
		return finish(out)
	}
	validateEvidenceForGate(&out, "$.evidence", result.Evidence)
	validateEvidenceForGate(&out, "$.counter_evidence", result.CounterEvidence)
	if len(result.Evidence) < 2 {
		out.Errors = append(out.Errors, err("worker_gate", "insufficient_evidence", "$.evidence", "worker result requires at least 2 evidence records"))
	}
	if len(result.CounterEvidence) < 1 {
		out.Errors = append(out.Errors, err("worker_gate", "missing_counter_evidence", "$.counter_evidence", "worker result requires at least 1 counter-evidence record"))
	}
	if len(result.UnresolvedAmbiguities) > 0 && result.SummarySignal.Decision == "open_candidate" {
		out.Errors = append(out.Errors, err("worker_gate", "open_with_unresolved_ambiguity", "$.unresolved_ambiguities", "open candidates cannot have unresolved ambiguities"))
	}
	expected := map[string]bool{}
	candidates := map[string]types.MarketCandidate{}
	for _, slug := range task.MarketSlugs {
		expected[slug] = false
	}
	for _, candidate := range task.Markets {
		candidates[candidate.MarketSlug] = candidate
	}
	openAssessments := map[string]types.MarketAssessment{}
	for i, item := range result.MarketAssessments {
		path := fmt.Sprintf("$.market_assessments[%d]", i)
		candidate, ok := candidates[item.MarketSlug]
		if _, assigned := expected[item.MarketSlug]; !assigned {
			out.Errors = append(out.Errors, err("worker_gate", "unknown_market_slug", path+".market_slug", "assessment market_slug is not in assigned task"))
		} else {
			expected[item.MarketSlug] = true
		}
		if !oneOf(item.Recommendation, "open_candidate", "watchlist", "skip") {
			out.Errors = append(out.Errors, err("worker_gate", "invalid_recommendation", path+".recommendation", "recommendation must be open_candidate, watchlist, or skip"))
		}
		if item.AIProbability < 0 || item.AIProbability > 1 {
			out.Errors = append(out.Errors, err("worker_gate", "invalid_ai_probability", path+".ai_probability", "ai_probability must be in [0,1]"))
		}
		if item.MarketProbability < 0 || item.MarketProbability > 1 {
			out.Errors = append(out.Errors, err("worker_gate", "invalid_market_probability", path+".market_probability", "market_probability must be in [0,1]"))
		}
		if math.Abs(item.Edge-(item.AIProbability-item.MarketProbability)) > floatTolerance {
			out.Errors = append(out.Errors, err("worker_gate", "incorrect_edge", path+".edge", "edge must equal ai_probability - market_probability"))
		}
		if item.Recommendation == "open_candidate" && placeholder(item.Thesis) {
			out.Errors = append(out.Errors, err("worker_gate", "missing_thesis", path+".thesis", "open_candidate requires a thesis"))
		}
		if item.Recommendation == "open_candidate" {
			openAssessments[item.MarketSlug] = item
			out.Errors = append(out.Errors, validateOpenAssessmentBinding(path, item, candidate, ok)...)
		}
	}
	for slug, covered := range expected {
		if !covered {
			out.Errors = append(out.Errors, err("worker_gate", "market_not_assessed", "$.market_assessments", "every assigned market must be assessed: "+slug))
		}
	}
	if result.SummarySignal.Decision == "open_candidate" {
		out.Errors = append(out.Errors, validateOpenSummarySignal(result.SummarySignal, openAssessments)...)
		out.Errors = append(out.Errors, domainEvidenceGate(task.Domain, result.Evidence)...)
	}
	return finish(out)
}

func validateOpenAssessmentBinding(path string, item types.MarketAssessment, candidate types.MarketCandidate, candidateOK bool) []types.ValidationError {
	var errors []types.ValidationError
	if placeholder(item.TokenID) {
		errors = append(errors, err("worker_binding_gate", "missing_token_id", path+".token_id", "open_candidate assessment requires token_id"))
	}
	if placeholder(item.Outcome) {
		errors = append(errors, err("worker_binding_gate", "missing_outcome", path+".outcome", "open_candidate assessment requires outcome"))
	}
	if !candidateOK {
		return errors
	}
	tokenIndex := indexOf(candidate.TokenIDs, item.TokenID)
	if tokenIndex < 0 {
		errors = append(errors, err("worker_binding_gate", "unknown_token_id", path+".token_id", "token_id is not present in assigned market candidate"))
	}
	outcomeIndex := indexOfFold(candidate.Outcomes, item.Outcome)
	if outcomeIndex < 0 {
		errors = append(errors, err("worker_binding_gate", "outcome_mismatch", path+".outcome", "outcome does not exist in assigned market candidate"))
		return errors
	}
	if tokenIndex >= 0 && tokenIndex != outcomeIndex {
		errors = append(errors, err("worker_binding_gate", "token_outcome_mismatch", path+".token_id", "token_id must correspond to the selected outcome index"))
	}
	if outcomeIndex < len(candidate.OutcomePrices) && relativeDiff(item.MarketProbability, candidate.OutcomePrices[outcomeIndex]) > 0.03 {
		errors = append(errors, err("worker_binding_gate", "market_probability_mismatch", path+".market_probability", "market_probability must match assigned candidate outcome price"))
	}
	return errors
}

func validateOpenSummarySignal(signal types.SummarySignal, openAssessments map[string]types.MarketAssessment) []types.ValidationError {
	var errors []types.ValidationError
	if signal.MaxEdge <= 0 {
		errors = append(errors, err("worker_signal_gate", "missing_positive_max_edge", "$.summary_signal.max_edge", "open_candidate summary requires positive max_edge"))
	}
	if signal.Confidence <= 0 {
		errors = append(errors, err("worker_signal_gate", "missing_positive_confidence", "$.summary_signal.confidence", "open_candidate summary requires positive confidence"))
	}
	if len(signal.MarketSlugsOfInterest) == 0 {
		errors = append(errors, err("worker_signal_gate", "missing_market_slugs_of_interest", "$.summary_signal.market_slugs_of_interest", "open_candidate summary must name market slugs of interest"))
	}
	if len(openAssessments) == 0 {
		errors = append(errors, err("worker_signal_gate", "missing_open_assessment", "$.market_assessments", "open_candidate summary requires at least one open_candidate market assessment"))
	}
	maxEdge := 0.0
	for _, item := range openAssessments {
		if item.Edge > maxEdge {
			maxEdge = item.Edge
		}
	}
	if signal.MaxEdge > 0 && maxEdge > 0 && math.Abs(signal.MaxEdge-maxEdge) > 0.03 {
		errors = append(errors, err("worker_signal_gate", "max_edge_mismatch", "$.summary_signal.max_edge", "summary max_edge should match the strongest open_candidate assessment edge"))
	}
	for i, slug := range signal.MarketSlugsOfInterest {
		if _, ok := openAssessments[slug]; !ok {
			errors = append(errors, err("worker_signal_gate", "interest_slug_not_open", fmt.Sprintf("$.summary_signal.market_slugs_of_interest[%d]", i), "market_slugs_of_interest must refer to open_candidate assessments"))
		}
	}
	return errors
}

func validateSummarySignal(out *types.ValidationResult, signal types.SummarySignal) {
	if !oneOf(signal.Decision, "open_candidate", "watchlist", "skip", "failed", "needs_followup") {
		out.Errors = append(out.Errors, err("worker_gate", "invalid_summary_decision", "$.summary_signal.decision", "summary decision is not supported"))
	}
	if signal.Confidence < 0 || signal.Confidence > 1 {
		out.Errors = append(out.Errors, err("worker_gate", "invalid_confidence", "$.summary_signal.confidence", "confidence must be in [0,1]"))
	}
	if placeholder(signal.ReasonShort) {
		out.Errors = append(out.Errors, err("worker_gate", "missing_reason_short", "$.summary_signal.reason_short", "summary_signal.reason_short is required"))
	}
}

func validateEvidenceForGate(out *types.ValidationResult, path string, evidence []types.Evidence) {
	out.Errors = append(out.Errors, validateEvidence(path, evidence)...)
	if onlyWeakEvidence(evidence) {
		out.Errors = append(out.Errors, err("worker_gate", "weak_evidence_only", path, "evidence cannot be only comment, social, or background"))
	}
}

func domainEvidenceGate(domain string, evidence []types.Evidence) []types.ValidationError {
	domain = strings.ToLower(domain)
	joined := strings.ToLower(evidenceText(evidence))
	switch {
	case strings.Contains(domain, "sport"):
		return requireAnyDomainEvidence("sports_domain_gate", joined, []string{"bookmaker", "odds", "injury", "lineup", "rating", "elo", "team news"})
	case strings.Contains(domain, "politic") || strings.Contains(domain, "geo") || strings.Contains(domain, "foreign"):
		return requireAnyDomainEvidence("politics_domain_gate", joined, []string{"official", "poll", "news", "reputable_news", "legal", "policy", "primary"})
	case strings.Contains(domain, "macro") || strings.Contains(domain, "economic") || strings.Contains(domain, "finance"):
		return requireAnyDomainEvidence("macro_domain_gate", joined, []string{"official", "central bank", "market_data", "news", "reputable_news", "statistical", "primary"})
	default:
		return requireAnyDomainEvidence("general_domain_gate", joined, []string{"official", "primary", "news", "reputable_news", "expert", "data"})
	}
}

func requireAnyDomainEvidence(gate, text string, keywords []string) []types.ValidationError {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return nil
		}
	}
	return []types.ValidationError{err(gate, "missing_domain_specific_evidence", "$.evidence", "open_candidate lacks domain-specific evidence")}
}

func evidenceText(evidence []types.Evidence) string {
	var b strings.Builder
	for _, ev := range evidence {
		b.WriteString(ev.SourceType)
		b.WriteByte(' ')
		b.WriteString(ev.Claim)
		b.WriteByte(' ')
		b.WriteString(ev.ContractRelevance)
		b.WriteByte(' ')
	}
	return b.String()
}

func candidatesByEvent(candidates []types.MarketCandidate) map[string][]types.MarketCandidate {
	out := map[string][]types.MarketCandidate{}
	for _, candidate := range candidates {
		out[candidate.EventSlug] = append(out[candidate.EventSlug], candidate)
	}
	return out
}

func candidatesByMarket(candidates []types.MarketCandidate) map[string]types.MarketCandidate {
	out := map[string]types.MarketCandidate{}
	for _, candidate := range candidates {
		out[candidate.MarketSlug] = candidate
	}
	return out
}

func indexOf(items []string, target string) int {
	for i, item := range items {
		if item == target {
			return i
		}
	}
	return -1
}

func finish(result types.ValidationResult) types.ValidationResult {
	if len(result.Errors) > 0 {
		result.Passed = false
	}
	return result
}
