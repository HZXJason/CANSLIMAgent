# Changelog

All notable changes are recorded here. This project follows Semantic Versioning once the first release is tagged.

## Unreleased — AlphaLens 2.0 Overhaul

### Product & Finance Core
- Added CANSLIM `M` Market Regime Gate (`MarketRegimeService`) with `21EMA` / `50SMA` / `200SMA` alignment and 25-day O'Neil Distribution Day tracking for US (`SPY`/`QQQ`) and CN (`000300.SH`/`399006.SZ`) benchmarks.
- Added deterministic `1R` Position Sizer (`POST /api/v1/position-sizer`) supporting market board-lot rounding (US 1, CN A-share 100, STAR 200 min), ATR stop-tightness checks, and Portfolio Heat enforcement.
- Upgraded `Trade` lifecycle aggregation (`RebuildUserTrades`) to compute `Realized R`, `Entry Slippage (R)`, automatic violation detection (`off_plan_unlinked`, `entry_chase_over_0_25r`, `stop_loss_violated`), 4-Quadrant classification (`model_win`, `good_loss`, `unforced_error`, `toxic_win`), and North Star `Disciplined R-Loop Rate`.
- Reorganized Web IA into 4 temporal workspaces (`/desk`, `/research`, `/positions`, `/system`) with a 45-second Research & Quick Plan Cockpit; added `作战室` Companion view to native iOS.

### AI & Architecture Simplification
- Replaced multi-agent debate centerpiece with deterministic `Evidence Card Compiler` (`POST /api/v1/stocks/:symbol/evidence-cards`) + optional `<5s` single-hop counter-thesis extraction and upgraded `Pre-Trade Red Team` risk review.
- Removed external `Vibe-Trading` subprocess bridge (`vibe_journal_runner.py`), premature `ProfileTag` taxonomy tables, obsolete `frontend/design2/`, and redundant legacy pages (`Today/`).

### Security & Reliability
- Retained exact-origin CORS, loopback-by-default binding, reverse-proxy API authentication, sidecar authentication, AES-GCM LLM/broker credential encryption, and idempotent trade execution/import.
