"""Losslessly reparse eligible first-attempt records under a parser-only protocol update."""

from __future__ import annotations

import argparse
import copy
import json
from pathlib import Path
from typing import Any

from ablation_study.artifacts import StudyArtifactStore
from ablation_study.runner import parse_explicit_action
from quant_trend.artifacts import stable_json_hash


ALLOWED_SOURCE_DIFFERENCES = (
    "sidecar/ablation_study/runner.py",
    "sidecar/ablation_study/verifier.py",
)


def _load(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"expected JSON object: {path}")
    return value


def protocol_treatment_core(protocol: dict[str, Any]) -> dict[str, Any]:
    core = copy.deepcopy(protocol)
    core.pop("study_id", None)
    core.pop("source", None)
    (core.get("graph") or {}).pop("parser_version", None)
    return core


def source_differences(
    source_protocol: dict[str, Any], target_protocol: dict[str, Any]
) -> list[str]:
    source = {
        item["path"]: item["sha256"]
        for item in (source_protocol.get("source") or {}).get("files") or []
    }
    target = {
        item["path"]: item["sha256"]
        for item in (target_protocol.get("source") or {}).get("files") or []
    }
    return sorted(
        path
        for path in set(source) | set(target)
        if source.get(path) != target.get(path)
    )


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


def migrate(source_root: Path, target_root: Path) -> dict[str, Any]:
    source_protocol = _load(source_root / "protocol.json")
    target_protocol = _load(target_root / "protocol.json")
    source_id = str(source_protocol.get("study_id") or "")
    target_id = str(target_protocol.get("study_id") or "")
    if source_root.name != source_id or target_root.name != target_id:
        raise ValueError("study directory and protocol identity differ")
    if source_protocol.get("graph", {}).get("parser_version") != "explicit-decision.v2":
        raise ValueError("source study is not parser v2")
    if target_protocol.get("graph", {}).get("parser_version") != "explicit-decision.v3":
        raise ValueError("target study is not parser v3")
    if protocol_treatment_core(source_protocol) != protocol_treatment_core(target_protocol):
        raise ValueError("source and target treatments differ beyond parser/source identity")
    differences = source_differences(source_protocol, target_protocol)
    if differences != list(ALLOWED_SOURCE_DIFFERENCES):
        raise ValueError(f"unexpected source differences: {differences}")

    target_store = StudyArtifactStore(target_root.parent, target_id)
    lineage_root = Path("import-lineage") / source_id
    target_store.write_immutable_json(lineage_root / "protocol.json", source_protocol)
    target_nodes = {
        str(item["node_id"]): item for item in target_protocol.get("nodes") or []
    }
    expected_model_data_id = target_protocol.get("model_data", {}).get("model_data_id")
    imported: list[dict[str, Any]] = []
    excluded_counts: dict[str, int] = {}

    def excluded(reason: str) -> None:
        excluded_counts[reason] = excluded_counts.get(reason, 0) + 1

    for source_path in sorted((source_root / "runs").glob("*/*/*.json")):
        source_record = _load(source_path)
        if source_record.get("status") != "complete":
            excluded("not_complete")
            continue
        if int(source_record.get("attempts") or 0) != 1:
            excluded("not_first_attempt")
            continue
        source_core = {
            key: value for key, value in source_record.items() if key != "record_id"
        }
        if source_record.get("record_id") != stable_json_hash(source_core):
            raise ValueError(f"source record hash mismatch: {source_path}")
        node_id = str(source_record.get("node_id") or "")
        context = str(source_record.get("context") or "")
        repeat = int(source_record.get("repeat") or 0)
        if node_id not in target_nodes or context not in {"blind", "quant_informed"}:
            excluded("outside_target_matrix")
            continue
        single = parse_explicit_action((source_record.get("state") or {}).get("market_report"))
        multi = parse_explicit_action(
            (source_record.get("state") or {}).get("final_trade_decision")
        ) or parse_explicit_action(source_record.get("fallback_decision"))
        if single is None or multi is None:
            excluded("not_explicit_under_v3")
            continue
        frozen_usage = source_record.get("frozen_ohlcv_usage") or []
        if not frozen_usage or any(
            item.get("model_data_id") != expected_model_data_id
            or str(item.get("returned_last_date") or "")
            > str(source_record.get("as_of_date") or "")
            for item in frozen_usage
        ):
            excluded("invalid_frozen_data_attestation")
            continue
        if not (source_record.get("usage") or {}).get("tool_calls"):
            excluded("missing_tool_trace")
            continue

        source_relative = source_path.relative_to(source_root)
        lineage_record_path = lineage_root / source_relative
        target_store.write_immutable_json(lineage_record_path, source_record)
        target_relative = Path("runs") / context / node_id / f"repeat-{repeat}.json"
        run_key = _record_key(target_id, context, node_id, repeat)
        migrated_core = copy.deepcopy(source_core)
        migrated_core.update(
            {
                "study_id": target_id,
                "run_key": run_key,
                "parser_version": "explicit-decision.v3",
                "single_action": single,
                "multi_action": multi,
                "migration": {
                    "schema_version": "ta.ablation_record_migration.v1",
                    "source_study_id": source_id,
                    "source_record_id": source_record["record_id"],
                    "source_record_artifact": str(lineage_record_path),
                    "eligibility": "complete_first_attempt_same_treatment_reparsed_v3",
                },
            }
        )
        migrated = {**migrated_core, "record_id": stable_json_hash(migrated_core)}
        target_store.write_immutable_json(target_relative, migrated)
        imported.append(
            {
                "source_record_id": source_record["record_id"],
                "target_record_id": migrated["record_id"],
                "source_path": str(lineage_record_path),
                "target_path": str(target_relative),
            }
        )

    index_core = {
        "schema_version": "ta.ablation_migration_index.v1",
        "source_study_id": source_id,
        "target_study_id": target_id,
        "source_parser_version": "explicit-decision.v2",
        "target_parser_version": "explicit-decision.v3",
        "eligibility_rules": [
            "source status is complete",
            "attempts equals one",
            "source record hash is valid",
            "treatment protocol is identical after removing parser and source identity",
            "only runner.py and verifier.py source hashes may differ",
            "both actions are explicit under parser v3",
            "frozen OHLCV attestation and tool trace are present and cutoff-safe",
        ],
        "allowed_source_differences": list(ALLOWED_SOURCE_DIFFERENCES),
        "actual_source_differences": differences,
        "imported_count": len(imported),
        "excluded_counts": excluded_counts,
        "records": imported,
    }
    index = {**index_core, "migration_id": stable_json_hash(index_core)}
    target_store.write_immutable_json("migration-index.json", index)
    manifest = target_store.finalize_manifest()
    return {**index, "manifest_id": manifest["manifest_id"]}


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("source_study", type=Path)
    parser.add_argument("target_study", type=Path)
    args = parser.parse_args()
    result = migrate(args.source_study.resolve(), args.target_study.resolve())
    print(
        json.dumps(
            {
                "source_study_id": result["source_study_id"],
                "target_study_id": result["target_study_id"],
                "imported_count": result["imported_count"],
                "excluded_counts": result["excluded_counts"],
                "migration_id": result["migration_id"],
                "manifest_id": result["manifest_id"],
            },
            ensure_ascii=False,
        )
    )


if __name__ == "__main__":
    main()
