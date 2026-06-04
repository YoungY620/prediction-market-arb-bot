package config

import "testing"

func TestLoadDefaultsUseKimiAndFiveWorkerRepairs(t *testing.T) {
	t.Setenv("AGENT_PROVIDER", "")
	t.Setenv("WORKER_REPAIR_ATTEMPTS", "")
	t.Setenv("COORDINATOR_PRESCREEN_MAX_CANDIDATES", "")
	t.Setenv("COORDINATOR_PRESCREEN_MIN_CANDIDATES", "")
	t.Setenv("GAMMA_REQUEST_RETRIES", "")
	t.Setenv("DISCOVERY_PARTIAL_MIN_CANDIDATES", "")
	t.Setenv("DISCOVERY_CACHE_ENABLED", "")
	t.Setenv("CANDIDATE_FILTER_PRESETS", "")
	t.Setenv("CANDIDATE_FILTER_KEYWORDS", "")
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
	if cfg.GammaRequestRetries != 3 {
		t.Fatalf("expected 3 gamma request retries, got %d", cfg.GammaRequestRetries)
	}
	if cfg.DiscoveryPartialMinCandidates != 500 {
		t.Fatalf("expected partial discovery floor of 500, got %d", cfg.DiscoveryPartialMinCandidates)
	}
	if !cfg.DiscoveryCacheEnabled {
		t.Fatal("expected discovery cache enabled by default")
	}
	if cfg.CandidateFilterPresets != "" || cfg.CandidateFilterKeywords != "" {
		t.Fatalf("expected candidate filters disabled by default, got presets=%q keywords=%q", cfg.CandidateFilterPresets, cfg.CandidateFilterKeywords)
	}
}
