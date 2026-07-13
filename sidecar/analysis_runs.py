"""Buffered SSE runs for reconnectable AI analysis streams."""

from __future__ import annotations

import asyncio
import hashlib
import json
import os
import re
import threading
import time
from collections.abc import AsyncIterator, Callable
from dataclasses import dataclass, field
from typing import Any


RUN_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{7,127}$")
RUN_RETENTION_SECONDS = 2 * 60 * 60
MAX_BUFFERED_RUNS = 32
MAX_ACTIVE_RUNS = int(os.getenv("AI_MAX_ACTIVE_RUNS", "4"))
MAX_EVENTS_PER_RUN = int(os.getenv("AI_MAX_EVENTS_PER_RUN", "10000"))
MAX_BUFFERED_BYTES_PER_RUN = int(os.getenv("AI_MAX_BUFFERED_BYTES_PER_RUN", str(16 * 1024 * 1024)))


class RunConflictError(ValueError):
    pass


class RunCapacityError(RuntimeError):
    pass


def validate_run_id(value: str) -> str:
    run_id = str(value or "").strip()
    if not RUN_ID_RE.fullmatch(run_id):
        raise ValueError("run_id must be 8-128 URL-safe characters")
    return run_id


def request_fingerprint(payload: dict[str, Any]) -> str:
    """Return a non-reversible request identity without logging secrets."""
    encoded = json.dumps(payload, ensure_ascii=False, sort_keys=True, default=str).encode("utf-8")
    return hashlib.sha256(encoded).hexdigest()


def _event_with_id(raw_event: str, event_id: int) -> str:
    return f"id: {event_id}\n{raw_event}"


def _terminal_event(raw_event: str) -> str | None:
    if raw_event.startswith("event: done\n"):
        return "done"
    if raw_event.startswith("event: error\n"):
        return "error"
    return None


@dataclass
class BufferedAnalysisRun:
    run_id: str
    fingerprint: str
    next_event_id: int = 1
    events: list[tuple[int, str]] = field(default_factory=list)
    terminal_event: str | None = None
    finished: bool = False
    updated_at: float = field(default_factory=time.monotonic)
    condition: asyncio.Condition = field(default_factory=asyncio.Condition)
    task: asyncio.Task[None] | None = None
    max_events: int = MAX_EVENTS_PER_RUN
    max_buffered_bytes: int = MAX_BUFFERED_BYTES_PER_RUN
    buffered_bytes: int = 0

    def start(self, source: AsyncIterator[str]) -> None:
        self.task = asyncio.create_task(self._pump(source), name=f"ai-analysis-{self.run_id}")

    async def _pump(self, source: AsyncIterator[str]) -> None:
        try:
            async for raw_event in source:
                async with self.condition:
                    event_id = self.next_event_id
                    self.next_event_id += 1
                    encoded = _event_with_id(raw_event, event_id)
                    encoded_bytes = len(encoded.encode("utf-8"))
                    if len(self.events) >= self.max_events or self.buffered_bytes + encoded_bytes > self.max_buffered_bytes:
                        overflow = _event_with_id(
                            'event: error\ndata: {"message":"analysis event buffer limit exceeded"}\n\n',
                            event_id,
                        )
                        self.events.append((event_id, overflow))
                        self.buffered_bytes += len(overflow.encode("utf-8"))
                        self.terminal_event = "error"
                        self.updated_at = time.monotonic()
                        self.condition.notify_all()
                        break
                    self.events.append((event_id, encoded))
                    self.buffered_bytes += encoded_bytes
                    self.terminal_event = _terminal_event(raw_event) or self.terminal_event
                    self.updated_at = time.monotonic()
                    self.condition.notify_all()
        except Exception as exc:  # noqa: BLE001 - preserve a resumable terminal event
            payload = json.dumps(
                {"message": str(exc).strip() or "AI analysis run failed"},
                ensure_ascii=False,
            )
            raw_event = f"event: error\ndata: {payload}\n\n"
            async with self.condition:
                event_id = self.next_event_id
                self.next_event_id += 1
                self.events.append((event_id, _event_with_id(raw_event, event_id)))
                self.terminal_event = "error"
                self.updated_at = time.monotonic()
                self.condition.notify_all()
        finally:
            async with self.condition:
                self.finished = True
                self.updated_at = time.monotonic()
                self.condition.notify_all()

    async def stream_after(self, last_event_id: int) -> AsyncIterator[str]:
        cursor = max(0, int(last_event_id))
        while True:
            async with self.condition:
                await self.condition.wait_for(
                    lambda: any(event_id > cursor for event_id, _ in self.events) or self.finished
                )
                batch = [(event_id, value) for event_id, value in self.events if event_id > cursor]
                finished = self.finished
            for event_id, value in batch:
                cursor = event_id
                yield value
            if finished and not any(event_id > cursor for event_id, _ in self.events):
                return


