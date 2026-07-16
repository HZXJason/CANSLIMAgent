from datetime import date
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import httpx
import pandas as pd

import main
from quant_trend.artifacts import QuantArtifactStore, stable_json_hash
from quant_trend.config import DEFAULT_STRATEGY_CONFIG
from quant_trend.data import CachedPriceSource, PriceDataError, normalize_price_frame
from quant_trend.evaluation import evaluate_trend_frame
from quant_trend.experiments import (
    ExperimentArm,
    build_experiment_context,
    build_experiment_manifest,
    build_outcome_record,
    build_prediction_record,
    compare_predictions,
    decision_node_id,
)
from quant_trend.service import QuantTrendService
from quant_trend.strategy import FactorState, TrendStrategy


def price_frame(values, *, start="2018-01-02"):
    close = pd.Series([float(value) for value in values])
    index = pd.bdate_range(start, periods=len(close))
    return pd.DataFrame(
        {
            "Open": close.to_numpy(),
            "High": (close * 1.002).to_numpy(),
            "Low": (close * 0.998).to_numpy(),
            "Close": close.to_numpy(),
            "Adj Close": close.to_numpy(),
            "Volume": [1_000_000.0] * len(close),
        },
        index=index,
    )


def geometric_frame(*, rows=700, daily_return=0.003, start="2018-01-02"):
    values = [100.0 * ((1.0 + daily_return) ** offset) for offset in range(rows)]
    return price_frame(values, start=start)


class FakePriceSource:
    provider_id = "fixture.adjusted_daily.v1"

    def __init__(self, frames):
        self.frames = {symbol.upper(): normalize_price_frame(frame, symbol=symbol) for symbol, frame in frames.items()}
        self.calls = []

    def history(self, symbol, start, end):
        safe = symbol.upper()
        self.calls.append((safe, start, end))
        if safe not in self.frames:
            raise PriceDataError(f"{safe}: missing fixture")
        frame = self.frames[safe]
        return frame.loc[(frame.index.date >= start) & (frame.index.date <= end)].copy()

    def history_many(self, symbols, start, end):
        result = {}
        for symbol in symbols:
            try:
                result[symbol.upper()] = self.history(symbol, start, end)
            except PriceDataError:
                continue
        return result


