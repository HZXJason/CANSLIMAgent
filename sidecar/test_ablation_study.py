from __future__ import annotations

from datetime import date, timedelta
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import pandas as pd

from ablation_study.artifacts import StudyArtifactStore
from ablation_study.design import (
    build_staged_design,
    evenly_spaced_positions,
    minimum_detectable_effect,
    stage_one_stopping_decision,
    staged_task_keys,
)
from ablation_study.runner import parse_explicit_action, run_study
from ablation_study.statistics import analyze_study_records
from ablation_study.verifier import verify_manifest, verify_study
from main import AblationStudyRequest
from quant_trend.artifacts import QuantArtifactStore
from quant_trend.data import PriceDataError, normalize_price_frame
from quant_trend.service import QuantTrendService
from tradingagents.dataflows.frozen_ohlcv import frozen_ohlcv_scope
from tradingagents.dataflows.stockstats_utils import load_ohlcv
from tradingagents.dataflows.y_finance import get_YFin_data_online


def protocol() -> dict:
    nodes = [
        {"node_id": "a" * 64, "as_of_date": "2024-01-02", "quant_action": "BUY"},
        {"node_id": "b" * 64, "as_of_date": "2024-02-01", "quant_action": "SELL"},
        {"node_id": "c" * 64, "as_of_date": "2024-03-01", "quant_action": "HOLD"},
    ]
    return {
        "study_id": "d" * 64,
        "nodes": nodes,
        "execution": {"cost_bps_per_position": 10},
        "statistics": {
            "bootstrap_iterations": 200,
            "randomization_iterations": 500,
            "block_length_nodes": 2,
            "seed": 20260716,
            "equivalence_margin_per_node": 0.01,
            "classification_rule": "test rule",
        },
    }


class ParseDecisionTests(unittest.TestCase):
    def test_only_explicit_decisions_are_parsed(self):
        self.assertEqual(parse_explicit_action("FINAL TRANSACTION PROPOSAL: **BUY**"), "BUY")
        self.assertEqual(parse_explicit_action("Rating: Underweight"), "SELL")
        self.assertEqual(
            parse_explicit_action("**方向：看涨**\n**评级：持有**"), "HOLD"
        )
        self.assertEqual(parse_explicit_action("行动：买入"), "BUY")
        self.assertEqual(parse_explicit_action("最终交易建议：卖出"), "SELL")
        self.assertEqual(
            parse_explicit_action("**FINAL TRANSACTION PROPOSAL: HOLD**"), "HOLD"
        )
        self.assertIsNone(parse_explicit_action("We considered SELL but rejected it."))


class ArtifactTests(unittest.TestCase):
    def test_secrets_are_rejected_and_immutable_content_is_enforced(self):
        with tempfile.TemporaryDirectory() as directory:
            store = StudyArtifactStore(directory, "a" * 64)
            store.write_immutable_json("protocol.json", {"schema": "v1"})
            store.write_immutable_json("protocol.json", {"schema": "v1"})
            with self.assertRaises(ValueError):
                store.write_immutable_json("protocol.json", {"schema": "v2"})
            with self.assertRaises(ValueError):
                store.write_json("bad.json", {"api_key": "must-not-be-written"})

    def test_manifest_accepts_a_real_zero_byte_file(self):
        with tempfile.TemporaryDirectory() as directory:
            store = StudyArtifactStore(directory, "a" * 64)
            store.path("empty.py").parent.mkdir(parents=True, exist_ok=True)
            store.path("empty.py").touch()
            store.finalize_manifest()

            self.assertEqual(verify_manifest(store.root), [])


class FrozenOHLCVTests(unittest.TestCase):
    def test_price_and_indicator_tools_use_the_run_scoped_frame_without_network(self):
        frame = pd.DataFrame(
            {
                "Date": pd.bdate_range("2024-01-02", periods=40),
                "Open": [100.0 + index for index in range(40)],
                "High": [101.0 + index for index in range(40)],
                "Low": [99.0 + index for index in range(40)],
                "Close": [100.5 + index for index in range(40)],
                "Volume": [1_000_000.0] * 40,
            }
        )
        with frozen_ohlcv_scope("MU", "f" * 64, frame) as frozen, patch(
            "tradingagents.dataflows.y_finance.yf.Ticker",
            side_effect=AssertionError("network must not be used"),
        ), patch(
            "tradingagents.dataflows.stockstats_utils.yf.download",
            side_effect=AssertionError("network must not be used"),
        ):
            prices = get_YFin_data_online("MU", "2024-01-10", "2024-02-15")
            indicators = load_ohlcv("MU.US", "2024-02-15")

        self.assertIn("protocol-frozen canonical OHLCV", prices)
        self.assertFalse(indicators.empty)
        self.assertLessEqual(indicators["Date"].max(), pd.Timestamp("2024-02-15"))
        self.assertEqual(
            {item["purpose"] for item in frozen.usage},
            {"get_stock_data", "technical_indicators"},
        )
        self.assertTrue(all(item["model_data_id"] == "f" * 64 for item in frozen.usage))


