# Prediction Market Arb Bot MVP Acceptance Standard

This document is the acceptance baseline for the first MVP and for future long-running development. The MVP is implemented in Go and must reach paper trading with real Polymarket market data and a real Kimi CLI agent. It must not include live trading, fake agents, fake accounts, fake market data, or GTC order lifecycle support.

## 1. Core Principle

- The system uses one complete `proposal.json` per cycle.
- The same `proposal.json` is saved, validated, judged, previewed, and used for paper execution.
- No execution path may consume a different cleaned or rewritten proposal.
- The execution layer must not mutate `proposal.json`.
- Any material mismatch between proposal intent and execution preview is a validation failure.
- The architecture should stay small: clear modules, few abstractions, no database or daemon in MVP.

## 2. MVP Scope

MVP must implement:

- Manual paper cycle trigger.
- Real Kimi CLI agent call.
- Real Polymarket public market and orderbook reads.
- Strict JSON proposal output.
- Schema scanner.
- Deterministic gates.
- Optional LLM judge with bounded format retry.
- Execution preview.
- Strategy consistency gate.
- Paper executor.
- Post-cycle portfolio snapshot with PnL metrics.

MVP must not implement:

- Live order submission.
- GTC / GTD order lifecycle.
- Fake agent mode.
- Fake account mode.
- Fake market data mode.
- Provider fallback.
- Web UI.
- Database-backed runtime.

## 3. Required Runtime Flow

The cycle must execute in this order:

1. Build cycle context.
2. Call Kimi CLI agent.
3. Save parsed agent output as `proposal.json`.
4. Run JSON schema scanner.
5. Run deterministic gates.
6. Run optional LLM judge if enabled.
7. Build execution preview.
8. Run strategy consistency gate.
9. Run paper executor.
10. Save post-cycle portfolio and execution results.

If any mandatory validation step fails, the full proposal must be repaired by the agent. Repairs are capped at 3 attempts. If validation still fails, no paper trade is executed.

## 4. Agent Requirements

- The agent must be Kimi CLI.
- The system must not expose a fake agent or fallback agent feature.
- If Kimi CLI is unavailable, the cycle fails.
- If Kimi output is not pure JSON, the schema gate fails.
- Agent output must not include Markdown, prose wrappers, code fences, or trailing commentary.
- The agent must always output a full proposal, not a patch.

## 5. Proposal JSON Requirements

`proposal.json` must include:

- `schema_version`
- `cycle_id`
- `created_at`
- `position_reviews`
- `new_opportunities`

Every decision item must include:

- Stable `decision_id`.
- `market_slug`.
- `token_id`.
- `outcome`.
- `action`.
- `ai_probability`.
- `market_probability`.
- `edge`.
- Evidence records.
- Counter-evidence records.
- Resolution information where applicable.
- Order intent where applicable.

The schema scanner must reject:

- Non-JSON output.
- Missing required fields.
- Invalid enum values.
- Probabilities outside `[0, 1]`.
- Incorrect `edge`; edge must equal `ai_probability - market_probability` within configured floating tolerance.
- Empty or placeholder values such as `N/A`, `unknown`, or `TODO` in required executable fields.

## 6. Polymarket Data Requirements

MVP may use Polymarket public APIs:

- Gamma API for market discovery and metadata.
- CLOB public read endpoints for orderbook, prices, and spreads.
- Data API public endpoints where needed.

MVP must not require:

- Polymarket private key.
- CLOB L2 API key.
- CLOB API secret.
- CLOB passphrase.
- Funder address.
- Signature type.

Those credentials are only relevant for future live trading and must not appear in MVP `.env.example`.

## 7. Deterministic Gates

MVP must include these mandatory gates:

- `schema_gate`
- `research_field_gate`
- `market_binding_gate`
- `probability_edge_gate`
- `portfolio_risk_gate`
- `execution_preview_gate`
- `strategy_consistency_gate`

Any deterministic gate failure blocks paper execution and enters the repair loop.

## 8. Research Field Gate

For each executable `open` item, require:

- Resolution source URL or equivalent source identifier.
- Resolution rule or rule excerpt.
- Time window.
- Threshold when the market has a threshold.
- At least 2 evidence records.
- At least 1 counter-evidence record.
- Each evidence record must include `source_url`, `source_type`, `retrieved_at`, `claim`, and `contract_relevance`.
- Evidence cannot be only comment, social, or generic background sources.
- If unresolved ambiguities are present, `open` must fail unless policy explicitly allows watchlist only.

This gate checks structured sufficiency. It does not claim to prove semantic truth.

## 9. Optional LLM Judge

- LLM judge is optional quality validation.
- Judge may inspect only `proposal.json`.
- Judge output must be JSON.
- If judge output format is invalid, retry once.
- If retry is still invalid, emit warning and skip judge.
- Default configuration allows execution when judge is skipped due to invalid output, provided deterministic gates pass.
- If judge output is valid and contains a hard failure, validation fails and enters repair.
- Judge cannot modify the proposal.
- Judge cannot override deterministic gate failures.

## 10. Market Binding Gate

Executable items must match real market data:

- `market_slug` exists.
- `token_id` exists.
- `outcome` exists in the market outcome set.
- The token ID matches the intended outcome.
- Market probability is consistent with current token price or orderbook reference within configured tolerance.
- Multi-market events cannot bind only by event slug.
- Threshold markets must match the proposal threshold.

Any binding mismatch is a hard failure.

## 11. Probability And Edge Gate

Rules:

