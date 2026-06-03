package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"prediction-market-arb-bot/internal/config"
	"prediction-market-arb-bot/internal/marketdata"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), timeoutFromEnv())
	defer cancel()

	cfg := config.Load()
	client := marketdata.New(cfg)
	candidates, report, err := client.FetchCandidateSet(ctx)
	payload := map[string]any{
		"candidate_count": len(candidates),
		"report":          report,
		"top_candidates":  candidates[:min(len(candidates), 20)],
	}
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "discovery failed: %v\n", err)
		payload["error"] = err.Error()
		_ = writePayload(payload)
		os.Exit(1)
	}
	if err := writePayload(payload); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "encode failed: %v\n", err)
		os.Exit(1)
	}
}

func writePayload(payload map[string]any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}

func timeoutFromEnv() time.Duration {
	value := os.Getenv("DISCOVERY_HEALTH_TIMEOUT_SECONDS")
	if value == "" {
		return 10 * time.Minute
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 10 * time.Minute
	}
	return time.Duration(parsed) * time.Second
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