class TrendStrategyTest(unittest.TestCase):
    def test_normalization_preserves_price_and_total_return_closes(self):
        frame = pd.DataFrame(
            {
                "Open": [100, 101],
                "High": [102, 103],
                "Low": [99, 100],
                "Close": [101, 102],
                "Adj Close": [90, 91.5],
            },
            index=pd.bdate_range("2024-01-02", periods=2),
        )
        normalized = normalize_price_frame(frame, symbol="TEST")
        self.assertEqual(normalized["close"].tolist(), [101.0, 102.0])
        self.assertEqual(normalized["total_return_close"].tolist(), [90.0, 91.5])

    def test_normalization_requires_an_explicit_total_return_series(self):
        frame = price_frame([100, 101]).drop(columns=["Adj Close"])
        with self.assertRaisesRegex(PriceDataError, "total-return close"):
            normalize_price_frame(frame, symbol="TEST")

    def test_normalization_rejects_invalid_daily_ohlc_and_duplicate_dates(self):
        invalid = price_frame([100, 101])
        invalid.iloc[0, invalid.columns.get_loc("High")] = 99.0
        with self.assertRaisesRegex(PriceDataError, "invalid OHLC"):
            normalize_price_frame(invalid, symbol="TEST")

        duplicate = price_frame([100, 101])
        extra = duplicate.iloc[[0]].copy()
        extra.index = extra.index + pd.Timedelta(hours=12)
        with self.assertRaisesRegex(PriceDataError, "one row per date"):
            normalize_price_frame(pd.concat([duplicate, extra]), symbol="TEST")

        non_finite = price_frame([100, 101])
        non_finite.iloc[0, non_finite.columns.get_loc("Close")] = float("inf")
        normalized = normalize_price_frame(non_finite, symbol="TEST")
        self.assertEqual(len(normalized), 1)

    def test_factor_lookbacks_use_the_exact_frozen_sessions(self):
        strategy = TrendStrategy()
        frame = normalize_price_frame(geometric_frame(rows=400, daily_return=0), symbol="SENTINEL")
        index = 300
        column = frame.columns.get_loc("total_return_close")
        frame.iloc[index, column] = 210.0
        frame.iloc[index - 21, column] = 70.0
        frame.iloc[index - 63, column] = 105.0
        frame.iloc[index - 252, column] = 42.0
        frame.iloc[index - 251, column] = 35.0
        factors = strategy.factors_at(frame, index)
        self.assertAlmostEqual(factors.tsm_1m, 2.0)
        self.assertAlmostEqual(factors.tsm_3m, 1.0)
        self.assertAlmostEqual(factors.tsm_12m, 4.0)
        self.assertAlmostEqual(factors.absolute_momentum_12_2, 1.0)

    def test_up_and_down_trends_follow_canonical_momentum_direction(self):
        strategy = TrendStrategy()
        rising = normalize_price_frame(geometric_frame(), symbol="UP")
        falling = normalize_price_frame(geometric_frame(daily_return=-0.002), symbol="DOWN")
        self.assertEqual(strategy.predict_at(rising, 500)["action"], "BUY")
        self.assertEqual(strategy.predict_at(falling, 500)["action"], "SELL")

        disagreement = FactorState(
            as_of_index=300,
            as_of_date=date(2024, 1, 2),
            close=80,
            tsm_1m=0.03,
            tsm_3m=0.08,
            tsm_12m=0.20,
            absolute_momentum_12_2=0.20,
            fast_ma=90,
            slow_ma=100,
            fast_ma_slope=-0.02,
            price_to_52w_high=0.95,
            atr_percent=0.02,
        )
        classified = strategy.classify(disagreement)
        self.assertEqual(classified["action"], "BUY")
        self.assertEqual(classified["component_actions"]["moving_average_regime"], "SELL")

    def test_future_rows_cannot_change_a_historical_prediction(self):
        strategy = TrendStrategy()
        original = normalize_price_frame(geometric_frame(), symbol="TEST")
        index = 400
        before = strategy.predict_at(original, index)
        mutated = original.copy()
        for column in ("open", "high", "low", "close", "total_return_close"):
            mutated.iloc[index + 1 :, mutated.columns.get_loc(column)] *= 0.01
        after = strategy.predict_at(mutated, index)
        self.assertEqual(before, after)

    def test_exact_minimum_history_is_sufficient(self):
        strategy = TrendStrategy()
        required = DEFAULT_STRATEGY_CONFIG.required_history_bars
        frame = normalize_price_frame(geometric_frame(rows=required), symbol="BOUNDARY")
        result = strategy.predict_at(frame, required - 1)
        self.assertEqual(result["action"], "BUY")
        with self.assertRaisesRegex(PriceDataError, "needs"):
            strategy.predict_at(frame.iloc[:-1], required - 2)


