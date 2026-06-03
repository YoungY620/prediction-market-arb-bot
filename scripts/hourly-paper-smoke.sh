#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

RUNS="${RUNS:-8}"
SLEEP_SECONDS="${SLEEP_SECONDS:-3600}"
AGENT_PROVIDER="${AGENT_PROVIDER:-kimi}"
KIMI_TIMEOUT_SECONDS="${KIMI_TIMEOUT_SECONDS:-900}"
CODEX_TIMEOUT_SECONDS="${CODEX_TIMEOUT_SECONDS:-900}"
WORKER_AGENT_TIMEOUT_SECONDS="${WORKER_AGENT_TIMEOUT_SECONDS:-180}"
WORKER_REPAIR_ATTEMPTS="${WORKER_REPAIR_ATTEMPTS:-5}"
SMOKE_MAX_CANDIDATES_PER_CYCLE="${SMOKE_MAX_CANDIDATES_PER_CYCLE:-0}"
MAX_WORKER_FAILURE_RATE="${MAX_WORKER_FAILURE_RATE:-0.5}"
LLM_JUDGE_ENABLED="${LLM_JUDGE_ENABLED:-true}"

LOG_DIR="${LOG_DIR:-$ROOT_DIR/runtime/hourly-smoke}"
mkdir -p "$LOG_DIR"

SUMMARY="$LOG_DIR/summary.jsonl"
overall_exit_code=0

newest_cycle_since() {
  local since_epoch="$1"
  local newest=""
  local newest_mtime=0
  local dir
  for dir in "$ROOT_DIR"/runtime/cycles/*; do
    [[ -d "$dir" ]] || continue
    local mtime
    mtime="$(stat -f "%m" "$dir" 2>/dev/null || echo 0)"
    if (( mtime >= since_epoch && mtime >= newest_mtime )); then
      newest="$dir"
      newest_mtime="$mtime"
    fi
  done
  if [[ -n "$newest" ]]; then
    realpath --relative-to="$ROOT_DIR" "$newest" 2>/dev/null || echo "$newest"
  fi
}

for ((i = 1; i <= RUNS; i++)); do
  started_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  started_epoch="$(date -u +"%s")"
  run_log="$LOG_DIR/run-$(date -u +"%Y%m%dT%H%M%SZ")-$i.log"
  audit_log="$LOG_DIR/audit-$(date -u +"%Y%m%dT%H%M%SZ")-$i.json"
  echo "{\"event\":\"start\",\"run\":$i,\"runs\":$RUNS,\"started_at\":\"$started_at\",\"log\":\"$run_log\"}" >> "$SUMMARY"

  AGENT_PROVIDER="$AGENT_PROVIDER" \
  KIMI_TIMEOUT_SECONDS="$KIMI_TIMEOUT_SECONDS" \
  CODEX_TIMEOUT_SECONDS="$CODEX_TIMEOUT_SECONDS" \
  WORKER_AGENT_TIMEOUT_SECONDS="$WORKER_AGENT_TIMEOUT_SECONDS" \
  WORKER_REPAIR_ATTEMPTS="$WORKER_REPAIR_ATTEMPTS" \
  SMOKE_MAX_CANDIDATES_PER_CYCLE="$SMOKE_MAX_CANDIDATES_PER_CYCLE" \
  MAX_WORKER_FAILURE_RATE="$MAX_WORKER_FAILURE_RATE" \
  LLM_JUDGE_ENABLED="$LLM_JUDGE_ENABLED" \
  go run ./cmd/paper-cycle > "$run_log" 2>&1
  exit_code=$?

  ended_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  cycle_dir="$(tail -n 1 "$run_log" | sed 's/[[:space:]]*$//')"
  if [[ ! -d "$cycle_dir" ]]; then
    cycle_dir="$(newest_cycle_since "$started_epoch")"
  fi
  audit_exit_code=2
  if [[ -d "$cycle_dir" ]]; then
    go run ./cmd/cycle-audit "$cycle_dir" > "$audit_log" 2>&1
    audit_exit_code=$?
  else
    printf '{"passed":false,"checks":[{"name":"cycle_dir","passed":false,"message":"cycle directory not found"}]}\n' > "$audit_log"
  fi
  if (( exit_code != 0 )); then
    overall_exit_code="$exit_code"
  elif (( audit_exit_code != 0 && overall_exit_code == 0 )); then
    overall_exit_code="$audit_exit_code"
  fi
  echo "{\"event\":\"end\",\"run\":$i,\"runs\":$RUNS,\"started_at\":\"$started_at\",\"ended_at\":\"$ended_at\",\"exit_code\":$exit_code,\"audit_exit_code\":$audit_exit_code,\"cycle_dir\":\"$cycle_dir\",\"log\":\"$run_log\",\"audit_log\":\"$audit_log\"}" >> "$SUMMARY"

  if (( i < RUNS )); then
    sleep "$SLEEP_SECONDS"
  fi
done

exit "$overall_exit_code"
