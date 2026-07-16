"""Strict contracts for later Q / TA / Q+TA paired ablation experiments."""

from __future__ import annotations

import copy
from datetime import date
from enum import Enum
import json
import math
import re
from typing import Any

from .artifacts import stable_json_hash


class ExperimentArm(str, Enum):
    QUANT_ONLY = "quant_only"
    TA_BLIND = "ta_blind"
    QUANT_INFORMED = "quant_informed"


_SYMBOL_PATTERN = re.compile(r"^[A-Z0-9][A-Z0-9.^=_-]{0,31}$")
_SHA256_PATTERN = re.compile(r"^[0-9a-f]{64}$")
_MODEL_ARM_TOKENS = ("ta_blind", "quant_informed", "quant_only")
_REMOVED_CONTEXT_KEYS = {
    "agent_memory",
    "experiment",
    "model_calibration",
    "quant_trend_baseline",
    "research_experiment",
    "reflection",
    "trade_journal",
}
_FORBIDDEN_CONTEXT_KEYS = {
    "arm",
    "experiment_arm",
    "future_path",
    "future_prices",
    "future_returns",
    "labels",
    "other_arm_predictions",
    "outcome",
    "outcomes",
    "prediction",
    "predictions",
}


def validate_snapshot(
    snapshot: dict[str, Any],
    *,
    expected: dict[str, Any] | None = None,
) -> None:
    """Fail closed on malformed, modified, or request-mismatched snapshots."""

    required = {
        "schema_version",
        "status",
        "snapshot_id",
        "strategy_version",
        "config_hash",
        "data_source",
        "benchmark",
        "data_hash",
        "symbol",
        "requested_as_of_date",
        "as_of_date",
        "horizon_trading_days",
        "action",
        "trend_score",
    }
    if not isinstance(snapshot, dict) or required.difference(snapshot):
        raise ValueError("a complete quant.trend_snapshot.v1 snapshot is required")
    if snapshot.get("schema_version") != "quant.trend_snapshot.v1" or snapshot.get("status") != "ok":
        raise ValueError("only successful quant.trend_snapshot.v1 snapshots can enter an experiment")
    try:
        json.dumps(snapshot, ensure_ascii=False, allow_nan=False)
    except (TypeError, ValueError) as exc:
        raise ValueError("quant snapshot must contain canonical JSON values") from exc
    symbol = str(snapshot.get("symbol") or "")
    if not _SYMBOL_PATTERN.fullmatch(symbol):
        raise ValueError("quant snapshot symbol is invalid")
    for field in ("requested_as_of_date", "as_of_date"):
        value = snapshot.get(field)
        if not isinstance(value, str):
            raise ValueError(f"quant snapshot {field} must be an ISO date")
        try:
            date.fromisoformat(value)
        except ValueError as exc:
            raise ValueError(f"quant snapshot {field} must be an ISO date") from exc
    requested_date = date.fromisoformat(snapshot["requested_as_of_date"])
    actual_date = date.fromisoformat(snapshot["as_of_date"])
    if actual_date > requested_date:
        raise ValueError("quant snapshot as_of_date cannot exceed requested_as_of_date")
    horizon = snapshot.get("horizon_trading_days")
    if isinstance(horizon, bool) or not isinstance(horizon, int) or not 5 <= horizon <= 252:
        raise ValueError("quant snapshot horizon must be an integer between 5 and 252")
    for field in ("config_hash", "data_hash"):
        if not _SHA256_PATTERN.fullmatch(str(snapshot.get(field) or "")):
            raise ValueError(f"quant snapshot {field} must be a SHA-256 hash")
    if not str(snapshot.get("strategy_version") or "").strip():
        raise ValueError("quant snapshot strategy_version is required")
    if not str(snapshot.get("data_source") or "").strip():
        raise ValueError("quant snapshot data_source is required")
    if not _SYMBOL_PATTERN.fullmatch(str(snapshot.get("benchmark") or "")):
        raise ValueError("quant snapshot benchmark is invalid")
    expected_id = stable_json_hash({key: value for key, value in snapshot.items() if key != "snapshot_id"})
    if not _SHA256_PATTERN.fullmatch(str(snapshot.get("snapshot_id") or "")) or snapshot.get("snapshot_id") != expected_id:
        raise ValueError("quant snapshot content does not match snapshot_id")
    action = str(snapshot.get("action")).upper()
    if action not in {"BUY", "HOLD", "SELL"}:
        raise ValueError("quant snapshot action must be BUY, HOLD, or SELL")
    score = snapshot.get("trend_score")
    if isinstance(score, bool) or not isinstance(score, (int, float)) or not math.isfinite(float(score)):
        raise ValueError("quant snapshot trend_score must be finite")
    score = float(score)
    if not -1.0 <= score <= 1.0:
        raise ValueError("quant snapshot trend_score must be between -1 and 1")
    if (action == "BUY" and score <= 0) or (action == "SELL" and score >= 0) or (action == "HOLD" and score != 0):
        raise ValueError("quant snapshot action and trend_score disagree")
    for field, value in (expected or {}).items():
        if snapshot.get(field) != value:
            raise ValueError(f"cached quant snapshot does not match request field {field}")


