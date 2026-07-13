import unittest
from datetime import datetime, timezone
from unittest.mock import patch

import broker_sync
import journal_reflection


class LongbridgeSyncTest(unittest.TestCase):
    def test_surfaces_longbridge_business_errors_returned_with_http_200(self):
        response = type("Response", (), {"status_code": 200, "json": lambda self: {"code": 401, "message": "permission denied"}})()
        with patch("broker_sync.requests.get", return_value=response):
            with self.assertRaisesRegex(RuntimeError, "permission denied"):
                broker_sync._get_json("https://openapi.longbridge.com/v1/trade/execution/history", headers={})

    def test_reads_executions_without_requesting_positions(self):
        requests = []

        def get_json(url, *, params=None, **_kwargs):
            requests.append((url, params))
            return {"data": {"trades": [], "orders": []}}

        with patch.object(broker_sync, "_get_json", side_effect=get_json):
            broker_sync._sync_longport(
                {"access_token": "server-only-token"},
                datetime(2026, 1, 1, tzinfo=timezone.utc),
                datetime(2026, 1, 2, tzinfo=timezone.utc),
            )

        self.assertEqual(requests, [
            ("https://openapi.longbridge.com/v1/trade/execution/history", {"start_at": 1767225600, "end_at": 1767312000}),
            ("https://openapi.longbridge.com/v1/trade/execution/today", None),
            ("https://openapi.longbridge.com/v1/trade/order/history", {"start_at": 1767225600, "end_at": 1767312000, "market": "US"}),
            ("https://openapi.longbridge.com/v1/trade/order/history", {"start_at": 1767225600, "end_at": 1767312000, "market": "HK"}),
            ("https://openapi.longbridge.com/v1/trade/order/today", {"market": "US"}),
            ("https://openapi.longbridge.com/v1/trade/order/today", {"market": "HK"}),
        ])

    def test_merges_history_and_today_executions_with_order_fields_required_by_vibe(self):
        def get_json(url, *, params=None, **_kwargs):
            if url.endswith("/v1/trade/execution/history"):
                return {"data": {"trades": [{"order_id": "order-1", "trade_id": "fill-history", "symbol": "AAPL.US", "quantity": "2", "price": "200", "trade_done_at": "1767225600"}]}}
            if url.endswith("/v1/trade/execution/today"):
                return {"data": {"trades": [{"order_id": "order-2", "trade_id": "fill-today", "symbol": "NVDA.US", "quantity": "1", "price": "180", "trade_done_at": "1767312000"}]}}
            if url.endswith("/v1/trade/order/history"):
                if params["market"] == "US":
                    return {"data": {"orders": [
                        {"order_id": "order-3", "executed_quantity": "3", "executed_price": "175", "side": "Buy", "stock_name": "Tesla", "symbol": "TSLA.US", "currency": "USD", "updated_at": "1767313000"},
                        {"order_id": "order-4", "executed_quantity": "0", "executed_price": "170", "side": "Sell", "stock_name": "Microsoft", "symbol": "MSFT.US", "currency": "USD", "updated_at": "1767314000"},
                    ]}}
                return {"data": {"orders": [{"order_id": "order-1", "executed_quantity": "2", "executed_price": "200", "side": "Buy", "stock_name": "Apple", "symbol": "AAPL.US", "currency": "USD", "updated_at": "1767225600"}]}}
            if url.endswith("/v1/trade/order/today"):
                return {"data": {"orders": []}}
            if url.endswith("/v1/trade/order"):
                details = {
                    "order-1": {"order_id": "order-1", "side": "Buy", "stock_name": "Apple", "currency": "USD"},
                    "order-2": {"order_id": "order-2", "side": "Sell", "stock_name": "NVIDIA", "currency": "USD"},
                }
                return {"data": details[params["order_id"]]}
            self.fail(f"unexpected URL: {url}")

        with patch.object(broker_sync, "_get_json", side_effect=get_json):
            result = broker_sync._sync_longport(
                {"access_token": "server-only-token"},
                datetime(2026, 1, 1, tzinfo=timezone.utc),
                datetime(2026, 1, 2, tzinfo=timezone.utc),
            )

        self.assertEqual([item["external_id"] for item in result["executions"]], ["fill-history", "fill-today", "order:order-3"])
        self.assertEqual(result["positions"], [])
        self.assertEqual([item["side"] for item in result["executions"]], ["Buy", "Sell", "Buy"])
        self.assertTrue(all("T" in item["executed_at"] for item in result["executions"]))
        records = [{"datetime": item["executed_at"], "symbol": item["symbol"], "name": item["name"], "side": item["side"].lower(), "quantity": item["quantity"], "price": item["price"], "amount": item["quantity"] * item["price"], "fee": item["fee"], "market": "us"} for item in result["executions"]]
        validation = journal_reflection.analyze_trade_records(records)["validation"]
        self.assertEqual(validation["valid_records"], 3)
        self.assertEqual(validation["invalid_records"], 0)
