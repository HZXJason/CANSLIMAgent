"""Read-only broker adapters used by the local AlphaLens sidecar.

Each adapter returns a deliberately small, vendor-neutral payload.  Secrets are
received from the Go backend for one request only and are never written to disk
or emitted in errors.  No order, replace, cancel, or account-mutating API is
imported by this module.
"""

from __future__ import annotations

from datetime import datetime, timezone
import ipaddress
from typing import Any, Callable

import requests


def discover_broker_account(broker: str, credentials: dict[str, str]) -> dict[str, object]:
    """Read a locally logged-in gateway's active account without placing orders."""
    normalized_broker = broker.strip().lower()
    adapters: dict[str, Callable[[dict[str, str]], str]] = {
        "futu": _discover_futu_account,
        "ibkr": _discover_ibkr_account,
    }
    adapter = adapters.get(normalized_broker)
    if adapter is None:
        raise ValueError("unsupported broker")

    _require_loopback_gateway(credentials)
    try:
        account_name = adapter(credentials)
    except BrokerAccountSelectionError:
        raise
    except Exception as exc:
        raise RuntimeError(_gateway_unavailable_message(normalized_broker)) from exc

    if not account_name.strip():
        raise RuntimeError(_gateway_unavailable_message(normalized_broker))
    return {"account_name": account_name, "credentials": dict(credentials)}


def _require_loopback_gateway(credentials: dict[str, str]) -> None:
    host = credentials.get("host", "").strip()
    # `localhost` is accepted as a literal only; do not DNS-resolve a hostname
    # supplied for broker gateway discovery.
    if host.casefold() == "localhost":
        return
    try:
        is_loopback = ipaddress.ip_address(host).is_loopback
    except ValueError as exc:
        raise ValueError("broker gateway host must be a loopback address") from exc
    if not is_loopback:
        raise ValueError("broker gateway host must be a loopback address")


def _discover_futu_account(credentials: dict[str, str]) -> str:
    try:
        from futu import OpenSecTradeContext, RET_OK
    except ImportError as exc:
        raise RuntimeError("富途适配器未安装") from exc

    context = OpenSecTradeContext(host=credentials["host"], port=int(credentials["port"]))
    try:
        status, accounts = context.get_acc_list()
        if status != RET_OK:
            raise RuntimeError("富途 OpenD 无法读取账户")
        account_id = _futu_active_account_id(accounts, credentials.get("account", ""))
        if not account_id:
            raise RuntimeError("富途 OpenD 未返回已登录账户")
        return f"Futu {account_id}"
    finally:
        context.close()


def _discover_ibkr_account(credentials: dict[str, str]) -> str:
    try:
        from ib_insync import IB
    except ImportError as exc:
        raise RuntimeError("IBKR 适配器未安装") from exc

    ib = IB()
    client_id = int(credentials.get("client_id", "9901"))
    try:
        ib.connect(credentials["host"], int(credentials["port"]), clientId=client_id, readonly=True, timeout=15)
        accounts = ib.managedAccounts()
        account_id = next((str(account).strip() for account in accounts if str(account).strip()), "")
        if not account_id:
            raise RuntimeError("IBKR 未返回已登录账户")
        return f"IBKR {account_id}"
    finally:
        if ib.isConnected():
            ib.disconnect()


def _futu_active_account_id(accounts: Any, configured_account: str) -> str:
    rows = _futu_account_rows(accounts)
    return _select_discovered_account(
        (_first(row, "acc_id", "account_id", "account") for row in rows),
        configured_account,
    )


def _select_discovered_account(account_ids: Any, configured_account: str) -> str:
    discovered = list(dict.fromkeys(account_id for account_id in account_ids if account_id))
    configured = str(configured_account).strip()
    if configured and configured in discovered:
        return configured
    if len(discovered) == 1:
        return discovered[0]
    if len(discovered) > 1:
        # Futu's get_acc_list() returns an account list, not a documented
        # active-account selection. Its order is not a safe binding signal.
        if configured:
            raise BrokerAccountSelectionError(
                "服务器配置的 account 未匹配已发现账户，自动绑定无法在多个账户中选择，请检查 account 配置"
            )
        raise BrokerAccountSelectionError("自动绑定无法在多个账户中选择，请在服务器配置中明确指定 account")
    return ""


def _futu_account_rows(accounts: Any) -> list[dict[str, Any]]:
    if hasattr(accounts, "to_dict"):
        try:
            rows = accounts.to_dict("records")
        except (TypeError, ValueError):
            rows = []
    elif isinstance(accounts, list):
        rows = accounts
    elif isinstance(accounts, dict):
        rows = accounts.get("data", [])
    else:
        rows = []
    return [row for row in rows if isinstance(row, dict)]


class BrokerAccountSelectionError(RuntimeError):
    """A local gateway exposed multiple accounts without a safe selection rule."""


