from __future__ import annotations

import json
import unittest

from journal_reflection import (
    INPUT_SCHEMA_VERSION,
    REFLECTION_SCHEMA_VERSION,
    analyze_trade_records,
    build_journal_reflection,
)


def _record(
    dt: str,
    symbol: str,
    side: str,
    quantity: float,
    price: float,
    *,
    fee: float = 0.0,
    market: str = "us",
    name: str = "",
) -> dict[str, object]:
    return {
        "datetime": dt,
        "symbol": symbol,
        "name": name,
        "side": side,
        "quantity": quantity,
        "price": price,
        "amount": quantity * price,
        "fee": fee,
        "market": market,
    }


class EmptyAndValidationTest(unittest.TestCase):
    def test_empty_input_is_honestly_insufficient_and_json_safe(self):
        result = analyze_trade_records([])

        self.assertEqual(result["schema_version"], REFLECTION_SCHEMA_VERSION)
        self.assertEqual(result["input_schema_version"], INPUT_SCHEMA_VERSION)
        self.assertEqual(result["status"], "insufficient_data")
        self.assertEqual(result["validation"]["valid_records"], 0)
        self.assertIsNone(result["profile"]["win_rate"])
        self.assertEqual(result["behavior"]["disposition_effect"]["severity"], "unknown")
        self.assertFalse(result["strategy_extraction_readiness"]["ready"])
        self.assertIn("没有", result["reflection_text"])
        json.dumps(result, allow_nan=False)

    def test_invalid_required_records_are_skipped_and_optional_fields_recover(self):
        valid_but_incomplete = {
            "datetime": "2026-01-01 10:00:00",
            "symbol": "aapl",
            "side": "buy",
            "quantity": "2",
            "price": "10",
        }
        result = analyze_trade_records(
            [
                None,
                {
                    "datetime": "not-a-date",
                    "symbol": "BAD",
                    "side": "buy",
                    "quantity": 1,
                    "price": 1,
                },
                valid_but_incomplete,
            ]
        )

        self.assertEqual(result["validation"]["input_records"], 3)
        self.assertEqual(result["validation"]["valid_records"], 1)
        self.assertEqual(result["validation"]["invalid_records"], 2)
        self.assertEqual(result["validation"]["skipped_record_indices"], [0, 1])
        codes = {warning["code"] for warning in result["validation_warnings"]}
        self.assertTrue(
            {"record_not_object", "invalid_datetime", "amount_recomputed", "fee_defaulted", "market_inferred"}
            <= codes
        )
        self.assertEqual(result["open_lots"][0]["symbol"], "AAPL")
        self.assertEqual(result["open_lots"][0]["cost_basis"], 20.0)

    def test_invalid_numeric_values_never_leak_nan_or_infinity(self):
        result = analyze_trade_records(
            [
                _record("2026-01-01", "AAPL", "buy", 1, 10),
                {
                    "datetime": "2026-01-02",
                    "symbol": "AAPL",
                    "side": "sell",
                    "quantity": 1,
                    "price": float("nan"),
                    "amount": float("inf"),
                    "fee": -1,
                    "market": "us",
                },
            ]
        )

        self.assertEqual(result["validation"]["invalid_records"], 1)
        self.assertIn("invalid_price", {warning["code"] for warning in result["validation_warnings"]})
        json.dumps(result, allow_nan=False)

    def test_amount_mismatch_is_recomputed(self):
        record = _record("2026-01-01", "AAPL", "buy", 2, 10)
        record["amount"] = 9_999

        result = analyze_trade_records([record])

        self.assertIn("amount_mismatch", {warning["code"] for warning in result["validation_warnings"]})
        self.assertEqual(result["profile"]["top_symbols"][0]["total_amount"], 20.0)


