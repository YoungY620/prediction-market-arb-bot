package types

import (
	"encoding/json"
	"strconv"
	"strings"
)

type Evidence struct {
	SourceURL         string  `json:"source_url"`
	SourceType        string  `json:"source_type"`
	RetrievedAt       string  `json:"retrieved_at"`
	Claim             string  `json:"claim"`
	ContractRelevance string  `json:"contract_relevance"`
	Supports          string  `json:"supports"`
	ProbabilityEffect float64 `json:"probability_effect_pp"`
}

func (e *Evidence) UnmarshalJSON(data []byte) error {
	type alias Evidence
	var raw struct {
		alias
		Supports          any `json:"supports"`
		ProbabilityEffect any `json:"probability_effect_pp"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*e = Evidence(raw.alias)
	e.Supports = stringify(raw.Supports)
	if parsed, ok := floatValue(raw.ProbabilityEffect); ok {
		e.ProbabilityEffect = parsed
	}
	return nil
}

type Resolution struct {
	SourceURL             string   `json:"source_url"`
	RuleExcerpt           string   `json:"rule_excerpt"`
	TimeWindow            string   `json:"time_window"`
	Threshold             string   `json:"threshold,omitempty"`
	UnresolvedAmbiguities []string `json:"unresolved_ambiguities"`
}

type OrderIntent struct {
	Side                string  `json:"side"`
	OrderType           string  `json:"order_type"`
	ProposedNotionalUSD float64 `json:"proposed_notional_usd"`
	Required            bool    `json:"required"`
}

type PositionReview struct {
	DecisionID        string      `json:"decision_id"`
	PositionID        string      `json:"position_id"`
	EventSlug         string      `json:"event_slug"`
	MarketSlug        string      `json:"market_slug"`
	TokenID           string      `json:"token_id"`
	Outcome           string      `json:"outcome"`
	Action            string      `json:"action"`
	AIProbability     float64     `json:"ai_probability"`
	MarketProbability float64     `json:"market_probability"`
	Edge              float64     `json:"edge"`
	Evidence          []Evidence  `json:"evidence"`
	CounterEvidence   []Evidence  `json:"counter_evidence"`
	Order             OrderIntent `json:"order,omitempty"`
}

type NewOpportunity struct {
	DecisionID        string      `json:"decision_id"`
	EventSlug         string      `json:"event_slug"`
	MarketSlug        string      `json:"market_slug"`
	TokenID           string      `json:"token_id"`
	Outcome           string      `json:"outcome"`
	Action            string      `json:"action"`
	AIProbability     float64     `json:"ai_probability"`
	MarketProbability float64     `json:"market_probability"`
	Edge              float64     `json:"edge"`
	Resolution        Resolution  `json:"resolution"`
	Evidence          []Evidence  `json:"evidence"`
	CounterEvidence   []Evidence  `json:"counter_evidence"`
	Order             OrderIntent `json:"order"`
}

type Proposal struct {
	SchemaVersion    string           `json:"schema_version"`
	CycleID          string           `json:"cycle_id"`
	CreatedAt        string           `json:"created_at"`
	PositionReviews  []PositionReview `json:"position_reviews"`
	NewOpportunities []NewOpportunity `json:"new_opportunities"`
	NoTrade          *NoTradeReason   `json:"no_trade,omitempty"`
}

type NoTradeReason struct {
	Reason              string   `json:"reason"`
	CandidateCount      int      `json:"candidate_count"`
	CheckedMarketSlugs  []string `json:"checked_market_slugs"`
	MissingRequirements []string `json:"missing_requirements"`
	DataHealth          []string `json:"data_health"`
}

type ValidationError struct {
	Gate    string `json:"gate"`
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

type ValidationResult struct {
	CycleID  string            `json:"cycle_id"`
	Passed   bool              `json:"passed"`
	Errors   []ValidationError `json:"errors"`
	Warnings []ValidationError `json:"warnings"`
}

type AgentRun struct {
	Output    string `json:"output"`
	SessionID string `json:"session_id,omitempty"`
}

type JudgeResult struct {
	CycleID      string            `json:"cycle_id"`
	Passed       bool              `json:"passed"`
	HardFailures []ValidationError `json:"hard_failures"`
	Warnings     []ValidationError `json:"warnings"`
}

type MarketCandidate struct {
	EventSlug       string     `json:"event_slug"`
	EventTitle      string     `json:"event_title,omitempty"`
	MarketSlug      string     `json:"market_slug"`
	Question        string     `json:"question"`
	Outcomes        []string   `json:"outcomes"`
	OutcomePrices   []float64  `json:"outcome_prices"`
	TokenIDs        []string   `json:"token_ids"`
	Resolution      Resolution `json:"resolution"`
	LiquidityUSD    float64    `json:"liquidity_usd"`
	Volume24hUSD    float64    `json:"volume_24h_usd"`
	BestBid         float64    `json:"best_bid,omitempty"`
	BestAsk         float64    `json:"best_ask,omitempty"`
	Spread          float64    `json:"spread,omitempty"`
	EnableOrderBook bool       `json:"enable_order_book"`
	CategorySlug    string     `json:"category_slug,omitempty"`
	CategoryLabel   string     `json:"category_label,omitempty"`
	Tags            []Tag      `json:"tags,omitempty"`
	Score           float64    `json:"score,omitempty"`
	FilterNotes     []string   `json:"filter_notes,omitempty"`
}

type DiscoveryReport struct {
	SortDimensions       []DiscoverySortReport `json:"sort_dimensions"`
	RawEventCount        int                   `json:"raw_event_count"`
	RawMarketCount       int                   `json:"raw_market_count"`
	AcceptedMarketCount  int                   `json:"accepted_market_count"`
	AcceptedEventCount   int                   `json:"accepted_event_count"`
	FilteredMarketCount  int                   `json:"filtered_market_count"`
	FilteredByReason     map[string]int        `json:"filtered_by_reason"`
	DuplicateMarketCount int                   `json:"duplicate_market_count"`
}

type DiscoverySortReport struct {
	Order     string `json:"order"`
	Pages     int    `json:"pages"`
	Events    int    `json:"events"`
	ShortPage bool   `json:"short_page"`
}

type DataSourceHealth struct {
	CheckedAt string            `json:"checked_at"`
	Checks    []DataSourceCheck `json:"checks"`
	Warnings  []ValidationError `json:"warnings"`
}

type DataSourceCheck struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Reachable  bool   `json:"reachable"`
	StatusCode int    `json:"status_code,omitempty"`
	Message    string `json:"message,omitempty"`
}

type Tag struct {
	Slug  string `json:"slug"`
	Label string `json:"label"`
}

type ResearchPlan struct {
	CycleID   string         `json:"cycle_id"`
	CreatedAt string         `json:"created_at"`
	Tasks     []ResearchTask `json:"tasks"`
}

type ResearchTask struct {
	TaskID          string            `json:"task_id"`
	EventSlug       string            `json:"event_slug"`
	EventTitle      string            `json:"event_title"`
	Domain          string            `json:"domain"`
	ResearchProfile string            `json:"research_profile"`
	Priority        float64           `json:"priority"`
	ResearchFocus   []string          `json:"research_focus"`
	MarketSlugs     []string          `json:"market_slugs"`
	Markets         []MarketCandidate `json:"markets"`
}

func (t *ResearchTask) UnmarshalJSON(data []byte) error {
	type alias ResearchTask
	var raw struct {
		alias
		Priority      any `json:"priority"`
		ResearchFocus any `json:"research_focus"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*t = ResearchTask(raw.alias)
	switch value := raw.Priority.(type) {
	case float64:
		t.Priority = value
	case string:
		t.Priority = priorityValue(value)
	}
	switch value := raw.ResearchFocus.(type) {
	case []any:
		t.ResearchFocus = nil
		for _, item := range value {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				t.ResearchFocus = append(t.ResearchFocus, strings.TrimSpace(text))
			}
		}
	case string:
		if strings.TrimSpace(value) != "" {
			t.ResearchFocus = []string{strings.TrimSpace(value)}
		}
	}
	return nil
}

