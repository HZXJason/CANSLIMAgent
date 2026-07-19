import json
import unittest
import tempfile
import httpx
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

import main
import broker_sync
import data_auditor
import vibe_decision_flow
from pydantic import ValidationError


class SecurityBoundaryTest(unittest.TestCase):
    def test_error_messages_redact_server_api_keys(self):
        with patch.dict(main.os.environ, {"OPENAI_API_KEY": "sk-super-secret-value-12345"}, clear=False):
            message = main.user_error_message(RuntimeError("failed with sk-super-secret-value-12345"))
        self.assertNotIn("sk-super-secret-value-12345", message)
        self.assertIn("[REDACTED]", message)

    def test_missing_compatible_gateway_key_explains_connection_error(self):
        request = main.AnalysisRequest(
            ticker="NVDA",
            llm_provider="openai_compatible",
            backend_url="https://gateway.example.com/v1",
        )
        with patch.dict(
            main.os.environ,
            {"OPENAI_COMPATIBLE_API_KEY": "", "AI_GATEWAY_API_KEY": ""},
            clear=False,
        ):
            message = main.user_error_message(RuntimeError("Connection error."), request)

        self.assertIn("未检测到 API Key", message)
        self.assertIn("设置中保存", message)

    def test_analysis_request_rejects_browser_api_keys(self):
        with self.assertRaises(ValidationError):
            main.AnalysisRequest(ticker="NVDA", api_keys={"openai": "secret"})

    def test_backend_url_requires_server_allowlist(self):
        public_dns = [(main.socket.AF_INET, main.socket.SOCK_STREAM, 6, "", ("8.8.8.8", 443))]
        with patch.dict(
            main.os.environ,
            {
                "APP_ENV": "production",
                "AI_ALLOWED_BACKEND_HOSTS": "gateway.example.com",
                "AI_REQUIRE_BACKEND_ALLOWLIST": "true",
            },
            clear=False,
        ), patch.object(main.socket, "getaddrinfo", return_value=public_dns):
            self.assertEqual(
                main.validate_backend_url("https://gateway.example.com/v1"),
                "https://gateway.example.com/v1",
            )
            for malicious in (
                "http://gateway.example.com/v1",
                "https://127.0.0.1/v1",
                "https://metadata.google.internal/computeMetadata/v1",
                "https://gateway.example.com/v1?target=http://127.0.0.1",
            ):
                with self.assertRaises(ValueError, msg=malicious):
                    main.validate_backend_url(malicious)

    def test_development_accepts_unlisted_public_https_backend(self):
        public_dns = [(main.socket.AF_INET, main.socket.SOCK_STREAM, 6, "", ("8.8.8.8", 443))]
        with patch.dict(
            main.os.environ,
            {
                "APP_ENV": "development",
                "AI_ALLOWED_BACKEND_HOSTS": "ai-gateway.vercel.sh",
                "AI_REQUIRE_BACKEND_ALLOWLIST": "false",
                "AI_ALLOW_PRIVATE_BACKENDS": "false",
            },
            clear=False,
        ), patch.object(main.socket, "getaddrinfo", return_value=public_dns):
            self.assertEqual(
                main.validate_backend_url("https://gateway.example.com/v1"),
                "https://gateway.example.com/v1",
            )

    def test_development_rejects_hostname_resolving_to_private_address(self):
        private_dns = [(main.socket.AF_INET, main.socket.SOCK_STREAM, 6, "", ("127.0.0.1", 443))]
        with patch.dict(
            main.os.environ,
            {
                "APP_ENV": "development",
                "AI_REQUIRE_BACKEND_ALLOWLIST": "false",
                "AI_ALLOW_PRIVATE_BACKENDS": "false",
            },
            clear=False,
        ), patch.object(main.socket, "getaddrinfo", return_value=private_dns):
            with self.assertRaisesRegex(ValueError, "private backend_url"):
                main.validate_backend_url("https://gateway.example.com/v1")

    def test_model_discovery_uses_server_key_and_filters_non_chat_models(self):
        response = SimpleNamespace(
            status_code=200,
            headers={},
            iter_content=lambda chunk_size: [
                b'{"data":[{"id":"gpt-5.4"},{"id":"text-embedding-3-small"},{"id":"gpt-5.4"}]}'
            ],
            close=lambda: None,
        )
        with patch.dict(main.os.environ, {"OPENAI_API_KEY": "server-only-key"}, clear=False), patch.object(
            main.requests, "get", return_value=response
        ) as get:
            result = main.discover_available_models(main.ModelDiscoveryRequest(provider="openai"))

        self.assertEqual(result["models"], ["gpt-5.4"])
        self.assertEqual(get.call_args.kwargs["headers"]["Authorization"], "Bearer server-only-key")
        self.assertFalse(get.call_args.kwargs["allow_redirects"])

    def test_custom_model_discovery_keeps_backend_allowlist(self):
        public_dns = [(main.socket.AF_INET, main.socket.SOCK_STREAM, 6, "", ("8.8.8.8", 443))]
        with patch.dict(
            main.os.environ,
            {
                "APP_ENV": "production",
                "AI_ALLOWED_BACKEND_HOSTS": "gateway.example.com",
                "AI_REQUIRE_BACKEND_ALLOWLIST": "true",
            },
            clear=False,
        ), patch.object(main.socket, "getaddrinfo", return_value=public_dns):
            self.assertEqual(
                main.resolve_model_endpoint("openai_compatible", "https://gateway.example.com/v1/chat/completions"),
                "https://gateway.example.com/v1/models",
            )
            with self.assertRaises(ValueError):
                main.resolve_model_endpoint("openai_compatible", "https://untrusted.example/v1")


