"""Resumable execution of the preregistered paired TradingAgents study."""

from __future__ import annotations

from concurrent.futures import FIRST_COMPLETED, ThreadPoolExecutor, wait
import copy
from datetime import date, timedelta
import json
from pathlib import Path
import re
import subprocess
import sys
from threading import Event
import time
from typing import Any, Callable

import pandas as pd

_VENDOR_ROOT = Path(__file__).resolve().parents[1] / "vendor"
if str(_VENDOR_ROOT) not in sys.path:
    sys.path.insert(0, str(_VENDOR_ROOT))

from quant_trend.artifacts import stable_json_hash
from quant_trend.data import normalize_price_frame
from quant_trend.experiments import ExperimentArm, build_experiment_context, decision_node_id
from tradingagents.dataflows.frozen_ohlcv import frozen_ohlcv_scope

from .artifacts import StudyArtifactStore
from .design import build_staged_design, stage_one_stopping_decision, staged_task_keys
from .statistics import analyze_study_records


PARSER_VERSION = "explicit-decision.v3"
_DECISION_PATTERNS = (
    re.compile(
        r"(?im)^\s*\**\s*rating\s*\**\s*[:：-]\s*\**\s*"
        r"(buy|overweight|hold|underweight|sell)\b"
    ),
    re.compile(r"(?im)^\s*\**\s*action\s*\**\s*[:：-]\s*\**\s*(buy|hold|sell)\b"),
    re.compile(
        r"(?im)^\s*final transaction proposal\s*[:：-]\s*\**\s*(buy|hold|sell)\b"
    ),
    re.compile(
        r"(?im)^\s*\**\s*(?:评级|行动|建议|最终交易建议|最终交易提案)\s*"
        r"\**\s*[:：-]\s*\**\s*(买入|增持|超配|持有|中性|减持|低配|卖出)"
    ),
)


def parse_explicit_action(value: Any) -> str | None:
    # Markdown emphasis is presentation, not part of an explicit field name.
    # Remove only bold markers; keep line structure and labels strict so prose
    # such as "we considered SELL" can never become a decision.
    text = str(value or "").replace("**", "").replace("__", "")
    for pattern in _DECISION_PATTERNS:
        match = pattern.search(text)
        if match:
            rating = match.group(1).upper()
            return {
                "OVERWEIGHT": "BUY",
                "UNDERWEIGHT": "SELL",
                "买入": "BUY",
                "增持": "BUY",
                "超配": "BUY",
                "持有": "HOLD",
                "中性": "HOLD",
                "减持": "SELL",
                "低配": "SELL",
                "卖出": "SELL",
            }.get(rating, rating)
    stripped = text.strip(" *.:：-").upper()
    stripped = {
        "买入": "BUY",
        "增持": "BUY",
        "超配": "BUY",
        "持有": "HOLD",
        "中性": "HOLD",
        "减持": "SELL",
        "低配": "SELL",
        "卖出": "SELL",
    }.get(stripped, stripped)
    if stripped in {"BUY", "HOLD", "SELL"}:
        return stripped
    return None


def _json_safe(value: Any) -> Any:
    try:
        json.dumps(value, ensure_ascii=False, allow_nan=False)
        return copy.deepcopy(value)
    except (TypeError, ValueError):
        if isinstance(value, dict):
            return {str(key): _json_safe(child) for key, child in value.items()}
        if isinstance(value, (list, tuple)):
            return [_json_safe(child) for child in value]
        return str(value)


def _evidence_state(state: dict[str, Any]) -> dict[str, Any]:
    keys = (
        "market_report",
        "final_trade_decision",
        "investment_plan",
        "trader_investment_plan",
        "investment_debate_state",
        "risk_debate_state",
        "messages",
    )
    return {key: _json_safe(state[key]) for key in keys if state.get(key) not in (None, "")}


