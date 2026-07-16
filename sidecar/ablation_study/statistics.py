"""Node-clustered statistics for the preregistered ablation estimands."""

from __future__ import annotations

from collections import Counter, defaultdict
import math
import random
import statistics
from typing import Any, Iterable


POSITIONS = {
    "long_cash": {"BUY": 1.0, "HOLD": 0.0, "SELL": 0.0},
    "long_short": {"BUY": 1.0, "HOLD": 0.0, "SELL": -1.0},
}


def action_return(action: str, outcome_return: float, cost_bps: float, mapping: str) -> float:
    position = POSITIONS[mapping][action]
    return position * outcome_return - abs(position) * cost_bps / 10_000.0


def _moving_block_sample(values: list[float], rng: random.Random, block_length: int) -> list[float]:
    size = len(values)
    sampled: list[float] = []
    while len(sampled) < size:
        start = rng.randrange(size)
        sampled.extend(values[(start + offset) % size] for offset in range(block_length))
    return sampled[:size]


def _percentile(sorted_values: list[float], probability: float) -> float:
    if not sorted_values:
        return math.nan
    position = probability * (len(sorted_values) - 1)
    low = math.floor(position)
    high = math.ceil(position)
    if low == high:
        return sorted_values[low]
    weight = position - low
    return sorted_values[low] * (1.0 - weight) + sorted_values[high] * weight


def moving_block_interval(
    values: list[float],
    *,
    confidence: float,
    iterations: int,
    block_length: int,
    seed: int,
) -> list[float] | None:
    if len(values) < 2:
        return None
    rng = random.Random(seed)
    estimates = sorted(
        statistics.fmean(_moving_block_sample(values, rng, block_length))
        for _ in range(iterations)
    )
    alpha = 1.0 - confidence
    return [
        round(_percentile(estimates, alpha / 2.0), 8),
        round(_percentile(estimates, 1.0 - alpha / 2.0), 8),
    ]


def paired_sign_flip_pvalue(values: list[float], *, iterations: int, seed: int) -> float | None:
    nonzero = [value for value in values if value != 0]
    if not nonzero:
        return 1.0
    observed = abs(statistics.fmean(nonzero))
    if len(nonzero) <= 20:
        extreme = 0
        total = 1 << len(nonzero)
        for mask in range(total):
            estimate = statistics.fmean(
                value if mask & (1 << index) else -value
                for index, value in enumerate(nonzero)
            )
            extreme += abs(estimate) >= observed - 1e-15
        return round(extreme / total, 8)
    rng = random.Random(seed)
    extreme = 0
    for _ in range(iterations):
        estimate = statistics.fmean(value if rng.random() < 0.5 else -value for value in nonzero)
        extreme += abs(estimate) >= observed - 1e-15
    return round((extreme + 1) / (iterations + 1), 8)


def _effect_summary(values: list[float], protocol: dict[str, Any], seed_offset: int) -> dict[str, Any]:
    bootstrap = protocol["statistics"]
    primary_confidence = float(bootstrap.get("primary_confidence_level", 0.95))
    equivalence_confidence = float(bootstrap.get("equivalence_confidence_level", 0.90))
    primary_interval = moving_block_interval(
        values,
        confidence=primary_confidence,
        iterations=int(bootstrap["bootstrap_iterations"]),
        block_length=int(bootstrap["block_length_nodes"]),
        seed=int(bootstrap["seed"]) + seed_offset,
    )
    equivalence_interval = moving_block_interval(
        values,
        confidence=equivalence_confidence,
        iterations=int(bootstrap["bootstrap_iterations"]),
        block_length=int(bootstrap["block_length_nodes"]),
        seed=int(bootstrap["seed"]) + seed_offset + 10_000,
    )
    result = {
        "nodes": len(values),
        "mean_delta": round(statistics.fmean(values), 8) if values else None,
        "median_delta": round(statistics.median(values), 8) if values else None,
        "positive_nodes": sum(value > 0 for value in values),
        "zero_nodes": sum(value == 0 for value in values),
        "negative_nodes": sum(value < 0 for value in values),
        "primary_confidence_level": primary_confidence,
        "moving_block_primary_interval": primary_interval,
        "equivalence_confidence_level": equivalence_confidence,
        "moving_block_equivalence_interval": equivalence_interval,
        "paired_sign_flip_two_sided_p": paired_sign_flip_pvalue(
            values,
            iterations=int(bootstrap["randomization_iterations"]),
            seed=int(bootstrap["seed"]) + seed_offset,
        ),
    }
    # Preserve the original result keys only for legacy one-look protocols.
    # New staged protocols must not label a 97.5% interval as a 95% interval.
    if primary_confidence == 0.95:
        result["moving_block_95pct"] = primary_interval
    if equivalence_confidence == 0.90:
        result["moving_block_90pct"] = equivalence_interval
    return result