class SidecarMiddlewareTest(unittest.IsolatedAsyncioTestCase):
    async def test_sidecar_shared_secret_protects_internal_routes(self):
        with patch.dict(main.os.environ, {"SIDECAR_SHARED_SECRET": "s" * 32}, clear=False):
            transport = httpx.ASGITransport(app=main.app)
            async with httpx.AsyncClient(transport=transport, base_url="http://sidecar.test") as client:
                self.assertEqual((await client.get("/health")).status_code, 200)
                self.assertEqual((await client.post("/broker/connection-status", json={})).status_code, 401)
                response = await client.post(
                    "/broker/connection-status",
                    headers={main.SIDECAR_TOKEN_HEADER: "s" * 32},
                    json={},
                )
            self.assertEqual(response.status_code, 422)

    async def test_models_route_accepts_only_server_or_trusted_go_credentials(self):
        response = SimpleNamespace(
            status_code=200,
            headers={},
            iter_content=lambda chunk_size: [b'{"data":[{"id":"gpt-5.4"}]}'],
            close=lambda: None,
        )
        with patch.dict(
            main.os.environ,
            {"SIDECAR_SHARED_SECRET": "s" * 32, "OPENAI_API_KEY": "server-only-key"},
            clear=False,
        ), patch.object(main.requests, "get", return_value=response) as get:
            transport = httpx.ASGITransport(app=main.app)
            async with httpx.AsyncClient(transport=transport, base_url="http://sidecar.test") as client:
                discovered = await client.post(
                    "/models",
                    headers={main.SIDECAR_TOKEN_HEADER: "s" * 32},
                    json={"provider": "openai"},
                )
                rejected = await client.post(
                    "/models",
                    headers={main.SIDECAR_TOKEN_HEADER: "s" * 32},
                    json={"provider": "openai", "api_key": "browser-key"},
                )
                unauthorized = await client.post(
                    "/models",
                    json={"provider": "openai", "api_key": "untrusted-key"},
                )

        self.assertEqual(discovered.status_code, 200)
        self.assertEqual(discovered.json()["models"], ["gpt-5.4"])
        self.assertEqual(get.call_args_list[0].kwargs["headers"]["Authorization"], "Bearer server-only-key")
        self.assertEqual(rejected.status_code, 200)
        self.assertEqual(get.call_args_list[1].kwargs["headers"]["Authorization"], "Bearer browser-key")
        self.assertEqual(unauthorized.status_code, 401)


