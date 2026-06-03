package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"prediction-market-arb-bot/internal/types"
)

type KimiAgent struct {
	command          string
	timeout          time.Duration
	autoApproveTools bool
}

func NewKimiAgent(command string, timeout time.Duration) KimiAgent {
	return KimiAgent{command: command, timeout: timeout}
}

func NewKimiAgentWithTools(command string, timeout time.Duration, autoApproveTools bool) KimiAgent {
	return KimiAgent{command: command, timeout: timeout, autoApproveTools: autoApproveTools}
}

func (a KimiAgent) Run(ctx context.Context, prompt string) (string, error) {
	run, err := a.RunSession(ctx, prompt)
	if err != nil {
		return "", err
	}
	return run.Output, nil
}

func (a KimiAgent) RunSession(ctx context.Context, prompt string) (types.AgentRun, error) {
	return a.run(ctx, kimiArgs(prompt, a.autoApproveTools))
}

func (a KimiAgent) ResumeSession(ctx context.Context, sessionID, prompt string) (types.AgentRun, error) {
	return a.run(ctx, kimiResumeArgs(sessionID, prompt, a.autoApproveTools))
}

func (a KimiAgent) run(ctx context.Context, args []string) (types.AgentRun, error) {
	if a.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, a.command, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return types.AgentRun{}, &CommandError{Err: err, Stderr: tailString(stderr.String(), 6000)}
	}
	output := stdout.String()
	content, err := extractAssistantContent(output)
	if err != nil {
		return types.AgentRun{}, err
	}
	return types.AgentRun{Output: content, SessionID: extractSessionID(output)}, nil
}

func kimiArgs(prompt string, autoApproveTools bool) []string {
	_ = autoApproveTools
	args := []string{"-p", prompt, "--output-format", "stream-json"}
	return args
}

func kimiResumeArgs(sessionID, prompt string, autoApproveTools bool) []string {
	_ = autoApproveTools
	args := []string{"-S", sessionID, "-p", prompt, "--output-format", "stream-json"}
	return args
}

func extractAssistantContent(output string) (string, error) {
	lines := strings.Split(output, "\n")
	var last string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Role == "assistant" {
			last = event.Content
		}
	}
	if strings.TrimSpace(last) == "" {
		return "", fmt.Errorf("kimi stream-json output did not include assistant content")
	}
	return last, nil
}

func extractSessionID(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		for _, key := range []string{"session_id", "sessionId", "sessionID"} {
			if value, ok := event[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		if nested, ok := event["session"].(map[string]any); ok {
			for _, key := range []string{"id", "session_id", "sessionId"} {
				if value, ok := nested[key].(string); ok && strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
			}
		}
	}
	return ""
}

type CommandError struct {
	Err    error
	Stderr string
}

func (e *CommandError) Error() string {
	if e.Stderr != "" {
		return e.Err.Error() + ": " + e.Stderr
	}
	return e.Err.Error()
}
