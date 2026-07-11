"""Write backtest outcomes back into TradingAgents' decision log — in pure stdlib.

This mirrors the on-disk format of ``TradingMemoryLog`` (TradingAgents'
``tradingagents/agents/utils/memory.py``) exactly, so the bridge can append and
resolve entries without importing TradingAgents or running its Python. The
format contract (verified against memory.py):

* entries are separated by ``\\n\\n<!-- ENTRY_END -->\\n\\n`` (an HTML comment an
  LLM won't emit);
* a *pending* entry's tag line is ``[date | ticker | rating | pending]`` and its
  body is ``DECISION:\\n<markdown>``;
* a *resolved* entry's tag becomes
  ``[date | ticker | rating | +r.r% | +a.a% | Nd]`` and gains a trailing
  ``REFLECTION:\\n<prose>`` section;
* returns are formatted ``{:+.1%}``.

Because TradingAgents' PM reads resolved same-ticker entries (tag + DECISION +
REFLECTION) via ``get_past_context``, writing the backtest verdict into the
REFLECTION prose is what feeds the loop back into the next decision — no change
to TradingAgents source is required.

Two design choices worth stating:

* We keep this a faithful *reimplementation*, not a fork of the parser. The unit
  tests parse our output with TradingAgents' own regexes to guarantee the two
  never drift (see tests/test_memory_roundtrip.py).
* Resolving competes with TradingAgents' built-in ``_resolve_pending_entries``
  for the same pending slot (whoever runs first wins). The closed loop resolves
  immediately after ``propagate()`` stores the pending entry and before the next
  ``propagate()`` for that ticker, so the backtest verdict lands first. This is
  documented in the closed-loop module and the README.
"""

from __future__ import annotations

from pathlib import Path

_SEPARATOR = "\n\n<!-- ENTRY_END -->\n\n"


class MemoryWriteback:
    """Append pending decisions and resolve them with backtest outcomes.

    Format-compatible with TradingAgents' ``TradingMemoryLog``; safe to point at
    the same ``trading_memory.md`` that TradingAgents itself writes.
    """

    def __init__(self, log_path: str | Path):
        self.log_path = Path(log_path).expanduser()
        self.log_path.parent.mkdir(parents=True, exist_ok=True)

    # --- write ---------------------------------------------------------------

    def store_decision(self, ticker: str, trade_date: str, decision_md: str, rating: str) -> bool:
        """Append a pending entry. Idempotent on ``(date, ticker)`` like TA.

        Returns True if a new entry was written, False if one already existed.
        """
        if self.log_path.exists():
            raw = self.log_path.read_text(encoding="utf-8")
            for line in raw.splitlines():
                if line.startswith(f"[{trade_date} | {ticker} |") and line.endswith("| pending]"):
                    return False
        tag = f"[{trade_date} | {ticker} | {rating} | pending]"
        entry = f"{tag}\n\nDECISION:\n{decision_md}{_SEPARATOR}"
        with open(self.log_path, "a", encoding="utf-8") as f:
            f.write(entry)
        return True

    def update_with_outcome(
        self,
        ticker: str,
        trade_date: str,
        raw_return: float,
        alpha_return: float,
        holding_days: int,
        reflection: str,
    ) -> bool:
        """Resolve the first matching pending entry with an atomic rewrite.

        Returns True if a pending entry was found and resolved, else False.
        Uses temp-file + ``replace`` so a crash mid-write can't corrupt the log.
        """
        if not self.log_path.exists():
            return False

        blocks = self.log_path.read_text(encoding="utf-8").split(_SEPARATOR)
        pending_prefix = f"[{trade_date} | {ticker} |"
        raw_pct = f"{raw_return:+.1%}"
        alpha_pct = f"{alpha_return:+.1%}"

        updated = False
        new_blocks: list[str] = []
        for block in blocks:
            stripped = block.strip()
            if updated or not stripped:
                new_blocks.append(block)
                continue
            lines = stripped.splitlines()
            tag_line = lines[0].strip()
            if tag_line.startswith(pending_prefix) and tag_line.endswith("| pending]"):
                fields = [f.strip() for f in tag_line[1:-1].split("|")]
                rating = fields[2]
                new_tag = (
                    f"[{trade_date} | {ticker} | {rating}"
                    f" | {raw_pct} | {alpha_pct} | {holding_days}d]"
                )
                rest = "\n".join(lines[1:])
                new_blocks.append(
                    f"{new_tag}\n\n{rest.lstrip()}\n\nREFLECTION:\n{reflection}"
                )
                updated = True
            else:
                new_blocks.append(block)

        if not updated:
            return False

        new_text = _SEPARATOR.join(new_blocks)
        tmp = self.log_path.with_suffix(".tmp")
        tmp.write_text(new_text, encoding="utf-8")
        tmp.replace(self.log_path)
        return True

    # --- read (for verification / dry runs) ----------------------------------

    def has_pending(self, ticker: str, trade_date: str) -> bool:
        if not self.log_path.exists():
            return False
        prefix = f"[{trade_date} | {ticker} |"
        for line in self.log_path.read_text(encoding="utf-8").splitlines():
            if line.startswith(prefix) and line.endswith("| pending]"):
                return True
        return False