class ApplicationContextTest(unittest.TestCase):
    def test_attaches_structured_go_context_to_instrument_context(self):
        class FakeGraph:
            def resolve_instrument_context(self, ticker, asset_type="stock"):
                return f"instrument={ticker};asset={asset_type}"

        graph = FakeGraph()
        request = main.AnalysisRequest(
            ticker="NVDA",
            analysis_context={
                "trade_journal": {
                    "schema_version": "vibe.trade_record.v1",
                    "records": [{"symbol": "NVDA", "side": "buy", "quantity": 5}],
                }
            },
        )

        main.attach_application_context(graph, request)
        context = graph.resolve_instrument_context("NVDA")

        self.assertIn("instrument=NVDA", context)
        self.assertIn("vibe.trade_record.v1", context)
        self.assertIn('"quantity": 5', context)
        self.assertIn("behavioral context", context)

    def test_a_share_context_uses_cny_and_does_not_penalize_us_only_gaps(self):
        class FakeGraph:
            def resolve_instrument_context(self, ticker, asset_type="stock"):
                return f"instrument={ticker};asset={asset_type}"

        graph = FakeGraph()
        request = main.AnalysisRequest(
            ticker="600519.SH",
            analysis_context={"price": {"currency": "CNY", "close": 1450}},
        )

        main.attach_application_context(graph, request)
        context = graph.resolve_instrument_context("600519.SH")
        payload = main.compact_context_json(request)

        self.assertIn("mainland China A-share", context)
        self.assertIn("CNY/RMB", context)
        self.assertIn("RMB turnover rather than dollar volume", context)
        self.assertIn("SEC filings", context)
        self.assertIn("never a negative fact", context)
        self.assertIn('"market": "china_a"', payload)
        self.assertIn('"currency": "CNY"', payload)
        self.assertIn('"canonical_symbol": "600519.SH"', payload)

    def test_checkpoint_directory_is_scoped_by_run_id(self):
        request = main.AnalysisRequest(ticker="NVDA", run_id="run-nvda-checkpoint-001")
        config = {}
        with tempfile.TemporaryDirectory() as directory, patch.dict(
            main.os.environ, {"AI_ANALYSIS_CHECKPOINT_DIR": directory}
        ):
            main.configure_analysis_checkpoint(config, request)

        self.assertTrue(config["checkpoint_enabled"])
        self.assertEqual(
            config["data_cache_dir"],
            str((Path(directory) / "run-nvda-checkpoint-001").resolve()),
        )

    def test_compact_context_redacts_unaudited_financial_payloads(self):
        request = main.AnalysisRequest(
            ticker="AUPH",
            analysis_context={
                "vibe_trading": {
                    "source_traces": [
                        {
                            "evidence_key": "financial_indicators_annual",
                            "payload": {"data": {"AUPH.US": {"revenue": 77114000}}},
                        }
                    ]
                },
                "data_audit": {"canonical_facts": [{"id": "annual_revenue_2025", "value": 283055000}]},
            },
        )

        payload = main.compact_context_json(request)

        self.assertNotIn("77114000", payload)
        self.assertIn("283055000", payload)
        self.assertIn("governed by data_audit", payload)


class CANSLIMSecondLayerMarketPromptTest(unittest.TestCase):
    def test_us_prompt_is_unchanged(self):
        self.assertEqual(
            main.canslim_second_layer_system_prompt("AAPL"),
            main.CANSLIM_SECOND_LAYER_SYSTEM_PROMPT,
        )

    def test_a_share_prompt_uses_rmb_and_treats_us_only_gaps_as_unavailable(self):
        observed = {}

        class FakeLLM:
            def invoke(self, messages):
                observed["messages"] = messages
                return SimpleNamespace(content="分析完成")

        request = main.AnalysisRequest(
            ticker="000001.SZ",
            analysts=["canslim_second_layer"],
            analysis_context={"price": {"currency": "CNY"}},
        )
        with patch.object(main, "create_role_llm", return_value=FakeLLM()):
            result = main.run_canslim_second_layer_analysis(request)

        system_prompt = observed["messages"][0].content
        user_prompt = observed["messages"][1].content
        self.assertEqual(result, "分析完成")
        self.assertIn("平均人民币成交额", system_prompt)
        self.assertIn("SEC EDGAR", system_prompt)
        self.assertIn("不得据此扣分", system_prompt)
        self.assertIn("不得外推或编造", system_prompt)
        self.assertNotIn("平均美元成交额", system_prompt)
        self.assertNotIn("second_layer_data.sec.insider_transactions", system_prompt)
        self.assertNotIn("增发/ATM", system_prompt)
        self.assertIn('"market": "china_a"', user_prompt)
        self.assertIn('"turnover_unit": "CNY"', user_prompt)


