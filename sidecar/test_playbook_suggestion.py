import unittest

from playbook_suggestion import validate_playbook_suggestions


class PlaybookSuggestionTests(unittest.TestCase):
    def test_requires_real_review_evidence_and_never_auto_applies(self):
        payload = {
            "suggestions": [{
                "name": "放量突破",
                "markets": ["US"],
                "horizons": ["swing"],
                "market_context": "指数位于上升趋势",
                "entry_rules": "收盘突破且成交量确认",
                "invalidation_rules": "跌回突破位",
                "stop_rules": "按计划止损",
                "target_rules": "分批退出",
                "position_rules": "不超过风险预算",
                "no_trade_rules": "财报前不入场",
                "notes": "仅为待确认建议",
                "evidence_review_ids": ["review-1"],
            }]
        }
        result = validate_playbook_suggestions(payload, {"review-1"})
        self.assertEqual(result["suggestions"][0]["evidence_count"], 1)
        self.assertIn("generated_at", result["suggestions"][0])
        self.assertNotIn("status", result["suggestions"][0])

    def test_rejects_unknown_evidence(self):
        with self.assertRaisesRegex(ValueError, "unknown review evidence"):
            validate_playbook_suggestions({"suggestions": [{"name": "无证据", "evidence_review_ids": ["missing"]}]}, {"review-1"})


if __name__ == "__main__":
    unittest.main()
