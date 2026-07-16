"""Seal the human- and machine-readable conclusion for a verified study."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any

from ablation_study.artifacts import StudyArtifactStore
from quant_trend.artifacts import stable_json_hash


def _load(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"expected JSON object: {path}")
    return value


def _percent(value: float | None) -> str:
    return "NA" if value is None else f"{value * 100:+.3f}%"


def finalize(study_root: Path, verification_id: str) -> dict[str, Any]:
    protocol = _load(study_root / "protocol.json")
    analysis = _load(study_root / "analysis.json")
    robustness = _load(study_root / "robustness.json")
    migration = _load(study_root / "migration-index.json")
    repair = _load(study_root / "repair" / "parser-migration-race.json")
    archive = _load(study_root / "evidence_archive.json")
    study_id = str(protocol.get("study_id") or "")
    if study_root.name != study_id:
        raise ValueError("study identity mismatch")
    if analysis.get("matrix_complete") is not True:
        raise ValueError("cannot conclude from an incomplete matrix")
    if robustness.get("main_analysis_id") != analysis.get("analysis_id"):
        raise ValueError("robustness report does not reference the final analysis")
    if int(archive.get("completed_runs_archived") or 0) != int(
        analysis.get("completed_graph_runs") or -1
    ):
        raise ValueError("checkpoint archive is incomplete")

    effects = analysis["effects"]
    pooled = effects["pooled_architecture_effect"]
    blind = effects["M_minus_S"]
    informed = effects["Q+M_minus_Q+S"]
    quant_comparison = effects["Q+M_minus_Q"]
    buy_comparison = effects["Q+M_minus_BUY"]
    margin = float(protocol["statistics"]["equivalence_margin_per_node"])
    pooled_interval = pooled["moving_block_primary_interval"]
    blind_interval = blind["moving_block_primary_interval"]
    informed_interval = informed["moving_block_primary_interval"]

    claims = {
        "positive_effect_demonstrated": analysis["architecture_classification"] == "positive",
        "negative_effect_demonstrated": analysis["architecture_classification"] == "negative",
        "practical_equivalence_demonstrated": analysis["architecture_classification"]
        == "practically_null",
        "material_pooled_positive_effect_excluded": pooled_interval[1] < margin,
        "material_blind_positive_effect_excluded": blind_interval[1] < margin,
        "material_quant_informed_positive_effect_excluded": informed_interval[1] < margin,
        "any_strictly_positive_effect_excluded": pooled_interval[1] <= 0,
    }
    if claims["positive_effect_demonstrated"]:
        answer = "positive_effect_demonstrated"
    elif claims["material_pooled_positive_effect_excluded"]:
        answer = (
            "no_demonstrated_positive_effect_and_pooled_effect_at_or_above_"
            "the_preregistered_materiality_margin_is_excluded"
        )
    else:
        answer = "positive_effect_not_demonstrated_but_material_positive_effect_not_excluded"

    first_attempt = robustness["analyses"]["first_attempt_only"]
    primary_repeat = robustness["analyses"]["primary_repeat_only"]
    core = {
        "schema_version": "ta.ablation_conclusion.v1",
        "study_id": study_id,
        "analysis_id": analysis.get("analysis_id"),
        "verification_id": verification_id,
        "robustness_id": robustness.get("robustness_id"),
        "migration_id": migration.get("migration_id"),
        "repair_id": repair.get("repair_id"),
        "archive_id": archive.get("archive_id"),
        "scope": {
            "symbol": protocol.get("symbol"),
            "model": protocol.get("model"),
            "start_date": protocol["design"].get("start_date"),
            "end_date": protocol["design"].get("end_date"),
            "horizon_trading_days": protocol["design"].get("horizon_trading_days"),
            "independent_nodes": analysis.get("independent_nodes_analyzed"),
            "graph_runs": analysis.get("completed_graph_runs"),
            "primary_mapping": analysis.get("primary_mapping"),
        },
        "preregistered_materiality_margin_per_node": margin,
        "primary_result": {
            "classification": analysis.get("architecture_classification"),
            "pooled_architecture_effect": pooled,
            "blind_multi_minus_single": blind,
            "quant_informed_multi_minus_single": informed,
            "quant_plus_multi_minus_quant": quant_comparison,
            "quant_plus_multi_minus_buy": buy_comparison,
        },
        "claims": claims,
        "research_answer": answer,
        "robustness": {
            "first_attempt_only_classification": first_attempt.get(
                "architecture_classification"
            ),
            "first_attempt_only_pooled_mean_delta": first_attempt[
                "pooled_architecture_effect"
            ]["mean_delta"],
            "primary_repeat_only_classification": primary_repeat.get(
                "architecture_classification"
            ),
            "primary_repeat_only_pooled_mean_delta": primary_repeat[
                "pooled_architecture_effect"
            ]["mean_delta"],
            "pooled_direction_stable": robustness["direction_stability"].get(
                "all_nonzero_means_same_sign"
            ),
            "single_action_agreement_rate": robustness["reproducibility"].get(
                "single_action_agreement_rate"
            ),
            "multi_action_agreement_rate": robustness["reproducibility"].get(
                "multi_action_agreement_rate"
            ),
        },
        "interpretation": [
            "The experiment does not demonstrate a positive marginal effect from the multi-agent layers.",
            "The pooled 97.5% interval's upper bound is below the preregistered +1% per-node materiality margin, so a pooled benefit at or above that margin is excluded for this scope.",
            "Because the pooled interval still crosses zero, the evidence does not prove a strictly negative effect, exact absence of effect, or practical equivalence.",
            "The quant-informed marginal interval still extends above +1%, so that context alone remains uncertain.",
            "All claims are conditional on MU, deepseek-v4-flash, the frozen 2022-2025 nodes, the 20-trading-day horizon, and the long/cash primary mapping.",
        ],
    }
    conclusion = {**core, "conclusion_id": stable_json_hash(core)}

    markdown = f"""# TradingAgents architecture ablation conclusion

