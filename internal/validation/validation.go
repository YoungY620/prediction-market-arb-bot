package validation

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"prediction-market-arb-bot/internal/config"
	"prediction-market-arb-bot/internal/types"
)

const floatTolerance = 0.000001

func ParseProposal(raw string) (types.Proposal, []types.ValidationError) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return types.Proposal{}, []types.ValidationError{err("schema_gate", "not_pure_json", "$", "agent output must be a single JSON object")}
	}
	var proposal types.Proposal
	if e := json.Unmarshal([]byte(trimmed), &proposal); e != nil {
		return types.Proposal{}, []types.ValidationError{err("schema_gate", "invalid_json", "$", e.Error())}
	}
	return proposal, nil
}

func ValidateProposal(proposal types.Proposal, cfg config.Config, candidates []types.MarketCandidate, portfolio types.PortfolioSnapshot) types.ValidationResult {
	result := types.ValidationResult{CycleID: proposal.CycleID, Passed: true}
	result.Errors = append(result.Errors, schemaGate(proposal)...)
	result.Errors = append(result.Errors, researchFieldGate(proposal)...)
	result.Errors = append(result.Errors, noTradeGate(proposal, candidates)...)
	result.Errors = append(result.Errors, marketBindingGate(proposal, cfg, candidates)...)
	result.Errors = append(result.Errors, probabilityEdgeGate(proposal, cfg)...)
	result.Errors = append(result.Errors, portfolioRiskGate(proposal, cfg, portfolio)...)
	if len(result.Errors) > 0 {
		result.Passed = false
	}
	return result
}

func noTradeGate(p types.Proposal, candidates []types.MarketCandidate) []types.ValidationError {
	if len(p.PositionReviews) > 0 || len(p.NewOpportunities) > 0 || p.NoTrade == nil {
		return nil
	}
	var errors []types.ValidationError
	if p.NoTrade.CandidateCount != len(candidates) {
		errors = append(errors, err("no_trade_gate", "candidate_count_mismatch", "$.no_trade.candidate_count", "no_trade.candidate_count must match current candidate count"))
	}
	if len(candidates) > 0 && len(p.NoTrade.CheckedMarketSlugs) == 0 {
		errors = append(errors, err("no_trade_gate", "missing_checked_markets", "$.no_trade.checked_market_slugs", "no_trade.checked_market_slugs must list checked candidates when candidates exist"))
	}
	return errors
}