class TradingAgentsMarketConfigurationTest(unittest.TestCase):
    def test_a_share_graph_captures_mode_benchmark_and_yfinance_routes(self):
        from tradingagents.graph import trading_graph as trading_graph_module

        observed = {}

        class PatchContext:
            def __enter__(self):
                return None

            def __exit__(self, *_args):
                return False

        def fake_patch(*, china_a_mode):
            observed["patched_mode"] = china_a_mode
            return PatchContext()

        class FakeGraph:
            def __init__(self, *, selected_analysts, debug, config, callbacks):
                observed["selected_analysts"] = selected_analysts
                observed["config"] = config
                self.config = config

        request = main.AnalysisRequest(
            ticker="600519.SH",
            api_key="request-secret",
            run_id="run-china-a-graph-001",
        )
        with patch.object(main, "configure_analysis_checkpoint"), patch.object(
            main,
            "patched_tradingagents_graph",
            side_effect=fake_patch,
        ), patch.object(trading_graph_module, "TradingAgentsGraph", FakeGraph):
            graph = main.configured_graph(request, analysts=("market", "news"))

        self.assertIs(graph.config, observed["config"])
        self.assertTrue(observed["patched_mode"])
        self.assertTrue(observed["config"]["china_a_mode"])
        self.assertEqual(observed["config"]["benchmark_ticker"], "000300.SS")
        for category in (
            "core_stock_apis",
            "technical_indicators",
            "fundamental_data",
            "news_data",
        ):
            self.assertEqual(observed["config"]["data_vendors"][category], "yfinance")
        # configured_graph removes the request key after clients have copied it.
        self.assertNotIn("api_key", observed["config"])

    def test_a_share_vibe_rendering_names_yahoo_boundary_and_source_provenance(self):
        rendered = main.render_vibe_evidence(
            {
                "market_symbol": "600519.SH",
                "status": "complete",
                "summary": {"requested": 1, "ok": 1, "failed": 0},
                "source_traces": [
                    {
                        "tool": "get_market_data",
                        "role": "market",
                        "status": "ok",
                        "source": "china-a-provider",
                        "duration_ms": 3,
                    }
                ],
            }
        )

        self.assertIn("Yahoo A股行情快照", rendered)
        self.assertIn("`.SH` 转为 `.SS`", rendered)
        self.assertIn("Vibe 实际来源", rendered)
        self.assertIn("单一来源不会被描述为跨源核验", rendered)
        self.assertIn("`china-a-provider`", rendered)