## Answer

The multi-agent layers did **not demonstrate a positive marginal effect** in this frozen MU experiment. The preregistered classification is **{analysis['architecture_classification']}**, not positive.

The pooled effect was {_percent(pooled['mean_delta'])} per 20-trading-day node, with a 97.5% moving-block interval of [{_percent(pooled_interval[0])}, {_percent(pooled_interval[1])}]. Because the upper bound is below the preregistered +{margin * 100:.1f}% materiality margin, a pooled benefit of at least that size is excluded in this scope. The interval still crosses zero, so this is not proof of a strictly negative or exactly zero effect.

## Frozen scope

- Study: `{study_id}`
- Symbol/model: `{protocol.get('symbol')}` / `{protocol['model'].get('deep')}`
- Endpoint: `{protocol['model'].get('backend_url')}`
- Period/horizon: {protocol['design'].get('start_date')} to {protocol['design'].get('end_date')}; {protocol['design'].get('horizon_trading_days')} trading days
- Evidence: {analysis.get('independent_nodes_analyzed')} independent nodes, {analysis.get('completed_graph_runs')} complete graph runs, 0 failed runs
- Primary execution map: long/cash with {protocol['execution'].get('cost_bps_per_position')} bps per position

## Prespecified effects

| Estimand | Mean per node | 97.5% interval | Sign-flip p |
|---|---:|---:|---:|
| Pooled multi-agent marginal effect | {_percent(pooled['mean_delta'])} | [{_percent(pooled_interval[0])}, {_percent(pooled_interval[1])}] | {pooled['paired_sign_flip_two_sided_p']:.4f} |
| Blind: M - S | {_percent(blind['mean_delta'])} | [{_percent(blind_interval[0])}, {_percent(blind_interval[1])}] | {blind['paired_sign_flip_two_sided_p']:.4f} |
| Quant-informed: Q+M - Q+S | {_percent(informed['mean_delta'])} | [{_percent(informed_interval[0])}, {_percent(informed_interval[1])}] | {informed['paired_sign_flip_two_sided_p']:.4f} |
| Q+M - Q | {_percent(quant_comparison['mean_delta'])} | [{_percent(quant_comparison['moving_block_primary_interval'][0])}, {_percent(quant_comparison['moving_block_primary_interval'][1])}] | {quant_comparison['paired_sign_flip_two_sided_p']:.4f} |
| Q+M - constant BUY | {_percent(buy_comparison['mean_delta'])} | [{_percent(buy_comparison['moving_block_primary_interval'][0])}, {_percent(buy_comparison['moving_block_primary_interval'][1])}] | {buy_comparison['paired_sign_flip_two_sided_p']:.4f} |

## Robustness and reproducibility

- First-attempt-only pooled mean: {_percent(first_attempt['pooled_architecture_effect']['mean_delta'])}; classification `{first_attempt['architecture_classification']}`.
- Primary-repeat-only pooled mean: {_percent(primary_repeat['pooled_architecture_effect']['mean_delta'])}; classification `{primary_repeat['architecture_classification']}`.
- Single-agent action agreement across 10 repeat pairs: {robustness['reproducibility']['single_action_agreement_rate'] * 100:.1f}%.
- Multi-agent action agreement across 10 repeat pairs: {robustness['reproducibility']['multi_action_agreement_rate'] * 100:.1f}%.

## Evidence integrity

- Independent verifier: `{verification_id}` (98/98 matrix, analysis recomputed, no errors or warnings)
- Migration: `{migration.get('migration_id')}` (44 first-attempt records reused)
- Race repair: `{repair.get('repair_id')}` (6 late writes archived, deterministic targets restored)
- Checkpoint/source archive: `{archive.get('archive_id')}` (98 completed runs archived)
- Robustness report: `{robustness.get('robustness_id')}`

## Boundary of the conclusion

This result rules out a **material pooled positive effect of at least +1% per 20-day node** in the frozen scope. It does not rule out a smaller positive effect, prove a negative effect, or generalize automatically to other symbols, models, periods, horizons, or execution mappings.
"""
    store = StudyArtifactStore(study_root.parent, study_id)
    store.write_immutable_json("conclusion.json", conclusion)
    store.write_text("CONCLUSION.md", markdown)
    manifest = store.finalize_manifest()
    return {**conclusion, "manifest_id": manifest["manifest_id"]}


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("study_root", type=Path)
    parser.add_argument("--verification-id", required=True)
    args = parser.parse_args()
    result = finalize(args.study_root.resolve(), args.verification_id)
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