class TrendEvaluationTest(unittest.TestCase):
    def test_single_node_forms_at_t_enters_at_t_plus_one_and_ends_at_t_plus_h(self):
        frame = normalize_price_frame(geometric_frame(rows=400), symbol="UP")
        decision_index = 300
        frame.iloc[decision_index + 1, frame.columns.get_loc("open")] = 123.456
        frame.iloc[decision_index + 1, frame.columns.get_loc("low")] = 123.0
        result = evaluate_trend_frame(
            frame,
            symbol="UP",
            start_date=frame.index[decision_index].date(),
            end_date=frame.index[decision_index].date(),
            horizon_days=5,
            transaction_cost_bps=0,
        )
        node = result["nodes"][0]
        self.assertEqual(node["as_of_date"], frame.index[decision_index].date().isoformat())
        self.assertEqual(node["entry_date"], frame.index[decision_index + 1].date().isoformat())
        self.assertAlmostEqual(node["entry_open"], 123.456)
        self.assertEqual(node["outcome_date"], frame.index[decision_index + 5].date().isoformat())

    def test_evaluation_uses_non_overlapping_next_open_nodes(self):
        frame = geometric_frame(rows=750, daily_return=0.003)
        normalized = normalize_price_frame(frame, symbol="UP")
        start = normalized.index[300].date()
        end = normalized.index[650].date()
        result = evaluate_trend_frame(
            normalized,
            symbol="UP",
            start_date=start,
            end_date=end,
            horizon_days=20,
            transaction_cost_bps=0,
        )
        self.assertEqual(result["step_trading_days"], 20)
        self.assertGreater(result["path_metrics"]["directional_nodes"], 5)
        self.assertGreater(result["path_metrics"]["directional_accuracy"], 0.9)
        for node in result["nodes"]:
            self.assertGreater(node["entry_date"], node["as_of_date"])

    def test_constant_baselines_are_reported_beside_quant_accuracy(self):
        frame = normalize_price_frame(geometric_frame(rows=700), symbol="UP")
        result = evaluate_trend_frame(
            frame,
            symbol="UP",
            start_date=frame.index[300].date(),
            end_date=frame.index[600].date(),
            horizon_days=20,
        )
        baselines = result["path_metrics"]["constant_action_baselines"]
        self.assertEqual(set(baselines), {"BUY", "HOLD", "SELL"})
        self.assertEqual(set(result["strategy_performance_baselines"]), {"BUY", "HOLD", "SELL"})
        self.assertIsNotNone(result["path_metrics"]["balanced_accuracy"])

    def test_overlapping_windows_are_rejected(self):
        frame = normalize_price_frame(geometric_frame(rows=700), symbol="UP")
        with self.assertRaisesRegex(ValueError, "do not overlap"):
            evaluate_trend_frame(
                frame,
                symbol="UP",
                start_date=frame.index[300].date(),
                end_date=frame.index[600].date(),
                horizon_days=20,
                step_days=1,
            )

    def test_ambiguous_path_is_not_removed_from_return_performance(self):
        frame = normalize_price_frame(geometric_frame(rows=400), symbol="UP")
        decision_index = 300
        frame.iloc[decision_index + 1, frame.columns.get_loc("high")] = frame["open"].iloc[decision_index + 1] * 1.20
        frame.iloc[decision_index + 1, frame.columns.get_loc("low")] = frame["open"].iloc[decision_index + 1] * 0.80
        result = evaluate_trend_frame(
            frame,
            symbol="UP",
            start_date=frame.index[decision_index].date(),
            end_date=frame.index[decision_index].date(),
            horizon_days=20,
        )
        self.assertEqual(result["ambiguous_nodes"], 1)
        self.assertEqual(result["path_metrics"]["scored_nodes"], 0)
        self.assertEqual(result["strategy_performance"]["active_signals"], 1)

    def test_price_labels_and_total_return_position_sizing_are_separate(self):
        frame = normalize_price_frame(price_frame([100.0] * 400), symbol="MIXED")
        decision_index = 300
        total_return_column = frame.columns.get_loc("total_return_close")
        frame.iloc[decision_index, total_return_column] = 100.0
        frame.iloc[decision_index - 21, total_return_column] = 90.0
        frame.iloc[decision_index - 63, total_return_column] = 80.0
        frame.iloc[decision_index - 252, total_return_column] = 110.0
        for offset, value in enumerate((100.0, 102.0, 104.0, 106.0, 109.0), start=1):
            frame.iloc[decision_index + offset, total_return_column] = value
        result = evaluate_trend_frame(
            frame,
            symbol="MIXED",
            start_date=frame.index[decision_index].date(),
            end_date=frame.index[decision_index].date(),
            horizon_days=5,
            transaction_cost_bps=0,
        )
        node = result["nodes"][0]
        self.assertEqual(node["prediction"], "BUY")
        self.assertAlmostEqual(node["trend_score"], 1 / 3)
        self.assertEqual(node["trend_label"], "HOLD")
        self.assertAlmostEqual(node["endpoint_return"], 0.0)
        self.assertAlmostEqual(node["endpoint_total_return"], 0.09)
        self.assertAlmostEqual(result["strategy_performance"]["total_compounded_return"], 0.03)
        self.assertAlmostEqual(
            result["strategy_performance_baselines"]["BUY"]["total_compounded_return"],
            0.09,
        )

    def test_full_short_baseline_is_liquidated_instead_of_compounding_negative_equity(self):
        frame = normalize_price_frame(price_frame([100.0] * 400), symbol="SQUEEZE")
        decision_index = 300
        for column in ("close", "total_return_close"):
            frame.iloc[decision_index + 5, frame.columns.get_loc(column)] = 250.0
        frame.iloc[decision_index + 5, frame.columns.get_loc("high")] = 251.0
        result = evaluate_trend_frame(
            frame,
            symbol="SQUEEZE",
            start_date=frame.index[decision_index].date(),
            end_date=frame.index[decision_index].date(),
            horizon_days=5,
            transaction_cost_bps=0,
        )
        short = result["strategy_performance_baselines"]["SELL"]
        self.assertTrue(short["liquidated"])
        self.assertEqual(short["total_compounded_return"], -1.0)


