"""Re-score existing technical-calibration audits as paired S vs M pilots."""

from __future__ import annotations

from collections import Counter
import argparse
import json
from pathlib import Path
import re
import statistics
import subprocess
from typing import Any

from ablation_study.artifacts import StudyArtifactStore
from ablation_study.runner import parse_explicit_action
from ablation_study.statistics import (
    action_return,
    moving_block_interval,
    paired_sign_flip_pvalue,
)
from quant_trend.artifacts import stable_json_hash


def parse_legacy_explicit_action(value: Any) -> str | None:
    parsed = parse_explicit_action(value)
    if parsed is not None:
        return parsed
    text = str(value or "")
    patterns = (
        r"(?im)^\s*最终评级\s*[:：-]\s*\**\s*(buy|overweight|hold|underweight|sell|买入|增持|持有|减持|卖出|低配|超配)",
        r"(?im)^\s*(?:综合判断|综合建议|最终交易建议|交易建议)\s*[:：|].*?(buy|hold|sell|买入|持有|卖出)",
        r"(?im)^\s*\|\s*(?:综合判断|综合建议)\s*\|.*?\**(buy|hold|sell|买入|持有|卖出)\**",
    )
    mapping = {
        "BUY": "BUY", "OVERWEIGHT": "BUY", "买入": "BUY", "增持": "BUY", "超配": "BUY",
        "HOLD": "HOLD", "持有": "HOLD",
        "SELL": "SELL", "UNDERWEIGHT": "SELL", "卖出": "SELL", "减持": "SELL", "低配": "SELL",
    }
    for pattern in patterns:
        match = re.search(pattern, text)
        if match:
            return mapping.get(match.group(1).upper(), mapping.get(match.group(1)))
    return None


def load_rows(container: str) -> list[dict[str, Any]]:
    sql = """
select json_build_object(
  'id',id,'created_at',created_at,'symbol',symbol,'provider',provider,
  'deep_model',deep_think_llm,'quick_model',quick_think_llm,
  'horizon_days',horizon_days,'strategy_version',strategy_version,
  'config_hash',config_hash,'nodes',payload->'nodes'
)
from ai_calibrations
where status='complete'
  and payload->'requested_analysts'='["technical"]'::jsonb
order by created_at;
"""
    output = subprocess.check_output(
        ["docker", "exec", container, "psql", "-U", "alphalens", "-d", "alphalens", "-Atc", sql],
        text=True,
    )
    return [json.loads(line) for line in output.splitlines() if line.strip()]


def performance(values: list[float]) -> dict[str, Any]:
    equity = 1.0
    peak = 1.0
    maximum_drawdown = 0.0
    for value in values:
        equity *= max(0.0, 1.0 + value)
        peak = max(peak, equity)
        maximum_drawdown = min(maximum_drawdown, equity / peak - 1.0)
    return {
        "mean_node_return": round(statistics.fmean(values), 8),
        "compounded_return": round(equity - 1.0, 8),
        "endpoint_maximum_drawdown": round(maximum_drawdown, 8),
    }


def effect(values: list[float], seed: int) -> dict[str, Any]:
    ci95 = moving_block_interval(
        values, confidence=0.95, iterations=20_000, block_length=3, seed=seed
    )
    ci90 = moving_block_interval(
        values, confidence=0.90, iterations=20_000, block_length=3, seed=seed + 10_000
    )
    if ci95 and ci95[0] > 0:
        classification = "positive"
    elif ci95 and ci95[1] < 0:
        classification = "negative"
    elif ci90 and ci90[0] > -0.01 and ci90[1] < 0.01:
        classification = "practically_null"
    else:
        classification = "inconclusive"
    return {
        "nodes": len(values),
        "mean_delta": round(statistics.fmean(values), 8),
        "median_delta": round(statistics.median(values), 8),
        "positive_nodes": sum(value > 0 for value in values),
        "zero_nodes": sum(value == 0 for value in values),
        "negative_nodes": sum(value < 0 for value in values),
        "moving_block_95pct": ci95,
        "moving_block_90pct": ci90,
        "paired_sign_flip_two_sided_p": paired_sign_flip_pvalue(
            values, iterations=100_000, seed=seed
        ),
        "classification": classification,
    }