func schemaGate(p types.Proposal) []types.ValidationError {
	var errors []types.ValidationError
	if p.SchemaVersion == "" {
		errors = append(errors, err("schema_gate", "missing_schema_version", "$.schema_version", "schema_version is required"))
	}
	if p.CycleID == "" {
		errors = append(errors, err("schema_gate", "missing_cycle_id", "$.cycle_id", "cycle_id is required"))
	}
	if p.CreatedAt == "" {
		errors = append(errors, err("schema_gate", "missing_created_at", "$.created_at", "created_at is required"))
	}
	if len(p.PositionReviews) == 0 && len(p.NewOpportunities) == 0 {
		if p.NoTrade == nil {
			errors = append(errors, err("schema_gate", "missing_no_trade_reason", "$.no_trade", "empty proposal requires structured no_trade reason"))
		} else {
			if placeholder(p.NoTrade.Reason) {
				errors = append(errors, err("schema_gate", "missing_no_trade_reason_text", "$.no_trade.reason", "no_trade.reason is required"))
			}
			if p.NoTrade.CandidateCount < 0 {
				errors = append(errors, err("schema_gate", "invalid_no_trade_candidate_count", "$.no_trade.candidate_count", "candidate_count cannot be negative"))
			}
			if len(p.NoTrade.DataHealth) == 0 {
				errors = append(errors, err("schema_gate", "missing_no_trade_data_health", "$.no_trade.data_health", "no_trade.data_health must describe data availability"))
			}
			if len(p.NoTrade.MissingRequirements) == 0 {
				errors = append(errors, err("schema_gate", "missing_no_trade_requirements", "$.no_trade.missing_requirements", "no_trade.missing_requirements must explain why no item is trade-ready"))
			}
		}
	}
	ids := map[string]bool{}
	for i, item := range p.NewOpportunities {
		path := fmt.Sprintf("$.new_opportunities[%d]", i)
		errors = append(errors, validateDecisionBasics(path, item.DecisionID, item.MarketSlug, item.TokenID, item.Outcome, item.Action, item.AIProbability, item.MarketProbability, item.Edge)...)
		if ids[item.DecisionID] && item.DecisionID != "" {
			errors = append(errors, err("schema_gate", "duplicate_decision_id", path+".decision_id", "decision_id must be unique"))
		}
		ids[item.DecisionID] = true
		if item.Action == "open" && item.Order.ProposedNotionalUSD <= 0 {
			errors = append(errors, err("schema_gate", "missing_order_notional", path+".order.proposed_notional_usd", "open requires positive proposed_notional_usd"))
		}
		if item.Order.OrderType == "GTC" || item.Order.OrderType == "GTD" {
			errors = append(errors, err("execution_preview_gate", "unsupported_gtc", path+".order.order_type", "MVP does not support GTC/GTD lifecycle"))
		}
	}
	for i, item := range p.PositionReviews {
		path := fmt.Sprintf("$.position_reviews[%d]", i)
		errors = append(errors, validateDecisionBasics(path, item.DecisionID, item.MarketSlug, item.TokenID, item.Outcome, item.Action, item.AIProbability, item.MarketProbability, item.Edge)...)
		if ids[item.DecisionID] && item.DecisionID != "" {
			errors = append(errors, err("schema_gate", "duplicate_decision_id", path+".decision_id", "decision_id must be unique"))
		}
		ids[item.DecisionID] = true
		if item.Order.OrderType == "GTC" || item.Order.OrderType == "GTD" {
			errors = append(errors, err("execution_preview_gate", "unsupported_gtc", path+".order.order_type", "MVP does not support GTC/GTD lifecycle"))
		}
	}
	return errors
}

func validateDecisionBasics(path, id, marketSlug, tokenID, outcome, action string, aiProb, marketProb, edge float64) []types.ValidationError {
	var errors []types.ValidationError
	if placeholder(id) {
		errors = append(errors, err("schema_gate", "missing_decision_id", path+".decision_id", "decision_id is required"))
	}
	if placeholder(marketSlug) {
		errors = append(errors, err("schema_gate", "missing_market_slug", path+".market_slug", "market_slug is required"))
	}
	if placeholder(tokenID) {
		errors = append(errors, err("schema_gate", "missing_token_id", path+".token_id", "token_id is required"))
	}
	if placeholder(outcome) {
		errors = append(errors, err("schema_gate", "missing_outcome", path+".outcome", "outcome is required"))
	}
	if !oneOf(action, "open", "watchlist", "skip", "hold", "reduce", "close", "redeem", "needs_review") {
		errors = append(errors, err("schema_gate", "invalid_action", path+".action", "action is not supported"))
	}
	if action == "redeem" {
		errors = append(errors, err("execution_preview_gate", "unsupported_redeem", path+".action", "MVP does not support redeem execution"))
	}
	if aiProb < 0 || aiProb > 1 {
		errors = append(errors, err("schema_gate", "invalid_ai_probability", path+".ai_probability", "ai_probability must be in [0,1]"))
	}
	if marketProb < 0 || marketProb > 1 {
		errors = append(errors, err("schema_gate", "invalid_market_probability", path+".market_probability", "market_probability must be in [0,1]"))
	}
	if math.Abs(edge-(aiProb-marketProb)) > floatTolerance {
		errors = append(errors, err("schema_gate", "incorrect_edge", path+".edge", "edge must equal ai_probability - market_probability"))
	}
	return errors
}

