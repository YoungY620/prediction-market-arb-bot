package types

type Evidence struct {
	SourceURL         string  `json:"source_url"`
	SourceType        string  `json:"source_type"`
	RetrievedAt       string  `json:"retrieved_at"`
	Claim             string  `json:"claim"`
	ContractRelevance string  `json:"contract_relevance"`
	Supports          string  `json:"supports"`
	ProbabilityEffect float64 `json:"probability_effect_pp"`
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

type JudgeResult struct {
	CycleID      string            `json:"cycle_id"`
	Passed       bool              `json:"passed"`
	HardFailures []ValidationError `json:"hard_failures"`
	Warnings     []ValidationError `json:"warnings"`
}

type MarketCandidate struct {
	EventSlug     string     `json:"event_slug"`
	MarketSlug    string     `json:"market_slug"`
	Question      string     `json:"question"`
	Outcomes      []string   `json:"outcomes"`
	OutcomePrices []float64  `json:"outcome_prices"`
	TokenIDs      []string   `json:"token_ids"`
	Resolution    Resolution `json:"resolution"`
	LiquidityUSD  float64    `json:"liquidity_usd"`
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