class QuantTrendServiceTest(unittest.TestCase):
    def test_price_cache_reuses_a_covered_range(self):
        frame = geometric_frame(rows=400)
        source = FakePriceSource({"MU": frame})
        start = frame.index[50].date()
        end = frame.index[350].date()
        with tempfile.TemporaryDirectory() as directory:
            cached = CachedPriceSource(source, directory)
            first = cached.history("MU", start, end)
            calls_after_first = len(source.calls)
            second = cached.history("MU", frame.index[100].date(), frame.index[300].date())
        self.assertEqual(calls_after_first, len(source.calls))
        pd.testing.assert_series_equal(
            first.loc[second.index, "close"], second["close"], check_exact=False, rtol=1e-12
        )

    def test_snapshot_is_content_addressed_and_reused(self):
        frame = geometric_frame(rows=700)
        source = FakePriceSource({"MU": frame, "SPY": frame})
        as_of = frame.index[600].date()
        with tempfile.TemporaryDirectory() as directory:
            store = QuantArtifactStore(directory)
            service = QuantTrendService(
                data_source=source,
                artifact_store=store,
            )
            first = service.predict(symbol="MU", as_of=as_of)
            calls_after_first = len(source.calls)
            second = service.predict(symbol="MU", as_of=as_of)
            preserved = store.load_snapshot_by_id(first["snapshot_id"])
        self.assertEqual(first, second)
        self.assertEqual(first, preserved)
        self.assertEqual(first["snapshot_id"], second["snapshot_id"])
        self.assertEqual(calls_after_first, len(source.calls))
        self.assertIsNone(first["probabilities"])

    def test_service_normalizes_unsorted_provider_frames_before_indexing(self):
        frame = geometric_frame(rows=700)

        class DirectSource:
            provider_id = "fixture.raw_daily.v1"

            def __init__(self, reverse):
                self.reverse = reverse

            def _frame(self, start, end):
                subset = frame.loc[(frame.index.date >= start) & (frame.index.date <= end)].copy()
                return subset.iloc[::-1] if self.reverse else subset

            def history(self, symbol, start, end):
                return self._frame(start, end)

            def history_many(self, symbols, start, end):
                return {symbol.lower(): self._frame(start, end) for symbol in symbols}

        as_of = frame.index[600].date()
        with tempfile.TemporaryDirectory() as first_directory, tempfile.TemporaryDirectory() as second_directory:
            ordered = QuantTrendService(
                data_source=DirectSource(False),
                artifact_store=QuantArtifactStore(first_directory),
            ).predict(symbol="MU", as_of=as_of)
            reversed_result = QuantTrendService(
                data_source=DirectSource(True),
                artifact_store=QuantArtifactStore(second_directory),
            ).predict(symbol="MU", as_of=as_of)
        self.assertEqual(ordered["snapshot_id"], reversed_result["snapshot_id"])
        self.assertEqual(ordered["data_hash"], reversed_result["data_hash"])
        self.assertEqual(ordered["factors"], reversed_result["factors"])

    def test_request_cache_rejects_a_valid_snapshot_for_another_symbol(self):
        frame = geometric_frame(rows=700)
        source = FakePriceSource({"MU": frame, "SPY": frame})
        as_of = frame.index[600].date()
        with tempfile.TemporaryDirectory() as directory:
            service = QuantTrendService(
                data_source=source,
                artifact_store=QuantArtifactStore(directory),
            )
            first = service.predict(symbol="MU", as_of=as_of)
            calls_after_first = len(source.calls)
            artifact = next((Path(directory) / "snapshot-requests").glob("*.json"))
            wrong = {**first, "symbol": "AAPL"}
            wrong["snapshot_id"] = stable_json_hash(
                {key: value for key, value in wrong.items() if key != "snapshot_id"}
            )
            artifact.write_text(json.dumps(wrong), encoding="utf-8")
            repaired = service.predict(symbol="MU", as_of=as_of)
        self.assertGreater(len(source.calls), calls_after_first)
        self.assertEqual(repaired["symbol"], "MU")
        self.assertEqual(repaired["snapshot_id"], first["snapshot_id"])

    def test_force_refresh_reloads_the_upstream_price_cache_and_preserves_both_snapshots(self):
        frame = geometric_frame(rows=700)
        source = FakePriceSource({"MU": frame, "SPY": frame})
        as_of = frame.index[600].date()
        with tempfile.TemporaryDirectory() as directory:
            store = QuantArtifactStore(Path(directory) / "artifacts")
            service = QuantTrendService(
                data_source=CachedPriceSource(source, Path(directory) / "prices"),
                artifact_store=store,
            )
            first = service.predict(symbol="MU", as_of=as_of)
            calls_after_first = len(source.calls)
            source.frames["MU"].iloc[
                590,
                source.frames["MU"].columns.get_loc("total_return_close"),
            ] *= 0.9
            refreshed = service.predict(symbol="MU", as_of=as_of, force_refresh=True)
            first_preserved = store.load_snapshot_by_id(first["snapshot_id"])
            refreshed_preserved = store.load_snapshot_by_id(refreshed["snapshot_id"])
        self.assertGreater(len(source.calls), calls_after_first)
        self.assertNotEqual(first["data_hash"], refreshed["data_hash"])
        self.assertNotEqual(first["snapshot_id"], refreshed["snapshot_id"])
        self.assertEqual(first_preserved, first)
        self.assertEqual(refreshed_preserved, refreshed)

    def test_benchmark_is_part_of_the_snapshot_cache_key(self):
        frame = geometric_frame(rows=700)
        source = FakePriceSource({"MU": frame, "SPY": frame, "QQQ": geometric_frame(rows=700, daily_return=0.001)})
        as_of = frame.index[600].date()
        with tempfile.TemporaryDirectory() as directory:
            store = QuantArtifactStore(directory)
            spy = QuantTrendService(data_source=source, artifact_store=store, benchmark="SPY").predict(
                symbol="MU", as_of=as_of
            )
            qqq = QuantTrendService(data_source=source, artifact_store=store, benchmark="QQQ").predict(
                symbol="MU", as_of=as_of
            )
        self.assertEqual(spy["benchmark"], "SPY")
        self.assertEqual(qqq["benchmark"], "QQQ")
        self.assertNotEqual(spy["snapshot_id"], qqq["snapshot_id"])

    def test_short_history_is_explicitly_unscorable_not_hold(self):
        frame = geometric_frame(rows=100)
        source = FakePriceSource({"MU": frame, "SPY": frame})
        with tempfile.TemporaryDirectory() as directory:
            service = QuantTrendService(
                data_source=source,
                artifact_store=QuantArtifactStore(directory),
            )
            result = service.predict(symbol="MU", as_of=frame.index[-1].date())
        self.assertEqual(result["status"], "insufficient_data")
        self.assertIsNone(result["action"])

    def test_evaluation_writes_versioned_run_card(self):
        frame = geometric_frame(rows=900)
        source = FakePriceSource({"MU": frame, "SPY": frame})
        with tempfile.TemporaryDirectory() as directory:
            service = QuantTrendService(
                data_source=source,
                artifact_store=QuantArtifactStore(directory),
            )
            normalized = source.frames["MU"]
            result = service.evaluate(
                symbol="MU",
                start_date=normalized.index[300].date(),
                end_date=normalized.index[750].date(),
                horizon_days=20,
            )
            artifact = Path(result["artifact_path"])
            payload = json.loads(artifact.read_text(encoding="utf-8"))
        self.assertEqual(payload["run_id"], result["run_id"])
        self.assertEqual(payload["protocol"]["strategy_version"], "daily_tsm_sign_21_63_252.v1")
        self.assertIn("data_hash", payload)

    def test_evaluation_run_id_ignores_rows_after_the_last_outcome(self):
        frame = geometric_frame(rows=900)
        source = FakePriceSource({"MU": frame, "SPY": frame})
        normalized = source.frames["MU"]
        with tempfile.TemporaryDirectory() as directory:
            service = QuantTrendService(
                data_source=source,
                artifact_store=QuantArtifactStore(directory),
            )
            first = service.evaluate(
                symbol="MU",
                start_date=normalized.index[300].date(),
                end_date=normalized.index[600].date(),
                horizon_days=20,
            )
            last_outcome = pd.Timestamp(first["nodes"][-1]["outcome_date"])
            tail = source.frames["MU"].index > last_outcome
            for column in ("open", "high", "low", "close", "total_return_close"):
                source.frames["MU"].loc[tail, column] *= 1.5
            second = service.evaluate(
                symbol="MU",
                start_date=normalized.index[300].date(),
                end_date=normalized.index[600].date(),
                horizon_days=20,
            )
        self.assertEqual(first["run_id"], second["run_id"])
        self.assertEqual(first["data_hash"], second["data_hash"])


