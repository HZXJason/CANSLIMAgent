"""Independent integrity and completeness verification for study evidence."""

from __future__ import annotations

import copy
import hashlib
import json
from pathlib import Path
import re
from typing import Any

from quant_trend.artifacts import stable_json_hash
from quant_trend.experiments import decision_node_id

from .artifacts import SECRET_KEYS
from .design import stage_one_stopping_decision, staged_task_keys
from .runner import parse_explicit_action
from .statistics import analyze_study_records


ISO_DATE = re.compile(r"\b(20\d{2}-\d{2}-\d{2})\b")
ALLOWED_PARSER_MIGRATION_SOURCE_DIFFERENCES = (
    "sidecar/ablation_study/runner.py",
    "sidecar/ablation_study/verifier.py",
)
PARSER_MIGRATION_ELIGIBILITY = (
    "complete_first_attempt_same_treatment_reparsed_v3"
)


def _sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _load(path: Path) -> dict[str, Any]:
    payload = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(payload, dict):
        raise ValueError(f"JSON artifact is not an object: {path}")
    return payload


def _protocol_treatment_core(protocol: dict[str, Any]) -> dict[str, Any]:
    core = copy.deepcopy(protocol)
    core.pop("study_id", None)
    core.pop("source", None)
    graph = core.get("graph")
    if isinstance(graph, dict):
        graph.pop("parser_version", None)
    return core


def _source_differences(
    source_protocol: dict[str, Any], target_protocol: dict[str, Any]
) -> list[str]:
    source = {
        str(item.get("path")): item.get("sha256")
        for item in (source_protocol.get("source") or {}).get("files") or []
    }
    target = {
        str(item.get("path")): item.get("sha256")
        for item in (target_protocol.get("source") or {}).get("files") or []
    }
    return sorted(
        path
        for path in set(source) | set(target)
        if source.get(path) != target.get(path)
    )


def _find_secret_paths(value: Any, path: str = "root") -> list[str]:
    found: list[str] = []
    if isinstance(value, dict):
        for key, child in value.items():
            normalized = str(key).strip().lower()
            if normalized in SECRET_KEYS or normalized.endswith("_api_key"):
                found.append(f"{path}.{key}")
            found.extend(_find_secret_paths(child, f"{path}.{key}"))
    elif isinstance(value, list):
        for index, child in enumerate(value):
            found.extend(_find_secret_paths(child, f"{path}[{index}]"))
    return found


def verify_manifest(root: Path) -> list[str]:
    errors: list[str] = []
    manifest_path = root / "manifest.json"
    if not manifest_path.exists():
        return ["manifest.json is missing"]
    manifest = _load(manifest_path)
    expected_core = {key: value for key, value in manifest.items() if key != "manifest_id"}
    if manifest.get("manifest_id") != stable_json_hash(expected_core):
        errors.append("manifest_id does not match manifest content")
    listed = {str(item.get("path")): item for item in manifest.get("files") or []}
    actual_paths = {
        str(path.relative_to(root))
        for path in root.rglob("*")
        if path.is_file() and path.name != "manifest.json" and not path.name.startswith(".")
    }
    if set(listed) != actual_paths:
        errors.append(
            f"manifest file set differs: missing={sorted(actual_paths - set(listed))}, extra={sorted(set(listed) - actual_paths)}"
        )
    for relative, item in listed.items():
        path = root / relative
        if not path.is_file():
            continue
        expected_bytes = item.get("bytes")
        if expected_bytes is None or int(expected_bytes) != path.stat().st_size:
            errors.append(f"size mismatch: {relative}")
        if item.get("sha256") != _sha256_file(path):
            errors.append(f"sha256 mismatch: {relative}")
    return errors


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


