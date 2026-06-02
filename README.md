# Prediction Market Arb Bot

Go MVP for an agent-driven prediction-market paper trading bot.

The first MVP uses:

- Real Kimi CLI agent calls.
- Real Polymarket public read data.
- Strict `proposal.json` validation.
- Paper execution only.

It intentionally does not support:

- Fake agent mode.
- Fake account mode.
- Fake market data mode.
- Live order submission.
- GTC/GTD lifecycle.
- Provider fallback.

Run:

```sh
go run ./cmd/paper-cycle
```

The cycle writes artifacts under `runtime/cycles/<cycle_id>/`.

