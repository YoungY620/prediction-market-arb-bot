package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	AgentProvider      string
	KimiCommand        string
	KimiTimeout        time.Duration
	WorkerAgentTimeout time.Duration
	CodexCommand       string
	CodexTimeout       time.Duration
	CodexSearchEnabled bool
	CodexSandbox       string
	CodexApproval      string

	GammaBaseURL string
	CLOBBaseURL  string
	DataBaseURL  string
	WebHealthURL string

	RuntimeDir string

	PaperStartingCashUSD      float64
	MaxSingleTradePct         float64
	MaxTotalExposurePct       float64
	MaxEventExposurePct       float64
	MaxPositions              int
	MinTradeUSD               float64
	EdgeThresholdPct          float64
	MaxRepairAttempts         int
	CoordinatorRepairAttempts int
	WorkerRepairAttempts      int
	ReducerRepairAttempts     int

	EventPagesPerSort                 int
	EventsPerPage                     int
	GammaRequestRetries               int
	GammaRetryBackoff                 time.Duration
	DiscoveryPartialMinCandidates     int
	DiscoveryCacheEnabled             bool
	MinMarketLiquidityUSD             float64
	PriceTolerancePct                 float64
	StrategyClipMinRatio              float64
	ResearchWorkerConcurrency         int
	MaxFollowUpTasksPerCycle          int
	MaxFollowUpDepth                  int
	SmokeMaxCandidatesPerCycle        int
	CoordinatorPreScreenMaxCandidates int
	CoordinatorPreScreenMinCandidates int
	MaxWorkerFailureRate              float64

	LLMJudgeEnabled                bool
	LLMJudgeFormatRetryCount       int
	AllowExecutionWhenJudgeSkipped bool

	KimiAutoApproveTools bool
}

func Load() Config {
	kimiTimeoutSeconds := envInt("KIMI_TIMEOUT_SECONDS", 900)
	return Config{
		AgentProvider:      envString("AGENT_PROVIDER", "kimi"),
		KimiCommand:        envString("KIMI_COMMAND", "kimi"),
		KimiTimeout:        time.Duration(kimiTimeoutSeconds) * time.Second,
		WorkerAgentTimeout: time.Duration(envInt("WORKER_AGENT_TIMEOUT_SECONDS", 180)) * time.Second,
		CodexCommand:       envString("CODEX_COMMAND", "codex"),
		CodexTimeout:       time.Duration(envInt("CODEX_TIMEOUT_SECONDS", kimiTimeoutSeconds)) * time.Second,
		CodexSearchEnabled: envBool("CODEX_SEARCH_ENABLED", true),
		CodexSandbox:       envString("CODEX_SANDBOX", "danger-full-access"),
		CodexApproval:      envString("CODEX_APPROVAL_POLICY", "never"),

		GammaBaseURL: envString("POLYMARKET_GAMMA_BASE_URL", "https://gamma-api.polymarket.com"),
		CLOBBaseURL:  envString("POLYMARKET_CLOB_BASE_URL", "https://clob.polymarket.com"),
		DataBaseURL:  envString("POLYMARKET_DATA_BASE_URL", "https://data-api.polymarket.com"),
		WebHealthURL: envString("WEB_RESEARCH_HEALTH_URL", "https://www.google.com/generate_204"),

		RuntimeDir: envString("RUNTIME_DIR", "runtime"),

		PaperStartingCashUSD:      envFloat("PAPER_STARTING_CASH_USD", 500),
		MaxSingleTradePct:         envFloat("MAX_SINGLE_TRADE_PCT", 0.15),
		MaxTotalExposurePct:       envFloat("MAX_TOTAL_EXPOSURE_PCT", 0.8),
		MaxEventExposurePct:       envFloat("MAX_EVENT_EXPOSURE_PCT", 0.3),
		MaxPositions:              envInt("MAX_POSITIONS", 22),
		MinTradeUSD:               envFloat("MIN_TRADE_USD", 5),
		EdgeThresholdPct:          envFloat("EDGE_THRESHOLD_PCT", 0.05),
		MaxRepairAttempts:         envInt("MAX_REPAIR_ATTEMPTS", 3),
		CoordinatorRepairAttempts: envInt("COORDINATOR_REPAIR_ATTEMPTS", 1),
		WorkerRepairAttempts:      envInt("WORKER_REPAIR_ATTEMPTS", 5),
		ReducerRepairAttempts:     envInt("REDUCER_REPAIR_ATTEMPTS", 5),

		EventPagesPerSort:                 envInt("EVENT_PAGES_PER_SORT", 0),
		EventsPerPage:                     envInt("EVENTS_PER_PAGE", 50),
		GammaRequestRetries:               envInt("GAMMA_REQUEST_RETRIES", 3),
		GammaRetryBackoff:                 time.Duration(envInt("GAMMA_RETRY_BACKOFF_MS", 500)) * time.Millisecond,
		DiscoveryPartialMinCandidates:     envInt("DISCOVERY_PARTIAL_MIN_CANDIDATES", 500),
		DiscoveryCacheEnabled:             envBool("DISCOVERY_CACHE_ENABLED", true),
		MinMarketLiquidityUSD:             envFloat("MIN_MARKET_LIQUIDITY_USD", 5000),
		PriceTolerancePct:                 envFloat("PRICE_TOLERANCE_PCT", 0.03),
		StrategyClipMinRatio:              envFloat("STRATEGY_CLIP_MIN_RATIO", 0.5),
		ResearchWorkerConcurrency:         envInt("RESEARCH_WORKER_CONCURRENCY", 0),
		MaxFollowUpTasksPerCycle:          envInt("MAX_FOLLOW_UP_TASKS_PER_CYCLE", 50),
		MaxFollowUpDepth:                  envInt("MAX_FOLLOW_UP_DEPTH", 1),
		SmokeMaxCandidatesPerCycle:        envInt("SMOKE_MAX_CANDIDATES_PER_CYCLE", 0),
		CoordinatorPreScreenMaxCandidates: envInt("COORDINATOR_PRESCREEN_MAX_CANDIDATES", 0),
		CoordinatorPreScreenMinCandidates: envInt("COORDINATOR_PRESCREEN_MIN_CANDIDATES", 24),
		MaxWorkerFailureRate:              envFloat("MAX_WORKER_FAILURE_RATE", 0.5),

		LLMJudgeEnabled:                envBool("LLM_JUDGE_ENABLED", true),
		LLMJudgeFormatRetryCount:       envInt("LLM_JUDGE_FORMAT_RETRY_COUNT", 1),
		AllowExecutionWhenJudgeSkipped: envBool("ALLOW_EXECUTION_WHEN_JUDGE_SKIPPED", true),

		KimiAutoApproveTools: envBool("KIMI_AUTO_APPROVE_TOOLS", false),
	}
}

func envString(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
