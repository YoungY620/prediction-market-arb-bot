package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"prediction-market-arb-bot/internal/agent"
	"prediction-market-arb-bot/internal/config"
)

type healthResult struct {
	SchemaVersion string   `json:"schema_version"`
	BashPWD       string   `json:"bash_pwd"`
	WebStatusCode int      `json:"web_status_code"`
	WebReachable  bool     `json:"web_reachable"`
	ToolNotes     []string `json:"tool_notes"`
}

type output struct {
	Passed bool         `json:"passed"`
	Raw    string       `json:"raw,omitempty"`
	Result healthResult `json:"result,omitempty"`
	Errors []string     `json:"errors"`
}

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.KimiTimeout)
	defer cancel()
	raw, err := agent.NewKimiAgentWithTools(cfg.KimiCommand, cfg.KimiTimeout, cfg.KimiAutoApproveTools).Run(ctx, prompt(cfg.WebHealthURL))
	out := output{Raw: raw, Errors: []string{}}
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		write(out)
		os.Exit(1)
	}
	var parsed healthResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		out.Errors = append(out.Errors, "invalid JSON: "+err.Error())
		write(out)
		os.Exit(1)
	}
	out.Result = parsed
	if parsed.SchemaVersion != "1" {
		out.Errors = append(out.Errors, "schema_version must be 1")
	}
	if strings.TrimSpace(parsed.BashPWD) == "" {
		out.Errors = append(out.Errors, "bash_pwd is empty; Bash tool use was not evidenced")
	}
	if !parsed.WebReachable || parsed.WebStatusCode < 200 || parsed.WebStatusCode >= 500 {
		out.Errors = append(out.Errors, "web health URL was not reachable according to Kimi result")
	}
	out.Passed = len(out.Errors) == 0
	write(out)
	if !out.Passed {
		os.Exit(1)
	}
}

func prompt(webURL string) string {
	return fmt.Sprintf(`You are checking read-only tool availability for a local paper-trading harness.

You must use Bash or your built-in terminal tool to run:
1. pwd
2. curl -L -s -o /dev/null -w '%%{http_code}' %s

Do not write files. Do not call wallets. Do not submit orders.

Return only one JSON object, with no markdown and no commentary:
{
  "schema_version": "1",
  "bash_pwd": "<stdout from pwd>",
  "web_status_code": <integer HTTP status code from curl>,
  "web_reachable": <true if status code is 200-499>,
  "tool_notes": ["short factual notes about the tool calls performed"]
}
`, webURL)
}

func write(out output) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(out)
}
