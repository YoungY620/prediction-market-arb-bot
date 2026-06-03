package agent

import (
	"context"
	"fmt"
	"strings"

	"prediction-market-arb-bot/internal/config"
)

type Runner interface {
	Run(ctx context.Context, prompt string) (string, error)
}

func NewConfiguredAgent(cfg config.Config) (Runner, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.AgentProvider)) {
	case "", "kimi":
		return NewKimiAgentWithTools(cfg.KimiCommand, cfg.KimiTimeout, cfg.KimiAutoApproveTools), nil
	case "codex":
		return NewCodexAgent(cfg.CodexCommand, cfg.CodexTimeout, cfg.CodexSearchEnabled, cfg.CodexSandbox, cfg.CodexApproval), nil
	default:
		return nil, fmt.Errorf("unsupported AGENT_PROVIDER %q; expected kimi or codex", cfg.AgentProvider)
	}
}