def _verify_parser_migration(
    study_root: Path,
    protocol: dict[str, Any],
    records_by_path: dict[str, dict[str, Any]],
    errors: list[str],
) -> int:
    index_path = study_root / "migration-index.json"
    migrated_records = {
        path: record
        for path, record in records_by_path.items()
        if record.get("migration") is not None
    }
    if not index_path.exists():
        if migrated_records:
            errors.append("migrated records exist without migration-index.json")
        return 0

    index = _load(index_path)
    index_core = {key: value for key, value in index.items() if key != "migration_id"}
    if index.get("migration_id") != stable_json_hash(index_core):
        errors.append("migration_id does not match migration index content")
    if index.get("schema_version") != "ta.ablation_migration_index.v1":
        errors.append("unsupported parser migration index schema")

    source_id = str(index.get("source_study_id") or "")
    target_id = str(protocol.get("study_id") or "")
    if index.get("target_study_id") != target_id:
        errors.append("migration target study reference mismatch")
    if (
        index.get("source_parser_version") != "explicit-decision.v2"
        or index.get("target_parser_version") != "explicit-decision.v3"
        or (protocol.get("graph") or {}).get("parser_version")
        != "explicit-decision.v3"
    ):
        errors.append("migration parser versions are not the preregistered v2-to-v3 pair")

    source_protocol_relative = Path("import-lineage") / source_id / "protocol.json"
    source_protocol_path = study_root / source_protocol_relative
    source_protocol: dict[str, Any] | None = None
    if not source_id or not source_protocol_path.is_file():
        errors.append("migration source protocol artifact is missing")
    else:
        source_protocol = _load(source_protocol_path)
        source_protocol_core = {
            key: value for key, value in source_protocol.items() if key != "study_id"
        }
        if source_protocol.get("study_id") != source_id:
            errors.append("migration source protocol identity mismatch")
        if source_id != stable_json_hash(source_protocol_core):
            errors.append("migration source protocol hash mismatch")
        if (source_protocol.get("graph") or {}).get("parser_version") != "explicit-decision.v2":
            errors.append("migration source protocol is not parser v2")
        if _protocol_treatment_core(source_protocol) != _protocol_treatment_core(protocol):
            errors.append("migrated source and target treatments are not identical")
        actual_differences = _source_differences(source_protocol, protocol)
        expected_differences = list(ALLOWED_PARSER_MIGRATION_SOURCE_DIFFERENCES)
        if actual_differences != expected_differences:
            errors.append(
                f"migration has non-parser source differences: {actual_differences}"
            )
        if index.get("actual_source_differences") != actual_differences:
            errors.append("migration index actual source differences mismatch")
        if index.get("allowed_source_differences") != expected_differences:
            errors.append("migration index allowed source differences mismatch")

    entries = index.get("records") or []
    if not isinstance(entries, list):
        errors.append("migration index records is not a list")
        entries = []
    if int(index.get("imported_count") or 0) != len(entries):
        errors.append("migration imported_count differs from index records")

    indexed_targets: set[str] = set()
    indexed_sources: set[str] = set()
    for entry in entries:
        if not isinstance(entry, dict):
            errors.append("migration index contains a non-object record entry")
            continue
        target_relative = str(entry.get("target_path") or "")
        source_relative = str(entry.get("source_path") or "")
        if target_relative in indexed_targets:
            errors.append(f"duplicate migration target entry: {target_relative}")
            continue
        if source_relative in indexed_sources:
            errors.append(f"duplicate migration source entry: {source_relative}")
            continue
        indexed_targets.add(target_relative)
        indexed_sources.add(source_relative)

        target_record = records_by_path.get(target_relative)
        if target_record is None:
            errors.append(f"indexed migrated target record is missing: {target_relative}")
            continue
        expected_source_prefix = f"import-lineage/{source_id}/runs/"
        if not source_relative.startswith(expected_source_prefix):
            errors.append(f"migration source path escapes frozen lineage: {source_relative}")
            continue
        source_path = study_root / source_relative
        if not source_path.is_file():
            errors.append(f"indexed migration source record is missing: {source_relative}")
            continue
        source_record = _load(source_path)
        source_core = {
            key: value for key, value in source_record.items() if key != "record_id"
        }
        if source_record.get("record_id") != stable_json_hash(source_core):
            errors.append(f"migration source record hash mismatch: {source_relative}")
        if entry.get("source_record_id") != source_record.get("record_id"):
            errors.append(f"migration source record reference mismatch: {source_relative}")
        if source_record.get("status") != "complete" or int(source_record.get("attempts") or 0) != 1:
            errors.append(f"migration source is not a first-attempt success: {source_relative}")

        single_action = parse_explicit_action(
            (source_record.get("state") or {}).get("market_report")
        )
        multi_action = parse_explicit_action(
            (source_record.get("state") or {}).get("final_trade_decision")
        ) or parse_explicit_action(source_record.get("fallback_decision"))
        if single_action is None or multi_action is None:
            errors.append(f"migration source cannot be explicitly reparsed: {source_relative}")
            continue

        node_id = str(source_record.get("node_id") or "")
        context = str(source_record.get("context") or "")
        repeat = int(source_record.get("repeat") or 0)
        expected_migration = {
            "schema_version": "ta.ablation_record_migration.v1",
            "source_study_id": source_id,
            "source_record_id": source_record.get("record_id"),
            "source_record_artifact": source_relative,
            "eligibility": PARSER_MIGRATION_ELIGIBILITY,
        }
        expected_target_core = copy.deepcopy(source_core)
        expected_target_core.update(
            {
                "study_id": target_id,
                "run_key": _record_key(target_id, context, node_id, repeat),
                "parser_version": "explicit-decision.v3",
                "single_action": single_action,
                "multi_action": multi_action,
                "migration": expected_migration,
            }
        )
        actual_target_core = {
            key: value for key, value in target_record.items() if key != "record_id"
        }
        if actual_target_core != expected_target_core:
            errors.append(f"migrated record differs from deterministic reparse: {target_relative}")
        expected_target_id = stable_json_hash(expected_target_core)
        if target_record.get("record_id") != expected_target_id:
            errors.append(f"migrated target record hash mismatch: {target_relative}")
        if entry.get("target_record_id") != expected_target_id:
            errors.append(f"migration target record reference mismatch: {target_relative}")

    migrated_paths = set(migrated_records)
    if indexed_targets != migrated_paths:
        errors.append(
            "migration index and migrated target set differ: "
            f"missing={sorted(migrated_paths - indexed_targets)}, "
            f"extra={sorted(indexed_targets - migrated_paths)}"
        )
    return len(migrated_paths)


