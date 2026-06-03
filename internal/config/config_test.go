package config

import "testing"

func TestLoadDefaultsUseKimiAndFiveWorkerRepairs(t *testing.T) {
	t.Setenv("AGENT_PROVIDER", "")
	t.Setenv("WORKER_REPAIR_ATTEMPTS", "")
	t.Setenv("COORDINATOR_PRESCREEN_MAX_CANDIDATES", "")
	t.Setenv("COORDINATOR_PRESCREEN_MIN_CANDIDATES", "")
	cfg := Load()
	if cfg.AgentProvider != "kimi" {
		t.Fatalf("expected kimi default provider, got %q", cfg.AgentProvider)
	}
	if cfg.WorkerRepairAttempts != 5 {
		t.Fatalf("expected 5 default worker repair attempts, got %d", cfg.WorkerRepairAttempts)
	}
	if cfg.CoordinatorPreScreenMaxCandidates != 0 {
		t.Fatalf("expected coordinator prescreen max default off, got %d", cfg.CoordinatorPreScreenMaxCandidates)
	}
	if cfg.CoordinatorPreScreenMinCandidates != 24 {
		t.Fatalf("expected broad coordinator prescreen floor of 24, got %d", cfg.CoordinatorPreScreenMinCandidates)
	}
}
