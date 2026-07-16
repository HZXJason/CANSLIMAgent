"""Rebuild ablation status from authoritative per-run evidence files."""

from __future__ import annotations

from collections import Counter
import argparse
import json
from pathlib import Path
from typing import Any

from ablation_study.artifacts import StudyArtifactStore
from quant_trend.artifacts import stable_json_hash


def reconcile(study_root: Path) -> dict[str, Any]:
    protocol = json.loads((study_root / "protocol.json").read_text(encoding="utf-8"))
    prior_path = study_root / "status.json"
    prior = json.loads(prior_path.read_text(encoding="utf-8")) if prior_path.exists() else {}
    records = []
    for path in sorted((study_root / "runs").glob("*/*/*.json")):
        record = json.loads(path.read_text(encoding="utf-8"))
        records.append(record)
    total = int(prior.get("total_runs") or (
        len(protocol["nodes"])
        * int(protocol["design"]["repeats_per_context"])
        * len(protocol["design"]["contexts"])
    ))
    status_counts = Counter(str(record.get("status") or "unknown") for record in records)
    error_counts = Counter(str(record.get("error")) for record in records if record.get("error"))
    successful = status_counts["complete"]
    failed = sum(count for status, count in status_counts.items() if status != "complete")
    processed = len(records)
    if (study_root / "analysis.json").exists():
        analysis = json.loads((study_root / "analysis.json").read_text(encoding="utf-8"))
        status = "complete" if failed == 0 and processed == total else "partial"
        classification = analysis.get("architecture_classification")
    elif str(prior.get("status")) == "cancelled" and processed < total:
        status = "cancelled"
        classification = None
    elif processed >= total and failed == 0:
        status = "ready_for_analysis"
        classification = None
    elif processed >= total and successful == 0:
        status = "failed"
        classification = None
    elif successful > 0:
        status = "partial"
        classification = None
    else:
        status = "cancelled" if processed < total else "failed"
        classification = None
    core = {
        "schema_version": "ta.ablation_status.v1",
        "study_id": protocol["study_id"],
        "status": status,
        "processed_runs": processed,
        "completed_runs": processed,
        "successful_runs": successful,
        "failed_runs": failed,
        "total_runs": total,
        "cached_runs": int(prior.get("cached_runs") or 0),
        "independent_nodes": len(protocol["nodes"]),
        "architecture_classification": classification,
        "reconciled_from_run_records": True,
        "run_status_counts": dict(status_counts),
        "error_counts": dict(error_counts),
    }
    status_record = {**core, "status_id": stable_json_hash(core)}
    store = StudyArtifactStore(study_root.parent, protocol["study_id"])
    store.write_json("status.json", status_record)
    manifest = store.finalize_manifest()
    return {**status_record, "manifest_id": manifest["manifest_id"]}


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("study_roots", nargs="+", type=Path)
    args = parser.parse_args()
    for root in args.study_roots:
        result = reconcile(root.resolve())
        print(json.dumps({
            "study_id": result["study_id"],
            "status": result["status"],
            "successful_runs": result["successful_runs"],
            "failed_runs": result["failed_runs"],
            "processed_runs": result["processed_runs"],
            "total_runs": result["total_runs"],
            "manifest_id": result["manifest_id"],
        }, ensure_ascii=False))


if __name__ == "__main__":
    main()