def _dates_after(value: Any, cutoff: str) -> list[str]:
    dates = ISO_DATE.findall(json.dumps(value, ensure_ascii=False, default=str))
    return sorted({date for date in dates if date > cutoff})


def _staged_expected_matrix(
    protocol: dict[str, Any],
    records: list[dict[str, Any]],
    outcomes: dict[str, dict[str, Any]],
    study_root: Path,
    errors: list[str],
    warnings: list[str],
) -> set[tuple[str, str, int]]:
    design = protocol["design"]
    contexts = tuple(design["contexts"])
    nodes = protocol.get("nodes") or []
    if not design.get("adaptive_stages"):
        return {
            (str(node["node_id"]), context, repeat)
            for node in nodes
            for context in contexts
            for repeat in range(1, int(design["repeats_per_context"]) + 1)
        }

    staged = design.get("staged_design")
    if not isinstance(staged, dict):
        errors.append("adaptive protocol is missing staged_design")
        return set()
    staged_core = {key: value for key, value in staged.items() if key != "design_id"}
    if staged.get("design_id") != stable_json_hash(staged_core):
        errors.append("embedded staged design_id mismatch")
    if int(staged.get("full_independent_nodes") or -1) != len(nodes):
        errors.append("embedded staged design node count mismatch")
    if staged.get("evaluation_run_id") != protocol.get("quant", {}).get("evaluation_run_id"):
        errors.append("embedded staged design evaluation reference mismatch")
    embedded_contracts = staged.get("node_contracts") or []
    if len(embedded_contracts) != len(nodes):
        errors.append("embedded staged node contracts are incomplete")
    else:
        for node, embedded in zip(nodes, embedded_contracts):
            for key in ("as_of_date", "stage", "reproducibility_repeat", "position"):
                if node.get(key) != embedded.get(key):
                    errors.append(f"embedded staged contract mismatch: {node.get('node_id')} {key}")

    try:
        task_matrix = staged_task_keys(nodes, contexts)
    except ValueError as exc:
        errors.append(str(exc))
        return set()
    stage_one = set(task_matrix["stage_one"])
    stage_two = set(task_matrix["stage_two"])
    complete_records = {
        (str(record.get("node_id")), str(record.get("context")), int(record.get("repeat") or 0)): record
        for record in records
        if record.get("status") == "complete"
    }
    stage_one_complete = stage_one.issubset(complete_records)
    if not stage_one_complete:
        warnings.append("staged study has not completed the stage-one matrix")
        return stage_one

    stage_one_records = [complete_records[key] for key in sorted(stage_one)]
    recomputed_interim = analyze_study_records(protocol, stage_one_records, outcomes)
    stopping = stage_one_stopping_decision(
        recomputed_interim["architecture_classification"], matrix_complete=True
    )
    expected_advance = bool(stopping["advance_to_stage_two"])
    interim_path = study_root / "interim" / "stage-1-analysis.json"
    decision_path = study_root / "interim" / "stage-1-decision.json"
    if not interim_path.exists() or not decision_path.exists():
        errors.append("completed stage one is missing frozen interim evidence")
        return stage_one | stage_two if expected_advance else stage_one

    stored_interim = _load(interim_path)
    for key in (
        "architecture_classification",
        "effects",
        "arm_performance",
        "action_distributions",
        "mean_change_rates",
        "node_level_estimands",
    ):
        if stored_interim.get(key) != recomputed_interim.get(key):
            errors.append(f"stage-one analysis recomputation mismatch: {key}")
    interim_core = {key: value for key, value in stored_interim.items() if key != "analysis_id"}
    if stored_interim.get("analysis_id") != stable_json_hash(interim_core):
        errors.append("stage-one analysis_id mismatch")

    decision = _load(decision_path)
    decision_core = {key: value for key, value in decision.items() if key != "decision_id"}
    if decision.get("decision_id") != stable_json_hash(decision_core):
        errors.append("stage-one decision_id mismatch")
    if decision.get("interim_analysis_id") != stored_interim.get("analysis_id"):
        errors.append("stage-one decision analysis reference mismatch")
    if bool(decision.get("advance_to_stage_two")) != expected_advance:
        errors.append("stage-one stopping decision differs from corrected interval rule")
    if decision.get("reason") != stopping["reason"]:
        errors.append("stage-one stopping reason mismatch")
    if decision.get("classification") != recomputed_interim["architecture_classification"]:
        errors.append("stage-one stopping classification mismatch")
    return stage_one | stage_two if expected_advance else stage_one


