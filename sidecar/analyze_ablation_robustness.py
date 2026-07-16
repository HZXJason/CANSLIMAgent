"""Create post-hoc robustness evidence without altering the frozen protocol."""

from __future__ import annotations

import argparse
import json
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any

from ablation_study.artifacts import StudyArtifactStore
from ablation_study.statistics import analyze_study_records
from quant_trend.artifacts import stable_json_hash


def _load(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"expected JSON object: {path}")
    return value


def _effect_digest(analysis: dict[str, Any]) -> dict[str, Any]:
    effects = analysis.get("effects") or {}
    return {
        "architecture_classification": analysis.get("architecture_classification"),
        "independent_nodes_analyzed": analysis.get("independent_nodes_analyzed"),
        "completed_graph_runs": analysis.get("completed_graph_runs"),
        "pooled_architecture_effect": effects.get("pooled_architecture_effect"),
        "M_minus_S": effects.get("M_minus_S"),
        "Q+M_minus_Q+S": effects.get("Q+M_minus_Q+S"),
        "Q+M_minus_Q": effects.get("Q+M_minus_Q"),
        "Q+M_minus_BUY": effects.get("Q+M_minus_BUY"),
    }


def analyze(study_root: Path) -> dict[str, Any]:
    protocol = _load(study_root / "protocol.json")
    stored_analysis = _load(study_root / "analysis.json")
    study_id = str(protocol.get("study_id") or "")
    if study_root.name != study_id:
        raise ValueError("study identity mismatch")
    outcomes = {
        path.stem: _load(path) for path in sorted((study_root / "outcomes").glob("*.json"))
    }
    records = [
        _load(path) for path in sorted((study_root / "runs").glob("*/*/*.json"))
    ]
    recomputed = analyze_study_records(protocol, records, outcomes)
    for key in (
        "architecture_classification",
        "effects",
        "arm_performance",
        "action_distributions",
        "mean_change_rates",
        "node_level_estimands",
    ):
        if recomputed.get(key) != stored_analysis.get(key):
            raise ValueError(f"stored main analysis differs from recomputation: {key}")

    first_attempt = [record for record in records if int(record.get("attempts") or 0) == 1]
    primary_repeat = [record for record in records if int(record.get("repeat") or 0) == 1]
    first_attempt_analysis = analyze_study_records(protocol, first_attempt, outcomes)
    primary_repeat_analysis = analyze_study_records(protocol, primary_repeat, outcomes)

    repeat_groups: dict[tuple[str, str], list[dict[str, Any]]] = defaultdict(list)
    for record in records:
        repeat_groups[(str(record.get("node_id")), str(record.get("context")))].append(record)
    reproducibility_pairs: list[dict[str, Any]] = []
    for (node_id, context), rows in sorted(repeat_groups.items()):
        ordered = sorted(rows, key=lambda row: int(row.get("repeat") or 0))
        if len(ordered) < 2:
            continue
        first, second = ordered[:2]
        reproducibility_pairs.append(
            {
                "node_id": node_id,
                "context": context,
                "first_record_id": first.get("record_id"),
                "second_record_id": second.get("record_id"),
                "single_action_agrees": first.get("single_action") == second.get("single_action"),
                "multi_action_agrees": first.get("multi_action") == second.get("multi_action"),
            }
        )

    retried = [record for record in records if int(record.get("attempts") or 0) > 1]
    attempts = Counter(int(record.get("attempts") or 0) for record in records)
    migrated = sum(record.get("migration") is not None for record in records)
    main_mean = recomputed["effects"]["pooled_architecture_effect"]["mean_delta"]
    first_mean = first_attempt_analysis["effects"]["pooled_architecture_effect"]["mean_delta"]
    primary_mean = primary_repeat_analysis["effects"]["pooled_architecture_effect"]["mean_delta"]

    core = {
        "schema_version": "ta.ablation_robustness.v1",
        "study_id": study_id,
        "main_analysis_id": stored_analysis.get("analysis_id"),
        "status": "post_hoc_sensitivity_not_a_replacement_for_preregistered_primary_analysis",
        "record_audit": {
            "records": len(records),
            "migrated_records": migrated,
            "live_records": len(records) - migrated,
            "attempt_distribution": {str(key): value for key, value in sorted(attempts.items())},
            "retried_records": [
                {
                    "record_id": record.get("record_id"),
                    "node_id": record.get("node_id"),
                    "as_of_date": record.get("as_of_date"),
                    "context": record.get("context"),
                    "repeat": record.get("repeat"),
                    "attempts": record.get("attempts"),
                }
                for record in retried
            ],
        },
        "analyses": {
            "preregistered_all_valid_records": _effect_digest(recomputed),
            "first_attempt_only": _effect_digest(first_attempt_analysis),
            "primary_repeat_only": _effect_digest(primary_repeat_analysis),
        },
        "direction_stability": {
            "pooled_mean_delta_all": main_mean,
            "pooled_mean_delta_first_attempt_only": first_mean,
            "pooled_mean_delta_primary_repeat_only": primary_mean,
            "all_nonzero_means_same_sign": (
                main_mean is not None
                and first_mean is not None
                and primary_mean is not None
                and (main_mean > 0) == (first_mean > 0) == (primary_mean > 0)
            ),
        },
        "reproducibility": {
            "pairs": len(reproducibility_pairs),
            "single_action_agreement_rate": (
                round(
                    sum(pair["single_action_agrees"] for pair in reproducibility_pairs)
                    / len(reproducibility_pairs),
                    8,
                )
                if reproducibility_pairs
                else None
            ),
            "multi_action_agreement_rate": (
                round(
                    sum(pair["multi_action_agrees"] for pair in reproducibility_pairs)
                    / len(reproducibility_pairs),
                    8,
                )
                if reproducibility_pairs
                else None
            ),
            "pair_evidence": reproducibility_pairs,
        },
        "interpretation_rules": [
            "The preregistered all-valid-record analysis remains primary.",
            "The first-attempt-only analysis tests whether retry behavior drives direction or classification.",
            "The primary-repeat-only analysis tests whether averaging reproducibility repeats drives the result.",
            "A sensitivity subset with fewer paired nodes has lower power and cannot prove equivalence by itself.",
        ],
    }
    result = {**core, "robustness_id": stable_json_hash(core)}
    store = StudyArtifactStore(study_root.parent, study_id)
    store.write_immutable_json("robustness.json", result)
    manifest = store.finalize_manifest()
    return {**result, "manifest_id": manifest["manifest_id"]}


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("study_root", type=Path)
    args = parser.parse_args()
    print(json.dumps(analyze(args.study_root.resolve()), ensure_ascii=False))


if __name__ == "__main__":
    main()
