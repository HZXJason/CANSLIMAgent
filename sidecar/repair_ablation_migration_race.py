"""Repair a narrowly defined cancelled-bootstrap/migration write race.

The repair is intentionally evidence preserving: every superseded live record is
archived, every replacement is reconstructed from the frozen lineage artifact,
and the migration index's preregistered target hash must match before a write.
"""

from __future__ import annotations

import argparse
import copy
import json
from pathlib import Path
from typing import Any

from ablation_study.artifacts import StudyArtifactStore
from ablation_study.runner import parse_explicit_action
from quant_trend.artifacts import stable_json_hash


ELIGIBILITY = "complete_first_attempt_same_treatment_reparsed_v3"


def _load(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"expected JSON object: {path}")
    return value


def _run_key(study_id: str, context: str, node_id: str, repeat: int) -> str:
    return stable_json_hash(
        {
            "schema_version": "ta.ablation_run_key.v1",
            "study_id": study_id,
            "context": context,
            "node_id": node_id,
            "repeat": repeat,
        }
    )


def _expected_migrated_record(
    source_record: dict[str, Any],
    *,
    source_study_id: str,
    target_study_id: str,
    source_relative: str,
) -> dict[str, Any]:
    source_core = {
        key: value for key, value in source_record.items() if key != "record_id"
    }
    if source_record.get("record_id") != stable_json_hash(source_core):
        raise ValueError(f"source record hash mismatch: {source_relative}")
    if source_record.get("status") != "complete" or int(source_record.get("attempts") or 0) != 1:
        raise ValueError(f"source record is not a first-attempt success: {source_relative}")
    single = parse_explicit_action((source_record.get("state") or {}).get("market_report"))
    multi = parse_explicit_action(
        (source_record.get("state") or {}).get("final_trade_decision")
    ) or parse_explicit_action(source_record.get("fallback_decision"))
    if single is None or multi is None:
        raise ValueError(f"source record is not explicitly parseable: {source_relative}")
    node_id = str(source_record.get("node_id") or "")
    context = str(source_record.get("context") or "")
    repeat = int(source_record.get("repeat") or 0)
    target_core = copy.deepcopy(source_core)
    target_core.update(
        {
            "study_id": target_study_id,
            "run_key": _run_key(target_study_id, context, node_id, repeat),
            "parser_version": "explicit-decision.v3",
            "single_action": single,
            "multi_action": multi,
            "migration": {
                "schema_version": "ta.ablation_record_migration.v1",
                "source_study_id": source_study_id,
                "source_record_id": source_record["record_id"],
                "source_record_artifact": source_relative,
                "eligibility": ELIGIBILITY,
            },
        }
    )
    return {**target_core, "record_id": stable_json_hash(target_core)}


def repair(study_root: Path) -> dict[str, Any]:
    protocol = _load(study_root / "protocol.json")
    index = _load(study_root / "migration-index.json")
    study_id = str(protocol.get("study_id") or "")
    source_study_id = str(index.get("source_study_id") or "")
    if study_root.name != study_id or index.get("target_study_id") != study_id:
        raise ValueError("study or migration target identity mismatch")
    if not (study_root / "analysis.json").is_file():
        raise ValueError("study is still active; repair is only allowed after final analysis")

    store = StudyArtifactStore(study_root.parent, study_id)
    repairs: list[dict[str, Any]] = []
    for entry in index.get("records") or []:
        target_relative = str(entry.get("target_path") or "")
        source_relative = str(entry.get("source_path") or "")
        target_path = study_root / target_relative
        source_path = study_root / source_relative
        current = _load(target_path)
        if current.get("record_id") == entry.get("target_record_id"):
            continue
        if current.get("migration") is not None:
            raise ValueError(f"unexpected modified migrated record: {target_relative}")
        current_core = {key: value for key, value in current.items() if key != "record_id"}
        if current.get("record_id") != stable_json_hash(current_core):
            raise ValueError(f"superseded live record hash mismatch: {target_relative}")
        if (
            current.get("study_id") != study_id
            or current.get("status") != "complete"
            or current.get("parser_version") != "explicit-decision.v3"
        ):
            raise ValueError(f"unexpected superseding record: {target_relative}")

        source = _load(source_path)
        expected = _expected_migrated_record(
            source,
            source_study_id=source_study_id,
            target_study_id=study_id,
            source_relative=source_relative,
        )
        if expected.get("record_id") != entry.get("target_record_id"):
            raise ValueError(f"migration index target hash mismatch: {target_relative}")
        archive_relative = str(Path("superseded-race") / target_relative)
        store.write_immutable_json(archive_relative, current)
        store.write_json(target_relative, expected)
        repairs.append(
            {
                "target_path": target_relative,
                "source_path": source_relative,
                "superseded_record_id": current["record_id"],
                "restored_record_id": expected["record_id"],
                "superseded_artifact": archive_relative,
            }
        )

    report_core = {
        "schema_version": "ta.ablation_migration_race_repair.v1",
        "study_id": study_id,
        "migration_id": index.get("migration_id"),
        "cause": "cancelled_bootstrap_workers_completed_after_parser_migration",
        "policy": (
            "Archive late bootstrap writes and restore the deterministic migration targets "
            "used by the resumed runner's cached stage-one record set."
        ),
        "repaired_count": len(repairs),
        "repairs": repairs,
    }
    report = {**report_core, "repair_id": stable_json_hash(report_core)}
    store.write_immutable_json("repair/parser-migration-race.json", report)
    manifest = store.finalize_manifest()
    return {**report, "manifest_id": manifest["manifest_id"]}


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("study_root", type=Path)
    args = parser.parse_args()
    print(json.dumps(repair(args.study_root.resolve()), ensure_ascii=False))


if __name__ == "__main__":
    main()