def _performance(node_returns: list[float]) -> dict[str, Any]:
    equity = 1.0
    peak = 1.0
    maximum_drawdown = 0.0
    for value in node_returns:
        equity *= max(0.0, 1.0 + value)
        peak = max(peak, equity)
        maximum_drawdown = min(maximum_drawdown, equity / peak - 1.0)
    return {
        "mean_node_return": round(statistics.fmean(node_returns), 8) if node_returns else None,
        "total_compounded_return": round(equity - 1.0, 8) if node_returns else None,
        "endpoint_maximum_drawdown": round(maximum_drawdown, 8) if node_returns else None,
        "positive_node_rate": round(sum(value > 0 for value in node_returns) / len(node_returns), 8)
        if node_returns
        else None,
    }


def analyze_study_records(
    protocol: dict[str, Any],
    records: Iterable[dict[str, Any]],
    outcomes: dict[str, dict[str, Any]],
) -> dict[str, Any]:
    completed = [record for record in records if record.get("status") == "complete"]
    grouped: dict[tuple[str, str], list[dict[str, Any]]] = defaultdict(list)
    for record in completed:
        grouped[(record["node_id"], record["context"])].append(record)

    cost_bps = float(protocol["execution"]["cost_bps_per_position"])
    node_rows: list[dict[str, Any]] = []
    for node in protocol["nodes"]:
        node_id = node["node_id"]
        outcome = outcomes[node_id]
        endpoint_return = float(outcome["endpoint_total_return"])
        blind = grouped.get((node_id, "blind"), [])
        informed = grouped.get((node_id, "quant_informed"), [])
        if not blind or not informed:
            continue

        def mean_arm(rows: list[dict[str, Any]], field: str, mapping: str) -> float:
            return statistics.fmean(
                action_return(str(row[field]), endpoint_return, cost_bps, mapping) for row in rows
            )

        q_action = str(node["quant_action"])
        values: dict[str, dict[str, float]] = {}
        for mapping in POSITIONS:
            values[mapping] = {
                "BUY": action_return("BUY", endpoint_return, cost_bps, mapping),
                "Q": action_return(q_action, endpoint_return, cost_bps, mapping),
                "S": mean_arm(blind, "single_action", mapping),
                "M": mean_arm(blind, "multi_action", mapping),
                "Q+S": mean_arm(informed, "single_action", mapping),
                "Q+M": mean_arm(informed, "multi_action", mapping),
            }
        node_rows.append(
            {
                "node_id": node_id,
                "as_of_date": node["as_of_date"],
                "endpoint_total_return": endpoint_return,
                "quant_action": q_action,
                "blind_repeats": len(blind),
                "quant_informed_repeats": len(informed),
                "returns": values,
                "blind_change_rate": sum(row["single_action"] != row["multi_action"] for row in blind) / len(blind),
                "quant_informed_change_rate": sum(
                    row["single_action"] != row["multi_action"] for row in informed
                )
                / len(informed),
            }
        )

    primary_rows = [row["returns"]["long_cash"] for row in node_rows]
    blind_effect = [row["M"] - row["S"] for row in primary_rows]
    informed_effect = [row["Q+M"] - row["Q+S"] for row in primary_rows]
    pooled_effect = [statistics.fmean((blind, informed)) for blind, informed in zip(blind_effect, informed_effect)]
    effects = {
        "M_minus_S": _effect_summary(blind_effect, protocol, 1),
        "Q+M_minus_Q+S": _effect_summary(informed_effect, protocol, 2),
        "pooled_architecture_effect": _effect_summary(pooled_effect, protocol, 3),
        "Q+M_minus_Q": _effect_summary(
            [row["Q+M"] - row["Q"] for row in primary_rows], protocol, 4
        ),
        "Q+M_minus_BUY": _effect_summary(
            [row["Q+M"] - row["BUY"] for row in primary_rows], protocol, 5
        ),
    }

    pooled = effects["pooled_architecture_effect"]
    primary_interval = pooled["moving_block_primary_interval"]
    equivalence_interval = pooled["moving_block_equivalence_interval"]
    margin = float(protocol["statistics"]["equivalence_margin_per_node"])
    blind_mean = effects["M_minus_S"]["mean_delta"]
    informed_mean = effects["Q+M_minus_Q+S"]["mean_delta"]
    if primary_interval and primary_interval[0] > 0 and blind_mean >= 0 and informed_mean >= 0:
        classification = "positive"
    elif primary_interval and primary_interval[1] < 0 and blind_mean <= 0 and informed_mean <= 0:
        classification = "negative"
    elif (
        equivalence_interval
        and equivalence_interval[0] > -margin
        and equivalence_interval[1] < margin
        and blind_mean is not None
        and informed_mean is not None
        and abs(blind_mean) < margin
        and abs(informed_mean) < margin
    ):
        classification = "practically_null"
    else:
        classification = "inconclusive"

    arm_performance: dict[str, dict[str, Any]] = {}
    for mapping in POSITIONS:
        arm_performance[mapping] = {
            arm: _performance([row["returns"][mapping][arm] for row in node_rows])
            for arm in ("BUY", "Q", "S", "M", "Q+S", "Q+M")
        }

    action_distributions: dict[str, Counter[str]] = {
        "S": Counter(record["single_action"] for record in completed if record["context"] == "blind"),
        "M": Counter(record["multi_action"] for record in completed if record["context"] == "blind"),
        "Q+S": Counter(record["single_action"] for record in completed if record["context"] == "quant_informed"),
        "Q+M": Counter(record["multi_action"] for record in completed if record["context"] == "quant_informed"),
    }
    return {
        "schema_version": "ta.ablation_analysis.v1",
        "study_id": protocol["study_id"],
        "independent_nodes_analyzed": len(node_rows),
        "completed_graph_runs": len(completed),
        "primary_mapping": "long_cash",
        "architecture_classification": classification,
        "classification_rule": protocol["statistics"]["classification_rule"],
        "effects": effects,
        "arm_performance": arm_performance,
        "action_distributions": {key: dict(value) for key, value in action_distributions.items()},
        "mean_change_rates": {
            "blind": round(statistics.fmean(row["blind_change_rate"] for row in node_rows), 8)
            if node_rows
            else None,
            "quant_informed": round(
                statistics.fmean(row["quant_informed_change_rate"] for row in node_rows), 8
            )
            if node_rows
            else None,
        },
        "node_level_estimands": node_rows,
        "limitations": [
            "Inference is conditional on one symbol, one frozen model configuration, and the selected historical period.",
            "Moving-block intervals reduce but cannot eliminate financial time-series dependence.",
            "Endpoint drawdown understates intraperiod drawdown.",
            "Yahoo Finance is exploratory evidence, not an institutional point-in-time market database.",
        ],
    }