class StatisticsTests(unittest.TestCase):
    def test_repeats_are_averaged_within_node(self):
        frozen = protocol()
        outcomes = {
            node["node_id"]: {"endpoint_total_return": value}
            for node, value in zip(frozen["nodes"], (0.10, -0.10, 0.05))
        }
        records = []
        for node in frozen["nodes"]:
            for context in ("blind", "quant_informed"):
                for repeat in (1, 2):
                    records.append(
                        {
                            "status": "complete",
                            "node_id": node["node_id"],
                            "context": context,
                            "repeat": repeat,
                            "single_action": "HOLD",
                            "multi_action": "HOLD",
                        }
                    )
        analysis = analyze_study_records(frozen, records, outcomes)
        self.assertEqual(analysis["independent_nodes_analyzed"], 3)
        self.assertEqual(analysis["completed_graph_runs"], 12)
        self.assertEqual(analysis["effects"]["pooled_architecture_effect"]["nodes"], 3)
        self.assertEqual(analysis["architecture_classification"], "practically_null")


class StagedDesignTests(unittest.TestCase):
    def test_stage_sets_are_nested_and_repeats_do_not_increase_n(self):
        nodes = [
            {"as_of_date": f"2024-01-{index + 1:02d}", "outcome_date": f"2024-02-{index + 1:02d}"}
            for index in range(45)
        ]
        evaluation = {
            "symbol": "MU",
            "horizon_trading_days": 20,
            "run_id": "a" * 64,
            "data_hash": "b" * 64,
            "nodes": nodes,
        }
        design = build_staged_design(evaluation)
        self.assertEqual(sum(node["stage"] == 1 for node in design["node_contracts"]), 30)
        self.assertEqual(sum(node["reproducibility_repeat"] for node in design["node_contracts"]), 5)
        self.assertEqual(design["stage_one_graph_runs"], 70)
        self.assertEqual(design["maximum_graph_runs"], 100)

    def test_spacing_and_mde_are_deterministic(self):
        self.assertEqual(len(evenly_spaced_positions(45, 30)), 30)
        self.assertLess(minimum_detectable_effect(0.04, 45), minimum_detectable_effect(0.04, 30))

    def test_exact_task_matrix_and_stopping_rule_are_deterministic(self):
        nodes = [
            {"node_id": "a" * 64, "stage": 1, "reproducibility_repeat": True},
            {"node_id": "b" * 64, "stage": 1, "reproducibility_repeat": False},
            {"node_id": "c" * 64, "stage": 2, "reproducibility_repeat": False},
        ]
        matrix = staged_task_keys(nodes, ("blind", "quant_informed"))
        self.assertEqual(len(matrix["stage_one"]), 6)
        self.assertEqual(len(matrix["stage_two"]), 2)
        self.assertEqual(
            stage_one_stopping_decision("inconclusive", matrix_complete=True)["advance_to_stage_two"],
            True,
        )
        self.assertEqual(
            stage_one_stopping_decision("positive", matrix_complete=True)["stopped_early"],
            True,
        )
        self.assertEqual(
            stage_one_stopping_decision("practically_null", matrix_complete=False)["reason"],
            "stage_one_matrix_incomplete",
        )


class _FixturePriceSource:
    provider_id = "fixture.ablation.adjusted.v1"

    def __init__(self, frame: pd.DataFrame):
        self.frame = normalize_price_frame(frame, symbol="MU")

    def history(self, symbol: str, start: date, end: date):
        if symbol.upper() not in {"MU", "SPY"}:
            raise PriceDataError("unknown fixture symbol")
        return self.frame.loc[
            (self.frame.index.date >= start) & (self.frame.index.date <= end)
        ].copy()

    def history_many(self, symbols, start: date, end: date):
        return {symbol.upper(): self.history(symbol, start, end) for symbol in symbols}


class _EvidenceCallback:
    def snapshot(self):
        return {
            "tool_calls": [
                {
                    "name": "fixture_point_in_time_market_data",
                    "input": {"end_date": "2020-01-01"},
                    "output": {"last_observation_date": "2020-01-01"},
                }
            ]
        }


