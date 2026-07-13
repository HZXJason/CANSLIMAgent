import asyncio
import unittest

from analysis_runs import AnalysisRunRegistry, RunCapacityError, RunConflictError, request_fingerprint


async def source(*events: str):
    for event in events:
        await asyncio.sleep(0)
        yield event


class AnalysisRunRegistryTest(unittest.IsolatedAsyncioTestCase):
    async def test_reconnect_replays_only_missing_events(self):
        registry = AnalysisRunRegistry()
        fingerprint = request_fingerprint({"ticker": "AAPL"})
        run, created = registry.get_or_create(
            run_id="run-aapl-001",
            fingerprint=fingerprint,
            last_event_id=0,
            resume=False,
            source_factory=lambda: source(
                "event: progress\ndata: {}\n\n",
                "event: done\ndata: {}\n\n",
            ),
        )
        self.assertTrue(created)
        all_events = [event async for event in run.stream_after(0)]
        self.assertEqual(len(all_events), 2)
        self.assertTrue(all_events[0].startswith("id: 1\n"))

        same, created = registry.get_or_create(
            run_id="run-aapl-001",
            fingerprint=fingerprint,
            last_event_id=1,
            resume=True,
            source_factory=lambda: source(),
        )
        self.assertFalse(created)
        missing = [event async for event in same.stream_after(1)]
        self.assertEqual(len(missing), 1)
        self.assertTrue(missing[0].startswith("id: 2\n"))

    async def test_failed_run_can_restart_with_monotonic_event_ids(self):
        registry = AnalysisRunRegistry()
        first, _ = registry.get_or_create(
            run_id="run-aapl-002",
            fingerprint="old",
            last_event_id=0,
            resume=False,
            source_factory=lambda: source("event: error\ndata: {}\n\n"),
        )
        consumed = [event async for event in first.stream_after(0)]
        self.assertTrue(consumed[0].startswith("id: 1\n"))

        resumed, created = registry.get_or_create(
            run_id="run-aapl-002",
            fingerprint="new",
            last_event_id=1,
            resume=True,
            source_factory=lambda: source("event: done\ndata: {}\n\n"),
        )
        self.assertTrue(created)
        events = [event async for event in resumed.stream_after(1)]
        self.assertTrue(events[0].startswith("id: 2\n"))

    async def test_failed_run_replays_unseen_error_before_restart(self):
        registry = AnalysisRunRegistry()
        first, _ = registry.get_or_create(
            run_id="run-aapl-004",
            fingerprint="same",
            last_event_id=0,
            resume=False,
            source_factory=lambda: source(
                "event: progress\ndata: {}\n\n",
                "event: error\ndata: {}\n\n",
            ),
        )
        await first.task
        replay, created = registry.get_or_create(
            run_id="run-aapl-004",
            fingerprint="same",
            last_event_id=1,
            resume=True,
            source_factory=lambda: source(),
        )
        self.assertFalse(created)
        missing = [event async for event in replay.stream_after(1)]
        self.assertEqual(len(missing), 1)
        self.assertTrue(missing[0].startswith("id: 2\n"))

    async def test_active_run_rejects_conflicting_request(self):
        registry = AnalysisRunRegistry()

        async def blocked_source():
            await asyncio.Event().wait()
            yield "event: done\ndata: {}\n\n"

        run, _ = registry.get_or_create(
            run_id="run-aapl-003",
            fingerprint="one",
            last_event_id=0,
            resume=False,
            source_factory=blocked_source,
        )
        with self.assertRaises(RunConflictError):
            registry.get_or_create(
                run_id="run-aapl-003",
                fingerprint="two",
                last_event_id=0,
                resume=True,
                source_factory=lambda: source(),
            )
        if run.task:
            run.task.cancel()
            with self.assertRaises(asyncio.CancelledError):
                await run.task

    async def test_active_run_capacity_is_enforced(self):
        registry = AnalysisRunRegistry(max_active_runs=1)

        async def blocked_source():
            await asyncio.Event().wait()
            yield "event: done\ndata: {}\n\n"

        run, _ = registry.get_or_create(
            run_id="run-capacity-001",
            fingerprint="one",
            last_event_id=0,
            resume=False,
            source_factory=blocked_source,
        )
        with self.assertRaises(RunCapacityError):
            registry.get_or_create(
                run_id="run-capacity-002",
                fingerprint="two",
                last_event_id=0,
                resume=False,
                source_factory=lambda: source(),
            )
        if run.task:
            run.task.cancel()
            with self.assertRaises(asyncio.CancelledError):
                await run.task


if __name__ == "__main__":
    unittest.main()