- `ai_probability` must be in `[0, 1]`.
- `market_probability` must be in `[0, 1]`.
- `edge = ai_probability - market_probability`.
- `open` requires positive edge above configured threshold.
- Default `open` edge threshold is `0.05`.
- Net edge after configured fee estimate must remain positive when fees are enabled.
- `open` with `edge <= 0` is a hard failure.
- Confidence or freeform text cannot override edge rules.

## 12. Portfolio Risk Gate

Risk parameters must be visible and configurable:

- `PAPER_STARTING_CASH_USD`
- `MAX_SINGLE_TRADE_PCT`
- `MAX_TOTAL_EXPOSURE_PCT`
- `MAX_EVENT_EXPOSURE_PCT`
- `MAX_POSITIONS`
- `MIN_TRADE_USD`
- `EDGE_THRESHOLD_PCT`
- `MAX_REPAIR_ATTEMPTS`

The gate must enforce:

- Single-trade cap.
- Total exposure cap.
- Event exposure cap.
- Max position count.
- Internal minimum trade size.
- No agent-provided override of risk limits.

## 13. Execution Preview Gate

Execution preview is validation, not execution.

For paper FOK-style preview:

- BUY uses current best ask.
- SELL uses current best bid.
- Open requires best ask.
- Reduce/close requires best bid.
- Check minimum order size when available.
- Apply Kelly sizing.
- Apply risk caps.
- Apply slippage cap when depth is available.
- Compute executable notional.
- If executable notional is below internal or exchange minimum, do not increase it to force execution.

MVP does not support GTC lifecycle. Any GTC proposal must fail or be treated as unsupported.

## 14. Strategy Consistency Gate

The system must not silently change strategy intent.

Fail validation if:

- Executable notional is materially lower than proposed notional.
- Default materiality threshold: `executable_notional / proposed_notional < 0.5`.
- Proposed required orders are not executable.
- Order type changes.
- A multi-item strategy becomes incomplete after preview.
- Risk clipping changes the proposal's stated portfolio intent.

The agent must repair the full proposal when consistency fails.

## 15. Paper Executor

Paper executor must:

- Run only after full validation passes.
- Never call Polymarket order submission APIs.
- Simulate BUY at best ask.
- Simulate SELL at best bid.
- Write `execution-result.json`.
- Write `post-portfolio.json`.
- Update cash, positions, average cost, realized PnL, unrealized PnL, and NAV.
- Reject unsupported actions rather than pretending they executed.

MVP may omit redeem. If redeem appears in proposal, it must fail as unsupported unless implemented explicitly.

## 16. Portfolio Accounting

Paper and future live modes must share the same accounting logic.

`post-portfolio.json` must include:

- `cycle_id`
- `mode`
- `snapshot_at`
- `cash_usd`
- `total_position_value_usd`
- `nav_usd`
- `starting_nav_usd`
- `return_pct`
- `positions`

Each position must include:

- `position_id`
- `market_slug`
- `event_slug`
- `token_id`
- `outcome`
- `size`
- `avg_cost`
- `mark_price`
- `cost_basis_usd`
- `current_value_usd`
- `unrealized_pnl_usd`
- `unrealized_pnl_pct`
- `realized_pnl_usd`
- `opened_at`
- `updated_at`

Accounting rules:

- BUY updates weighted average cost.
- SELL reduces size and realizes PnL.
- Mark price defaults to held-token best bid.
- `nav_usd = cash_usd + total_position_value_usd`.
- `return_pct = (nav_usd - starting_nav_usd) / starting_nav_usd`.

## 17. Required Artifacts

Each cycle must write:

```text
runtime/cycles/<cycle_id>/
  proposal.json
  validation-result.json
  execution-preview.json
  execution-result.json
  post-portfolio.json
  error.json              # only when failed
```

No long handoff document is required. Evidence must live inside `proposal.json`.

## 18. Anti-Bypass Requirements

The system must reject or block:

- Markdown-wrapped JSON.
- Natural language plus JSON.
- Missing required fields explained in prose.
- `override_validation`.
- `ignore_risk`.
- Fake account or fake market data modes.
- Provider fallback.
- Low-edge open proposals.
- Source placeholders in executable items.
- Counter-evidence without source.
- Token / market / outcome mismatches.
- Agent-proposed order inflation to satisfy minimum order size.
- Judge pass overriding deterministic fail.
- Execution running after validation failure.
- Execution layer rewriting proposal.

## 19. Code Quality Requirements

MVP must include:

- Type definitions for proposal, validation result, execution preview, execution result, and portfolio snapshot.
- Runtime schema validation.
- Unit tests for schema scanner.
- Unit tests for deterministic gates.
- Unit tests for repair loop behavior.
- Unit tests for paper accounting.
- Fixture tests for invalid JSON, missing token, missing resolution source, bad edge, strategy inconsistency, unsupported GTC, and validation-failed no-execution.
- Clear non-zero exit code on failed cycle.
- No swallowed errors.
- No secrets in artifacts.
- No live credentials in MVP config.

## 20. Done Definition

MVP is accepted only when:

- `paper:cycle` runs with real Kimi CLI and real Polymarket read data.
- A valid cycle produces `proposal.json`, `validation-result.json`, `execution-preview.json`, `execution-result.json`, and `post-portfolio.json`.
- Paper portfolio PnL metrics are correct under tests.
- Invalid proposals never execute.
- Deterministic gate failures always block execution.
- Judge invalid output produces warning and skips judge by default.
- Judge valid hard failure blocks execution.
- Unsupported GTC does not execute.
- No fake agent/account/market-data feature exists in the runnable MVP.