class AblationContractTest(unittest.TestCase):
    def setUp(self):
        snapshot = {
            "schema_version": "quant.trend_snapshot.v1",
            "status": "ok",
            "strategy_version": "daily_tsm_sign_21_63_252.v1",
            "config_hash": "a" * 64,
            "data_source": "fixture.adjusted_daily.v1",
            "benchmark": "SPY",
            "data_hash": "b" * 64,
            "symbol": "MU",
            "requested_as_of_date": "2024-06-28",
            "as_of_date": "2024-06-28",
            "horizon_trading_days": 20,
            "action": "BUY",
            "trend_score": 1 / 3,
        }
        self.snapshot = {**snapshot, "snapshot_id": stable_json_hash(snapshot)}

    def test_blind_and_informed_contexts_differ_only_by_quant_snapshot(self):
        original = {"model_calibration": {"accuracy": 0.9}, "trade_journal": {"records": []}}
        blind = build_experiment_context(ExperimentArm.TA_BLIND, self.snapshot, original)
        informed = build_experiment_context(ExperimentArm.QUANT_INFORMED, self.snapshot, original)
        self.assertNotIn("model_calibration", blind)
        self.assertNotIn("model_calibration", informed)
        self.assertNotIn("quant_trend_baseline", blind)
        self.assertEqual(informed["quant_trend_baseline"], self.snapshot)
        self.assertEqual(blind["research_experiment"]["node_id"], informed["research_experiment"]["node_id"])
        informed_without_snapshot = dict(informed)
        informed_without_snapshot.pop("quant_trend_baseline")
        self.assertEqual(blind, informed_without_snapshot)
        self.assertNotIn("arm", blind["research_experiment"])

    def test_same_snapshot_can_be_reused_across_model_fingerprints(self):
        first = build_prediction_record(
            ExperimentArm.TA_BLIND,
            self.snapshot,
            action="SELL",
            model_fingerprint="model-a",
        )
        second = build_prediction_record(
            ExperimentArm.TA_BLIND,
            self.snapshot,
            action="HOLD",
            model_fingerprint="model-b",
        )
        self.assertEqual(first["node_id"], second["node_id"])
        self.assertIsNone(first["quant_snapshot_id"])
        self.assertIsNone(second["quant_snapshot_id"])
        self.assertNotEqual(first["prediction_id"], second["prediction_id"])

    def test_decision_node_is_strategy_independent_and_outcomes_are_versioned(self):
        changed_core = {
            **{key: value for key, value in self.snapshot.items() if key != "snapshot_id"},
            "strategy_version": "replacement-strategy.v2",
            "config_hash": "c" * 64,
            "data_hash": "d" * 64,
        }
        changed = {**changed_core, "snapshot_id": stable_json_hash(changed_core)}
        self.assertEqual(decision_node_id(self.snapshot), decision_node_id(changed))

        protocol = {
            "schema_version": "quant.label_protocol.v1",
            "entry_rule": "next_session_open",
            "horizon_trading_days": 20,
            "neutral_band": 0.01,
        }
        first = build_outcome_record(
            self.snapshot,
            label_protocol=protocol,
            future_path_hash="e" * 64,
            labels={"trend": "BUY"},
        )
        different_protocol = build_outcome_record(
            self.snapshot,
            label_protocol={**protocol, "neutral_band": 0.02},
            future_path_hash="e" * 64,
            labels={"trend": "HOLD"},
        )
        different_path = build_outcome_record(
            self.snapshot,
            label_protocol=protocol,
            future_path_hash="f" * 64,
            labels={"trend": "SELL"},
        )
        self.assertNotEqual(first["outcome_id"], different_protocol["outcome_id"])
        self.assertNotEqual(first["outcome_id"], different_path["outcome_id"])

    def test_prediction_outcome_and_manifest_have_atomic_persistence_boundaries(self):
        context = build_experiment_context(
            ExperimentArm.TA_BLIND,
            self.snapshot,
            {"market_data": {"as_of": "2024-06-28"}},
        )
        prediction = build_prediction_record(
            ExperimentArm.TA_BLIND,
            self.snapshot,
            action="HOLD",
            model_fingerprint="provider:model:prompt-v1:temperature-0",
        )
        outcome = build_outcome_record(
            self.snapshot,
            label_protocol={"horizon_trading_days": 20, "entry_rule": "next_session_open"},
            future_path_hash="e" * 64,
            labels={"trend": "BUY"},
        )
        manifest = build_experiment_manifest(
            self.snapshot,
            common_context_hash=context["research_experiment"]["common_context_hash"],
            model_protocol_id="provider:model:prompt-v1:temperature-0",
            repeat_count=3,
            outcome_id=outcome["outcome_id"],
        )
        with tempfile.TemporaryDirectory() as directory:
            store = QuantArtifactStore(directory)
            prediction_path = store.save_prediction(prediction)
            outcome_path = store.save_outcome(outcome)
            manifest_path = store.save_experiment_manifest(manifest)
            self.assertTrue(prediction_path.is_file())
            self.assertTrue(outcome_path.is_file())
            self.assertTrue(manifest_path.is_file())

    def test_comparison_rejects_quant_only_and_context_rejects_nested_outcomes(self):
        with self.assertRaisesRegex(ValueError, "ta_blind or quant_informed"):
            compare_predictions(
                self.snapshot,
                agent_action="BUY",
                arm=ExperimentArm.QUANT_ONLY,
            )
        with self.assertRaisesRegex(ValueError, "reserved experiment field"):
            build_experiment_context(
                ExperimentArm.TA_BLIND,
                self.snapshot,
                {"market_data": {"future_returns": [0.1]}},
            )

    def test_invalid_experiment_records_fail_closed(self):
        with self.assertRaisesRegex(ValueError, "must equal"):
            build_prediction_record(ExperimentArm.QUANT_ONLY, self.snapshot, action="SELL")
        with self.assertRaisesRegex(ValueError, "model fingerprint"):
            build_prediction_record(ExperimentArm.TA_BLIND, self.snapshot, action="BUY")
        with self.assertRaisesRegex(ValueError, "between 0 and 99"):
            build_prediction_record(
                ExperimentArm.TA_BLIND,
                self.snapshot,
                action="BUY",
                model_fingerprint="model-a",
                repeat_index=-1,
            )

    def test_quant_package_has_no_llm_or_api_key_dependency(self):
        package = Path(__file__).with_name("quant_trend")
        source = "\n".join(path.read_text(encoding="utf-8") for path in package.glob("*.py"))
        self.assertNotIn("langchain", source.lower())
        self.assertNotIn("openai_api_key", source.lower())
        self.assertNotIn("anthropic_api_key", source.lower())


