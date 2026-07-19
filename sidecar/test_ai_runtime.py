import json
import unittest
from types import SimpleNamespace
from unittest.mock import patch

import pandas as pd

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
    def test_recognizes_only_canonical_china_a_symbols(self):
        for symbol in (
            "600519.SH",
            "688981.SH",
            "000001.SZ",
            "300750.SZ",
            "920000.BJ",
            "430047.BJ",
            "830799.BJ",
            "600519.sh",
        ):
            self.assertTrue(runtime.is_china_a_symbol(symbol), symbol)
        for symbol in (
            "600519.SS",
            "AAPL.US",
            "600519",
            "12345.SH",
            "ABCDEF.SH",
            "123456.SZ",
            "900901.SH",
            "200001.SZ",
        ):
            self.assertFalse(runtime.is_china_a_symbol(symbol), symbol)

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

    async def test_china_a_prefetch_uses_only_supported_tools_and_preserves_symbol(self):
        calls = []
        tools = {
            "get_stock_profile": FakeAsyncTool(
                "get_stock_profile", success("yahoo"), calls=calls
            ),
            "get_financial_statements": FakeAsyncTool(
                "get_financial_statements",
                lambda args: success("china-a-disclosure", {"period": args["period"]}),
                properties={"code": {}, "statement": {}, "period": {}},
                calls=calls,
            ),
            "get_sec_filings": FakeAsyncTool(
                "get_sec_filings", success("sec_edgar"), calls=calls
            ),
            "get_market_data": FakeAsyncTool(
                "get_market_data",
                json.dumps({"600519.SH": [{"date": "2026-07-09", "close": 1450}]}),
                calls=calls,
            ),
            "get_stock_news": FakeAsyncTool(
                "get_stock_news", success("china-a-news"), calls=calls
            ),
            "get_options_chain": FakeAsyncTool(
                "get_options_chain", success("yahoo"), calls=calls
            ),
        }

        with patch.object(runtime, "vibe_tools_enabled", return_value=True), patch.object(
            runtime, "_load_tools", return_value=tools
        ):
            result = await runtime.prefetch_vibe_evidence_async(
                "600519.sh",
                analysis_date="2026-07-10",
                analysts=("fundamentals", "market", "news", "social"),
                timeout_seconds=1,
            )

        self.assertEqual(
            [name for name, _ in calls],
            [
                "get_financial_statements",
                "get_financial_statements",
                "get_market_data",
                "get_stock_news",
            ],
        )
        self.assertNotIn(
            "get_stock_profile",
            {trace["tool"] for trace in result["source_traces"]},
        )
        self.assertNotIn(
            "get_sec_filings",
            {trace["tool"] for trace in result["source_traces"]},
        )
        self.assertNotIn(
            "get_options_chain",
            {trace["tool"] for trace in result["source_traces"]},
        )
        finance_args = [args for name, args in calls if name == "get_financial_statements"]
        self.assertEqual([args["code"] for args in finance_args], ["600519.SH", "600519.SH"])
        market_args = next(args for name, args in calls if name == "get_market_data")
        news_args = next(args for name, args in calls if name == "get_stock_news")
        self.assertEqual(market_args["codes"], ["600519.SH"])
        self.assertEqual(news_args["code"], "600519.SH")
        self.assertEqual(result["market_symbol"], "600519.SH")
        self.assertEqual(result["summary"], {"requested": 4, "ok": 4, "failed": 0})
        self.assertEqual(result["evidence"]["social"], {})

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

    async def test_retries_transient_mcp_task_group_error_once(self):
        attempts = 0

        def flaky_news(_arguments):
            nonlocal attempts
            attempts += 1
            if attempts == 1:
                raise RuntimeError("unhandled errors in a TaskGroup (1 sub-exception)")
            return success("yahoo")

        tools = {"get_stock_news": FakeAsyncTool("get_stock_news", flaky_news)}
        with patch.object(runtime, "vibe_tools_enabled", return_value=True), patch.object(
            runtime, "_load_tools", return_value=tools
        ):
            result = await runtime.prefetch_vibe_evidence_async(
                "AAPL", analysts=("news",), timeout_seconds=1
            )

        self.assertEqual(attempts, 2)
        self.assertEqual(result["status"], "complete")
        self.assertEqual(result["source_traces"][0]["status"], "ok")

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


