package agent

import "testing"

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