class FIFOAndFeeTest(unittest.TestCase):
    def test_single_roundtrip_matches_vibe_fifo_and_deducts_both_fees(self):
        result = analyze_trade_records(
            [
                _record("2026-01-01 10:00:00", "AAPL", "buy", 100, 10, fee=10),
                _record("2026-01-02 10:00:00", "AAPL", "sell", 100, 12, fee=6),
            ]
        )

        self.assertEqual(len(result["roundtrips"]), 1)
        trip = result["roundtrips"][0]
        self.assertEqual(trip["qty"], 100.0)
        self.assertEqual(trip["gross_pnl"], 200.0)
        self.assertEqual(trip["buy_fee"], 10.0)
        self.assertEqual(trip["sell_fee"], 6.0)
        self.assertEqual(trip["pnl"], 184.0)
        self.assertEqual(trip["pnl_pct"], 0.184)
        self.assertEqual(trip["hold_days"], 1.0)

    def test_partial_sells_do_not_over_allocate_original_buy_fee(self):
        result = analyze_trade_records(
            [
                _record("2026-01-01", "AAPL", "buy", 100, 10, fee=10),
                _record("2026-01-02", "AAPL", "sell", 40, 12, fee=4),
                _record("2026-01-03", "AAPL", "sell", 60, 11, fee=6),
            ]
        )

        first, second = result["roundtrips"]
        self.assertEqual(first["buy_fee"], 4.0)
        self.assertEqual(second["buy_fee"], 6.0)
        self.assertEqual(sum(trip["buy_fee"] for trip in result["roundtrips"]), 10.0)
        self.assertEqual(result["profile"]["realized_fees"], 20.0)
        self.assertEqual(result["profile"]["total_pnl"], 120.0)
        self.assertEqual(result["open_lots"], [])

    def test_sell_spans_fifo_lots_and_preserves_remaining_lot_fee(self):
        result = analyze_trade_records(
            [
                _record("2026-01-01", "AAPL", "buy", 100, 10, fee=10),
                _record("2026-01-02", "AAPL", "buy", 100, 20, fee=20),
                _record("2026-01-03", "AAPL", "sell", 150, 30, fee=15),
            ]
        )

        self.assertEqual([trip["qty"] for trip in result["roundtrips"]], [100.0, 50.0])
        self.assertEqual([trip["buy_price"] for trip in result["roundtrips"]], [10.0, 20.0])
        self.assertEqual(result["roundtrips"][0]["pnl"], 1_980.0)
        self.assertEqual(result["roundtrips"][1]["pnl"], 485.0)
        self.assertEqual(len(result["open_lots"]), 1)
        self.assertEqual(result["open_lots"][0]["quantity"], 50.0)
        self.assertEqual(result["open_lots"][0]["remaining_buy_fee"], 10.0)
        self.assertEqual(result["profile"]["realized_fees"], 35.0)
        self.assertEqual(result["profile"]["open_lot_fees"], 10.0)

    def test_unmatched_sell_is_reported_and_does_not_create_short_pnl(self):
        result = analyze_trade_records(
            [
                _record("2026-01-01", "AAPL", "sell", 5, 12, fee=1),
                _record("2026-01-02", "AAPL", "buy", 10, 10, fee=2),
                _record("2026-01-03", "AAPL", "sell", 3, 11, fee=0.3),
            ]
        )

        self.assertEqual(result["profile"]["unmatched_sell_quantity"], 5.0)
        self.assertEqual(result["unmatched_sells"][0]["quantity"], 5.0)
        self.assertEqual(result["profile"]["open_quantity"], 7.0)
        self.assertEqual(result["open_lots"][0]["remaining_buy_fee"], 1.4)
        self.assertEqual(result["profile"]["total_roundtrips"], 1)
        self.assertIn("unmatched_sell_quantity", {warning["code"] for warning in result["validation_warnings"]})
        self.assertIn("缺少更早买入", result["reflection_text"])

    def test_unsorted_input_is_reordered_before_fifo(self):
        result = analyze_trade_records(
            [
                _record("2026-01-02", "AAPL", "sell", 1, 12),
                _record("2026-01-01", "AAPL", "buy", 1, 10),
            ]
        )

        self.assertEqual(result["profile"]["total_roundtrips"], 1)
        self.assertEqual(result["roundtrips"][0]["pnl"], 2.0)
        self.assertIn("records_reordered", {warning["code"] for warning in result["validation_warnings"]})


