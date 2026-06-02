package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	KimiCommand string
	KimiTimeout time.Duration

	GammaBaseURL string
	CLOBBaseURL  string
	DataBaseURL  string

	RuntimeDir string

	PaperStartingCashUSD float64
	MaxSingleTradePct    float64
	MaxTotalExposurePct  float64
	MaxEventExposurePct  float64
	MaxPositions         int
	MinTradeUSD          float64
	EdgeThresholdPct     float64
	MaxRepairAttempts    int

	MarketCandidateLimit int
	PriceTolerancePct    float64
	StrategyClipMinRatio float64

	LLMJudgeEnabled                bool
	LLMJudgeFormatRetryCount       int
	AllowExecutionWhenJudgeSkipped bool
}

func Load() Config {
	return Config{
		KimiCommand: envString("KIMI_COMMAND", "kimi"),
		KimiTimeout: time.Duration(envInt("KIMI_TIMEOUT_SECONDS", 120)) * time.Second,

		GammaBaseURL: envString("POLYMARKET_GAMMA_BASE_URL", "https://gamma-api.polymarket.com"),
		CLOBBaseURL:  envString("POLYMARKET_CLOB_BASE_URL", "https://clob.polymarket.com"),
		DataBaseURL:  envString("POLYMARKET_DATA_BASE_URL", "https://data-api.polymarket.com"),

		RuntimeDir: envString("RUNTIME_DIR", "runtime"),

		PaperStartingCashUSD: envFloat("PAPER_STARTING_CASH_USD", 500),
		MaxSingleTradePct:    envFloat("MAX_SINGLE_TRADE_PCT", 0.15),
		MaxTotalExposurePct:  envFloat("MAX_TOTAL_EXPOSURE_PCT", 0.8),
		MaxEventExposurePct:  envFloat("MAX_EVENT_EXPOSURE_PCT", 0.3),
		MaxPositions:         envInt("MAX_POSITIONS", 22),
		MinTradeUSD:          envFloat("MIN_TRADE_USD", 5),
		EdgeThresholdPct:     envFloat("EDGE_THRESHOLD_PCT", 0.05),
		MaxRepairAttempts:    envInt("MAX_REPAIR_ATTEMPTS", 3),

		MarketCandidateLimit: envInt("MARKET_CANDIDATE_LIMIT", 20),
		PriceTolerancePct:    envFloat("PRICE_TOLERANCE_PCT", 0.03),
		StrategyClipMinRatio: envFloat("STRATEGY_CLIP_MIN_RATIO", 0.5),

		LLMJudgeEnabled:                envBool("LLM_JUDGE_ENABLED", true),
		LLMJudgeFormatRetryCount:       envInt("LLM_JUDGE_FORMAT_RETRY_COUNT", 1),
		AllowExecutionWhenJudgeSkipped: envBool("ALLOW_EXECUTION_WHEN_JUDGE_SKIPPED", true),
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
