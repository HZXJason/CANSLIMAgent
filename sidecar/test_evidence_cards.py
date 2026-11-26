import json
import unittest
from types import SimpleNamespace

from evidence_cards import compile_deterministic_cards, enrich_counter_thesis_with_llm, validate_counter_thesis_payload


class EvidenceCardsTest(unittest.TestCase):
    def test_compile_deterministic_cards_marks_missing_as_unknown(self):
        pack = compile_deterministic_cards(
            "NVDA",
            {
                "stock_summary": {
                    "symbol": "NVDA",
                    "data_as_of": "2026-08-01",
                    "metrics": {"c_eps_growth_pct": 42.5, "c_data_sufficient": True, "l_rs_rating": 92, "l_data_sufficient": True},
                },
                "kline_stats": {"latest_close": 120.0, "atr14": 4.2, "atr14_percent": 3.5, "source": "stooq", "data_as_of": "2026-08-01"},
            },
        )
        self.assertEqual(pack["symbol"], "NVDA")
        self.assertEqual(len(pack["cards"]), 4)
        self.assertEqual(pack["ai_status"], "not_requested")
        growth_card = pack["cards"][0]
        self.assertTrue(any("未知" in item["value"] for item in growth_card["items"]))

    def test_counter_thesis_rejects_buy_sell_and_unknown_sources(self):
        with self.assertRaisesRegex(ValueError, "decision language"):
            validate_counter_thesis_payload(
                {
                    "bull_anchors": [{"title": "建议买入", "detail": "业绩强劲", "source_ids": ["summary-metrics"]}],
                    "bear_falsifiers": [],
                    "blind_spots": [],
                },
                {"summary-metrics"},
            )
        with self.assertRaisesRegex(ValueError, "unknown source_id"):
            validate_counter_thesis_payload(
                {
                    "bull_anchors": [{"title": "增速拐点", "detail": "EPS 加速", "source_ids": ["fabricated"]}],
                    "bear_falsifiers": [],
                    "blind_spots": [],
                },
                {"summary-metrics"},
            )

    def test_llm_enrichment_succeeds_with_valid_citations(self):
        pack = compile_deterministic_cards(
            "NVDA",
            {"stock_summary": {"symbol": "NVDA", "data_as_of": "2026-08-01", "metrics": {}}},
        )
        llm_json = {
            "bull_anchors": [{"title": "高相对强度", "detail": "处于强势趋势", "source_ids": ["summary-metrics"]}],
            "bear_falsifiers": [{"title": "放量跌破枢轴", "detail": "收盘跌破支撑位则论点失效", "source_ids": ["summary-metrics"]}],
            "blind_spots": [{"title": "毛利率序列缺失", "detail": "需核查最新季度财报", "source_ids": ["summary-metrics"]}],
        }
        llm = SimpleNamespace(invoke=lambda messages: SimpleNamespace(content=json.dumps(llm_json, ensure_ascii=False)))
        enriched = enrich_counter_thesis_with_llm(pack, llm)
        self.assertEqual(enriched["ai_status"], "completed")
        self.assertEqual(enriched["counter_thesis"]["bear_falsifiers"][0]["title"], "放量跌破枢轴")


if __name__ == "__main__":
    unittest.main()