def verify_study(root: str | Path) -> dict[str, Any]:
    study_root = Path(root).resolve()
    errors = verify_manifest(study_root)
    warnings: list[str] = []
    protocol = _load(study_root / "protocol.json")
    study_id = str(protocol.get("study_id") or "")
    if study_id != study_root.name:
        errors.append("study directory name does not match protocol study_id")
    if study_id != stable_json_hash({key: value for key, value in protocol.items() if key != "study_id"}):
        errors.append("protocol study_id does not match protocol content")
    secret_paths = _find_secret_paths(protocol)
    if secret_paths:
        errors.append(f"protocol contains secret-like fields: {secret_paths}")

    requirements = protocol.get("evidence_requirements") or {}
    model_data = protocol.get("model_data") or {}
    model_data_id = str(model_data.get("model_data_id") or "")
    if requirements.get("protocol_frozen_ohlcv_required"):
        model_data_path = study_root / "model-data" / "ohlcv.json"
        if not model_data_path.exists():
            errors.append("protocol-frozen OHLCV artifact is missing")
        else:
            artifact = _load(model_data_path)
            price_rows = artifact.get("price_rows")
            artifact_metadata = {
                key: value for key, value in artifact.items() if key != "price_rows"
            }
            if artifact_metadata != model_data:
                errors.append("protocol-frozen OHLCV metadata mismatch")
            if not isinstance(price_rows, list) or not price_rows:
                errors.append("protocol-frozen OHLCV rows are missing")
            else:
                if model_data.get("content_hash") != stable_json_hash(price_rows):
                    errors.append("protocol-frozen OHLCV content hash mismatch")
                if int(model_data.get("rows") or -1) != len(price_rows):
                    errors.append("protocol-frozen OHLCV row count mismatch")
                if (
                    price_rows[0].get("date") != model_data.get("first_date")
                    or price_rows[-1].get("date") != model_data.get("last_date")
                ):
                    errors.append("protocol-frozen OHLCV date bounds mismatch")
            model_data_core = {
                key: value for key, value in model_data.items() if key != "model_data_id"
            }
            if not model_data_id or model_data_id != stable_json_hash(model_data_core):
                errors.append("protocol-frozen OHLCV model_data_id mismatch")

    nodes = {str(node["node_id"]): node for node in protocol.get("nodes") or []}
    snapshots: dict[str, dict[str, Any]] = {}
    outcomes: dict[str, dict[str, Any]] = {}
    for node_id, contract in nodes.items():
        snapshot_path = study_root / "snapshots" / f"{node_id}.json"
        outcome_path = study_root / "outcomes" / f"{node_id}.json"
        if not snapshot_path.exists() or not outcome_path.exists():
            errors.append(f"node evidence missing: {node_id}")
            continue
        snapshot = _load(snapshot_path)
        outcome = _load(outcome_path)
        snapshots[node_id] = snapshot
        outcomes[node_id] = outcome
        snapshot_core = {key: value for key, value in snapshot.items() if key != "snapshot_id"}
        if snapshot.get("snapshot_id") != stable_json_hash(snapshot_core):
            errors.append(f"snapshot hash mismatch: {node_id}")
        try:
            resolved_node_id = decision_node_id(snapshot)
        except ValueError as exc:
            errors.append(f"invalid snapshot {node_id}: {exc}")
        else:
            if resolved_node_id != node_id:
                errors.append(f"snapshot node identity mismatch: {node_id}")
        if contract.get("quant_snapshot_id") != snapshot.get("snapshot_id"):
            errors.append(f"protocol snapshot reference mismatch: {node_id}")
        outcome_core = {key: value for key, value in outcome.items() if key != "outcome_id"}
        if outcome.get("outcome_id") != stable_json_hash(outcome_core):
            errors.append(f"outcome hash mismatch: {node_id}")
        daily_path = outcome.get("daily_future_path")
        if isinstance(daily_path, list):
            if outcome.get("future_path_hash") != stable_json_hash(daily_path):
                errors.append(f"future path hash mismatch: {node_id}")
            if len(daily_path) != int(protocol["design"]["horizon_trading_days"]):
                errors.append(f"future path length mismatch: {node_id}")
            if daily_path and (
                daily_path[0].get("date") != outcome.get("entry_date")
                or daily_path[-1].get("date") != outcome.get("outcome_date")
            ):
                errors.append(f"future path endpoint mismatch: {node_id}")
        else:
            warnings.append(f"daily future path is absent for legacy study node {node_id}")

    records: list[dict[str, Any]] = []
    records_by_path: dict[str, dict[str, Any]] = {}
    completed_keys: set[tuple[str, str, int]] = set()
    tool_evidence_runs = 0
    for path in sorted((study_root / "runs").glob("*/*/*.json")):
        record = _load(path)
        records.append(record)
        records_by_path[str(path.relative_to(study_root))] = record
        record_core = {key: value for key, value in record.items() if key != "record_id"}
        if record.get("record_id") != stable_json_hash(record_core):
            errors.append(f"record hash mismatch: {path.relative_to(study_root)}")
        node_id = str(record.get("node_id") or "")
        context = str(record.get("context") or "")
        repeat = int(record.get("repeat") or 0)
        if node_id not in nodes or context not in {"blind", "quant_informed"} or repeat < 1:
            errors.append(f"invalid record identity: {path.relative_to(study_root)}")
            continue
        expected_key = _record_key(study_id, context, node_id, repeat)
        if record.get("run_key") != expected_key:
            errors.append(f"run key mismatch: {path.relative_to(study_root)}")
        if _find_secret_paths(record):
            errors.append(f"secret-like field in record: {path.relative_to(study_root)}")
        visible = record.get("model_visible_context") or {}
        serialized_visible = json.dumps(visible, ensure_ascii=False).lower()
        for forbidden in ("daily_future_path", "endpoint_total_return", "outcome_id", "future_path_hash"):
            if forbidden in serialized_visible:
                errors.append(f"future outcome leaked into model context: {path.relative_to(study_root)} {forbidden}")
        for forbidden in ("past_context", "trade_journal", "reflection", "alpha_return", "raw_return"):
            if forbidden in serialized_visible:
                errors.append(f"post-outcome memory leaked into model context: {path.relative_to(study_root)} {forbidden}")
        if context == "blind" and "quant_trend_baseline" in visible:
            errors.append(f"quant snapshot leaked into blind context: {path.relative_to(study_root)}")
        if context == "quant_informed":
            baseline = visible.get("quant_trend_baseline")
            if not isinstance(baseline, dict) or baseline.get("snapshot_id") != snapshots.get(node_id, {}).get("snapshot_id"):
                errors.append(f"informed context snapshot mismatch: {path.relative_to(study_root)}")
        cutoff = str(record.get("as_of_date") or "")
        if protocol.get("evidence_requirements", {}).get("global_memory_disabled"):
            isolation = (record.get("request_config") or {}).get("historical_isolation") or {}
            if isolation.get("global_memory") != "disabled_by_calibration_context":
                errors.append(f"historical memory isolation is not attested: {path.relative_to(study_root)}")
            expected_namespace = (record.get("request_config") or {}).get("run_id")
            if isolation.get("checkpoint_namespace") != expected_namespace:
                errors.append(f"checkpoint namespace is not run-scoped: {path.relative_to(study_root)}")
            if isolation.get("future_outcome_visible") is not False:
                errors.append(f"future outcome visibility is not disabled: {path.relative_to(study_root)}")
        if requirements.get("frozen_ohlcv_usage_attestation_required"):
            if isolation.get("market_data_mode") != "protocol_frozen_canonical_ohlcv":
                errors.append(f"frozen market-data mode is not attested: {path.relative_to(study_root)}")
            if isolation.get("model_data_id") != model_data_id:
                errors.append(f"frozen model-data reference mismatch: {path.relative_to(study_root)}")
            frozen_usage = record.get("frozen_ohlcv_usage")
            if record.get("status") == "complete" and not frozen_usage:
                errors.append(f"frozen OHLCV usage evidence is missing: {path.relative_to(study_root)}")
            for item in frozen_usage or []:
                if item.get("model_data_id") != model_data_id:
                    errors.append(f"frozen OHLCV usage reference mismatch: {path.relative_to(study_root)}")
                returned_last = str(item.get("returned_last_date") or "")
                if not returned_last or returned_last > cutoff:
                    errors.append(f"frozen OHLCV usage exceeds cutoff: {path.relative_to(study_root)}")
                if int(item.get("returned_rows") or 0) < 1:
                    errors.append(f"frozen OHLCV usage has no rows: {path.relative_to(study_root)}")
        calibration = visible.get("calibration") if isinstance(visible, dict) else None
        if not isinstance(calibration, dict) or calibration.get("as_of_date") != cutoff:
            errors.append(f"model cutoff mismatch: {path.relative_to(study_root)}")
        tool_calls = ((record.get("usage") or {}).get("tool_calls") or [])
        if tool_calls:
            tool_evidence_runs += 1
            for call in tool_calls:
                later_inputs = _dates_after(call.get("input"), cutoff)
                later_outputs = _dates_after(call.get("output"), cutoff)
                if later_inputs:
                    errors.append(f"future tool input dates {later_inputs}: {path.relative_to(study_root)}")
                if later_outputs:
                    errors.append(f"future tool output dates {later_outputs}: {path.relative_to(study_root)}")
        elif record.get("status") == "complete":
            if protocol.get("evidence_requirements", {}).get("tool_call_trace_required"):
                errors.append(f"required tool-call evidence is missing: {path.relative_to(study_root)}")
            else:
                warnings.append(f"successful legacy run lacks tool-call evidence: {path.relative_to(study_root)}")
        if record.get("status") == "complete":
            if record.get("single_action") not in {"BUY", "HOLD", "SELL"} or record.get("multi_action") not in {"BUY", "HOLD", "SELL"}:
                errors.append(f"completed run has invalid action: {path.relative_to(study_root)}")
            completed_keys.add((node_id, context, repeat))

    migrated_runs = _verify_parser_migration(
        study_root, protocol, records_by_path, errors
    )
    expected = _staged_expected_matrix(protocol, records, outcomes, study_root, errors, warnings)
    matrix_complete = expected == completed_keys
    missing_completed = sorted(expected - completed_keys)
    unexpected_completed = sorted(completed_keys - expected)
    if not matrix_complete:
        warnings.append(
            "confirmatory matrix differs from the frozen stopping rule: "
            f"{len(missing_completed)} successful runs missing, "
            f"{len(unexpected_completed)} unexpected successful runs"
        )

    analysis_path = study_root / "analysis.json"
    analysis_verified = False
    if analysis_path.exists() and matrix_complete and not errors:
        stored_analysis = _load(analysis_path)
        recomputed = analyze_study_records(protocol, records, outcomes)
        for key in (
            "architecture_classification",
            "effects",
            "arm_performance",
            "action_distributions",
            "mean_change_rates",
            "node_level_estimands",
        ):
            if stored_analysis.get(key) != recomputed.get(key):
                errors.append(f"analysis recomputation mismatch: {key}")
        analysis_core = {key: value for key, value in stored_analysis.items() if key != "analysis_id"}
        if stored_analysis.get("analysis_id") != stable_json_hash(analysis_core):
            errors.append("analysis_id does not match analysis content")
        analysis_verified = not errors

    verification_core = {
        "schema_version": "ta.ablation_verification.v1",
        "verifier_version": "independent-evidence-verifier.v3",
        "study_id": study_id,
        "integrity_verified": not errors,
        "matrix_complete": matrix_complete,
        "analysis_verified": analysis_verified,
        "protocol_nodes": len(nodes),
        "run_records": len(records),
        "successful_runs": len(completed_keys),
        "expected_successful_runs": len(expected),
        "tool_evidence_runs": tool_evidence_runs,
        "migrated_runs": migrated_runs,
        "errors": errors,
        "warnings": warnings,
    }
    return {**verification_core, "verification_id": stable_json_hash(verification_core)}


