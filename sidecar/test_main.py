import unittest
from types import SimpleNamespace
from unittest.mock import patch

import main


class DisplayTextSanitizationTest(unittest.TestCase):
    def test_removes_raw_tool_call_blocks(self):
        raw = """Giya

{"look_back_days": 7, "curr_date": "2026-07-06", "limit": 30}

moment
```json
{"indicator": "cpi", "curr_date": "2026-07-06", "look_back_days": 7}

functionget_prediction_markets
```json
{"topic": "Fed rate cut 2026"}

expansion
```json
{"ticker": "GEV", "start_date": "2026-06-29", "end_date": "2026-07-06"}"""

        self.assertEqual(main.sanitize_display_text(raw), "")

    def test_removes_tool_call_placeholder(self):
        self.assertEqual(main.sanitize_display_text("正在调用工具获取详细报告..."), "")

    def test_keeps_normal_chinese_report(self):
        report = "GEV 近期收入增长较快，但估值较高，需结合突破量能确认。"

        self.assertEqual(main.sanitize_display_text(report), report)

    def test_response_to_text_skips_messages_with_tool_calls(self):
        message = SimpleNamespace(
            content="正在调用工具获取详细报告...",
            tool_calls=[{"name": "get_fundamentals", "args": {"ticker": "GEV"}}],
        )
        response = SimpleNamespace(generations=[[SimpleNamespace(message=message)]])

        self.assertEqual(main.response_to_text(response), "")


class CustomGradeExtractionTest(unittest.TestCase):
    def test_prefers_final_conclusion_over_grade_scale(self):
        report = """九、总评分与分级
汇总：盈利质量 3/20，增长可持续性 4/15，行业领导力 5/15，供需结构 2/15，技术形态 6/20，估值与预期 3/10，事件与风控 1/5，总分 24/100。
分级标准：85-100 为 A级重点候选；75-84 为 B级观察候选；60-74 为 C级弱候选；60 以下为 D级淘汰。

十、最终结论
这只股票目前属于：**D级**，原因是盈利数据存在单季严重失真、技术形态处于放量派发后的松散反弹期。"""

        self.assertEqual(main.extract_custom_grade(report), "D级淘汰")

    def test_uses_total_score_when_final_sentence_is_missing(self):
        report = """九、总评分与分级
汇总：盈利质量 18/20，增长可持续性 13/15，行业领导力 14/15，供需结构 13/15，技术形态 18/20，估值与预期 8/10，事件与风控 4/5，总分：88/100。
分级标准：85-100 为 A级重点候选；75-84 为 B级观察候选；60-74 为 C级弱候选；60 以下为 D级淘汰。"""

        self.assertEqual(main.extract_custom_grade(report), "A级重点候选")


class AnalysisEventsOrderTest(unittest.IsolatedAsyncioTestCase):
    async def test_original_progress_done_precedes_combined_report_delta(self):
        request = main.AnalysisRequest(
            ticker="GEV",
            analysts=["fundamental", "sentiment"],
        )
        state = {
            "fundamentals_report": "基本面真实分析正文",
            "sentiment_report": "情绪真实分析正文",
            "investment_debate_state": {"judge_decision": "研究团队结论"},
            "trader_investment_plan": "交易计划",
            "risk_debate_state": {"judge_decision": "风险结论"},
            "final_trade_decision": "HOLD",
        }

        with patch.object(main, "run_analysis", return_value=(state, "HOLD")), patch.object(main, "REPORT_DELTA_DELAY", 0):
            events = [event async for event in main.analysis_events(request)]

        fundamental_report_index = next(
            index for index, event in enumerate(events)
            if event.startswith("event: report_delta") and "## 基本面分析师" in event
        )
        fundamental_done_index = next(
            index for index, event in enumerate(events)
            if event.startswith("event: progress")
            and '"agent": "FundamentalsAnalyst"' in event
            and '"status": "done"' in event
        )
        sentiment_report_index = next(
            index for index, event in enumerate(events)
            if event.startswith("event: report_delta") and "## 情绪分析师" in event
        )
        sentiment_done_index = next(
            index for index, event in enumerate(events)
            if event.startswith("event: progress")
            and '"agent": "SentimentAnalyst"' in event
            and '"status": "done"' in event
        )

        self.assertLess(fundamental_done_index, fundamental_report_index)
        self.assertLess(sentiment_done_index, sentiment_report_index)
        self.assertTrue(events[-1].startswith("event: done"))

    async def test_missing_selected_report_keeps_original_done_contract(self):
        request = main.AnalysisRequest(
            ticker="GEV",
            analysts=["fundamental", "sentiment"],
        )
        state = {
            "fundamentals_report": "基本面真实分析正文",
            "sentiment_report": "",
            "final_trade_decision": "HOLD",
        }

        with patch.object(main, "run_analysis", return_value=(state, "HOLD")), patch.object(main, "REPORT_DELTA_DELAY", 0):
            events = [event async for event in main.analysis_events(request)]

        fundamental_report_index = next(
            index for index, event in enumerate(events)
            if event.startswith("event: report_delta") and "## 基本面分析师" in event
        )
        fundamental_done_index = next(
            index for index, event in enumerate(events)
            if event.startswith("event: progress")
            and '"agent": "FundamentalsAnalyst"' in event
            and '"status": "done"' in event
        )

        self.assertLess(fundamental_done_index, fundamental_report_index)
        self.assertFalse(any('"agent": "SentimentAnalyst"' in event and '"status": "error"' in event for event in events))
        self.assertFalse(any(event.startswith("event: error") for event in events))
        self.assertTrue(any(event.startswith("event: done") for event in events))

    async def test_graph_failure_errors_without_local_report_or_done(self):
        request = main.AnalysisRequest(ticker="GEV", analysts=["fundamental", "sentiment"])

        with patch.object(main, "run_analysis", side_effect=RuntimeError("tool calling failed")):
            events = [event async for event in main.analysis_events(request)]

        self.assertTrue(any(event.startswith("event: error") for event in events))
        self.assertFalse(any(event.startswith("event: report_delta") for event in events))
        self.assertFalse(any(event.startswith("event: done") for event in events))


if __name__ == "__main__":
    unittest.main()