class ChinaATradingAgentsBoundaryTest(unittest.TestCase):
    def test_yahoo_symbol_boundary_converts_only_shanghai_suffix(self):
        from tradingagents.dataflows.symbol_utils import (
            is_china_a_symbol,
            normalize_symbol,
            yahoo_report_symbol,
        )

        self.assertEqual(normalize_symbol("600519.SH"), "600519.SS")
        self.assertEqual(normalize_symbol("000001.SZ"), "000001.SZ")
        self.assertEqual(normalize_symbol("920000.BJ"), "920000.BJ")
        self.assertEqual(normalize_symbol("AAPL"), "AAPL")
        self.assertEqual(yahoo_report_symbol("600519.SH", "600519.SS"), "600519.SH")
        self.assertTrue(is_china_a_symbol("430047.BJ"))
        self.assertFalse(is_china_a_symbol("123456.SZ"))
        self.assertFalse(is_china_a_symbol("900901.SH"))
        self.assertFalse(is_china_a_symbol("200001.SZ"))

    def test_yfinance_fetch_uses_ss_but_report_keeps_sh(self):
        from tradingagents.dataflows import y_finance

        requested_symbols = []
        frame = pd.DataFrame(
            {
                "Open": [1400.0, 1410.0],
                "High": [1420.0, 1430.0],
                "Low": [1390.0, 1400.0],
                "Close": [1415.0, 1425.0],
                "Volume": [100, 120],
            },
            index=pd.to_datetime(["2026-07-09", "2026-07-10"]),
        )

        class FakeTicker:
            def history(self, **_kwargs):
                return frame.copy()

        def fake_ticker(symbol):
            requested_symbols.append(symbol)
            return FakeTicker()

        with patch.object(y_finance.yf, "Ticker", side_effect=fake_ticker):
            report = y_finance.get_YFin_data_online(
                "600519.SH", "2026-07-09", "2026-07-10"
            )

        self.assertEqual(requested_symbols, ["600519.SS"])
        self.assertIn("Stock data for 600519.SH", report)
        self.assertNotIn("600519.SS", report)

    def test_a_share_vibe_patch_exposes_only_market_compatible_tools(self):
        from ta_vibe_bridge import ta_patch

        observed = {}

        def fake_apply(tools, *, assignment=None):
            observed["tools"] = tools
            observed["assignment"] = assignment
            return lambda: observed.setdefault("undone", True)

        loaded_tools = {
            name: SimpleNamespace(name=name)
            for name in (
                "get_financial_statements",
                "get_market_data",
                "get_stock_news",
                "get_sec_filings",
                "get_stock_profile",
                "get_options_chain",
                "screen_market",
                "search_symbol",
            )
        }
        with patch.object(runtime, "vibe_tools_enabled", return_value=True), patch.object(
            runtime, "_load_tools", return_value=loaded_tools
        ), patch.object(ta_patch, "apply_mcp_tools", side_effect=fake_apply):
            with runtime.patched_tradingagents_graph(china_a_mode=True):
                pass

        self.assertEqual(
            observed["assignment"],
            {
                "fundamentals": ("get_financial_statements",),
                "market": ("get_market_data",),
                "news": ("get_stock_news",),
                "social": (),
            },
        )
        assigned = {name for names in observed["assignment"].values() for name in names}
        self.assertTrue(
            assigned.isdisjoint(
                {
                    "get_sec_filings",
                    "get_stock_profile",
                    "get_options_chain",
                    "screen_market",
                    "search_symbol",
                }
            )
        )
        self.assertTrue(observed["undone"])

    def test_graph_construction_lock_is_used_when_vibe_is_disabled(self):
        observed = []

        class TrackingLock:
            def __enter__(self):
                observed.append("enter")

            def __exit__(self, *_args):
                observed.append("exit")

        with patch.object(runtime, "_PATCH_LOCK", TrackingLock()), patch.object(
            runtime, "vibe_tools_enabled", return_value=False
        ):
            with runtime.patched_tradingagents_graph(china_a_mode=True):
                observed.append("body")

        self.assertEqual(observed, ["enter", "body", "exit"])

    def test_a_share_builtin_tool_nodes_exclude_us_news_enrichment(self):
        from tradingagents.graph.trading_graph import TradingAgentsGraph

        graph = object.__new__(TradingAgentsGraph)
        graph.config = {"china_a_mode": True}
        nodes = graph._create_tool_nodes()

        self.assertEqual(set(nodes["news"].tools_by_name), {"get_news"})
        self.assertEqual(set(nodes["social"].tools_by_name), {"get_news"})
        self.assertEqual(
            set(nodes["fundamentals"].tools_by_name),
            {"get_fundamentals", "get_balance_sheet", "get_cashflow", "get_income_statement"},
        )
        self.assertEqual(
            set(nodes["market"].tools_by_name),
            {"get_stock_data", "get_indicators", "get_verified_market_snapshot"},
        )

    def test_a_share_news_and_sentiment_skip_us_only_sources(self):
        from tradingagents.agents.analysts import news_analyst, sentiment_analyst

        self.assertEqual(
            [tool.name for tool in news_analyst.news_tools_for_market(True)],
            ["get_news"],
        )
        news_prompt = news_analyst.news_system_message("company", True)
        self.assertIn("mainland China A-share", news_prompt)
        self.assertIn("Missing US-only evidence is not adverse evidence", news_prompt)

        fake_news = SimpleNamespace(func=lambda *_args: "A-share company news")
        with patch.object(sentiment_analyst, "get_news", fake_news), patch.object(
            sentiment_analyst,
            "fetch_stocktwits_messages",
            side_effect=AssertionError("StockTwits must not run for A shares"),
        ), patch.object(
            sentiment_analyst,
            "fetch_reddit_posts",
            side_effect=AssertionError("Reddit must not run for A shares"),
        ):
            blocks = sentiment_analyst.prefetch_sentiment_sources(
                "600519.SH",
                "2026-07-03",
                "2026-07-10",
                china_a_mode=True,
            )

        self.assertEqual(blocks, ("A-share company news", None, None))
        sentiment_prompt = sentiment_analyst._build_system_message(
            ticker="600519.SH",
            start_date="2026-07-03",
            end_date="2026-07-10",
            news_block=blocks[0],
            stocktwits_block=blocks[1],
            reddit_block=blocks[2],
            china_a_mode=True,
        )
        self.assertIn("company news", sentiment_prompt)
        self.assertIn("count their absence as negative evidence", sentiment_prompt)
        self.assertNotIn("<start_of_stocktwits>", sentiment_prompt)
        self.assertNotIn("<start_of_reddit>", sentiment_prompt)

    def test_interleaved_a_and_us_factories_keep_their_captured_mode(self):
        from langchain_core.messages import AIMessage, HumanMessage
        from langchain_core.runnables import RunnableLambda
        from tradingagents.agents.analysts import news_analyst, sentiment_analyst

        bound_tools = {}

        class NewsLLM:
            def __init__(self, key):
                self.key = key

            def bind_tools(self, tools):
                bound_tools[self.key] = [tool.name for tool in tools]
                return RunnableLambda(
                    lambda _messages: AIMessage(content="done", tool_calls=[])
                )

        a_news_node = news_analyst.create_news_analyst(
            NewsLLM("a"), china_a_mode=True
        )
        us_news_node = news_analyst.create_news_analyst(
            NewsLLM("us"), china_a_mode=False
        )
        state = {
            "trade_date": "2026-07-10",
            "asset_type": "stock",
            "company_of_interest": "600519.SH",
            "instrument_context": "application context",
            "messages": [HumanMessage(content="analyze")],
        }

        # Invoke after both factories exist: neither closure may consult the
        # process-global config written by whichever graph was built last.
        a_news_node(state)
        us_news_node({**state, "company_of_interest": "AAPL"})

        self.assertEqual(bound_tools["a"], ["get_news"])
        self.assertEqual(
            bound_tools["us"],
            ["get_news", "get_global_news", "get_macro_indicators", "get_prediction_markets"],
        )

        class SentimentLLM:
            def with_structured_output(self, _schema):
                return self

            def invoke(self, _messages):
                return AIMessage(content="done")

        a_sentiment_node = sentiment_analyst.create_sentiment_analyst(
            SentimentLLM(), china_a_mode=True
        )
        us_sentiment_node = sentiment_analyst.create_sentiment_analyst(
            SentimentLLM(), china_a_mode=False
        )
        captured_modes = []

        def fake_sources(*_args, china_a_mode):
            captured_modes.append(china_a_mode)
            return "news", None, None

        with patch.object(
            sentiment_analyst,
            "prefetch_sentiment_sources",
            side_effect=fake_sources,
        ), patch.object(
            sentiment_analyst,
            "invoke_structured_or_freetext",
            return_value="sentiment report",
        ):
            a_sentiment_node(state)
            us_sentiment_node({**state, "company_of_interest": "AAPL"})

        self.assertEqual(captured_modes, [True, False])

    def test_shared_dataflow_config_excludes_request_api_key(self):
        from tradingagents.graph.trading_graph import _shared_dataflow_config

        config = {
            "api_key": "request-secret",
            "china_a_mode": True,
            "data_vendors": {"news_data": "yfinance"},
        }
        shared = _shared_dataflow_config(config)

        self.assertNotIn("api_key", shared)
        self.assertTrue(shared["china_a_mode"])
        self.assertEqual(shared["data_vendors"], {"news_data": "yfinance"})


if __name__ == "__main__":
    unittest.main()
