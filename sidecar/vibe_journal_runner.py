"""Execute Vibe-Trading's journal implementation inside its own interpreter.

The sidecar launches this file with ``Vibe-Trading/.venv/bin/python`` and sends
``vibe.trade_record.v1`` JSON over stdin.  Keeping this tiny runner free of
sidecar imports preserves the dependency boundary between TradingAgents and
Vibe-Trading.
"""

from __future__ import annotations

import json
import math
import os
import sys
from pathlib import Path
from typing import Any


def _json_safe(value: Any) -> Any:
    if value is None or isinstance(value, (str, bool, int)):
        return value
    if isinstance(value, float):
        return value if math.isfinite(value) else None
    if isinstance(value, dict):
        return {str(key): _json_safe(item) for key, item in value.items()}
    if isinstance(value, (list, tuple)):
        return [_json_safe(item) for item in value]
    if hasattr(value, "item"):
        try:
            return _json_safe(value.item())
        except (TypeError, ValueError):
            pass
    if hasattr(value, "isoformat"):
        try:
            return value.isoformat()
        except (TypeError, ValueError):
            pass
    return str(value)


def main() -> int:
    vibe_root = Path(os.environ["VIBE_TRADING_ROOT"]).expanduser().resolve()
    agent_root = vibe_root / "agent"
    sys.path.insert(0, str(agent_root))

    from src.tools.trade_journal_parsers import TradeRecord, records_to_dataframe
    from src.tools.trade_journal_tool import (
        _compute_behavior,
        _compute_profile,
        pair_trades_fifo,
    )

    payload = json.load(sys.stdin)
    raw_records = payload.get("records") if isinstance(payload, dict) else None
    if not isinstance(raw_records, list):
        raise ValueError("records must be an array")

    fields = tuple(TradeRecord.__dataclass_fields__)
    records = [TradeRecord(**{field: raw[field] for field in fields}) for raw in raw_records]
    dataframe = records_to_dataframe(records)
    result = {
        "engine": "vibe-trading",
        "implementation": {
            "trade_record": "src.tools.trade_journal_parsers.TradeRecord",
            "dataframe": "src.tools.trade_journal_parsers.records_to_dataframe",
            "profile": "src.tools.trade_journal_tool._compute_profile",
            "behavior": "src.tools.trade_journal_tool._compute_behavior",
            "fifo": "src.tools.trade_journal_tool.pair_trades_fifo",
        },
        "record_count": len(records),
        "profile": _compute_profile(dataframe),
        "behavior": _compute_behavior(dataframe),
        "roundtrips": pair_trades_fifo(dataframe),
    }
    json.dump(_json_safe(result), sys.stdout, ensure_ascii=False, allow_nan=False)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