class QuantTrendAPITest(unittest.IsolatedAsyncioTestCase):
    async def test_predict_endpoint_does_not_require_an_llm_request(self):
        class FakeService:
            def predict(self, **kwargs):
                return {
                    "schema_version": "quant.trend_snapshot.v1",
                    "status": "ok",
                    "symbol": kwargs["symbol"],
                    "action": "BUY",
                    "probabilities": None,
                }

        with patch.dict(main.os.environ, {"SIDECAR_SHARED_SECRET": "s" * 32}, clear=False), patch.object(
            main, "quant_trend_service", FakeService()
        ):
            transport = httpx.ASGITransport(app=main.app)
            async with httpx.AsyncClient(transport=transport, base_url="http://sidecar.test") as client:
                response = await client.post(
                    "/quant/trend/predict",
                    headers={main.SIDECAR_TOKEN_HEADER: "s" * 32},
                    json={"symbol": "MU", "as_of_date": "2024-06-28", "horizon_days": 20},
                )
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.json()["action"], "BUY")

    async def test_evaluate_endpoint_exposes_artifact_id_not_local_path(self):
        class FakeService:
            def evaluate(self, **kwargs):
                return {
                    "schema_version": "quant.trend_evaluation.v1",
                    "run_id": "run-123",
                    "artifact_path": "/private/sidecar/data/run-123.json",
                }

        with patch.dict(main.os.environ, {"SIDECAR_SHARED_SECRET": "s" * 32}, clear=False), patch.object(
            main, "quant_trend_service", FakeService()
        ):
            transport = httpx.ASGITransport(app=main.app)
            async with httpx.AsyncClient(transport=transport, base_url="http://sidecar.test") as client:
                response = await client.post(
                    "/quant/trend/evaluate",
                    headers={main.SIDECAR_TOKEN_HEADER: "s" * 32},
                    json={
                        "symbol": "MU",
                        "start_date": "2020-01-01",
                        "end_date": "2024-12-31",
                    },
                )
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.json()["artifact_id"], "run-123")
        self.assertNotIn("artifact_path", response.json())


if __name__ == "__main__":
    unittest.main()
