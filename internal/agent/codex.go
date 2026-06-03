package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type CodexAgent struct {
	command       string
	timeout       time.Duration
	searchEnabled bool
	sandbox       string
	approval      string
	workDir       string
}

func NewCodexAgent(command string, timeout time.Duration, searchEnabled bool, sandbox, approval string) CodexAgent {
	workDir, _ := os.Getwd()
	return CodexAgent{
		command:       command,
		timeout:       timeout,
		searchEnabled: searchEnabled,
		sandbox:       sandbox,
		approval:      approval,
		workDir:       workDir,
	}
}

func (a CodexAgent) Run(ctx context.Context, prompt string) (string, error) {
	if a.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.timeout)
		defer cancel()
	}
	tmpDir, err := os.MkdirTemp("", "pmarb-codex-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)
	lastMessagePath := filepath.Join(tmpDir, "last-message.txt")
	cmd := exec.CommandContext(ctx, a.command, codexArgs(a, lastMessagePath)...)
	cmd.Stdin = strings.NewReader(prompt)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return "", &CommandError{Err: err, Stderr: tailString(stderr.String()+"\n"+stdout.String(), 6000)}
	}
	content, err := os.ReadFile(lastMessagePath)
	if err != nil {
		return "", fmt.Errorf("codex did not write last message: %w; output: %s", err, tailString(stderr.String()+"\n"+stdout.String(), 6000))
	}
	if strings.TrimSpace(string(content)) == "" {
		return "", fmt.Errorf("codex last message was empty; output: %s", tailString(stderr.String()+"\n"+stdout.String(), 6000))
	}
	return strings.TrimSpace(string(content)), nil
}

func tailString(value string, maxLen int) string {
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	return "...<truncated>...\n" + value[len(value)-maxLen:]
}

func codexArgs(a CodexAgent, lastMessagePath string) []string {
	args := []string{
		"--cd", a.workDir,
		"--sandbox", fallback(a.sandbox, "danger-full-access"),
		"--ask-for-approval", fallback(a.approval, "never"),
	}
	if a.searchEnabled {
		args = append(args, "--search")
	}
	args = append(args,
		"exec",
		"--ephemeral",
		"--color", "never",
		"--output-last-message", lastMessagePath,
		"-",
	)
	return args
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}