def _validate_snapshot(snapshot: dict[str, Any]) -> None:
    """Compatibility wrapper for the internal contract helpers."""

    validate_snapshot(snapshot)


def _reject_nested_experiment_data(value: Any, *, path: str = "application_context") -> None:
    if isinstance(value, dict):
        for raw_key, child in value.items():
            key = str(raw_key).strip().lower()
            if key in _FORBIDDEN_CONTEXT_KEYS or key in _REMOVED_CONTEXT_KEYS:
                raise ValueError(f"model context contains reserved experiment field at {path}.{raw_key}")
            _reject_nested_experiment_data(child, path=f"{path}.{raw_key}")
    elif isinstance(value, (list, tuple)):
        for index, child in enumerate(value):
            _reject_nested_experiment_data(child, path=f"{path}[{index}]")


def _reject_arm_tokens(value: dict[str, Any]) -> None:
    serialized = json.dumps(value, ensure_ascii=False, sort_keys=True).lower()
    if any(token in serialized for token in _MODEL_ARM_TOKENS):
        raise ValueError("model-visible context must not reveal the experiment arm")


def build_experiment_context(
    arm: ExperimentArm | str,
    snapshot: dict[str, Any],
    application_context: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Return an isolated context without cross-arm information leakage.

    Both TradingAgents arms remove the existing model calibration so the only
    experimental difference is whether the frozen quant snapshot is visible.
    Quant-only must bypass the LLM runner entirely; its empty context makes that
    invariant explicit to future orchestrators.
    """

    selected = ExperimentArm(arm)
    _validate_snapshot(snapshot)
    if selected is ExperimentArm.QUANT_ONLY:
        return {}
    context = copy.deepcopy(application_context or {})
    if not isinstance(context, dict):
        raise ValueError("application_context must be an object")
    for key in tuple(context):
        if str(key).strip().lower() in _REMOVED_CONTEXT_KEYS:
            context.pop(key, None)
    _reject_nested_experiment_data(context)
    common_context_hash = stable_json_hash(context)
    context["research_experiment"] = {
        "schema_version": "quant.ablation_context.v1",
        "node_id": decision_node_id(snapshot),
        "common_context_hash": common_context_hash,
        "rules": [
            "This analysis uses a frozen point-in-time node and a separately stored outcome label.",
            "Do not infer or import predictions from another run.",
        ],
    }
    if selected is ExperimentArm.QUANT_INFORMED:
        context["quant_trend_baseline"] = copy.deepcopy(snapshot)
    _reject_arm_tokens(context)
    return context


def decision_node_id(snapshot: dict[str, Any]) -> str:
    """Identify the market decision independently of any strategy or vendor.

    A corrected data snapshot or a replacement quant strategy must still join
    to the same future outcome. Their exact inputs remain traceable through
    ``quant_snapshot_id`` rather than being smuggled into the node identity.
    """

    _validate_snapshot(snapshot)
    return stable_json_hash(
        {
            "schema_version": "quant.decision_node.v1",
            "symbol": snapshot.get("symbol"),
            "market": snapshot.get("market"),
            "as_of_date": snapshot.get("as_of_date"),
            "horizon_trading_days": snapshot.get("horizon_trading_days"),
            "entry_rule": "next_session_open",
        }
    )


def experiment_node_id(snapshot: dict[str, Any]) -> str:
    """Backward-compatible alias for the strategy-independent decision node."""

    return decision_node_id(snapshot)


def build_outcome_record(
    snapshot: dict[str, Any],
    *,
    label_protocol: dict[str, Any],
    future_path_hash: str,
    labels: dict[str, Any],
) -> dict[str, Any]:
    """Build a content-addressed future outcome shared by all experiment arms."""

    _validate_snapshot(snapshot)
    if not isinstance(label_protocol, dict) or not label_protocol:
        raise ValueError("label_protocol must be a non-empty object")
    if not isinstance(labels, dict) or not labels:
        raise ValueError("labels must be a non-empty object")
    try:
        json.dumps(label_protocol, ensure_ascii=False, allow_nan=False)
        json.dumps(labels, ensure_ascii=False, allow_nan=False)
    except (TypeError, ValueError) as exc:
        raise ValueError("label protocol and labels must contain canonical JSON values") from exc
    normalized_future_hash = str(future_path_hash or "").strip()
    if not _SHA256_PATTERN.fullmatch(normalized_future_hash):
        raise ValueError("future_path_hash must be a SHA-256 hash")
    protocol_horizon = label_protocol.get("horizon_trading_days")
    if (
        protocol_horizon is not None
        and int(protocol_horizon) != int(snapshot["horizon_trading_days"])
    ):
        raise ValueError("label protocol horizon must match the decision node")
    protocol_hash = stable_json_hash(label_protocol)
    core = {
        "schema_version": "quant.ablation_outcome.v1",
        "node_id": decision_node_id(snapshot),
        "label_protocol": copy.deepcopy(label_protocol),
        "label_protocol_hash": protocol_hash,
        "future_path_hash": normalized_future_hash,
        "labels": copy.deepcopy(labels),
    }
    return {**core, "outcome_id": stable_json_hash(core)}


def build_prediction_record(
    arm: ExperimentArm | str,
    snapshot: dict[str, Any],
    *,
    action: str,
    model_fingerprint: str | None = None,
    repeat_index: int = 0,
) -> dict[str, Any]:
    selected = ExperimentArm(arm)
    _validate_snapshot(snapshot)
    normalized = str(action or "").strip().upper()
    if normalized not in {"BUY", "HOLD", "SELL"}:
        raise ValueError("prediction action must be BUY, HOLD, or SELL")
    if isinstance(repeat_index, bool) or not isinstance(repeat_index, int) or not 0 <= repeat_index < 100:
        raise ValueError("repeat_index must be an integer between 0 and 99")
    if selected is ExperimentArm.QUANT_ONLY:
        if normalized != str(snapshot["action"]).upper():
            raise ValueError("quant_only action must equal the frozen quant snapshot")
        if model_fingerprint:
            raise ValueError("quant_only must not include a model fingerprint")
    elif not str(model_fingerprint or "").strip():
        raise ValueError("TradingAgents experiment arms require a model fingerprint")
    core = {
        "schema_version": "quant.ablation_prediction.v1",
        "node_id": decision_node_id(snapshot),
        "arm": selected.value,
        "symbol": snapshot.get("symbol"),
        "as_of_date": snapshot.get("as_of_date"),
        "horizon_trading_days": snapshot.get("horizon_trading_days"),
        "action": normalized,
        "quant_snapshot_id": (
            None if selected is ExperimentArm.TA_BLIND else snapshot.get("snapshot_id")
        ),
        "model_fingerprint": str(model_fingerprint).strip() if model_fingerprint else None,
        "repeat_index": int(repeat_index),
    }
    return {**core, "prediction_id": stable_json_hash(core)}


def build_experiment_manifest(
    snapshot: dict[str, Any],
    *,
    common_context_hash: str,
    model_protocol_id: str,
    repeat_count: int,
    outcome_id: str | None = None,
) -> dict[str, Any]:
    """Freeze the join keys and repeated-run protocol for a paired study."""

    _validate_snapshot(snapshot)
    if not _SHA256_PATTERN.fullmatch(str(common_context_hash or "")):
        raise ValueError("common_context_hash must be a SHA-256 hash")
    if not str(model_protocol_id or "").strip():
        raise ValueError("model_protocol_id is required")
    if isinstance(repeat_count, bool) or not isinstance(repeat_count, int) or not 1 <= repeat_count <= 100:
        raise ValueError("repeat_count must be an integer between 1 and 100")
    if outcome_id is not None and not _SHA256_PATTERN.fullmatch(str(outcome_id)):
        raise ValueError("outcome_id must be a SHA-256 hash")
    core = {
        "schema_version": "quant.ablation_manifest.v1",
        "node_id": decision_node_id(snapshot),
        "quant_snapshot_id": snapshot["snapshot_id"],
        "arms": [arm.value for arm in ExperimentArm],
        "common_context_hash": common_context_hash,
        "model_protocol_id": str(model_protocol_id).strip(),
        "repeat_count": repeat_count,
        "outcome_id": outcome_id,
    }
    return {**core, "manifest_id": stable_json_hash(core)}


def compare_predictions(
    snapshot: dict[str, Any],
    *,
    agent_action: str,
    arm: ExperimentArm | str,
) -> dict[str, Any]:
    _validate_snapshot(snapshot)
    quant_action = str(snapshot.get("action") or "").upper()
    agent = str(agent_action or "").strip().upper()
    if quant_action not in {"BUY", "HOLD", "SELL"} or agent not in {"BUY", "HOLD", "SELL"}:
        raise ValueError("both quant and agent actions must be BUY, HOLD, or SELL")
    selected = ExperimentArm(arm)
    if selected is ExperimentArm.QUANT_ONLY:
        raise ValueError("comparison requires ta_blind or quant_informed")
    core = {
        "schema_version": "quant.ablation_comparison.v1",
        "node_id": decision_node_id(snapshot),
        "arm": selected.value,
        "quant_action": quant_action,
        "agent_action": agent,
        "agent_overrode_quant": agent != quant_action,
        "transition": f"{quant_action}_TO_{agent}",
        "quant_snapshot_id": snapshot.get("snapshot_id"),
    }
    return {**core, "comparison_id": stable_json_hash(core)}
