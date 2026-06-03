package agent

import (
	"strings"
	"testing"
)

func TestCodexArgsUseDefaultModelAndSearch(t *testing.T) {
	args := codexArgs(CodexAgent{
		workDir:       "/repo",
		searchEnabled: true,
		sandbox:       "danger-full-access",
		approval:      "never",
	}, "/tmp/last.txt")
	joined := " " + strings.Join(args, " ") + " "
	if strings.Contains(joined, " --model ") || strings.Contains(joined, " -m ") {
		t.Fatalf("codex adapter must not override the default model, got %#v", args)
	}
	if !strings.Contains(joined, " --search ") {
		t.Fatalf("expected web search enabled, got %#v", args)
	}
	if args[len(args)-1] != "-" {
		t.Fatalf("expected prompt to be read from stdin, got %#v", args)
	}
}

func TestCodexArgsCanDisableSearch(t *testing.T) {
	args := codexArgs(CodexAgent{workDir: "/repo", searchEnabled: false}, "/tmp/last.txt")
	for _, arg := range args {
		if arg == "--search" {
			t.Fatalf("did not expect --search, got %#v", args)
		}
	}
}

func TestTailStringTruncatesFromEnd(t *testing.T) {
	got := tailString("abcdef", 3)
	if got != "...<truncated>...\ndef" {
		t.Fatalf("unexpected tail: %q", got)
	}
}
