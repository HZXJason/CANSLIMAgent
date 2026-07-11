import json
import unittest
from unittest.mock import patch

import ai_runtime as runtime


class FakeAsyncTool:
    def __init__(self, name, result, *, properties=None, calls=None):
        self.name = name
        self.result = result
        self.calls = calls if calls is not None else []
        self.args_schema = {
            "type": "object",
            "properties": properties or {},
        }

    async def ainvoke(self, arguments):
        self.calls.append((self.name, arguments))
        if isinstance(self.result, Exception):
            raise self.result
        if callable(self.result):
            return self.result(arguments)
        return self.result


class FakeSyncTool:
    def __init__(self, name, result, *, calls=None):
        self.name = name
        self.result = result
        self.calls = calls if calls is not None else []
        self.args_schema = {"type": "object", "properties": {}}

    def invoke(self, arguments):
        self.calls.append((self.name, arguments))
        return self.result


def success(source="test", data=None):
    return json.dumps(
        {"ok": True, "source": source, "data": data or {"value": 1}}
    )


class VibePrefetchTest(unittest.IsolatedAsyncioTestCase):
    async def test_prefetches_role_tools_in_deterministic_order(self):
        calls = []
        finance_properties = {
            "code": {},
            "statement": {},
            "period": {},
        }
        tools = {
            "get_stock_profile": FakeAsyncTool(
                "get_stock_profile", success("yahoo"), calls=calls
            ),
            "get_financial_statements": FakeAsyncTool(
                "get_financial_statements",
                lambda args: success("sec_edgar", {"period": args["period"]}),
                properties=finance_properties,
                calls=calls,
            ),
            "get_sec_filings": FakeAsyncTool(
                "get_sec_filings", success("sec_edgar"), calls=calls
            ),
            "get_market_data": FakeAsyncTool(
                "get_market_data",
                json.dumps({"AAPL.US": [{"date": "2026-07-09", "close": 200}]}),
                calls=calls,
            ),
            "get_stock_news": FakeAsyncTool(
                "get_stock_news", success("yahoo"), calls=calls
            ),
            "get_options_chain": FakeAsyncTool(
                "get_options_chain", success("yahoo"), calls=calls
            ),
        }

        with patch.object(runtime, "vibe_tools_enabled", return_value=True), patch.object(
            runtime, "_load_tools", return_value=tools
        ):
            result = await runtime.prefetch_vibe_evidence_async(
                "aapl",
                analysis_date="2026-07-10",
                analysts=("fundamentals", "market", "news", "social"),
                timeout_seconds=1,
            )

        self.assertEqual(
            [name for name, _ in calls],
            [
                "get_stock_profile",
                "get_financial_statements",
                "get_financial_statements",
                "get_sec_filings",
                "get_market_data",
                "get_stock_news",
                "get_options_chain",
            ],
        )
        finance_calls = [args for name, args in calls if name == "get_financial_statements"]
        self.assertEqual([args["period"] for args in finance_calls], ["annual", "quarter"])
        market_args = next(args for name, args in calls if name == "get_market_data")
        self.assertEqual(market_args["codes"], ["AAPL.US"])
        self.assertEqual(market_args["end_date"], "2026-07-10")
        self.assertEqual(result["schema_version"], "vibe.research_evidence.v1")
        self.assertEqual(result["status"], "complete")
        self.assertEqual(result["summary"], {"requested": 7, "ok": 7, "failed": 0})
        quarterly_evidence = result["evidence"]["fundamentals"][
            "financial_indicators_quarterly"
        ]
        quarterly_trace = result["source_traces"][
            quarterly_evidence["source_trace_index"]
        ]
        self.assertEqual(quarterly_trace["payload"]["data"]["period"], "quarter")
        self.assertEqual(
            quarterly_evidence["payload_ref"],
            f'source_traces[{quarterly_evidence["source_trace_index"]}].payload',
        )
        market_trace = next(
            trace
            for trace in result["source_traces"]
            if trace["tool"] == "get_market_data"
        )
        self.assertEqual(market_trace["source"], "vibe-trading:auto")
        for trace in result["source_traces"]:
            self.assertIn("duration_ms", trace)
            self.assertIn("error", trace)
            self.assertIn("payload", trace)

    async def test_supports_sync_tool_and_omits_unsupported_financial_period(self):
        calls = []
        tool = FakeSyncTool(
            "get_financial_statements",
            success("sec_edgar"),
            calls=calls,
        )
        tool.args_schema = {
            "type": "object",
            "properties": {"code": {}, "statement": {}},
        }
        tools = {"get_financial_statements": tool}

        with patch.object(runtime, "vibe_tools_enabled", return_value=True), patch.object(
            runtime, "_load_tools", return_value=tools
        ):
            result = await runtime.prefetch_vibe_evidence_async(
                "AAPL", analysts=("fundamentals",), timeout_seconds=1
            )

        finance_calls = [args for name, args in calls if name == "get_financial_statements"]
        self.assertEqual(finance_calls, [{"code": "AAPL.US", "statement": "indicators"}])
        self.assertIn(
            "financial_indicators",
            result["evidence"]["fundamentals"],
        )

    async def test_gracefully_degrades_each_tool_failure(self):
        def financial_result(arguments):
            if arguments["period"] == "quarter":
                return json.dumps({"ok": False, "error": "quarter unavailable"})
            return success("sec_edgar")

        tools = {
            "get_stock_profile": FakeAsyncTool(
                "get_stock_profile", RuntimeError("profile transport failed")
            ),
            "get_financial_statements": FakeAsyncTool(
                "get_financial_statements",
                financial_result,
                properties={"code": {}, "statement": {}, "period": {}},
            ),
            # get_sec_filings deliberately absent.
        }

        with patch.object(runtime, "vibe_tools_enabled", return_value=True), patch.object(
            runtime, "_load_tools", return_value=tools
        ):
            result = await runtime.prefetch_vibe_evidence_async(
                "AAPL", analysts=("fundamentals",), timeout_seconds=1
            )

        self.assertEqual(result["status"], "partial")
        self.assertEqual(result["summary"], {"requested": 4, "ok": 1, "failed": 3})
        by_key = {trace["evidence_key"]: trace for trace in result["source_traces"]}
        self.assertEqual(by_key["stock_profile"]["status"], "error")
        self.assertIn("profile transport failed", by_key["stock_profile"]["error"])
        self.assertEqual(
            by_key["financial_indicators_quarterly"]["error"],
            "quarter unavailable",
        )
        self.assertEqual(by_key["sec_filings"]["status"], "unavailable")
        self.assertIsNone(by_key["sec_filings"]["payload"])

    async def test_sync_wrapper_is_safe_when_caller_has_running_event_loop(self):
        tools = {"get_stock_news": FakeAsyncTool("get_stock_news", success("yahoo"))}

        with patch.object(runtime, "vibe_tools_enabled", return_value=True), patch.object(
            runtime, "_load_tools", return_value=tools
        ):
            result = runtime.prefetch_vibe_evidence(
                "AAPL", analysts=("news",), timeout_seconds=1
            )

        self.assertEqual(result["status"], "complete")

    async def test_compacts_large_tool_payloads(self):
        rows = [{"index": index, "text": "x" * 10} for index in range(100)]
        tools = {
            "get_market_data": FakeAsyncTool(
                "get_market_data", json.dumps({"AAPL.US": rows})
            )
        }

        with patch.object(runtime, "vibe_tools_enabled", return_value=True), patch.object(
            runtime, "_load_tools", return_value=tools
        ):
            result = await runtime.prefetch_vibe_evidence_async(
                "AAPL", analysts=("market",), timeout_seconds=1
            )

        payload_rows = result["source_traces"][0]["payload"]["AAPL.US"]
        self.assertEqual(payload_rows["total_items"], 100)
        self.assertTrue(payload_rows["truncated"])
        self.assertEqual(len(payload_rows["items"]), 16)


if __name__ == "__main__":
    unittest.main()
