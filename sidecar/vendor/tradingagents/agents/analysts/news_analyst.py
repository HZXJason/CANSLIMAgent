from langchain_core.prompts import ChatPromptTemplate, MessagesPlaceholder

from tradingagents.agents.utils.agent_utils import (
    get_global_news,
    get_instrument_context_from_state,
    get_language_instruction,
    get_macro_indicators,
    get_news,
    get_prediction_markets,
)


def news_tools_for_market(china_a_mode: bool):
    if china_a_mode:
        return [get_news]
    return [
        get_news,
        get_global_news,
        get_macro_indicators,
        get_prediction_markets,
    ]


def news_system_message(asset_label: str, china_a_mode: bool) -> str:
    if china_a_mode:
        return (
            "You are a mainland China A-share company-news researcher. Analyze only recent, "
            "ticker-specific A-share company news returned by get_news(ticker, start_date, end_date) "
            f"for this {asset_label}, together with the application context supplied in the instrument "
            "context. Do not request or infer US/global macro, FRED, prediction-market, SEC, insider, "
            "StockTwits, Reddit, or US-options evidence. Missing US-only evidence is not adverse evidence "
            "and must not reduce a company score; keep it unknown or not applicable. If company-news "
            "coverage is missing or insufficient, say so, lower confidence, and use NEUTRAL/HOLD rather "
            "than inventing a directional view. Treat monetary context as CNY unless explicitly labeled "
            "otherwise. Provide specific insights only when supported by the supplied sources."
            " Make sure to append a Markdown table at the end of the report to organize key points."
            + get_language_instruction()
        )
    return (
        f"You are a news researcher tasked with analyzing recent news and trends over the past week. Please write a comprehensive report of the current state of the world that is relevant for trading and macroeconomics. Use the available tools: get_news(ticker, start_date, end_date) for {asset_label}-specific news by ticker symbol, get_global_news(curr_date, look_back_days, limit) for broader macroeconomic news, get_macro_indicators(indicator, curr_date, look_back_days) to ground macro commentary in actual data from FRED (e.g. 'cpi', 'core_pce', 'unemployment', 'fed_funds_rate', '10y_treasury', 'yield_curve'), and get_prediction_markets(topic, limit) for live market-implied probabilities of forward-looking events (e.g. 'Fed rate cut', 'recession 2026', geopolitical or sector events). Provide specific, actionable insights with supporting evidence to help traders make informed decisions."
        + " Make sure to append a Markdown table at the end of the report to organize key points in the report, organized and easy to read."
        + get_language_instruction()
    )


def create_news_analyst(llm, *, china_a_mode: bool = False):
    def news_analyst_node(state):
        current_date = state["trade_date"]
        asset_type = state.get("asset_type", "stock")
        asset_label = "company" if asset_type == "stock" else "asset"
        instrument_context = get_instrument_context_from_state(state)

        tools = news_tools_for_market(china_a_mode)
        system_message = news_system_message(asset_label, china_a_mode)

        prompt = ChatPromptTemplate.from_messages(
            [
                (
                    "system",
                    "You are a helpful AI assistant, collaborating with other assistants."
                    " Use the provided tools to progress towards answering the question."
                    " If you are unable to fully answer, that's OK; another assistant with different tools"
                    " will help where you left off. Execute what you can to make progress."
                    " When returning the final report without a tool call, end it with exactly these three explicit lines:"
                    " EVIDENCE STATUS: **SUFFICIENT** or **INSUFFICIENT**;"
                    " DIRECTIONAL OUTLOOK: **BULLISH**, **NEUTRAL**, or **BEARISH**;"
                    " FINAL TRANSACTION PROPOSAL: **BUY**, **HOLD**, or **SELL**."
                    " Directional outlook predicts price direction over the horizon stated in application context,"
                    " independently from position action. A bullish outlook may coexist with HOLD."
                    " If point-in-time news is missing or too incomplete to support a forecast, mark evidence"
                    " INSUFFICIENT and use NEUTRAL and HOLD; that node will be excluded from accuracy."
                    " You have access to the following tools: {tool_names}."
                    " Today's date is {current_date}; treat it as 'now' for all analysis and tool-call date ranges. {instrument_context}\n"
                    "{system_message}",
                ),
                MessagesPlaceholder(variable_name="messages"),
            ]
        )

        prompt = prompt.partial(system_message=system_message)
        prompt = prompt.partial(tool_names=", ".join([tool.name for tool in tools]))
        prompt = prompt.partial(current_date=current_date)
        prompt = prompt.partial(instrument_context=instrument_context)

        chain = prompt | llm.bind_tools(tools)
        result = chain.invoke(state["messages"])

        report = ""

        if len(result.tool_calls) == 0:
            report = result.content

        return {
            "messages": [result],
            "news_report": report,
        }

    return news_analyst_node