class StagedRunnerIntegrationTests(unittest.TestCase):
    def test_practically_null_interim_stops_and_verifies_exact_matrix(self):
        index = pd.bdate_range("2018-01-02", periods=1_200)
        close = pd.Series([100.0 * (1.0005**offset) for offset in range(len(index))])
        frame = pd.DataFrame(
            {
                "Open": close.to_numpy(),
                "High": (close * 1.002).to_numpy(),
                "Low": (close * 0.998).to_numpy(),
                "Close": close.to_numpy(),
                "Adj Close": close.to_numpy(),
                "Volume": [1_000_000.0] * len(index),
            },
            index=index,
        )
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            service = QuantTrendService(
                data_source=_FixturePriceSource(frame),
                artifact_store=QuantArtifactStore(root / "quant"),
            )
            request = AblationStudyRequest(
                ticker="MU",
                llm_provider="openai_compatible",
                deep_think_llm="deepseek-v4-flash",
                quick_think_llm="deepseek-v4-flash",
                backend_url="https://fixture.example/v1",
                analysts=["technical"],
                start_date=date(2021, 1, 4),
                end_date=date(2022, 12, 30),
                horizon_days=20,
                maximum_nodes=8,
                repeats=1,
                concurrency=4,
                adaptive_stages=True,
                stage_one_nodes=5,
                reproducibility_nodes=2,
                max_attempts=1,
            )

            def fake_analysis(_request, _callbacks, *, analysis_date, analysts):
                get_YFin_data_online(
                    "MU",
                    (analysis_date - timedelta(days=30)).isoformat(),
                    analysis_date.isoformat(),
                )
                return (
                    {
                        "market_report": "FINAL TRANSACTION PROPOSAL: **HOLD**",
                        "final_trade_decision": "组合经理结论：持有",
                    },
                    "HOLD",
                )

            result = run_study(
                request,
                quant_service=service,
                run_analysis=fake_analysis,
                callback_factory=_EvidenceCallback,
                artifact_root=root / "studies",
                repo_root=Path(__file__).resolve().parent.parent,
            )
            self.assertEqual(result["status"], "complete")
            self.assertTrue(result["stopped_early"])
            self.assertEqual(result["completed_runs"], 14)
            self.assertEqual(result["maximum_runs"], 20)
            verification = verify_study(result["artifact_root"])
            self.assertTrue(verification["integrity_verified"], verification["errors"])
            self.assertTrue(verification["matrix_complete"])
            self.assertTrue(verification["analysis_verified"])

    def test_inconclusive_interim_expands_to_every_stage_two_node(self):
        index = pd.bdate_range("2018-01-02", periods=1_200)
        close = pd.Series([100.0 * (1.0015**offset) for offset in range(len(index))])
        frame = pd.DataFrame(
            {
                "Open": close.to_numpy(),
                "High": (close * 1.002).to_numpy(),
                "Low": (close * 0.998).to_numpy(),
                "Close": close.to_numpy(),
                "Adj Close": close.to_numpy(),
                "Volume": [1_000_000.0] * len(index),
            },
            index=index,
        )
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            service = QuantTrendService(
                data_source=_FixturePriceSource(frame),
                artifact_store=QuantArtifactStore(root / "quant"),
            )
            request = AblationStudyRequest(
                ticker="MU",
                llm_provider="openai_compatible",
                deep_think_llm="deepseek-v4-flash",
                quick_think_llm="deepseek-v4-flash",
                backend_url="https://fixture.example/v1",
                analysts=["technical"],
                start_date=date(2021, 1, 4),
                end_date=date(2022, 12, 30),
                horizon_days=20,
                maximum_nodes=8,
                repeats=1,
                concurrency=4,
                adaptive_stages=True,
                stage_one_nodes=5,
                reproducibility_nodes=2,
                max_attempts=1,
            )
            preview = service.evaluate(
                symbol="MU",
                start_date=request.start_date,
                end_date=request.end_date,
                horizon_days=request.horizon_days,
                step_days=request.horizon_days,
                maximum_nodes=request.maximum_nodes,
                transaction_cost_bps=request.transaction_cost_bps,
            )
            action_pairs = {
                node["as_of_date"]: (("HOLD", "BUY") if position % 2 == 0 else ("BUY", "HOLD"))
                for position, node in enumerate(preview["nodes"])
            }

            def fake_analysis(_request, _callbacks, *, analysis_date, analysts):
                self.assertEqual(analysts, ("market",))
                get_YFin_data_online(
                    "MU",
                    (analysis_date - timedelta(days=30)).isoformat(),
                    analysis_date.isoformat(),
                )
                single, multi = action_pairs[analysis_date.isoformat()]
                return (
                    {
                        "market_report": f"FINAL TRANSACTION PROPOSAL: **{single}**",
                        "final_trade_decision": f"Rating: {multi}",
                    },
                    multi,
                )

            result = run_study(
                request,
                quant_service=service,
                run_analysis=fake_analysis,
                callback_factory=_EvidenceCallback,
                artifact_root=root / "studies",
                repo_root=Path(__file__).resolve().parent.parent,
            )
            self.assertEqual(result["status"], "complete")
            self.assertFalse(result["stopped_early"])
            self.assertEqual(result["stage"], 2)
            self.assertEqual(result["completed_runs"], 20)
            self.assertEqual(result["architecture_classification"], "practically_null")
            verification = verify_study(result["artifact_root"])
            self.assertTrue(verification["integrity_verified"], verification["errors"])
            self.assertTrue(verification["matrix_complete"])
            self.assertTrue(verification["analysis_verified"])


if __name__ == "__main__":
    unittest.main()
