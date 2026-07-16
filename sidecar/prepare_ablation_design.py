"""Freeze the model-independent staged node and power design."""

from __future__ import annotations

from datetime import date
import json
from pathlib import Path

from ablation_study.artifacts import StudyArtifactStore
from ablation_study.design import build_staged_design
from quant_trend import QuantTrendService


def render(design: dict) -> str:
    lines = [
        "# Staged ablation design and power envelope",
        "",
        f"- Design ID: `{design['design_id']}`",
        f"- Independent nodes: {design['stage_one_independent_nodes']} then {design['full_independent_nodes']}",
        f"- Graph runs: {design['stage_one_graph_runs']} then at most {design['maximum_graph_runs']}",
        f"- Per-look interval: {design['primary_confidence_level_each_look'] * 100:.1f}%",
        f"- Practical-equivalence margin: +/-{design['equivalence_margin_per_20d_node'] * 100:.1f}% per node",
        "",
        "| Paired-delta SD | Stage-1 MDE iid | Full MDE iid | Stage-1 MDE block-3 | Full MDE block-3 |",
        "|---:|---:|---:|---:|---:|",
    ]
    for row in design["power_assumptions"]:
        lines.append(
            f"| {row['paired_delta_standard_deviation']:.2%} | {row['stage_one_mde_iid']:.2%} | "
            f"{row['full_mde_iid']:.2%} | {row['stage_one_mde_block3_conservative']:.2%} | "
            f"{row['full_mde_block3_conservative']:.2%} |"
        )
    stage_one_dates = [node["as_of_date"] for node in design["node_contracts"] if node["stage"] == 1]
    stage_two_dates = [node["as_of_date"] for node in design["node_contracts"] if node["stage"] == 2]
    repeat_dates = [node["as_of_date"] for node in design["node_contracts"] if node["reproducibility_repeat"]]
    lines.extend(
        [
            "",
            "## Frozen dates",
            "",
            f"- Stage one ({len(stage_one_dates)}): {', '.join(stage_one_dates)}",
            f"- Stage two ({len(stage_two_dates)}): {', '.join(stage_two_dates)}",
            f"- Reproducibility subset ({len(repeat_dates)}): {', '.join(repeat_dates)}",
            "",
            "MDE values are planning approximations. The final inference uses corrected moving-block intervals on paired node effects.",
        ]
    )
    return "\n".join(lines)


def main() -> None:
    service = QuantTrendService()
    evaluation = service.evaluate(
        symbol="MU",
        start_date=date(2022, 8, 1),
        end_date=date(2026, 5, 29),
        horizon_days=20,
        step_days=20,
        maximum_nodes=45,
        transaction_cost_bps=10,
    )
    design = build_staged_design(evaluation)
    store = StudyArtifactStore(Path("data/ablation-designs"), design["design_id"])
    store.write_immutable_json("design.json", design)
    store.write_immutable_json(
        "quant_evaluation.json",
        {key: value for key, value in evaluation.items() if key != "artifact_path"},
    )
    store.write_text("REPORT.md", render(design))
    manifest = store.finalize_manifest()
    print(json.dumps({
        "design_id": design["design_id"],
        "evaluation_run_id": evaluation["run_id"],
        "manifest_id": manifest["manifest_id"],
        "artifact_root": str(store.root),
    }, ensure_ascii=False))


if __name__ == "__main__":
    main()
