"""Pre-registered staged sampling and power calculations."""

from __future__ import annotations

import math
from statistics import NormalDist
from typing import Any

from quant_trend.artifacts import stable_json_hash


TaskKey = tuple[str, str, int]


def evenly_spaced_positions(total: int, maximum: int) -> list[int]:
    if total <= 0 or maximum <= 0:
        raise ValueError("total and maximum must be positive")
    if maximum >= total:
        return list(range(total))
    if maximum == 1:
        return [total - 1]
    positions = {
        round(offset * (total - 1) / (maximum - 1))
        for offset in range(maximum)
    }
    result = sorted(positions)
    if len(result) != maximum:
        raise ValueError("even spacing did not produce the requested sample size")
    return result


def minimum_detectable_effect(
    standard_deviation: float,
    nodes: int,
    *,
    per_look_two_sided_alpha: float = 0.025,
    power: float = 0.80,
    design_effect: float = 1.0,
) -> float:
    if standard_deviation < 0 or nodes <= 1 or not 0 < per_look_two_sided_alpha < 1 or not 0 < power < 1:
        raise ValueError("invalid power parameters")
    effective_nodes = nodes / max(1.0, design_effect)
    normal = NormalDist()
    critical = normal.inv_cdf(1.0 - per_look_two_sided_alpha / 2.0)
    powered = normal.inv_cdf(power)
    return (critical + powered) * standard_deviation / math.sqrt(effective_nodes)


def build_staged_design(
    evaluation: dict[str, Any],
    *,
    stage_one_nodes: int = 30,
    reproducibility_nodes: int = 5,
) -> dict[str, Any]:
    nodes = evaluation.get("nodes") or []
    total = len(nodes)
    if total <= stage_one_nodes:
        raise ValueError("full evaluation must contain more nodes than stage one")
    stage_one_positions = evenly_spaced_positions(total, stage_one_nodes)
    stage_one_set = set(stage_one_positions)
    reproducibility_offsets = evenly_spaced_positions(stage_one_nodes, reproducibility_nodes)
    reproducibility_positions = [stage_one_positions[offset] for offset in reproducibility_offsets]
    reproducibility_set = set(reproducibility_positions)
    node_contracts = []
    for position, node in enumerate(nodes):
        node_contracts.append(
            {
                "position": position,
                "as_of_date": node["as_of_date"],
                "outcome_date": node["outcome_date"],
                "stage": 1 if position in stage_one_set else 2,
                "reproducibility_repeat": position in reproducibility_set,
            }
        )
    power_assumptions = []
    for standard_deviation in (0.02, 0.04, 0.06, 0.08):
        power_assumptions.append(
            {
                "paired_delta_standard_deviation": standard_deviation,
                "stage_one_mde_iid": round(
                    minimum_detectable_effect(standard_deviation, stage_one_nodes), 8
                ),
                "full_mde_iid": round(minimum_detectable_effect(standard_deviation, total), 8),
                "stage_one_mde_block3_conservative": round(
                    minimum_detectable_effect(
                        standard_deviation, stage_one_nodes, design_effect=3.0
                    ),
                    8,
                ),
                "full_mde_block3_conservative": round(
                    minimum_detectable_effect(standard_deviation, total, design_effect=3.0),
                    8,
                ),
            }
        )
    core = {
        "schema_version": "ta.staged_ablation_design.v1",
        "symbol": evaluation["symbol"],
        "horizon_trading_days": evaluation["horizon_trading_days"],
        "full_independent_nodes": total,
        "stage_one_independent_nodes": stage_one_nodes,
        "stage_two_additional_nodes": total - stage_one_nodes,
        "reproducibility_nodes": reproducibility_nodes,
        "contexts": ["blind", "quant_informed"],
        "primary_repeats": 1,
        "reproducibility_repeats": 2,
        "stage_one_graph_runs": stage_one_nodes * 2 + reproducibility_nodes * 2,
        "maximum_graph_runs": total * 2 + reproducibility_nodes * 2,
        "interim_looks": 2,
        "familywise_two_sided_alpha": 0.05,
        "per_look_two_sided_alpha": 0.025,
        "primary_confidence_level_each_look": 0.975,
        "equivalence_confidence_level_each_look": 0.975,
        "equivalence_margin_per_20d_node": 0.01,
        "stopping_rule": (
            "After stage one stop only for positive, negative, or practically_null under the corrected "
            "interval rules; otherwise run every stage-two node. Reproducibility repeats never increase n."
        ),
        "evaluation_run_id": evaluation["run_id"],
        "evaluation_data_hash": evaluation["data_hash"],
        "node_contracts": node_contracts,
        "power_assumptions": power_assumptions,
        "warnings": [
            "MDE calculations are planning approximations, not substitutes for the moving-block analysis.",
            "The block-length-three design effect is deliberately conservative.",
            "Failure to cross a positive boundary is not proof of a null effect unless the equivalence boundary is crossed.",
        ],
    }
    return {**core, "design_id": stable_json_hash(core)}


def staged_task_keys(
    node_contracts: list[dict[str, Any]], contexts: list[str] | tuple[str, ...]
) -> dict[str, list[TaskKey]]:
    """Return the exact preregistered graph-run keys for each stage."""

    stage_one: list[TaskKey] = []
    stage_two: list[TaskKey] = []
    for node in node_contracts:
        node_id = str(node["node_id"])
        stage = int(node.get("stage") or 0)
        if stage not in {1, 2}:
            raise ValueError(f"invalid staged node: {node_id}")
        for context in contexts:
            target = stage_one if stage == 1 else stage_two
            target.append((node_id, str(context), 1))
            if stage == 1 and node.get("reproducibility_repeat"):
                stage_one.append((node_id, str(context), 2))
    if len(stage_one) != len(set(stage_one)) or len(stage_two) != len(set(stage_two)):
        raise ValueError("staged task matrix contains duplicate keys")
    return {"stage_one": stage_one, "stage_two": stage_two}


def stage_one_stopping_decision(classification: str, *, matrix_complete: bool) -> dict[str, Any]:
    """Apply the frozen corrected-boundary stopping rule without discretion."""

    allowed = {"positive", "negative", "practically_null", "inconclusive"}
    if classification not in allowed:
        raise ValueError(f"invalid interim classification: {classification}")
    if not matrix_complete:
        return {
            "advance_to_stage_two": False,
            "stopped_early": False,
            "reason": "stage_one_matrix_incomplete",
        }
    if classification == "inconclusive":
        return {
            "advance_to_stage_two": True,
            "stopped_early": False,
            "reason": "corrected_interim_result_inconclusive",
        }
    return {
        "advance_to_stage_two": False,
        "stopped_early": True,
        "reason": f"corrected_interim_boundary_crossed:{classification}",
    }
