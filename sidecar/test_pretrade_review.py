import json
import unittest
from types import SimpleNamespace
from unittest.mock import patch

import main
from pretrade_review import review_with_llm, validate_review_payload


def valid_payload():
    return {
        "risk_warnings": [
            {
                "code": "earnings_gap_risk",
                "severity": "high",
                "category": "event",
                "message": "财报窗口可能放大隔夜跳空风险。",
                "field": "valid_until",
                "evidence": {"source_ids": ["news-1"], "event_date": "2026-08-06"},
            }
        ],
        "behavior_reminders": [],
        "unresolved_questions": [],
        "changes_since_research": [],
        "freshness": {"news": {"status": "fresh", "data_as_of": "2026-08-04"}},
        "sources": [
            {"id": "news-1", "label": "公司公告", "url": "https://example.com/filing", "data_as_of": "2026-08-04", "status": "available"}
        ],
    }


class PreTradeReviewValidationTest(unittest.TestCase):
    def test_accepts_cited_risks_from_supplied_sources(self):
        result = validate_review_payload(
            valid_payload(),
            {"news-1": {"url": "https://example.com/filing", "label": "公司公告"}},
        )
        self.assertEqual(result["risk_warnings"][0]["code"], "earnings_gap_risk")

    def test_rejects_approval_language_and_fabricated_sources(self):
        approval = valid_payload()
        approval["risk_warnings"][0]["message"] = "计划通过，可以买入。"
        with self.assertRaisesRegex(ValueError, "decision language"):
            validate_review_payload(approval, {"news-1": {"url": "https://example.com/filing"}})

        fabricated = valid_payload()
        fabricated["sources"][0]["url"] = "https://fabricated.example/risk"
        with self.assertRaisesRegex(ValueError, "source"):
            validate_review_payload(fabricated, {"news-1": {"url": "https://example.com/filing"}})

    def test_rejects_uncited_high_risk_instead_of_inventing_evidence(self):
        payload = valid_payload()
        payload["risk_warnings"][0]["evidence"] = {"source_ids": []}
        with self.assertRaisesRegex(ValueError, "citation"):
            validate_review_payload(payload, {"news-1": {"url": "https://example.com/filing"}})

    def test_llm_path_uses_only_supplied_context_and_returns_validated_json(self):
        llm = SimpleNamespace(invoke=lambda messages: SimpleNamespace(content=json.dumps(valid_payload(), ensure_ascii=False)))
        result = review_with_llm(
            {
                "ticker": "NVDA",
                "plan_snapshot": {"entry_price": 120, "stop_loss": 115},
                "research_evidence": {"status": "available"},
                "current_evidence": {
                    "sources": [{"id": "news-1", "label": "公司公告", "url": "https://example.com/filing"}]
                },
                "behavior_observations": [],
            },
            llm,
        )
        self.assertEqual(result["sources"][0]["id"], "news-1")


if __name__ == "__main__":
    unittest.main()


class PreTradeReviewEndpointTest(unittest.IsolatedAsyncioTestCase):
    async def test_endpoint_uses_server_side_provider_configuration(self):
        llm = SimpleNamespace(invoke=lambda messages: SimpleNamespace(content=json.dumps(valid_payload(), ensure_ascii=False)))
        request = main.PreTradeReviewRequest(
            ticker="NVDA",
            llm_provider="openai",
            model="gpt-test",
            api_key="server-only-secret",
            plan_snapshot={"entry_price": 120, "stop_loss": 115},
            current_evidence={"sources": [{"id": "news-1", "label": "公司公告", "url": "https://example.com/filing"}]},
        )
        with patch.object(main, "create_role_llm", return_value=llm) as create, patch.object(
            main, "prefetch_vibe_evidence_async", return_value={"analysis_date": "2026-08-04", "source_traces": []}
        ):
            result = await main.review_trade_plan(request)
        self.assertEqual(result["risk_warnings"][0]["code"], "earnings_gap_risk")
        self.assertEqual(create.call_args.args[0].api_key, "server-only-secret")

    async def test_endpoint_prefetches_technical_news_and_sentiment_evidence(self):
        response = valid_payload()
        response["risk_warnings"][0]["evidence"]["source_ids"] = ["vibe-0"]
        response["sources"] = [{"id": "vibe-0", "label": "exchange-feed", "url": None, "data_as_of": "2026-08-04", "status": "available"}]
        llm = SimpleNamespace(invoke=lambda messages: SimpleNamespace(content=json.dumps(response, ensure_ascii=False)))
        bundle = {
            "analysis_date": "2026-08-04",
            "status": "complete",
            "source_traces": [{"role": "news", "status": "ok", "source": "exchange-feed", "payload": {"headline": "财报日确认"}}],
        }
        request = main.PreTradeReviewRequest(
            ticker="NVDA", llm_provider="openai", model="gpt-test", plan_snapshot={"entry_price": 120}
        )
        with patch.object(main, "create_role_llm", return_value=llm), patch.object(
            main, "prefetch_vibe_evidence_async", return_value=bundle
        ) as prefetch:
            result = await main.review_trade_plan(request)
        self.assertEqual(result["sources"][0]["id"], "vibe-0")
        self.assertEqual(prefetch.call_args.kwargs["analysts"], ["market", "news", "social"])