func researchFieldGate(p types.Proposal) []types.ValidationError {
	var errors []types.ValidationError
	for i, item := range p.NewOpportunities {
		if item.Action != "open" {
			continue
		}
		path := fmt.Sprintf("$.new_opportunities[%d]", i)
		if placeholder(item.Resolution.SourceURL) {
			errors = append(errors, err("research_field_gate", "missing_resolution_source", path+".resolution.source_url", "open requires resolution source"))
		}
		if placeholder(item.Resolution.RuleExcerpt) {
			errors = append(errors, err("research_field_gate", "missing_resolution_rule", path+".resolution.rule_excerpt", "open requires resolution rule excerpt"))
		}
		if placeholder(item.Resolution.TimeWindow) {
			errors = append(errors, err("research_field_gate", "missing_time_window", path+".resolution.time_window", "open requires resolution time window"))
		}
		if len(item.Resolution.UnresolvedAmbiguities) > 0 {
			errors = append(errors, err("research_field_gate", "unresolved_ambiguity", path+".resolution.unresolved_ambiguities", "open cannot have unresolved ambiguities"))
		}
		if len(item.Evidence) < 2 {
			errors = append(errors, err("research_field_gate", "insufficient_evidence", path+".evidence", "open requires at least 2 evidence records"))
		}
		if len(item.CounterEvidence) < 1 {
			errors = append(errors, err("research_field_gate", "missing_counter_evidence", path+".counter_evidence", "open requires at least 1 counter-evidence record"))
		}
		errors = append(errors, validateEvidence(path+".evidence", item.Evidence)...)
		errors = append(errors, validateEvidence(path+".counter_evidence", item.CounterEvidence)...)
		if onlyWeakEvidence(item.Evidence) {
			errors = append(errors, err("research_field_gate", "weak_evidence_only", path+".evidence", "evidence cannot be only comment, social, or background"))
		}
	}
	return errors
}

func validateEvidence(path string, evidence []types.Evidence) []types.ValidationError {
	var errors []types.ValidationError
	for i, ev := range evidence {
		p := fmt.Sprintf("%s[%d]", path, i)
		if placeholder(ev.SourceURL) {
			errors = append(errors, err("research_field_gate", "missing_source_url", p+".source_url", "source_url is required"))
		}
		if placeholder(ev.SourceType) {
			errors = append(errors, err("research_field_gate", "missing_source_type", p+".source_type", "source_type is required"))
		}
		if placeholder(ev.RetrievedAt) {
			errors = append(errors, err("research_field_gate", "missing_retrieved_at", p+".retrieved_at", "retrieved_at is required"))
		}
		if placeholder(ev.Claim) {
			errors = append(errors, err("research_field_gate", "missing_claim", p+".claim", "claim is required"))
		}
		if placeholder(ev.ContractRelevance) {
			errors = append(errors, err("research_field_gate", "missing_contract_relevance", p+".contract_relevance", "contract_relevance is required"))
		}
	}
	return errors
}

func marketBindingGate(p types.Proposal, cfg config.Config, candidates []types.MarketCandidate) []types.ValidationError {
	var errors []types.ValidationError
	byToken := map[string]types.MarketCandidate{}
	for _, candidate := range candidates {
		for _, tokenID := range candidate.TokenIDs {
			byToken[tokenID] = candidate
		}
	}
	for i, item := range p.NewOpportunities {
		if item.Action != "open" {
			continue
		}
		path := fmt.Sprintf("$.new_opportunities[%d]", i)
		candidate, ok := byToken[item.TokenID]
		if !ok {
			errors = append(errors, err("market_binding_gate", "unknown_token_id", path+".token_id", "token_id is not present in current market candidates"))
			continue
		}
		if candidate.MarketSlug != item.MarketSlug {
			errors = append(errors, err("market_binding_gate", "market_slug_mismatch", path+".market_slug", "market_slug does not match token candidate"))
		}
		idx := indexOfFold(candidate.Outcomes, item.Outcome)
		if idx < 0 {
			errors = append(errors, err("market_binding_gate", "outcome_mismatch", path+".outcome", "outcome does not exist in market"))
			continue
		}
		if idx < len(candidate.OutcomePrices) {
			diff := relativeDiff(item.MarketProbability, candidate.OutcomePrices[idx])
			if diff > cfg.PriceTolerancePct {
				errors = append(errors, err("market_binding_gate", "market_probability_mismatch", path+".market_probability", "market_probability differs from candidate outcome price"))
			}
		}
	}
	return errors
}

