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

        validation = {
            "schema_version": "vibe.execution_validation.v1",
            "status": "complete",
            "reflection": "The signal held up in backtest.",
        }
        with patch.object(main, "prefetch_vibe_evidence_async", return_value=bundle), patch.object(
            main, "run_analysis", side_effect=fake_analysis
        ), patch.object(main, "validate_execution_plan", return_value=validation), patch.object(main, "REPORT_DELTA_DELAY", 0):
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
        self.assertTrue(any('"agent": "VibeExecutionValidation"' in event for event in events))


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

    def test_execution_gate_only_blocks_completed_negative_validation(self):
        action, _ = vibe_decision_flow.apply_execution_gate(
            "BUY", {"status": "unavailable", "error": "source offline"}
        )
        self.assertEqual(action, "BUY")

        action, note = vibe_decision_flow.apply_execution_gate(
            "BUY", {"status": "complete", "reflection": "The signal did not hold up in backtest."}
        )
        self.assertEqual(action, "HOLD")
        self.assertIn("未通过", note)


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
