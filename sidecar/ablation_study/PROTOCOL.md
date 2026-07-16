# TradingAgents architecture ablation protocol

This protocol is deliberately fixed before the confirmatory model calls. Pilot
calibration files created by older models may be used only for feasibility and
power planning; they are excluded from the confirmatory estimates.

## Question and experimental arms

The study asks whether the downstream TradingAgents architecture adds value
after the first market analyst has already produced a report. One graph run
contains both outputs, so the comparison is paired and does not confuse model,
date, or market-data differences with an architecture effect.

For each point-in-time node the observable decisions are:

* `BUY`: constant long benchmark.
* `Q`: frozen deterministic quant trend snapshot.
* `S`: final transaction proposal in the market analyst report, blind to Q.
* `M`: portfolio-manager decision after research, trader, and risk agents,
  blind to Q.
* `Q+S`: market analyst final proposal when Q is visible.
* `Q+M`: downstream portfolio-manager decision from the same Q-visible run.

The architecture contrasts are `M-S` and `(Q+M)-(Q+S)`. The AI overlay on the
quant baseline is `(Q+M)-Q`. Constant BUY is an absolute benchmark, not a
substitute for the paired architecture contrast.

## Frozen design

* Primary symbol: `MU`.
* Decision interval: 20 trading days.
* Candidate dates: 2022-01-03 through 2025-06-30, subject to complete warm-up
  and future data.
* Independent units: up to 45 evenly spaced, non-overlapping decision windows.
* Stage one uses 30 independent nodes. Five stage-one nodes receive one extra
  repeat per context only to measure stochasticity. Repeats are averaged inside
  the node and never counted as independent market observations.
* Analysts: market/technical only. This is the only current analyst whose
  historical inputs can be reconstructed consistently without live social or
  later-filed fundamental/news leakage.
* Debate rounds: one. Risk rounds: one.
* Parallelism: seven graph runs.
* Entry: next trading-session open. Exit: close of the twentieth session.
* Cost: 10 basis points per independent full-notional position.
* Primary portfolio mapping: BUY=1, HOLD=0, SELL=0 (long/cash).
* Secondary diagnostic mapping: BUY=1, HOLD=0, SELL=-1 (long/short).
* Quant strategy, provider, exact models, backend URL, price hashes, node
  dates, source revision, and model-visible contexts are frozen in the
  machine-readable `protocol.json` before any confirmatory model call.
* Every historical price, indicator, and verified-snapshot tool is served from
  one protocol-frozen canonical OHLCV artifact. The artifact is total-return
  adjusted, content hashed, and sliced to each node's cutoff. Runtime access to
  Yahoo or another live price vendor is not part of the confirmatory treatment.

## Estimands and decision rule

The primary node-level architecture effect is the mean of the blind and
quant-informed paired return differences after averaging repeats. Its 97.5%
moving-block bootstrap interval is the confirmatory uncertainty interval at
each of the two possible looks. Block length is three chronological nodes and
the bootstrap seed is 20260716.

The architecture is classified as:

* **positive** only when the 97.5% interval is wholly above zero and neither
  context-specific point estimate is negative;
* **negative** only when the 97.5% interval is wholly below zero and neither
  context-specific point estimate is positive;
* **practically null** when the 97.5% interval and both context-specific means
  lie wholly inside an economic equivalence margin of +/-1 percentage point
  per 20-day node;
* **inconclusive** otherwise.

Absolute usefulness is reported separately: `Q+M` must also be compared with
constant BUY, Q, S, and M. Accuracy, action distributions, change rate,
turnover, compounded return, and endpoint maximum drawdown are secondary and
cannot override the preregistered primary return result.

## Evidence and exclusions

Every run stores the exact non-secret request configuration, model-visible
context, quant snapshot and hashes, future outcome and price-path hash, frozen
OHLCV ID and actual per-tool slice usage, raw analyst/manager reports, parsed
decisions, latency, best-effort token usage, errors, and content hashes. API
keys and provider secrets must never be stored.

A node is excluded only for a predeclared reason: missing warm-up/future price
data, irrecoverable provider failure after retries, or an output without a
parseable explicit decision. All exclusions remain in the evidence ledger.
Failed runs may be resumed by their deterministic run key; completed evidence
must be reused unless the entire study is explicitly superseded by a new
protocol ID.
