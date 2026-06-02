#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

RUNS="${RUNS:-8}"
MARKET_CANDIDATE_LIMIT="${MARKET_CANDIDATE_LIMIT:-2}"
KIMI_TIMEOUT_SECONDS="${KIMI_TIMEOUT_SECONDS:-120}"
LLM_JUDGE_ENABLED="${LLM_JUDGE_ENABLED:-true}"
LABEL="${LABEL:-com.prediction-market-arb-bot.hourly-smoke}"
PLIST_PATH="${PLIST_PATH:-}"

LOG_DIR="$ROOT_DIR/runtime/hourly-smoke"
mkdir -p "$LOG_DIR"

STATE="$LOG_DIR/launchd-state"
SUMMARY="$LOG_DIR/summary.jsonl"

count=0
if [[ -f "$STATE" ]]; then
  count="$(cat "$STATE")"
fi

if (( count >= RUNS )); then
  echo "{\"event\":\"complete\",\"runs\":$RUNS,\"checked_at\":\"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\"}" >> "$SUMMARY"
  if [[ -n "$PLIST_PATH" ]]; then
    launchctl bootout "gui/$(id -u)" "$PLIST_PATH" >/dev/null 2>&1 || true
  else
    launchctl bootout "gui/$(id -u)/$LABEL" >/dev/null 2>&1 || true
  fi
  exit 0
fi

run=$((count + 1))
echo "$run" > "$STATE"

started_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
run_log="$LOG_DIR/launchd-run-$(date -u +"%Y%m%dT%H%M%SZ")-$run.log"
echo "{\"event\":\"start\",\"run\":$run,\"runs\":$RUNS,\"started_at\":\"$started_at\",\"log\":\"$run_log\"}" >> "$SUMMARY"

MARKET_CANDIDATE_LIMIT="$MARKET_CANDIDATE_LIMIT" \
KIMI_TIMEOUT_SECONDS="$KIMI_TIMEOUT_SECONDS" \
LLM_JUDGE_ENABLED="$LLM_JUDGE_ENABLED" \
go run ./cmd/paper-cycle > "$run_log" 2>&1
exit_code=$?

ended_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
cycle_dir="$(tail -n 1 "$run_log" | sed 's/[[:space:]]*$//')"
echo "{\"event\":\"end\",\"run\":$run,\"runs\":$RUNS,\"started_at\":\"$started_at\",\"ended_at\":\"$ended_at\",\"exit_code\":$exit_code,\"cycle_dir\":\"$cycle_dir\",\"log\":\"$run_log\"}" >> "$SUMMARY"

if (( run >= RUNS )); then
  echo "{\"event\":\"complete\",\"runs\":$RUNS,\"completed_at\":\"$ended_at\"}" >> "$SUMMARY"
  if [[ -n "$PLIST_PATH" ]]; then
    launchctl bootout "gui/$(id -u)" "$PLIST_PATH" >/dev/null 2>&1 || true
  else
    launchctl bootout "gui/$(id -u)/$LABEL" >/dev/null 2>&1 || true
  fi
fi

exit "$exit_code"