def _source_fingerprint(repo_root: Path) -> dict[str, Any]:
    try:
        head = subprocess.check_output(
            ["git", "rev-parse", "HEAD"], cwd=repo_root, text=True, stderr=subprocess.DEVNULL
        ).strip()
    except (OSError, subprocess.CalledProcessError):
        head = "unknown"
    relevant = []
    for relative in (
        "sidecar/main.py",
        "sidecar/ablation_study",
        "sidecar/quant_trend",
        "sidecar/vendor/tradingagents/agents",
        "sidecar/vendor/tradingagents/dataflows",
        "sidecar/vendor/tradingagents/graph",
    ):
        path = repo_root / relative
        candidates = (
            [path]
            if path.is_file()
            else sorted(
                candidate
                for candidate in path.rglob("*")
                if candidate.is_file() and candidate.suffix.lower() in {".py", ".md"}
            )
            if path.exists()
            else []
        )
        for candidate in candidates:
            relevant.append(
                {
                    "path": str(candidate.relative_to(repo_root)),
                    "sha256": stable_json_hash(candidate.read_text(encoding="utf-8")),
                }
            )
    return {
        "git_head": head,
        "relevant_source_hash": stable_json_hash(relevant),
        "files": relevant,
    }


def _outcome_from_evaluation_node(
    node: dict[str, Any], node_id: str, daily_future_path: list[dict[str, Any]]
) -> dict[str, Any]:
    future_summary = {
        key: node.get(key)
        for key in (
            "entry_date",
            "entry_open",
            "total_return_entry_open",
            "outcome_date",
            "barrier",
            "neutral_return_band",
            "path_label",
            "trend_label",
            "endpoint_barrier_label",
            "first_hit",
            "barrier_hit_day",
            "barrier_hit_date",
            "ambiguous",
            "endpoint_return",
            "endpoint_total_return",
            "maximum_favorable_move",
            "maximum_adverse_move",
        )
    }
    core = {
        "schema_version": "ta.ablation_outcome.v1",
        "node_id": node_id,
        "as_of_date": node["as_of_date"],
        "entry_rule": "next_session_open",
        "exit_rule": "horizon_session_close",
        **future_summary,
        "daily_future_path": daily_future_path,
        "future_path_hash": stable_json_hash(daily_future_path),
    }
    return {**core, "outcome_id": stable_json_hash(core)}


