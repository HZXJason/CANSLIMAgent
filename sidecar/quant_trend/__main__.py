"""Command-line entrypoint for deterministic prediction and evaluation."""

from __future__ import annotations

import argparse
from datetime import date
import json
import sys

from .data import PriceDataError
from .service import QuantTrendService


def _date(value: str) -> date:
    try:
        return date.fromisoformat(value)
    except ValueError as exc:
        raise argparse.ArgumentTypeError("expected YYYY-MM-DD") from exc


def parser() -> argparse.ArgumentParser:
    root = argparse.ArgumentParser(description="CANSLIMAgent deterministic trend baseline")
    commands = root.add_subparsers(dest="command", required=True)

    predict = commands.add_parser("predict", help="produce one point-in-time trend snapshot")
    predict.add_argument("symbol")
    predict.add_argument("--as-of", type=_date, default=date.today())
    predict.add_argument("--horizon", type=int, default=20)
    predict.add_argument("--universe", default="", help="optional comma-separated symbols")
    predict.add_argument("--universe-version")
    predict.add_argument("--force-refresh", action="store_true")

    evaluate = commands.add_parser("evaluate", help="run a causal single-symbol historical evaluation")
    evaluate.add_argument("symbol")
    evaluate.add_argument("--start", type=_date, required=True)
    evaluate.add_argument("--end", type=_date, required=True)
    evaluate.add_argument("--horizon", type=int, default=20)
    evaluate.add_argument("--step", type=int)
    evaluate.add_argument("--max-nodes", type=int)
    evaluate.add_argument("--cost-bps", type=float, default=10.0)
    return root


def main(argv: list[str] | None = None) -> int:
    args = parser().parse_args(argv)
    service = QuantTrendService()
    try:
        if args.command == "predict":
            result = service.predict(
                symbol=args.symbol,
                as_of=args.as_of,
                horizon_days=args.horizon,
                universe=tuple(item.strip() for item in args.universe.split(",") if item.strip()),
                universe_version=args.universe_version,
                force_refresh=args.force_refresh,
            )
        else:
            result = service.evaluate(
                symbol=args.symbol,
                start_date=args.start,
                end_date=args.end,
                horizon_days=args.horizon,
                step_days=args.step,
                maximum_nodes=args.max_nodes,
                transaction_cost_bps=args.cost_bps,
            )
    except (PriceDataError, ValueError) as exc:
        print(json.dumps({"status": "error", "error": str(exc)}, ensure_ascii=False), file=sys.stderr)
        return 2
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True, default=str))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
