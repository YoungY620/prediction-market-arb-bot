# Prediction Market Arb Bot

Go MVP for an agent-driven prediction-market paper trading bot.

The first MVP uses:

- Real CLI agent calls. `AGENT_PROVIDER=kimi` is the default; set
  `AGENT_PROVIDER=codex` to run the same harness through Codex CLI.
- Real Polymarket public read data with Gamma event-based discovery.
- Agent prompts explicitly allow read-only Bash/web research. Kimi uses `kimi -p`
  prompt mode without `--yolo` because Kimi CLI does not allow combining them.
  Codex uses `codex exec` with the user's default model and `--search`.
- Coordinator, worker-pool deep research, and reducer agent stages.
- Strict JSON validation at every agent boundary.
- Paper execution only.

It intentionally does not support:

- Fake agent mode.
- Fake account mode.
- Fake market data mode.
- Live order submission.
- GTC/GTD lifecycle.
- Provider fallback.

Every cycle fetches active, non-closed Gamma events across multiple sort
dimensions (`volume24hr`, `liquidity`, `startDate`, `competitive`) and paginates
until the API returns a short page. It expands events into markets, deduplicates,
and applies deterministic filters for order-book availability, liquidity,
missing prices, extreme prices, coin-flips, and short-term price keywords.

The harness writes `market-candidates.json`, `market-data-health.json`, and
`data-source-health.json` before any agent stage. The coordinator must assign
every candidate event and market to an event-level research task. Workers run in
a Go `errgroup` pool, write one JSON result per task, and must provide
structured evidence and counter-evidence. The reducer reads the research index
and worker result files, then emits the final executable `proposal.json`.

The agent is instructed to use Bash and read-only web/data-source access to
inspect candidates, refetch pages, read order books, and gather evidence. It
must still return a single JSON object at each stage; the harness writes
artifacts and enforces schema, research, portfolio, reducer, and
execution-preview gates.

Run:

```sh
go run ./cmd/paper-cycle
```

The cycle writes artifacts under `runtime/cycles/<cycle_id>/`.

Switch to Codex CLI without overriding the default model:

```sh
AGENT_PROVIDER=codex go run ./cmd/paper-cycle
```

Bounded Codex smoke:

```sh
AGENT_PROVIDER=codex \
EVENT_PAGES_PER_SORT=1 \
SMOKE_MAX_CANDIDATES_PER_CYCLE=2 \
LLM_JUDGE_ENABLED=false \
go run ./cmd/paper-cycle
```

Discovery-only smoke:

```sh
go run ./cmd/discovery-health > runtime/smoke/discovery-health.json
```

`DISCOVERY_HEALTH_TIMEOUT_SECONDS` controls the smoke timeout. If discovery
times out, the command still writes partial JSON with the candidates and page
progress collected before failure. `EVENT_PAGES_PER_SORT=2` can be used for a
fast API smoke; the production default `0` means paginate until short page.
Gamma event-page reads retry transient failures (`GAMMA_REQUEST_RETRIES`,
`GAMMA_RETRY_BACKOFF_MS`). During full discovery, if a later Gamma page still
fails after retries but at least `DISCOVERY_PARTIAL_MIN_CANDIDATES` executable
candidates have already been found, the cycle continues with `partial=true` and
a warning in `discovery_report`. Discovery checkpoints are written to
`$RUNTIME_DIR/discovery-cache.json` when `DISCOVERY_CACHE_ENABLED=true`; if a
fresh run fails before enough candidates are found, the last cache can be used
with `cache_hit=true`. Non-strict cycle audits accept partial/cache discovery
for trading so a single deep Gamma offset cannot invalidate the whole soak;
`--strict` still requires every sort dimension to reach a short page.
`SMOKE_MAX_CANDIDATES_PER_CYCLE` is default-off and should only be set for
bounded real-agent smoke runs; when set, `market-data-health.json` records the
full discovered candidate count and the truncated execution candidate count.
The coordinator pre-screen writes `coordinator-prescreen.json` before planning.
It is a broad-coverage noise gate, not a tiny Top-N funnel: by default
`COORDINATOR_PRESCREEN_MAX_CANDIDATES=0` keeps all executable candidates and
only removes candidates missing order-book, token, outcome, price, or liquidity
fields. If an upper bound is configured, `COORDINATOR_PRESCREEN_MIN_CANDIDATES`
defaults to `24` so real runs do not collapse to a handful of markets.
`WORKER_AGENT_TIMEOUT_SECONDS` bounds each individual worker research call so a
single slow event cannot block the entire cycle; timed-out workers are recorded
as failed and excluded from open decisions for that cycle. `WORKER_REPAIR_ATTEMPTS`
defaults to `5` so invalid JSON/schema outputs get several bounded repair
chances before the worker is marked failed. When the agent supports sessions,
worker parse/validation repair attempts resume the same session rather than
starting the research from scratch; agent timeouts still fresh-retry because no
usable output was produced.
`MAX_WORKER_FAILURE_RATE` is an end-to-end reducer health gate. If the fraction
of failed worker tasks exceeds this threshold, the cycle is considered invalid
and stops before final execution rather than allowing the reducer to rationalize
an under-researched no-trade decision.

Kimi tool smoke:

```sh
go run ./cmd/kimi-tool-health > runtime/smoke/kimi-tool-health.json
```

This makes a real Kimi CLI call and asks it to perform read-only Bash/web
checks before returning JSON.

Cycle audit:

```sh
go run ./cmd/cycle-audit runtime/cycles/<cycle_id>
```

Use `--strict` when the cycle was expected to run full discovery through short
pages. The audit checks discovery scope, data-source health, research-plan
coverage, coordinator pre-screen breadth, worker result quality, final proposal
validation, execution preview, and paper portfolio artifacts. Data-source health
includes Gamma events, CLOB order-book reads when candidates exist, and the
configured web research health URL.

For closure testing, add explicit scope thresholds so a tiny smoke run cannot
accidentally stand in for a broad market scan:

```sh
go run ./cmd/cycle-audit --strict \
  --min-raw-events 1000 \
  --min-raw-markets 10000 \
  --min-discovered-candidates 1000 \
  --min-passed-workers 10 \
  runtime/cycles/<cycle_id>
```

Tune the thresholds to the intended run profile. Bounded Kimi smoke runs should
use small or zero thresholds; production-range verification should require full
pagination and enough passed workers to support reducer decisions.

Hourly/overnight smoke:

```sh
RUNS=8 SLEEP_SECONDS=3600 ./scripts/hourly-paper-smoke.sh
go run ./cmd/smoke-summary runtime/hourly-smoke/summary.jsonl
```

Each hourly run writes both a paper-cycle log and a cycle-audit JSON. The summary
JSONL records `exit_code` and `audit_exit_code`; a run is only acceptable when
both are zero. `smoke-summary` aggregates discovered candidates, active
candidates, worker pass/fail counts, and proposal activity across the run set.