def _protocol_core(
    request: Any,
    evaluation: dict[str, Any],
    node_contracts: list[dict[str, Any]],
    model_data: dict[str, Any],
    source: dict[str, Any],
    staged_design: dict[str, Any] | None,
) -> dict[str, Any]:
    adaptive_stages = staged_design is not None
    primary_confidence = (
        float(staged_design["primary_confidence_level_each_look"])
        if staged_design
        else 0.95
    )
    equivalence_confidence = (
        float(staged_design["equivalence_confidence_level_each_look"])
        if staged_design
        else 0.90
    )
    return {
        "schema_version": "ta.ablation_protocol.v1",
        "question": "Does the downstream TradingAgents architecture improve decisions beyond its first market analyst?",
        "symbol": request.ticker.strip().upper(),
        "model": {
            "provider": request.llm_provider,
            "deep": request.deep_think_llm,
            "quick": request.quick_think_llm,
            "backend_url": (request.backend_url or "").rstrip("/"),
            "online_tools": request.online_tools,
        },
        "graph": {
            "analysts": ["market"],
            "max_debate_rounds": request.max_debate_rounds,
            "max_risk_rounds": request.max_risk_rounds,
            "single_decision": "market_report.final_transaction_proposal",
            "multi_decision": "final_trade_decision.rating_or_action",
            "parser_version": PARSER_VERSION,
        },
        "design": {
            "start_date": request.start_date.isoformat(),
            "end_date": request.end_date.isoformat(),
            "horizon_trading_days": request.horizon_days,
            "step_trading_days": request.horizon_days,
            "maximum_nodes": request.maximum_nodes,
            "repeats_per_context": request.repeats,
            "contexts": ["blind", "quant_informed"],
            "parallelism": request.concurrency,
            "adaptive_stages": adaptive_stages,
            "staged_design": staged_design,
            "independent_unit": "non_overlapping_market_node",
            "repeat_rule": "average repeats within node; never count repeats as independent observations",
        },
        "execution": {
            "entry": "next_session_open",
            "exit": "horizon_session_close",
            "return_basis": "total_return_adjusted_close_from_adjusted_next_session_open",
            "cost_bps_per_position": request.transaction_cost_bps,
            "primary_position_mapping": {"BUY": 1, "HOLD": 0, "SELL": 0},
            "secondary_position_mapping": {"BUY": 1, "HOLD": 0, "SELL": -1},
        },
        "statistics": {
            "primary_estimand": "mean of node-level M-S and (Q+M)-(Q+S) long/cash return deltas",
            "bootstrap": "circular_moving_block_percentile",
            "block_length_nodes": 3,
            "bootstrap_iterations": 20_000,
            "randomization_iterations": 100_000,
            "seed": 20260716,
            "equivalence_margin_per_node": 0.01,
            "primary_confidence_level": primary_confidence,
            "equivalence_confidence_level": equivalence_confidence,
            "classification_rule": (
                f"positive iff {primary_confidence:.3f} CI > 0 and both context means >= 0; "
                f"negative iff {primary_confidence:.3f} CI < 0 and both context means <= 0; "
                f"practically_null iff {equivalence_confidence:.3f} CI and both context means are "
                "within +/-1%; otherwise inconclusive"
            ),
        },
        "quant": {
            "strategy_version": evaluation["strategy_version"],
            "config_hash": evaluation["config_hash"],
            "data_source": evaluation["data_source"],
            "evaluation_run_id": evaluation["run_id"],
            "evaluation_data_hash": evaluation["data_hash"],
        },
        "model_data": model_data,
        "nodes": node_contracts,
        "source": source,
        "evidence_requirements": {
            "tool_call_trace_required": True,
            "model_visible_context_required": True,
            "raw_decision_state_required": True,
            "future_outcome_hidden_until_scoring": True,
            "global_memory_disabled": True,
            "per_run_checkpoint_namespace": True,
            "protocol_frozen_ohlcv_required": True,
            "frozen_ohlcv_usage_attestation_required": True,
        },
        "exclusions": [
            "missing warm-up or future prices",
            "irrecoverable provider failure after retries",
            "missing explicit analyst or portfolio-manager decision",
        ],
        "pilot_exclusion": "All pre-existing calibration runs and all older models are excluded from confirmation.",
    }


def _run_relative(context: str, node_id: str, repeat: int) -> Path:
    return Path("runs") / context / node_id / f"repeat-{repeat}.json"


def _record_key(study_id: str, context: str, node_id: str, repeat: int) -> str:
    return stable_json_hash(
        {
            "schema_version": "ta.ablation_run_key.v1",
            "study_id": study_id,
            "context": context,
            "node_id": node_id,
            "repeat": repeat,
        }
    )


def _render_report(analysis: dict[str, Any]) -> str:
    pooled = analysis["effects"]["pooled_architecture_effect"]
    blind = analysis["effects"]["M_minus_S"]
    informed = analysis["effects"]["Q+M_minus_Q+S"]
    confidence = float(pooled["primary_confidence_level"])
    confidence_label = f"{confidence * 100:g}%"
    lines = [
        "# TradingAgents architecture ablation result",
        "",
        f"- Study ID: `{analysis['study_id']}`",
        f"- Independent nodes: {analysis['independent_nodes_analyzed']}",
        f"- Completed graph runs: {analysis['completed_graph_runs']}",
        f"- Preregistered classification: **{analysis['architecture_classification']}**",
        "",
        "## Primary paired effects",
        "",
        f"| Contrast | Mean return delta/node | {confidence_label} moving-block CI | Sign-flip p |",
        "|---|---:|---:|---:|",
    ]
    for label, item in (("M-S", blind), ("(Q+M)-(Q+S)", informed), ("Pooled architecture", pooled)):
        ci = item["moving_block_primary_interval"] or [None, None]
        lines.append(
            f"| {label} | {item['mean_delta']} | [{ci[0]}, {ci[1]}] | {item['paired_sign_flip_two_sided_p']} |"
        )
    lines.extend(
        [
            "",
            "The classification follows the frozen protocol. Accuracy and compounded returns below are secondary; they do not override the paired primary estimand.",
            "",
            "## Long/cash arm performance",
            "",
            "| Arm | Mean node return | Compounded return | Endpoint max drawdown |",
            "|---|---:|---:|---:|",
        ]
    )
    for arm, item in analysis["arm_performance"]["long_cash"].items():
        lines.append(
            f"| {arm} | {item['mean_node_return']} | {item['total_compounded_return']} | {item['endpoint_maximum_drawdown']} |"
        )
    lines.extend(["", "## Limitations", ""])
    lines.extend(f"- {limitation}" for limitation in analysis["limitations"])
    return "\n".join(lines)


