package main

import (
	"context"
	"fmt"
	"os"

	"prediction-market-arb-bot/internal/agent"
	"prediction-market-arb-bot/internal/config"
	"prediction-market-arb-bot/internal/runtime"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Println("Usage: paper-cycle\n\nRuns one real Kimi CLI + real Polymarket read-data paper trading cycle.")
		return
	}
	ctx := context.Background()
	cfg := config.Load()
	result, err := runtime.RunPaperCycle(ctx, cfg, agent.NewKimiAgent(cfg.KimiCommand, cfg.KimiTimeout))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(result.CycleDir)
}