func priorityValue(value string) float64 {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0
		}
		return parsed
	}
}

func stringify(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(data)
	}
}

func floatValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

type ResearchResult struct {
	TaskID                string             `json:"task_id"`
	EventSlug             string             `json:"event_slug"`
	Status                string             `json:"status"`
	SummarySignal         SummarySignal      `json:"summary_signal"`
	MarketAssessments     []MarketAssessment `json:"market_assessments"`
	Evidence              []Evidence         `json:"evidence"`
	CounterEvidence       []Evidence         `json:"counter_evidence"`
	UnresolvedAmbiguities []string           `json:"unresolved_ambiguities"`
	FollowUpRequests      []FollowUpRequest  `json:"follow_up_requests"`
	Error                 string             `json:"error,omitempty"`
}

func (r *ResearchResult) UnmarshalJSON(data []byte) error {
	type alias ResearchResult
	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = ResearchResult(raw)
	switch strings.ToLower(strings.TrimSpace(r.Status)) {
	case "complete", "completed", "ok", "valid":
		r.Status = "passed"
	}
	return nil
}

type SummarySignal struct {
	Decision                  string   `json:"decision"`
	MaxEdge                   float64  `json:"max_edge"`
	Confidence                float64  `json:"confidence"`
	MarketSlugsOfInterest     []string `json:"market_slugs_of_interest"`
	PositionActionsOfInterest []string `json:"position_actions_of_interest"`
	ReasonShort               string   `json:"reason_short"`
}