class ProfileBehaviorAndInsightTest(unittest.TestCase):
    def test_profile_computes_win_rate_payoff_and_drawdown(self):
        result = analyze_trade_records(
            [
                _record("2026-01-01", "WIN", "buy", 100, 10),
                _record("2026-01-03", "WIN", "sell", 100, 12),
                _record("2026-01-02", "LOSS", "buy", 100, 50),
                _record("2026-01-12", "LOSS", "sell", 100, 45),
            ]
        )

        profile = result["profile"]
        self.assertEqual(profile["total_roundtrips"], 2)
        self.assertEqual(profile["win_rate"], 0.5)
        self.assertEqual(profile["profit_loss_ratio"], 0.4)
        self.assertEqual(profile["total_pnl"], -300.0)
        self.assertEqual(profile["max_drawdown"], -500.0)
        self.assertEqual(profile["avg_holding_days"], 6.0)

    def test_disposition_effect_detects_losers_held_longer(self):
        result = analyze_trade_records(
            [
                _record("2026-01-01", "WIN", "buy", 1, 10),
                _record("2026-01-03", "WIN", "sell", 1, 12),
                _record("2026-01-01", "LOSS", "buy", 1, 10),
                _record("2026-01-11", "LOSS", "sell", 1, 8),
            ]
        )

        diagnostic = result["behavior"]["disposition_effect"]
        self.assertEqual(diagnostic["status"], "ok")
        self.assertEqual(diagnostic["severity"], "high")
        self.assertEqual(diagnostic["ratio_loss_to_win_hold"], 5.0)
        self.assertTrue(any("处置效应" in item for item in result["insights"]["overall"]))

    def test_chasing_and_anchoring_use_vibe_thresholds(self):
        records = [
            _record(f"2026-01-0{index + 1}", "AAPL", "buy", 1, price)
            for index, price in enumerate([100, 101, 102, 106, 107])
        ]

        result = analyze_trade_records(records)

        chasing = result["behavior"]["chasing_momentum"]
        anchoring = result["behavior"]["anchoring"]
        self.assertEqual(chasing["severity"], "high")
        self.assertEqual(chasing["chase_ratio"], 1.0)
        self.assertEqual(chasing["buys_evaluated"], 2)
        self.assertEqual(anchoring["severity"], "high")
        self.assertEqual(anchoring["anchored_symbols"][0]["symbol"], "AAPL")

    def test_per_symbol_stats_include_closed_open_and_reflection_items(self):
        result = analyze_trade_records(
            [
                _record("2026-01-01", "AAPL", "buy", 3, 10, name="Apple"),
                _record("2026-01-02", "AAPL", "sell", 2, 12, name="Apple"),
                _record("2026-01-01", "MSFT", "buy", 1, 20, name="Microsoft"),
            ]
        )

        stats = {row["symbol"]: row for row in result["per_symbol"]}
        self.assertEqual(stats["AAPL"]["realized_pnl"], 4.0)
        self.assertEqual(stats["AAPL"]["open_quantity"], 1.0)
        self.assertEqual(stats["MSFT"]["roundtrips"], 0)
        symbol_insights = {row["symbol"]: row["items"] for row in result["insights"]["per_symbol"]}
        self.assertTrue(any("未平仓" in item for item in symbol_insights["AAPL"]))
        self.assertTrue(any("尚无完整" in item for item in symbol_insights["MSFT"]))

    def test_mixed_markets_are_not_presented_as_currency_normalized(self):
        result = analyze_trade_records(
            [
                _record("2026-01-01", "AAPL", "buy", 1, 10, market="us"),
                _record("2026-01-02", "AAPL", "sell", 1, 12, market="us"),
                _record("2026-01-01", "00700.HK", "buy", 1, 300, market="hk"),
                _record("2026-01-02", "00700.HK", "sell", 1, 310, market="hk"),
            ]
        )

        self.assertIn(
            "mixed_markets_without_currency_conversion",
            {warning["code"] for warning in result["validation_warnings"]},
        )
        self.assertTrue(any("未换算币种" in item for item in result["insights"]["overall"]))

    def test_shadow_strategy_readiness_requires_five_profitable_roundtrips(self):
        records: list[dict[str, object]] = []
        for index in range(5):
            records.extend(
                [
                    _record(f"2026-01-{index * 2 + 1:02d}", f"S{index}", "buy", 1, 10),
                    _record(f"2026-01-{index * 2 + 2:02d}", f"S{index}", "sell", 1, 11),
                ]
            )

        result = analyze_trade_records(records)

        self.assertTrue(result["strategy_extraction_readiness"]["ready"])
        self.assertEqual(result["strategy_extraction_readiness"]["profitable_roundtrips"], 5)

    def test_alias_and_repeat_calls_are_deterministic(self):
        records = [
            _record("2026-01-01T10:00:00+08:00", "AAPL", "buy", 1, 10),
            _record("2026-01-02T10:00:00+08:00", "AAPL", "sell", 1, 11),
        ]

        first = analyze_trade_records(records)
        second = build_journal_reflection(records)

        self.assertEqual(first, second)
        self.assertEqual(first["roundtrips"][0]["buy_dt"], "2026-01-01T02:00:00Z")


if __name__ == "__main__":
    unittest.main()
