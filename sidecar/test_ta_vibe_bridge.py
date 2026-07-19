import asyncio
import inspect
import threading
import unittest
from typing import Annotated

from langchain_core.messages import AIMessage, ToolMessage
from langchain_core.tools import InjectedToolArg, StructuredTool
from langgraph.graph import END, START, MessagesState, StateGraph
from langgraph.prebuilt import ToolNode

from vendor.ta_vibe_bridge.mcp_data_tools import make_sync_capable


def _async_only_tool(*, seen=None):
    calls = seen if seen is not None else []

    async def fetch_market_data(
        symbol: str,
        runtime: Annotated[object | None, InjectedToolArg()] = None,
    ):
        calls.append(
            {
                "symbol": symbol,
                "runtime": runtime,
                "thread": threading.current_thread().name,
            }
        )
        return (
            f"market data for {symbol}",
            {"symbol": symbol, "runtime_injected": runtime is not None},
        )

    return StructuredTool(
        name="fetch_market_data",
        description="Fetch market data for one symbol.",
        args_schema={
            "type": "object",
            "properties": {"symbol": {"type": "string"}},
            "required": ["symbol"],
            "additionalProperties": False,
        },
        coroutine=fetch_market_data,
        response_format="content_and_artifact",
        metadata={"provider": "test-mcp"},
    )


class SyncMCPToolAdapterTest(unittest.TestCase):
    def test_async_only_tool_becomes_dual_mode_without_losing_contract(self):
        raw = _async_only_tool()

        with self.assertRaisesRegex(
            NotImplementedError, "does not support sync invocation"
        ):
            raw.invoke({"symbol": "600519.SH"})

        adapted = make_sync_capable(raw)

        self.assertIsNot(adapted, raw)
        self.assertIsNone(raw.func)
        self.assertTrue(callable(adapted.func))
        self.assertIs(adapted.coroutine, raw.coroutine)
        self.assertEqual(adapted.args_schema, raw.args_schema)
        self.assertEqual(adapted.response_format, "content_and_artifact")
        self.assertEqual(adapted.metadata, {"provider": "test-mcp"})
        self.assertEqual(
            inspect.signature(adapted.func), inspect.signature(raw.coroutine)
        )

        sync_result = adapted.invoke({"symbol": "600519.SH"})
        async_result = asyncio.run(adapted.ainvoke({"symbol": "AAPL.US"}))
        self.assertEqual(sync_result, "market data for 600519.SH")
        self.assertEqual(async_result, "market data for AAPL.US")

    def test_sync_tool_is_not_copied(self):
        def fetch(symbol: str) -> str:
            """Fetch a symbol."""
            return symbol

        tool = StructuredTool.from_function(fetch)
        self.assertIs(make_sync_capable(tool), tool)

    def test_synchronous_tool_node_preserves_runtime_and_artifact(self):
        seen = []
        adapted = make_sync_capable(_async_only_tool(seen=seen))
        builder = StateGraph(MessagesState)
        builder.add_node("tools", ToolNode([adapted]))
        builder.add_edge(START, "tools")
        builder.add_edge("tools", END)
        graph = builder.compile()

        result = graph.invoke(
            {
                "messages": [
                    AIMessage(
                        content="",
                        tool_calls=[
                            {
                                "name": adapted.name,
                                "args": {"symbol": "300001.SZ"},
                                "id": "call-a-share",
                                "type": "tool_call",
                            }
                        ],
                    )
                ]
            }
        )

        message = result["messages"][-1]
        self.assertIsInstance(message, ToolMessage)
        self.assertEqual(message.status, "success")
        self.assertEqual(message.tool_call_id, "call-a-share")
        self.assertEqual(message.content, "market data for 300001.SZ")
        self.assertEqual(
            message.artifact,
            {"symbol": "300001.SZ", "runtime_injected": True},
        )
        self.assertIsNotNone(seen[0]["runtime"])


class SyncMCPToolAdapterEventLoopTest(unittest.IsolatedAsyncioTestCase):
    async def test_sync_invocation_is_safe_inside_an_active_event_loop(self):
        seen = []
        adapted = make_sync_capable(_async_only_tool(seen=seen))

        result = adapted.invoke({"symbol": "688981.SH"})

        self.assertEqual(result, "market data for 688981.SH")
        self.assertEqual(seen[0]["thread"], "vibe-mcp-sync-bridge")


if __name__ == "__main__":
    unittest.main()
