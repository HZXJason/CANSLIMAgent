# AlphaLens / CANSLIMAgent Architecture

> Portfolio-ready version: this diagram presents the project as an end-to-end investment research workbench with real-time data flows, persistent research state, and an internal multi-agent AI orchestration layer.

![AlphaLens portfolio architecture](./portfolio-architecture.svg)

## Portfolio Summary

AlphaLens is a local-first US equity research workbench for the CANSLIM workflow. It combines market universe scanning, metric caching, single-stock research, multi-agent AI analysis, N-label writeback, and trade journaling through a React frontend, Go API layer, Python AI sidecar, and Dockerized PostgreSQL runtime.

## Architecture Highlights

- Frontend workbench: React, TypeScript, Zustand, and SSE parsing for the CANSLIM screener, stock analysis, AI report stream, settings, and trade journal.
- Go API layer: Gin-based REST and SSE gateway for screener scans, stock metrics, chart data, journal records, health checks, and AI sidecar proxying.
- Data and cache layer: Dockerized PostgreSQL persists stocks, metrics, threshold presets, N-label analysis summaries, and trade journal entries.
- Multi-agent research layer: the FastAPI sidecar receives model and role configuration, then orchestrates analyst agents, research debate, trader planning, risk review, and portfolio decision output.
- Streaming UX: the screener emits `scan_status` and `stock_result`; AI analysis emits `progress`, `narration_delta`, `report_delta`, and `decision`.
- Reusable local development: `make dev` / `scripts/dev.sh` starts PostgreSQL, the Go API, the Python sidecar, and the Vite frontend with health checks.

## Mermaid Source

```mermaid
flowchart TB
  User["User / Investment Research Workflow"]

  subgraph FE["Frontend Workbench - React + TypeScript"]
    Shell["AppShell / Routing"]
    Screener["Module 1: CANSLIM Screener"]
    Analysis["Module 2: Stock Analysis"]
    AIUI["AI Config + Streaming Report"]
    Journal["Module 3: Trade Journal"]
    Store["Zustand State + SSE Parser"]
  end

  subgraph API["Go Gin API Layer"]
    Gateway["REST + SSE Gateway"]
    Scanner["ScannerService\nUniverse Scan / CANSLIM Signals"]
    AnalysisSvc["AnalysisService\nK-line / RS / EPS / Revenue / Margin"]
    AIProxy["AIAnalysisHandler\nSidecar Proxy / SSE Relay"]
    JournalAPI["JournalHandler\nTrade Entries / Review Updates"]
  end

  subgraph AI["AI Research Orchestration - FastAPI Sidecar"]
    Adapter["Request Adapter\nModel / API Key / Agent Config"]
    Runtime["Multi-agent Research Graph"]
    Analysts["Fundamental / Sentiment / News / Technical Analysts"]
    Debate["Research Team Bull/Bear Debate"]
    Trader["Trader Investment Plan"]
    Risk["Risk Team Position and Drawdown Review"]
    PM["Portfolio Manager Final Decision"]
    Stream["SSE Event Builder\nprogress / report_delta / decision"]
  end

  subgraph DATA["Data and Infrastructure"]
    DB[("PostgreSQL Docker\nstocks / stock_metrics / presets / journals")]
    Market["Market and Fundamental Data\nSEC EDGAR / Stooq / Yahoo / yfinance\nFMP / Polygon optional"]
    LLM["LLM Providers\nOpenAI / Anthropic / Gemini / Vercel Gateway"]
    Dev["Local Dev Runtime\nmake dev / scripts/dev.sh"]
  end

  User --> FE
  Shell --> Screener
  Shell --> Analysis
  Analysis --> AIUI
  Shell --> Journal
  Screener --> Store
  AIUI --> Store

  FE -->|"REST / SSE"| Gateway
  Gateway --> Scanner
  Gateway --> AnalysisSvc
  Gateway --> AIProxy
  Gateway --> JournalAPI

  Scanner --> DB
  AnalysisSvc --> DB
  JournalAPI --> DB
  Scanner --> Market
  AnalysisSvc --> Market

  AIProxy -->|"POST /analyze + SSE"| Adapter
  Adapter --> Runtime
  Runtime --> Analysts
  Analysts --> Debate
  Debate --> Trader
  Trader --> Risk
  Risk --> PM
  PM --> Stream
  Stream --> AIProxy
  Runtime --> LLM
  Runtime --> Market
  AIProxy -->|"N-label writeback"| Scanner

  Dev --> DB
  Dev --> API
  Dev --> FE
  Dev --> AI
```