class BrokerGatewayDiscoveryTest(unittest.IsolatedAsyncioTestCase):
    def test_futu_account_selection_honors_configured_account(self):
        account_id = broker_sync._futu_active_account_id(
            [{"acc_id": "12345678"}, {"acc_id": "87654321"}],
            configured_account="87654321",
        )

        self.assertEqual(account_id, "87654321")

    def test_futu_account_selection_rejects_ambiguous_accounts(self):
        with self.assertRaisesRegex(RuntimeError, "自动绑定无法在多个账户中选择"):
            broker_sync._futu_active_account_id(
                [{"acc_id": "12345678"}, {"acc_id": "87654321"}],
                configured_account="",
            )

    def test_futu_account_selection_accepts_single_discovered_account_when_configured_one_is_stale(self):
        account_id = broker_sync._futu_active_account_id(
            [{"acc_id": "12345678"}],
            configured_account="87654321",
        )

        self.assertEqual(account_id, "12345678")

    def test_ibkr_account_selection_requires_the_configured_account_to_match(self):
        account_id = broker_sync._select_ibkr_account(["DU123456", "DU654321"], "DU654321")

        self.assertEqual(account_id, "DU654321")

    def test_ibkr_account_selection_accepts_only_a_sole_account_when_unset(self):
        account_id = broker_sync._select_ibkr_account(["DU123456"], "")

        self.assertEqual(account_id, "DU123456")

    def test_ibkr_account_selection_rejects_ambiguous_or_mismatched_accounts(self):
        with self.assertRaisesRegex(RuntimeError, "多个账户"):
            broker_sync._select_ibkr_account(["DU123456", "DU654321"], "")
        with self.assertRaisesRegex(RuntimeError, "未匹配"):
            broker_sync._select_ibkr_account(["DU123456"], "DU654321")

    def test_gateway_status_surfaces_ambiguous_futu_account_error(self):
        with patch.object(
            broker_sync,
            "_discover_futu_account",
            side_effect=broker_sync.BrokerAccountSelectionError(
                "自动绑定无法在多个账户中选择，请在服务器配置中明确指定 account"
            ),
        ):
            with self.assertRaisesRegex(RuntimeError, "自动绑定无法在多个账户中选择"):
                broker_sync.discover_broker_account("futu", {"host": "127.0.0.1", "port": "11111"})

    def test_gateway_status_returns_futu_account_name(self):
        credentials = {"host": "127.0.0.1", "port": "11111"}
        with patch.object(broker_sync, "_discover_futu_account", return_value="Futu 12345678"):
            result = broker_sync.discover_broker_account("futu", credentials)

        self.assertEqual(result["account_name"], "Futu 12345678")
        self.assertEqual(result["credentials"], credentials)

    def test_gateway_status_rejects_non_loopback_host(self):
        with self.assertRaisesRegex(ValueError, "loopback"):
            broker_sync.discover_broker_account("ibkr", {"host": "broker.example.com", "port": "7497"})

    def test_gateway_status_accepts_literal_localhost_without_name_resolution(self):
        credentials = {"host": "localhost", "port": "7497"}
        with patch.object(broker_sync, "_discover_ibkr_account", return_value="IBKR DU123456"):
            result = broker_sync.discover_broker_account("ibkr", credentials)

        self.assertEqual(result["account_name"], "IBKR DU123456")
        self.assertEqual(result["credentials"], credentials)

    async def test_connection_status_endpoint_returns_discovery_result(self):
        credentials = {"host": "127.0.0.1", "port": "11111"}
        with patch.object(
            main,
            "discover_broker_account",
            return_value={"account_name": "Futu 12345678", "credentials": credentials},
        ) as discover:
            result = await main.broker_connection_status(
                main.BrokerGatewayDiscoveryRequest(broker="futu", credentials=credentials)
            )

        self.assertEqual(result["account_name"], "Futu 12345678")
        discover.assert_called_once_with("futu", credentials)


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
            online_tools=False,
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
            online_tools=False,
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
        request = main.AnalysisRequest(
            ticker="GEV",
            analysts=["fundamental", "sentiment"],
            online_tools=False,
        )

        with patch.object(main, "run_analysis", side_effect=RuntimeError("tool calling failed")):
            events = [event async for event in main.analysis_events(request)]

        self.assertTrue(any(event.startswith("event: error") for event in events))
        self.assertFalse(any(event.startswith("event: report_delta") for event in events))
        self.assertFalse(any(event.startswith("event: done") for event in events))