func (s *SummarySignal) UnmarshalJSON(data []byte) error {
	type alias SummarySignal
	var raw struct {
		alias
		Confidence  any    `json:"confidence"`
		Reason      string `json:"reason"`
		Rationale   string `json:"rationale"`
		Reasoning   string `json:"reasoning"`
		ReasonShort string `json:"reason_short"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = SummarySignal(raw.alias)
	switch strings.ToLower(strings.TrimSpace(s.Decision)) {
	case "hold":
		s.Decision = "watchlist"
	case "no_trade", "no-trade", "none":
		s.Decision = "skip"
	}
	if parsed, ok := floatValue(raw.Confidence); ok {
		s.Confidence = parsed
	}
	for _, candidate := range []string{raw.ReasonShort, raw.Reason, raw.Rationale, raw.Reasoning} {
		if strings.TrimSpace(candidate) != "" {
			s.ReasonShort = strings.TrimSpace(candidate)
			break
		}
	}
	return nil
}

type MarketAssessment struct {
	MarketSlug        string  `json:"market_slug"`
	TokenID           string  `json:"token_id"`
	Outcome           string  `json:"outcome"`
	Recommendation    string  `json:"recommendation"`
	AIProbability     float64 `json:"ai_probability"`
	MarketProbability float64 `json:"market_probability"`
	Edge              float64 `json:"edge"`
	Thesis            string  `json:"thesis"`
}

func (m *MarketAssessment) UnmarshalJSON(data []byte) error {
	type alias MarketAssessment
	var raw struct {
		alias
		AIProbability     any `json:"ai_probability"`
		MarketProbability any `json:"market_probability"`
		Edge              any `json:"edge"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = MarketAssessment(raw.alias)
	if parsed, ok := floatValue(raw.AIProbability); ok {
		m.AIProbability = parsed
	}
	if parsed, ok := floatValue(raw.MarketProbability); ok {
		m.MarketProbability = parsed
	}
	if parsed, ok := floatValue(raw.Edge); ok {
		m.Edge = parsed
	}
	switch strings.ToLower(strings.TrimSpace(m.Recommendation)) {
	case "hold":
		m.Recommendation = "watchlist"
	case "no_trade", "no-trade", "none":
		m.Recommendation = "skip"
	}
	return nil
}

type FollowUpRequest struct {
	Reason      string   `json:"reason"`
	MarketSlugs []string `json:"market_slugs"`
	NeededData  []string `json:"needed_data"`
}

type WorkerRecord struct {
	TaskID         string            `json:"task_id"`
	EventSlug      string            `json:"event_slug"`
	Domain         string            `json:"domain"`
	Status         string            `json:"status"`
	ResultPath     string            `json:"result_path"`
	ValidationPath string            `json:"validation_path"`
	SummarySignal  SummarySignal     `json:"summary_signal"`
	Errors         []ValidationError `json:"errors"`
}

type ResearchIndex struct {
	CycleID         string         `json:"cycle_id"`
	TaskCount       int            `json:"task_count"`
	PassedCount     int            `json:"passed_count"`
	FailedCount     int            `json:"failed_count"`
	FollowUpCount   int            `json:"follow_up_count"`
	OpenCandidates  []WorkerRecord `json:"open_candidates"`
	Watchlist       []WorkerRecord `json:"watchlist"`
	PositionRelated []WorkerRecord `json:"position_related"`
	Records         []WorkerRecord `json:"records"`
}

type OrderBook struct {
	TokenID      string      `json:"token_id"`
	BestBid      *float64    `json:"best_bid"`
	BestAsk      *float64    `json:"best_ask"`
	MinOrderSize *float64    `json:"min_order_size"`
	Bids         []BookLevel `json:"bids"`
	Asks         []BookLevel `json:"asks"`
}

type BookLevel struct {
	Price float64 `json:"price"`
	Size  float64 `json:"size"`
}

type ExecutionPreviewItem struct {
	DecisionID             string   `json:"decision_id"`
	Action                 string   `json:"action"`
	MarketSlug             string   `json:"market_slug"`
	TokenID                string   `json:"token_id"`
	Outcome                string   `json:"outcome"`
	ProposedNotionalUSD    float64  `json:"proposed_notional_usd"`
	KellyNotionalUSD       float64  `json:"kelly_notional_usd"`
	RiskCappedNotionalUSD  float64  `json:"risk_capped_notional_usd"`
	ExecutableNotionalUSD  float64  `json:"executable_notional_usd"`
	ExchangeMinNotionalUSD *float64 `json:"exchange_min_notional_usd"`
	WouldExecute           bool     `json:"would_execute"`
	FillPrice              *float64 `json:"fill_price"`
	Reason                 string   `json:"reason,omitempty"`
}

type ExecutionPreview struct {
	CycleID string                 `json:"cycle_id"`
	Items   []ExecutionPreviewItem `json:"items"`
}

type PaperTrade struct {
	TradeID     string  `json:"trade_id"`
	CycleID     string  `json:"cycle_id"`
	DecisionID  string  `json:"decision_id"`
	Action      string  `json:"action"`
	Side        string  `json:"side"`
	MarketSlug  string  `json:"market_slug"`
	EventSlug   string  `json:"event_slug"`
	TokenID     string  `json:"token_id"`
	Outcome     string  `json:"outcome"`
	Shares      float64 `json:"shares"`
	Price       float64 `json:"price"`
	NotionalUSD float64 `json:"notional_usd"`
	Timestamp   string  `json:"timestamp"`
}

type PaperPosition struct {
	PositionID       string  `json:"position_id"`
	MarketSlug       string  `json:"market_slug"`
	EventSlug        string  `json:"event_slug"`
	TokenID          string  `json:"token_id"`
	Outcome          string  `json:"outcome"`
	Size             float64 `json:"size"`
	AvgCost          float64 `json:"avg_cost"`
	MarkPrice        float64 `json:"mark_price"`
	CostBasisUSD     float64 `json:"cost_basis_usd"`
	CurrentValueUSD  float64 `json:"current_value_usd"`
	UnrealizedPnLUSD float64 `json:"unrealized_pnl_usd"`
	UnrealizedPnLPct float64 `json:"unrealized_pnl_pct"`
	RealizedPnLUSD   float64 `json:"realized_pnl_usd"`
	OpenedAt         string  `json:"opened_at"`
	UpdatedAt        string  `json:"updated_at"`
}

type PortfolioSnapshot struct {
	CycleID               string          `json:"cycle_id"`
	Mode                  string          `json:"mode"`
	SnapshotAt            string          `json:"snapshot_at"`
	CashUSD               float64         `json:"cash_usd"`
	TotalPositionValueUSD float64         `json:"total_position_value_usd"`
	NavUSD                float64         `json:"nav_usd"`
	StartingNavUSD        float64         `json:"starting_nav_usd"`
	ReturnPct             float64         `json:"return_pct"`
	Positions             []PaperPosition `json:"positions"`
}

type ExecutionResult struct {
	CycleID string       `json:"cycle_id"`
	Mode    string       `json:"mode"`
	Trades  []PaperTrade `json:"trades"`
}