def render_report(result: dict[str, Any]) -> str:
    lines = [
        "# Retrospective paired TradingAgents ablation",
        "",
        "> Exploratory only. These runs were created before this protocol and cannot replace the current-model confirmatory study.",
        "",
        "| Calibration run | Model | Symbol | Horizon | M-S long/cash | 95% block CI | Classification | Change rate |",
        "|---|---|---|---:|---:|---:|---|---:|",
    ]
    for cell in result["cells"]:
        item = cell["architecture_effect_long_cash"]
        ci = item["moving_block_95pct"] or [None, None]
        lines.append(
            f"| `{cell['calibration_id'][:8]}` | {cell['model']} | {cell['symbol']} | {cell['horizon_days']} | "
            f"{item['mean_delta']} | [{ci[0]}, {ci[1]}] | {item['classification']} | {cell['change_rate']} |"
        )
    aggregate = result["cell_level_summary"]
    lines.extend(
        [
            "",
            "## Interpretation",
            "",
            f"Across {aggregate['cells']} separately analyzed calibration cells, architecture classifications were "
            f"{json.dumps(aggregate['classification_counts'], ensure_ascii=False)}. The mean of cell-level effects was "
            f"{aggregate['mean_cell_effect']}. This cross-cell summary is descriptive because cells reuse market regimes and were not preregistered.",
            "",
            "A profitable base strategy is not required to identify the architecture effect: M and S are scored on the same node and outcome, so M-S isolates the downstream transformation. Constant BUY remains an absolute benchmark for whether either arm is investable.",
            "",
            "## Limitations",
            "",
        ]
    )
    lines.extend(f"- {item}" for item in result["limitations"])
    return "\n".join(lines)