def _gateway_unavailable_message(broker: str) -> str:
    if broker == "futu":
        return "未检测到已登录的富途本地网关，请确认 OpenD 已启动、已登录且 API 端口可访问"
    return "未检测到已登录的 IBKR 本地网关，请确认 TWS 或 IB Gateway 已启动、已登录且 API 已开启"


def sync_broker(
    broker: str,
    credentials: dict[str, str],
    start_at: datetime,
    end_at: datetime,
) -> dict[str, list[dict[str, Any]]]:
    adapters: dict[str, Callable[..., dict[str, list[dict[str, Any]]]]] = {
        "longport": _sync_longport,
        "futu": _sync_futu,
        "tiger": _sync_tiger,
        "ibkr": _sync_ibkr,
    }
    adapter = adapters.get(broker)
    if adapter is None:
        raise ValueError("unsupported broker")
    return adapter(credentials, start_at, end_at)


def _sync_longport(credentials: dict[str, str], start_at: datetime, end_at: datetime) -> dict[str, list[dict[str, Any]]]:
    base_url = credentials.get("base_url", "https://openapi.longportapp.com").rstrip("/")
    token = credentials["access_token"]
    headers = {"Authorization": f"Bearer {token}", "Accept": "application/json"}
    positions_payload = _get_json(f"{base_url}/v1/trade/asset/stock", headers=headers)
    history_payload = _get_json(
        f"{base_url}/v1/trade/execution/history",
        headers=headers,
        params={"start_at": int(start_at.timestamp()), "end_at": int(end_at.timestamp())},
    )
    positions = [_position_from_mapping(row) for row in _items(positions_payload)]
    executions = [_execution_from_mapping(row) for row in _items(history_payload)]
    return _result(positions, executions)


def _sync_futu(credentials: dict[str, str], start_at: datetime, end_at: datetime) -> dict[str, list[dict[str, Any]]]:
    try:
        from futu import OpenSecTradeContext, RET_OK
    except ImportError as exc:
        raise RuntimeError("富途适配器未安装；请在 sidecar 环境安装 futu-api") from exc

    context = OpenSecTradeContext(host=credentials["host"], port=int(credentials["port"]))
    try:
        status, position_frame = context.position_list_query(acc_id=_optional_int(credentials.get("account")))
        if status != RET_OK:
            raise RuntimeError("富途 OpenD 无法读取持仓")
        status, execution_frame = context.history_deal_list_query(
            start=start_at.strftime("%Y-%m-%d"), end=end_at.strftime("%Y-%m-%d"), acc_id=_optional_int(credentials.get("account"))
        )
        if status != RET_OK:
            raise RuntimeError("富途 OpenD 无法读取历史成交")
        positions = [_position_from_mapping(row) for row in position_frame.to_dict("records")]
        executions = [_execution_from_mapping(row) for row in execution_frame.to_dict("records")]
        return _result(positions, executions)
    finally:
        context.close()


def _sync_tiger(credentials: dict[str, str], start_at: datetime, end_at: datetime) -> dict[str, list[dict[str, Any]]]:
    try:
        from tigeropen.tiger_open_config import TigerOpenClientConfig
        from tigeropen.trade.trade_client import TradeClient
    except ImportError as exc:
        raise RuntimeError("老虎适配器未安装；请在 sidecar 环境安装 tigeropen") from exc

    config = TigerOpenClientConfig(sandbox_debug=credentials.get("sandbox", "false").lower() == "true")
    config.tiger_id = credentials["tiger_id"]
    config.private_key = credentials["private_key"]
    config.account = credentials["account"]
    client = TradeClient(config)
    account = credentials["account"]
    try:
        positions_raw = client.get_positions(account=account)
        executions_raw = client.get_filled_orders(
            account=account,
            start_time=start_at.strftime("%Y-%m-%d %H:%M:%S"),
            end_time=end_at.strftime("%Y-%m-%d %H:%M:%S"),
        )
    except Exception as exc:
        raise RuntimeError("老虎账户读取失败，请确认 OpenAPI 凭据、账户地区与历史成交权限") from exc
    positions = [_position_from_mapping(_object_mapping(row)) for row in positions_raw or []]
    executions = [_execution_from_mapping(_object_mapping(row)) for row in executions_raw or []]
    return _result(positions, executions)