func probabilityEdgeGate(p types.Proposal, cfg config.Config) []types.ValidationError {
	var errors []types.ValidationError
	for i, item := range p.NewOpportunities {
		if item.Action != "open" {
			continue
		}
		path := fmt.Sprintf("$.new_opportunities[%d]", i)
		if item.Edge <= 0 {
			errors = append(errors, err("probability_edge_gate", "non_positive_edge_open", path+".edge", "open requires positive edge"))
		}
		if item.Edge < cfg.EdgeThresholdPct {
			errors = append(errors, err("probability_edge_gate", "edge_below_threshold", path+".edge", "open edge is below configured threshold"))
		}
	}
	return errors
}

func portfolioRiskGate(p types.Proposal, cfg config.Config, portfolio types.PortfolioSnapshot) []types.ValidationError {
	var errors []types.ValidationError
	openCount := len(portfolio.Positions)
	totalExposure := portfolio.TotalPositionValueUSD
	eventExposure := map[string]float64{}
	for _, pos := range portfolio.Positions {
		eventExposure[pos.EventSlug] += pos.CurrentValueUSD
	}
	for i, item := range p.NewOpportunities {
		if item.Action != "open" {
			continue
		}
		path := fmt.Sprintf("$.new_opportunities[%d]", i)
		if openCount >= cfg.MaxPositions {
			errors = append(errors, err("portfolio_risk_gate", "max_positions", path, "max positions reached"))
		}
		if item.Order.ProposedNotionalUSD < cfg.MinTradeUSD {
			errors = append(errors, err("portfolio_risk_gate", "below_min_trade", path+".order.proposed_notional_usd", "proposed order is below internal minimum trade size"))
		}
		if item.Order.ProposedNotionalUSD > portfolio.NavUSD*cfg.MaxSingleTradePct {
			errors = append(errors, err("portfolio_risk_gate", "max_single_trade", path+".order.proposed_notional_usd", "proposed order exceeds max single trade cap"))
		}
		if totalExposure+item.Order.ProposedNotionalUSD > portfolio.NavUSD*cfg.MaxTotalExposurePct {
			errors = append(errors, err("portfolio_risk_gate", "max_total_exposure", path+".order.proposed_notional_usd", "proposed order exceeds total exposure cap"))
		}
		if eventExposure[item.EventSlug]+item.Order.ProposedNotionalUSD > portfolio.NavUSD*cfg.MaxEventExposurePct {
			errors = append(errors, err("portfolio_risk_gate", "max_event_exposure", path+".order.proposed_notional_usd", "proposed order exceeds event exposure cap"))
		}
	}
	return errors
}

func err(gate, code, path, message string) types.ValidationError {
	return types.ValidationError{Gate: gate, Code: code, Path: path, Message: message}
}

func placeholder(value string) bool {
	v := strings.TrimSpace(strings.ToLower(value))
	return v == "" || v == "n/a" || v == "unknown" || v == "todo" || v == "null"
}

func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

func onlyWeakEvidence(evidence []types.Evidence) bool {
	if len(evidence) == 0 {
		return false
	}
	for _, ev := range evidence {
		t := strings.ToLower(ev.SourceType)
		r := strings.ToLower(ev.ContractRelevance)
		if t != "comment" && t != "social" && r != "background" {
			return false
		}
	}
	return true
}

func indexOfFold(items []string, target string) int {
	for i, item := range items {
		if strings.EqualFold(item, target) {
			return i
		}
	}
	return -1
}

func relativeDiff(a, b float64) float64 {
	return math.Abs(a-b) / math.Max(math.Abs(b), 0.01)
}