def verify_design(root: str | Path) -> dict[str, Any]:
    design_root = Path(root).resolve()
    errors = verify_manifest(design_root)
    design = _load(design_root / "design.json")
    evaluation = _load(design_root / "quant_evaluation.json")
    design_id = str(design.get("design_id") or "")
    if design_id != design_root.name:
        errors.append("design directory name mismatch")
    if design_id != stable_json_hash({key: value for key, value in design.items() if key != "design_id"}):
        errors.append("design_id mismatch")
    evaluation_core = {key: value for key, value in evaluation.items() if key != "run_id"}
    if evaluation.get("run_id") != stable_json_hash(evaluation_core):
        errors.append("evaluation run_id mismatch")
    if design.get("evaluation_run_id") != evaluation.get("run_id"):
        errors.append("design evaluation reference mismatch")
    dates = [node["as_of_date"] for node in evaluation.get("nodes") or []]
    contract_dates = [node["as_of_date"] for node in design.get("node_contracts") or []]
    if dates != contract_dates:
        errors.append("design node dates differ from evaluation")
    verification_core = {
        "schema_version": "ta.ablation_design_verification.v1",
        "verifier_version": "independent-evidence-verifier.v1",
        "design_id": design_id,
        "integrity_verified": not errors,
        "errors": errors,
    }
    return {**verification_core, "verification_id": stable_json_hash(verification_core)}