class AnalysisRunRegistry:
    def __init__(
        self,
        *,
        max_active_runs: int = MAX_ACTIVE_RUNS,
        max_events_per_run: int = MAX_EVENTS_PER_RUN,
        max_buffered_bytes_per_run: int = MAX_BUFFERED_BYTES_PER_RUN,
    ) -> None:
        self._runs: dict[str, BufferedAnalysisRun] = {}
        self._lock = threading.Lock()
        self._max_active_runs = max(1, max_active_runs)
        self._max_events_per_run = max(2, max_events_per_run)
        self._max_buffered_bytes_per_run = max(1024, max_buffered_bytes_per_run)

    def clear(self) -> None:
        with self._lock:
            self._runs.clear()

    def get_or_create(
        self,
        *,
        run_id: str,
        fingerprint: str,
        last_event_id: int,
        resume: bool,
        source_factory: Callable[[], AsyncIterator[str]],
    ) -> tuple[BufferedAnalysisRun, bool]:
        run_id = validate_run_id(run_id)
        with self._lock:
            self._prune_locked()
            existing = self._runs.get(run_id)
            if existing is not None:
                last_buffered_event_id = existing.next_event_id - 1
                if (
                    existing.finished
                    and existing.terminal_event == "error"
                    and resume
                    and int(last_event_id) >= last_buffered_event_id
                ):
                    replacement = BufferedAnalysisRun(
                        run_id=run_id,
                        fingerprint=fingerprint,
                        next_event_id=max(0, int(last_event_id)) + 1,
                    )
                    self._runs[run_id] = replacement
                    replacement.start(source_factory())
                    return replacement, True
                if existing.fingerprint != fingerprint:
                    raise RunConflictError("run_id is already bound to another analysis request")
                return existing, False

            active_count = sum(1 for item in self._runs.values() if not item.finished)
            if active_count >= self._max_active_runs:
                raise RunCapacityError("too many active analysis runs")

            run = BufferedAnalysisRun(
                run_id=run_id,
                fingerprint=fingerprint,
                next_event_id=(max(0, int(last_event_id)) + 1) if resume else 1,
                max_events=self._max_events_per_run,
                max_buffered_bytes=self._max_buffered_bytes_per_run,
            )
            self._runs[run_id] = run
            run.start(source_factory())
            return run, True

    def _prune_locked(self) -> None:
        now = time.monotonic()
        expired = [
            run_id
            for run_id, run in self._runs.items()
            if run.finished and now - run.updated_at > RUN_RETENTION_SECONDS
        ]
        for run_id in expired:
            self._runs.pop(run_id, None)
        if len(self._runs) < MAX_BUFFERED_RUNS:
            return
        finished = sorted(
            (run for run in self._runs.values() if run.finished),
            key=lambda run: run.updated_at,
        )
        while len(self._runs) >= MAX_BUFFERED_RUNS and finished:
            self._runs.pop(finished.pop(0).run_id, None)


analysis_run_registry = AnalysisRunRegistry()
