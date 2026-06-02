#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

RUNS="${RUNS:-8}"
SLEEP_SECONDS="${SLEEP_SECONDS:-3600}"
MARKET_CANDIDATE_LIMIT="${MARKET_CANDIDATE_LIMIT:-2}"
KIMI_TIMEOUT_SECONDS="${KIMI_TIMEOUT_SECONDS:-120}"
LLM_JUDGE_ENABLED="${LLM_JUDGE_ENABLED:-true}"

LOG_DIR="$ROOT_DIR/runtime/hourly-smoke"
mkdir -p "$LOG_DIR"

SUMMARY="$LOG_DIR/summary.jsonl"

for ((i = 1; i <= RUNS; i++)); do
  started_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  run_log="$LOG_DIR/run-$(date -u +"%Y%m%dT%H%M%SZ")-$i.log"
  echo "{\"event\":\"start\",\"run\":$i,\"runs\":$RUNS,\"started_at\":\"$started_at\",\"log\":\"$run_log\"}" >> "$SUMMARY"

  MARKET_CANDIDATE_LIMIT="$MARKET_CANDIDATE_LIMIT" \
  KIMI_TIMEOUT_SECONDS="$KIMI_TIMEOUT_SECONDS" \
  LLM_JUDGE_ENABLED="$LLM_JUDGE_ENABLED" \
  go run ./cmd/paper-cycle > "$run_log" 2>&1
  exit_code=$?

  ended_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  cycle_dir="$(tail -n 1 "$run_log" | sed 's/[[:space:]]*$//')"
  echo "{\"event\":\"end\",\"run\":$i,\"runs\":$RUNS,\"started_at\":\"$started_at\",\"ended_at\":\"$ended_at\",\"exit_code\":$exit_code,\"cycle_dir\":\"$cycle_dir\",\"log\":\"$run_log\"}" >> "$SUMMARY"

  if (( i < RUNS )); then
    sleep "$SLEEP_SECONDS"
  fi
done

