package agent

import (
	"strings"
	"testing"
)

func TestKimiArgsDoNotUseYoloInPromptMode(t *testing.T) {
	args := kimiArgs("prompt", true)
	for _, arg := range args {
		if arg == "--yolo" {
			t.Fatalf("prompt mode cannot combine with --yolo, got %#v", args)
		}
	}
}

func TestKimiArgsCanDisableYolo(t *testing.T) {
	args := kimiArgs("prompt", false)
	for _, arg := range args {
		if arg == "--yolo" {
			t.Fatalf("did not expect --yolo, got %#v", args)
		}
	}
}

func TestKimiResumeArgsUseSession(t *testing.T) {
	args := kimiResumeArgs("session-1", "prompt", false)
	got := strings.Join(args, " ")
	if !strings.Contains(got, "-S session-1") || !strings.Contains(got, "-p prompt") {
		t.Fatalf("expected session resume args, got %#v", args)
	}
}

func TestExtractAssistantContentIgnoresMeta(t *testing.T) {
	output := "{\"role\":\"assistant\",\"content\":\"{\\\"ok\\\":true}\"}\n{\"role\":\"meta\",\"content\":\"resume\"}\n"
	content, err := extractAssistantContent(output)
	if err != nil {
		t.Fatal(err)
	}
	if content != "{\"ok\":true}" {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestExtractSessionIDFromStream(t *testing.T) {
	output := "{\"type\":\"session\",\"session_id\":\"abc123\"}\n{\"role\":\"assistant\",\"content\":\"ok\"}\n"
	if got := extractSessionID(output); got != "abc123" {
		t.Fatalf("expected session id, got %q", got)
	}
}
