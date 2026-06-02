package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"prediction-market-arb-bot/internal/config"
)

type testAgent struct {
	response string
}

func (a testAgent) Run(context.Context, string) (string, error) {
	return a.response, nil
}

func TestValidationFailureDoesNotWriteExecutionArtifacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/markets" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	dir := t.TempDir()
	cfg := config.Load()
	cfg.RuntimeDir = dir
	cfg.GammaBaseURL = server.URL
	cfg.MaxRepairAttempts = 0
	cfg.LLMJudgeEnabled = false
	_, err := RunPaperCycle(context.Background(), cfg, testAgent{response: "not json"})
	if err == nil {
		t.Fatal("expected failed cycle")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "paper", "portfolio.json")); !os.IsNotExist(statErr) {
		t.Fatalf("paper portfolio should not be written after validation failure, stat err=%v", statErr)
	}
	matches, globErr := filepath.Glob(filepath.Join(dir, "cycles", "*", "error.json"))
	if globErr != nil || len(matches) != 1 {
		t.Fatalf("expected one error artifact, matches=%v err=%v", matches, globErr)
	}
}