def _sync_ibkr(credentials: dict[str, str], start_at: datetime, end_at: datetime) -> dict[str, list[dict[str, Any]]]:
    try:
        from ib_insync import ExecutionFilter, IB
    except ImportError as exc:
        raise RuntimeError("IBKR 适配器未安装；请在 sidecar 环境安装 ib-insync，并启动 TWS 或 IB Gateway") from exc

    ib = IB()
    client_id = int(credentials.get("client_id", "9901"))
    try:
        ib.connect(credentials["host"], int(credentials["port"]), clientId=client_id, readonly=True, timeout=15)
        positions = [
            {
                "symbol": item.contract.symbol,
                "name": item.contract.localSymbol or item.contract.symbol,
                "quantity": float(item.position),
                "market": _ibkr_market(item.contract.exchange, item.contract.currency),
                "currency": item.contract.currency or "",
            }
            for item in ib.positions()
        ]
        fills = ib.reqExecutions(ExecutionFilter(time=start_at.strftime("%Y%m%d %H:%M:%S")))
        executions = []
        for fill in fills:
            execution = fill.execution
            if execution.time.replace(tzinfo=timezone.utc) > end_at.astimezone(timezone.utc):
                continue
            executions.append(
                {
                    "external_id": execution.execId,
                    "symbol": fill.contract.symbol,
                    "name": fill.contract.localSymbol or fill.contract.symbol,
                    "side": execution.side,
                    "quantity": float(execution.shares),
                    "price": float(execution.price),
                    "fee": float(getattr(fill.commissionReport, "commission", 0) or 0),
                    "currency": fill.contract.currency or "",
                    "market": _ibkr_market(fill.contract.exchange, fill.contract.currency),
                    "executed_at": execution.time.replace(tzinfo=timezone.utc).isoformat(),
                }
            )
        return _result(positions, executions)
    except Exception as exc:
        raise RuntimeError("IBKR 读取失败，请确认 TWS/IB Gateway 已登录、API 已开启且端口可达") from exc
    finally:
        if ib.isConnected():
            ib.disconnect()


def _get_json(url: str, *, headers: dict[str, str], params: dict[str, Any] | None = None) -> Any:
    response = requests.get(url, headers=headers, params=params, timeout=25)
    if response.status_code >= 400:
        raise RuntimeError("长桥 OpenAPI 返回授权或账户读取错误")
    return response.json()


def _items(payload: Any) -> list[dict[str, Any]]:
    if isinstance(payload, list):
        return [item for item in payload if isinstance(item, dict)]
    if not isinstance(payload, dict):
        return []
    for key in ("data", "list", "items", "positions", "executions"):
        value = payload.get(key)
        if isinstance(value, list):
            return [item for item in value if isinstance(item, dict)]
        if isinstance(value, dict):
            nested = _items(value)
            if nested:
                return nested
    return []


def _position_from_mapping(row: dict[str, Any]) -> dict[str, Any]:
    return {
        "symbol": _first(row, "symbol", "stock_code", "contract_code", "ticker"),
        "name": _first(row, "name", "stock_name", "contract_name"),
        "quantity": _number(_first(row, "quantity", "available_quantity", "qty", "position")),
        "market": _first(row, "market", "market_name", "exchange"),
        "currency": _first(row, "currency", "currency_name"),
    }


def _execution_from_mapping(row: dict[str, Any]) -> dict[str, Any]:
    return {
        "external_id": _first(row, "execution_id", "exec_id", "deal_id", "id", "order_id"),
        "symbol": _first(row, "symbol", "stock_code", "contract_code", "ticker"),
        "name": _first(row, "name", "stock_name", "contract_name"),
        "side": _first(row, "side", "trd_side", "action"),
        "quantity": _number(_first(row, "quantity", "filled_quantity", "dealt_qty", "filled", "shares")),
        "price": _number(_first(row, "price", "executed_price", "dealt_avg_price", "avg_fill_price")),
        "fee": _number(_first(row, "fee", "commission", "total_commission")),
        "currency": _first(row, "currency", "currency_name"),
        "market": _first(row, "market", "market_name", "exchange"),
        "executed_at": _first(row, "trade_done_at", "executed_at", "time", "create_time", "updated_at"),
    }


def _result(positions: list[dict[str, Any]], executions: list[dict[str, Any]]) -> dict[str, list[dict[str, Any]]]:
    return {"positions": positions, "executions": executions, "warnings": []}


def _first(row: dict[str, Any], *keys: str) -> str:
    for key in keys:
        value = row.get(key)
        if value is not None and str(value).strip():
            return str(value).strip()
    return ""


def _number(value: Any) -> float:
    try:
        return float(value)
    except (TypeError, ValueError):
        return 0.0


def _object_mapping(value: Any) -> dict[str, Any]:
    if isinstance(value, dict):
        return value
    return vars(value) if hasattr(value, "__dict__") else {}


def _optional_int(value: str | None) -> int | None:
    try:
        return int(value) if value else None
    except ValueError:
        return None


def _ibkr_market(exchange: str, currency: str) -> str:
    if currency == "HKD":
        return "hk"
    if currency in {"CNY", "CNH"}:
        return "china_a"
    if currency == "USD":
        return "us"
    return "other"