def run(audit_root: Path, output_root: Path, container: str) -> dict[str, Any]:
    rows = load_rows(container)
    source_rows = []
    evidence_nodes = []
    for row in rows:
        source_rows.append({key: value for key, value in row.items() if key != "nodes"})
        for node in row["nodes"]:
            audit_path = audit_root / str(node["audit_file"])
            audit = json.loads(audit_path.read_text(encoding="utf-8"))
            state = audit.get("state") or {}
            single = parse_legacy_explicit_action(state.get("market_report") or audit.get("raw_decision"))
            multi = parse_legacy_explicit_action(state.get("final_trade_decision"))
            if single is None or multi is None:
                raise ValueError(f"unparseable paired decision: {row['id']} {node['as_of_date']}")
            evidence_nodes.append(
                {
                    "calibration_id": row["id"],
                    "model": row["deep_model"],
                    "symbol": row["symbol"],
                    "horizon_days": row["horizon_days"],
                    "as_of_date": node["as_of_date"],
                    "outcome_date": node["outcome_date"],
                    "raw_return": float(node["raw_return"]),
                    "single_action": single,
                    "multi_action": multi,
                    "audit_file": audit_path.name,
                    "audit_hash": stable_json_hash(audit),
                    "source_node": node,
                    "audit": audit,
                }
            )

    protocol_core = {
        "schema_version": "ta.retrospective_ablation_protocol.v1",
        "purpose": "Exploratory paired re-scoring of pre-existing technical calibration audits",
        "selection_rule": "all complete DB rows whose requested_analysts is exactly [technical]",
        "single_decision": "market_report explicit final transaction proposal",
        "multi_decision": "final_trade_decision explicit rating/action",
        "primary_mapping": {"BUY": 1, "HOLD": 0, "SELL": 0},
        "secondary_mapping": {"BUY": 1, "HOLD": 0, "SELL": -1},
        "cost_bps_per_position": 10,
        "unit": "historical node within calibration run",
        "no_pooling_rule": "each calibration run is analyzed separately; duplicate regimes are not treated as independent",
        "source_rows": source_rows,
        "source_node_hash": stable_json_hash(
            [
                {key: value for key, value in node.items() if key != "audit"}
                for node in evidence_nodes
            ]
        ),
    }
    study_id = stable_json_hash(protocol_core)
    protocol = {**protocol_core, "study_id": study_id}
    store = StudyArtifactStore(output_root, study_id)
    store.write_immutable_json("protocol.json", protocol)

    by_calibration: dict[str, list[dict[str, Any]]] = {}
    for node in evidence_nodes:
        by_calibration.setdefault(node["calibration_id"], []).append(node)
        store.write_immutable_json(
            Path("audits") / f"{node['audit_hash']}.json", node["audit"]
        )

    cells = []
    for offset, (calibration_id, nodes) in enumerate(by_calibration.items(), start=1):
        nodes.sort(key=lambda item: item["as_of_date"])
        returns = [node["raw_return"] for node in nodes]
        arms: dict[str, dict[str, Any]] = {}
        effects: dict[str, dict[str, Any]] = {}
        for mapping in ("long_cash", "long_short"):
            s_values = [action_return(node["single_action"], node["raw_return"], 10, mapping) for node in nodes]
            m_values = [action_return(node["multi_action"], node["raw_return"], 10, mapping) for node in nodes]
            buy_values = [action_return("BUY", value, 10, mapping) for value in returns]
            arms[mapping] = {
                "BUY": performance(buy_values),
                "S": performance(s_values),
                "M": performance(m_values),
            }
            effects[mapping] = effect(
                [multi - single for single, multi in zip(s_values, m_values)],
                20260716 + offset * 100 + (0 if mapping == "long_cash" else 1),
            )
        cells.append(
            {
                "calibration_id": calibration_id,
                "model": nodes[0]["model"],
                "symbol": nodes[0]["symbol"],
                "horizon_days": nodes[0]["horizon_days"],
                "nodes": len(nodes),
                "single_actions": dict(Counter(node["single_action"] for node in nodes)),
                "multi_actions": dict(Counter(node["multi_action"] for node in nodes)),
                "changed_nodes": sum(node["single_action"] != node["multi_action"] for node in nodes),
                "change_rate": round(sum(node["single_action"] != node["multi_action"] for node in nodes) / len(nodes), 8),
                "architecture_effect_long_cash": effects["long_cash"],
                "architecture_effect_long_short": effects["long_short"],
                "arm_performance": arms,
                "node_evidence": [
                    {key: value for key, value in node.items() if key != "audit"} for node in nodes
                ],
            }
        )

    cell_effects = [cell["architecture_effect_long_cash"]["mean_delta"] for cell in cells]
    result_core = {
        "schema_version": "ta.retrospective_ablation_result.v1",
        "study_id": study_id,
        "cells": cells,
        "cell_level_summary": {
            "cells": len(cells),
            "mean_cell_effect": round(statistics.fmean(cell_effects), 8),
            "median_cell_effect": round(statistics.median(cell_effects), 8),
            "classification_counts": dict(
                Counter(cell["architecture_effect_long_cash"]["classification"] for cell in cells)
            ),
            "warning": "descriptive only; cells are not independent and the protocol was retrospective",
        },
        "limitations": [
            "The protocol was written after the calibration outputs existed, so all estimates are exploratory.",
            "Each cell has only ten nodes and moving-block intervals have low power.",
            "Some cells share dates/regimes; they are not pooled as independent observations.",
            "Returns are the calibration's raw next-open-to-horizon-close returns, not total-return adjusted returns.",
            "The old model configurations differ from the currently selected codex-auto-review model.",
        ],
    }
    result = {**result_core, "result_id": stable_json_hash(result_core)}
    store.write_json("result.json", result)
    store.write_text("REPORT.md", render_report(result))
    manifest = store.finalize_manifest()
    return {**result, "manifest_id": manifest["manifest_id"], "artifact_root": str(store.root)}


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--audit-root", type=Path, default=Path("data/calibration-nodes"))
    parser.add_argument("--output-root", type=Path, default=Path("data/retrospective-ablation"))
    parser.add_argument("--postgres-container", default="alphalens-postgres")
    args = parser.parse_args()
    result = run(args.audit_root.resolve(), args.output_root.resolve(), args.postgres_container)
    print(json.dumps({
        "study_id": result["study_id"],
        "result_id": result["result_id"],
        "manifest_id": result["manifest_id"],
        "artifact_root": result["artifact_root"],
    }, ensure_ascii=False))


if __name__ == "__main__":
    main()
