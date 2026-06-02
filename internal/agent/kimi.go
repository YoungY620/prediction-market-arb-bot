package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type KimiAgent struct {
	command string
	timeout time.Duration
}

func NewKimiAgent(command string, timeout time.Duration) KimiAgent {
	return KimiAgent{command: command, timeout: timeout}
}

func (a KimiAgent) Run(ctx context.Context, prompt string) (string, error) {
	if a.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, a.command, "-p", prompt, "--output-format", "stream-json")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", &CommandError{Err: err, Stderr: string(ee.Stderr)}
		}
		return "", err
	}
	content, err := extractAssistantContent(string(out))
	if err != nil {
		return "", err
	}
	return content, nil
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