class VibeEvidenceIntegrationTest(unittest.IsolatedAsyncioTestCase):
    async def test_prefetch_result_is_injected_before_tradingagents_runs(self):
        bundle = {
            "schema_version": "vibe.research_evidence.v1",
            "provider": "vibe-trading",
            "symbol": "AUPH",
            "market_symbol": "AUPH.US",
            "status": "complete",
            "summary": {"requested": 1, "ok": 1, "failed": 0},
            "evidence": {"market": {}},
            "source_traces": [
                {
                    "tool": "get_market_data",
                    "role": "market",
                    "status": "ok",
                    "source": "vibe-trading:auto",
                    "duration_ms": 12,
                }
            ],
        }
        request = main.AnalysisRequest(ticker="AUPH", analysts=["technical"])
        observed_context = {}

        def fake_analysis(received, _callbacks=None):
            observed_context.update(received.analysis_context)
            return {"market_report": "技术结论", "final_trade_decision": "HOLD"}, "HOLD"

        with patch.object(main, "prefetch_vibe_evidence_async", return_value=bundle), patch.object(
            main, "run_analysis", side_effect=fake_analysis
        ), patch.object(main, "REPORT_DELTA_DELAY", 0):
            events = [event async for event in main.analysis_events(request)]

        self.assertEqual(observed_context["vibe_trading"], bundle)
        self.assertEqual(observed_context["vibe_research_pack"]["schema_version"], "vibe.research_pack.v1")
        self.assertEqual(observed_context["data_audit"]["schema_version"], "vibe.data_audit.v1")
        vibe_result_index = next(
            index
            for index, event in enumerate(events)
            if event.startswith("event: result") and '"agent": "VibeEvidence"' in event
        )
        technical_report_index = next(
            index
            for index, event in enumerate(events)
            if event.startswith("event: report_delta") and "技术分析师" in event
        )
        self.assertLess(vibe_result_index, technical_report_index)
        self.assertIn("get_market_data", events[vibe_result_index])
        self.assertTrue(any('"agent": "DataAuditor"' in event for event in events))
        self.assertTrue(any('"agent": "VibeEvidenceReconciler"' in event for event in events))


class VibeDecisionFlowTest(unittest.TestCase):
    def test_research_pack_keeps_failed_evidence_out_of_available_coverage(self):
        pack = vibe_decision_flow.build_research_pack(
            {
                "symbol": "AAPL",
                "status": "partial",
                "source_traces": [
                    {"role": "market", "status": "ok", "source": "yahoo"},
                    {"role": "market", "status": "error", "source": "stooq", "error": "timeout"},
                ],
            }
        )

        self.assertEqual(pack["roles"]["market"]["coverage"], "partial")
        self.assertEqual(pack["roles"]["market"]["available"], 1)
        self.assertEqual(pack["roles"]["market"]["gaps"][0]["reason"], "timeout")