def run_study(
    request: Any,
    *,
    quant_service: Any,
    run_analysis: Callable[..., tuple[dict[str, Any], Any]],
    callback_factory: Callable[[], Any] | None,
    artifact_root: str | Path,
    repo_root: str | Path,
    progress_callback: Callable[[dict[str, Any]], None] | None = None,
    cancel_event: Event | None = None,
) -> dict[str, Any]:
    """Freeze inputs, run paired contexts, and persist all evidence."""

    def cancelled() -> bool:
        return bool(cancel_event is not None and cancel_event.is_set())

    if cancelled():
        raise RuntimeError("ablation study was cancelled")
    evaluation = quant_service.evaluate(
        symbol=request.ticker,
        start_date=request.start_date,
        end_date=request.end_date,
        horizon_days=request.horizon_days,
        step_days=request.horizon_days,
        maximum_nodes=request.maximum_nodes,
        transaction_cost_bps=request.transaction_cost_bps,
    )
    adaptive_stages = bool(getattr(request, "adaptive_stages", False))
    staged_design = (
        build_staged_design(
            evaluation,
            stage_one_nodes=int(getattr(request, "stage_one_nodes", 30)),
            reproducibility_nodes=int(getattr(request, "reproducibility_nodes", 5)),
        )
        if adaptive_stages
        else None
    )
    staged_contracts = (staged_design or {}).get("node_contracts") or []
    snapshots: dict[str, dict[str, Any]] = {}
    outcomes: dict[str, dict[str, Any]] = {}
    node_contracts: list[dict[str, Any]] = []
    first_entry = date.fromisoformat(evaluation["nodes"][0]["entry_date"])
    last_outcome = date.fromisoformat(evaluation["nodes"][-1]["outcome_date"])
    outcome_frame = normalize_price_frame(
        quant_service.data_source.history(request.ticker, first_entry, last_outcome),
        symbol=request.ticker,
    )
    for position, node in enumerate(evaluation["nodes"]):
        as_of = date.fromisoformat(node["as_of_date"])
        snapshot = quant_service.predict(
            symbol=request.ticker,
            as_of=as_of,
            horizon_days=request.horizon_days,
        )
        if snapshot.get("status") != "ok":
            raise ValueError(f"quant snapshot unavailable at {as_of.isoformat()}")
        node_id = decision_node_id(snapshot)
        snapshots[node_id] = snapshot
        entry_date = date.fromisoformat(node["entry_date"])
        outcome_date = date.fromisoformat(node["outcome_date"])
        path_frame = outcome_frame.loc[
            (outcome_frame.index.date >= entry_date) & (outcome_frame.index.date <= outcome_date)
        ]
        if len(path_frame) != request.horizon_days:
            raise ValueError(f"future path has {len(path_frame)} rows at {node['as_of_date']}")
        daily_future_path = [
            {
                "date": timestamp.date().isoformat(),
                "open": round(float(row["open"]), 8),
                "high": round(float(row["high"]), 8),
                "low": round(float(row["low"]), 8),
                "close": round(float(row["close"]), 8),
                "total_return_close": round(float(row["total_return_close"]), 8),
            }
            for timestamp, row in path_frame.iterrows()
        ]
        outcomes[node_id] = _outcome_from_evaluation_node(node, node_id, daily_future_path)
        contract = {
                "node_id": node_id,
                "as_of_date": node["as_of_date"],
                "outcome_date": node["outcome_date"],
                "quant_snapshot_id": snapshot["snapshot_id"],
                "quant_data_hash": snapshot["data_hash"],
                "quant_action": snapshot["action"],
                "outcome_id": outcomes[node_id]["outcome_id"],
                "future_path_hash": outcomes[node_id]["future_path_hash"],
        }
        if staged_design:
            stage_contract = staged_contracts[position]
            if stage_contract["as_of_date"] != node["as_of_date"]:
                raise ValueError("staged design node order differs from quant evaluation")
            contract.update(
                {
                    "position": position,
                    "stage": int(stage_contract["stage"]),
                    "reproducibility_repeat": bool(stage_contract["reproducibility_repeat"]),
                }
            )
        node_contracts.append(contract)

    first_as_of = date.fromisoformat(evaluation["nodes"][0]["as_of_date"])
    last_as_of = date.fromisoformat(evaluation["nodes"][-1]["as_of_date"])
    model_history = normalize_price_frame(
        quant_service.data_source.history(
            request.ticker,
            first_as_of - timedelta(days=550),
            last_as_of,
        ),
        symbol=request.ticker,
    )
    adjustment = model_history["total_return_close"] / model_history["close"]
    agent_frame = pd.DataFrame(
        {
            "Date": model_history.index,
            "Open": model_history["open"] * adjustment,
            "High": model_history["high"] * adjustment,
            "Low": model_history["low"] * adjustment,
            "Close": model_history["total_return_close"],
            "Volume": (
                model_history["volume"]
                if "volume" in model_history.columns
                else pd.Series(0.0, index=model_history.index)
            ),
        }
    ).reset_index(drop=True)
    model_rows = [
        {
            "date": row.Date.date().isoformat(),
            "open": round(float(row.Open), 8),
            "high": round(float(row.High), 8),
            "low": round(float(row.Low), 8),
            "close": round(float(row.Close), 8),
            "volume": round(float(row.Volume), 8),
        }
        for row in agent_frame.itertuples(index=False)
    ]
    model_data_core = {
        "schema_version": "ta.protocol_frozen_ohlcv.v1",
        "symbol": request.ticker.strip().upper(),
        "first_date": model_rows[0]["date"],
        "last_date": model_rows[-1]["date"],
        "rows": len(model_rows),
        "content_hash": stable_json_hash(model_rows),
        "adjustment_rule": "OHLC multiplied by total_return_close/raw_close; Close equals total_return_close",
        "future_rows_visible_per_node": False,
    }
    model_data = {
        **model_data_core,
        "model_data_id": stable_json_hash(model_data_core),
    }
    agent_frame = pd.DataFrame(
        {
            "Date": pd.to_datetime([row["date"] for row in model_rows]),
            "Open": [row["open"] for row in model_rows],
            "High": [row["high"] for row in model_rows],
            "Low": [row["low"] for row in model_rows],
            "Close": [row["close"] for row in model_rows],
            "Volume": [row["volume"] for row in model_rows],
        }
    )

    source = _source_fingerprint(Path(repo_root).resolve())
    core = _protocol_core(
        request, evaluation, node_contracts, model_data, source, staged_design
    )
    study_id = stable_json_hash(core)
    protocol = {**core, "study_id": study_id}
    store = StudyArtifactStore(artifact_root, study_id)
    store.write_immutable_json("protocol.json", protocol)
    store.write_immutable_json(
        "quant/evaluation.json",
        {key: value for key, value in evaluation.items() if key != "artifact_path"},
    )
    store.write_immutable_json(
        "model-data/ohlcv.json",
        {**model_data, "price_rows": model_rows},
    )
    for contract in node_contracts:
        node_id = contract["node_id"]
        store.write_immutable_json(Path("snapshots") / f"{node_id}.json", snapshots[node_id])
        store.write_immutable_json(Path("outcomes") / f"{node_id}.json", outcomes[node_id])

    if staged_design:
        contracts_by_id = {contract["node_id"]: contract for contract in node_contracts}
        task_matrix = staged_task_keys(node_contracts, ("blind", "quant_informed"))
        stage_one_tasks = [
            (context, contracts_by_id[node_id], repeat)
            for node_id, context, repeat in task_matrix["stage_one"]
        ]
        stage_two_tasks = [
            (context, contracts_by_id[node_id], repeat)
            for node_id, context, repeat in task_matrix["stage_two"]
        ]
    else:
        stage_one_tasks = [
            (context, contract, repeat)
            for contract in node_contracts
            for context in ("blind", "quant_informed")
            for repeat in range(1, request.repeats + 1)
        ]
        stage_two_tasks = []
    maximum_runs = len(stage_one_tasks) + len(stage_two_tasks)
    scheduled_runs = len(stage_one_tasks)
    completed = 0
    cached = 0
    failed = 0
    records: list[dict[str, Any]] = []
    current_stage = 1

    def publish(status_value: str = "running") -> None:
        status = {
            "schema_version": "ta.ablation_status.v1",
            "study_id": study_id,
            "status": status_value,
            "completed_runs": completed,
            "total_runs": maximum_runs if status_value == "running" else scheduled_runs,
            "scheduled_runs": scheduled_runs,
            "maximum_runs": maximum_runs,
            "cached_runs": cached,
            "failed_runs": failed,
            "independent_nodes": len(node_contracts),
            "stage": current_stage,
        }
        store.write_json("status.json", status)
        if progress_callback is not None:
            progress_callback(status)

    publish()

    def execute(task: tuple[str, dict[str, Any], int]) -> dict[str, Any]:
        context, contract, repeat = task
        node_id = contract["node_id"]
        relative = _run_relative(context, node_id, repeat)
        run_key = _record_key(study_id, context, node_id, repeat)
        existing = store.load_json(relative)
        if (
            existing is not None
            and existing.get("status") == "complete"
            and existing.get("run_key") == run_key
            and existing.get("study_id") == study_id
        ):
            return {**existing, "_cached": True}
        snapshot = snapshots[node_id]
        experiment_arm = (
            ExperimentArm.TA_BLIND if context == "blind" else ExperimentArm.QUANT_INFORMED
        )
        common_context = {
            "calibration": {
                "schema_version": "ta.ablation_node.v1",
                "as_of_date": contract["as_of_date"],
                "horizon_trading_days": request.horizon_days,
                "task": (
                    f"Forecast and choose a transaction action for the next {request.horizon_days} "
                    "trading days using only evidence available by the as-of date."
                ),
                "rule": "Do not use future information or memories from other nodes.",
            }
        }
        model_context = build_experiment_context(experiment_arm, snapshot, common_context)
        run_id = f"abl-{run_key[:48]}"
        node_request = request.model_copy(
            update={
                "analysis_context": model_context,
                "analysts": ["technical"],
                "run_id": run_id,
                "resume": False,
                "last_event_id": 0,
            }
        )
        started = time.monotonic()
        usage_callback = callback_factory() if callback_factory is not None else None
        callbacks = [usage_callback] if usage_callback is not None else None
        state: dict[str, Any] = {}
        decision: Any = None
        error: str | None = None
        attempts = max(1, min(5, int(request.max_attempts)))
        with frozen_ohlcv_scope(
            request.ticker, model_data["model_data_id"], agent_frame
        ) as frozen_state:
            for attempt in range(1, attempts + 1):
                if cancelled():
                    raise RuntimeError("ablation study was cancelled")
                try:
                    state, decision = run_analysis(
                        node_request,
                        callbacks,
                        analysis_date=date.fromisoformat(contract["as_of_date"]),
                        analysts=("market",),
                    )
                    if not frozen_state.usage:
                        raise ValueError(
                            "historical analysis did not use protocol-frozen OHLCV"
                        )
                    single_action = parse_explicit_action(state.get("market_report"))
                    multi_action = parse_explicit_action(
                        state.get("final_trade_decision")
                    ) or parse_explicit_action(decision)
                    if single_action is None or multi_action is None:
                        raise ValueError(
                            f"missing explicit decision: single={single_action}, multi={multi_action}"
                        )
                    error = None
                    break
                except Exception as exc:  # noqa: BLE001 - each evidence run is isolated and retried
                    error = f"{type(exc).__name__}: {exc}"
                    if request.api_key:
                        error = error.replace(request.api_key, "[REDACTED]")
                    error = error[:2000]
                    if attempt >= attempts:
                        single_action = None
                        multi_action = None
                        break
                    node_request.resume = True
                    time.sleep(min(2 ** (attempt - 1), 8))
        usage = usage_callback.snapshot() if usage_callback is not None and hasattr(usage_callback, "snapshot") else {}
        config = {
            "ticker": request.ticker.strip().upper(),
            "llm_provider": request.llm_provider,
            "deep_think_llm": request.deep_think_llm,
            "quick_think_llm": request.quick_think_llm,
            "backend_url": (request.backend_url or "").rstrip("/"),
            "max_debate_rounds": request.max_debate_rounds,
            "max_risk_rounds": request.max_risk_rounds,
            "online_tools": request.online_tools,
            "analysts": ["technical"],
            "run_id": run_id,
            "historical_isolation": {
                "global_memory": "disabled_by_calibration_context",
                "checkpoint_namespace": run_id,
                "future_outcome_visible": False,
                "market_data_mode": "protocol_frozen_canonical_ohlcv",
                "model_data_id": model_data["model_data_id"],
            },
        }
        core_record = {
            "schema_version": "ta.ablation_run.v1",
            "study_id": study_id,
            "run_key": run_key,
            "node_id": node_id,
            "as_of_date": contract["as_of_date"],
            "context": context,
            "repeat": repeat,
            "status": "complete" if error is None else "failed",
            "attempts": attempt,
            "request_config": config,
            "model_visible_context": model_context,
            "model_visible_context_hash": stable_json_hash(model_context),
            "quant_snapshot_id": snapshot["snapshot_id"] if context == "quant_informed" else None,
            "outcome_id": contract["outcome_id"],
            "parser_version": PARSER_VERSION,
            "single_action": single_action,
            "multi_action": multi_action,
            "latency_seconds": round(time.monotonic() - started, 6),
            "usage": usage,
            "frozen_ohlcv_usage": _json_safe(frozen_state.usage),
            "state": _evidence_state(state),
            "fallback_decision": _json_safe(decision),
            "error": error,
        }
        record = {**core_record, "record_id": stable_json_hash(core_record)}
        store.write_json(relative, record)
        return {**record, "_cached": False}

    def run_batch(tasks: list[tuple[str, dict[str, Any], int]]) -> None:
        nonlocal completed, cached, failed
        if not tasks:
            return
        executor = ThreadPoolExecutor(
            max_workers=min(request.concurrency, len(tasks)), thread_name_prefix="ta-ablation"
        )
        futures = {executor.submit(execute, task): task for task in tasks}
        unfinished = set(futures)
        try:
            while unfinished:
                if cancelled():
                    raise RuntimeError("ablation study was cancelled")
                done, unfinished = wait(unfinished, timeout=0.25, return_when=FIRST_COMPLETED)
                for future in done:
                    record = future.result()
                    was_cached = bool(record.pop("_cached", False))
                    records.append(record)
                    completed += 1
                    cached += int(was_cached)
                    failed += int(record.get("status") != "complete")
                    publish()
        except Exception:
            for future in unfinished:
                future.cancel()
            # Evidence cannot be sealed while an in-flight worker can still
            # write into the study directory. The cancel endpoint reports
            # "stopping" immediately; finalization waits for those calls.
            executor.shutdown(wait=True, cancel_futures=True)
            raise
        else:
            executor.shutdown(wait=True)

    stopped_early = False
    try:
        run_batch(stage_one_tasks)
        if staged_design:
            stage_one_keys = {
                (contract["node_id"], context, repeat)
                for context, contract, repeat in stage_one_tasks
            }
            stage_one_records = [
                record
                for record in records
                if (record["node_id"], record["context"], int(record["repeat"])) in stage_one_keys
            ]
            stage_one_successes = sum(
                record.get("status") == "complete" for record in stage_one_records
            )
            interim = analyze_study_records(protocol, stage_one_records, outcomes)
            interim_core = {
                **interim,
                "stage": 1,
                "failed_graph_runs": len(stage_one_tasks) - stage_one_successes,
                "expected_graph_runs": len(stage_one_tasks),
            }
            interim_record = {
                **interim_core,
                "analysis_id": stable_json_hash(interim_core),
            }
            store.write_json("interim/stage-1-analysis.json", interim_record)
            stage_one_complete = stage_one_successes == len(stage_one_tasks)
            stopping = stage_one_stopping_decision(
                interim["architecture_classification"], matrix_complete=stage_one_complete
            )
            advance = bool(stopping["advance_to_stage_two"])
            reason = str(stopping["reason"])
            stopped_early = bool(stopping["stopped_early"])
            decision_core = {
                "schema_version": "ta.ablation_stage_decision.v1",
                "study_id": study_id,
                "stage": 1,
                "stage_one_complete": stage_one_complete,
                "classification": interim["architecture_classification"],
                "advance_to_stage_two": advance,
                "reason": reason,
                "interim_analysis_id": interim_record["analysis_id"],
            }
            store.write_json(
                "interim/stage-1-decision.json",
                {**decision_core, "decision_id": stable_json_hash(decision_core)},
            )
            if advance:
                current_stage = 2
                scheduled_runs = maximum_runs
                publish()
                run_batch(stage_two_tasks)
    except Exception:
        terminal_status = {
            "schema_version": "ta.ablation_status.v1",
            "study_id": study_id,
            "status": "cancelled" if cancelled() else "failed",
            "completed_runs": completed,
            "total_runs": scheduled_runs,
            "scheduled_runs": scheduled_runs,
            "maximum_runs": maximum_runs,
            "cached_runs": cached,
            "failed_runs": failed,
            "independent_nodes": len(node_contracts),
            "stage": current_stage,
        }
        store.write_json("status.json", terminal_status)
        store.finalize_manifest()
        if progress_callback is not None:
            progress_callback(terminal_status)
        raise

    analysis = analyze_study_records(protocol, records, outcomes)
    matrix_complete = failed == 0 and completed == scheduled_runs
    if not matrix_complete:
        analysis = {**analysis, "architecture_classification": "inconclusive_incomplete_matrix"}
    analysis_core = {
        **analysis,
        "failed_graph_runs": failed,
        "expected_graph_runs": scheduled_runs,
        "maximum_graph_runs": maximum_runs,
        "stopped_early": stopped_early,
        "final_stage": current_stage,
        "matrix_complete": matrix_complete,
    }
    analysis_record = {**analysis_core, "analysis_id": stable_json_hash(analysis_core)}
    store.write_json("analysis.json", analysis_record)
    store.write_text("REPORT.md", _render_report(analysis_record))
    final_status = {
        "schema_version": "ta.ablation_status.v1",
        "study_id": study_id,
        "status": "complete" if matrix_complete else "partial",
        "completed_runs": completed,
        "total_runs": scheduled_runs,
        "scheduled_runs": scheduled_runs,
        "maximum_runs": maximum_runs,
        "cached_runs": cached,
        "failed_runs": failed,
        "independent_nodes": len(node_contracts),
        "stage": current_stage,
        "stopped_early": stopped_early,
        "architecture_classification": analysis_record["architecture_classification"],
        "analysis_id": analysis_record["analysis_id"],
    }
    store.write_json("status.json", final_status)
    manifest = store.finalize_manifest()
    return {
        **final_status,
        "manifest_id": manifest["manifest_id"],
        "artifact_root": str(store.root),
        "analysis": analysis_record,
    }