class DataAuditorTest(unittest.TestCase):
    def test_prefers_period_aware_annual_series_and_blocks_ambiguous_vibe_10k_value(self):
        annual_payload = {
            "data": {
                "AUPH.US": {
                    "periods": {
                        "items": [
                            {
                                "REPORT_DATE": "2025-12-31",
                                "FISCAL_YEAR": 2025,
                                "FISCAL_PERIOD": "FY",
                                "FORM": "10-K",
                                "ACCESSION": "0001600620-26-000017",
                                "RevenueFromContractWithCustomerExcludingAssessedTax": 77114000.0,
                            }
                        ]
                    }
                }
            }
        }
        quarterly_payload = {
            "data": {
                "AUPH.US": {
                    "periods": {
                        "items": [
                            {
                                "REPORT_DATE": "2026-03-31",
                                "FISCAL_YEAR": 2026,
                                "FISCAL_PERIOD": "Q1",
                                "FORM": "10-Q",
                                "ACCESSION": "0001600620-26-000049",
                                "EarningsPerShareDiluted": 0.25,
                            }
                        ]
                    }
                }
            }
        }
        bundle = {
            "source_traces": [
                {"evidence_key": "financial_indicators_annual", "payload": annual_payload},
                {"evidence_key": "financial_indicators_quarterly", "payload": quarterly_payload},
            ]
        }
        context = {
            "revenue": {"source": "SEC EDGAR", "years": [{"year": 2025, "revenue": 283055000, "data_sufficient": True}]},
            "eps": {"source": "SEC EDGAR", "quarters": [{"period": "2026Q1", "eps": 0.25, "data_sufficient": True}]},
        }

        audit = data_auditor.audit_data(bundle, context)
        annual = next(item for item in audit["findings"] if item["id"] == "annual_revenue_2025")
        quarter = next(item for item in audit["findings"] if item["id"] == "diluted_eps_2026Q1")

        self.assertEqual(annual["status"], "conflict")
        self.assertEqual(annual["selected"]["value"], 283055000.0)
        self.assertEqual(quarter["status"], "verified")
        self.assertEqual(quarter["selected"]["value"], 0.25)
        self.assertIn("annual_revenue_2025", {item["id"] for item in audit["canonical_facts"]})

    def test_a_share_audit_uses_disclosure_sources_without_sec_claims(self):
        bundle = {
            "market_symbol": "600519.SH",
            "source_traces": [
                {"evidence_key": "financial_indicators_annual", "payload": {"data": {}}},
                {"evidence_key": "financial_indicators_quarterly", "payload": {"data": {}}},
            ],
        }
        context = {
            "stock_summary": {"symbol": "600519.SH", "market": "SSE_MAIN"},
            "revenue": {
                "source": "东方财富 F10",
                "years": [{"year": 2025, "revenue": 174144000000.0, "data_sufficient": True}],
            },
            "eps": {
                "source": "东方财富 F10",
                "quarters": [{"period": "2026Q1", "eps": 19.0, "data_sufficient": True}],
            },
        }

        audit = data_auditor.audit_data(bundle, context)
        rendered = data_auditor.render_data_audit(audit)

        self.assertEqual(audit["status"], "source_only")
        self.assertTrue(all(item["status"] == "source_only" for item in audit["findings"]))
        self.assertTrue(all(item["verification"] == "single_source" for item in audit["canonical_facts"]))
        self.assertEqual({item["source"] for item in audit["canonical_facts"]}, {"东方财富 F10"})
        self.assertNotIn(
            "SEC",
            json.dumps(
                {
                    "scope": audit["scope"],
                    "findings": audit["findings"],
                    "canonical_facts": audit["canonical_facts"],
                },
                ensure_ascii=False,
            ),
        )
        self.assertIn("单一来源", rendered)

    def test_invalid_exchange_prefix_does_not_enter_a_share_source_only_path(self):
        audit = data_auditor.audit_data(
            {"market_symbol": "123456.SZ", "source_traces": []},
            {
                "stock_summary": {"symbol": "123456.SZ"},
                "revenue": {
                    "source": "test",
                    "years": [{"year": 2025, "revenue": 1, "data_sufficient": True}],
                },
            },
        )

        self.assertNotEqual(audit["status"], "source_only")


class ReconnectableAnalyzeEndpointTest(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        main.analysis_run_registry.clear()

    async def test_resume_replays_events_after_cursor(self):
        async def fake_events(_request):
            yield main.sse("progress", {"agent": "Test", "status": "running", "message": "one"})
            yield main.sse("done", {})

        first_request = main.AnalysisRequest(
            ticker="AAPL",
            analysts=["technical"],
            online_tools=False,
            run_id="run-test-aapl-001",
        )
        with patch.object(main, "analysis_events", side_effect=fake_events):
            first_response = await main.analyze(first_request)
            first_events = [chunk async for chunk in first_response.body_iterator]

            resume_request = main.AnalysisRequest(
                ticker="AAPL",
                analysts=["technical"],
                online_tools=False,
                run_id="run-test-aapl-001",
                resume=True,
                last_event_id=1,
            )
            resumed_response = await main.analyze(resume_request)
            resumed_events = [chunk async for chunk in resumed_response.body_iterator]

        self.assertEqual(len(first_events), 2)
        self.assertTrue(str(first_events[0]).startswith("id: 1"))
        self.assertEqual(len(resumed_events), 1)
        self.assertTrue(str(resumed_events[0]).startswith("id: 2"))


if __name__ == "__main__":
    unittest.main()
