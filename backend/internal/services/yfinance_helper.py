import json
import math
import sys
from io import BytesIO
from urllib.request import Request, urlopen


def emit(payload):
    print(json.dumps(payload, ensure_ascii=False))


def fail(message):
    emit({"error": message})
    return 1


try:
    import pandas as pd
    import yfinance as yf
except Exception as exc:  # pragma: no cover - runtime dependency check
    sys.exit(fail(f"yfinance 未安装或不可用：{exc}"))


def clean_float(value):
    try:
        if value is None or pd.isna(value):
            return None
        value = float(value)
        if math.isnan(value) or math.isinf(value):
            return None
        return round(value, 2)
    except Exception:
        return None


def read_html_tables(url):
    request = Request(url, headers={"User-Agent": "Mozilla/5.0 AlphaLens/1.0"})
    with urlopen(request, timeout=30) as response:
        return pd.read_html(BytesIO(response.read()))


def growth(current, previous):
    current = clean_float(current)
    previous = clean_float(previous)
    if current is None or previous in (None, 0):
        return None
    return round((current - previous) / abs(previous) * 100, 2)


def flatten_columns(frame):
    if frame is not None and isinstance(frame.columns, pd.MultiIndex):
        frame = frame.copy()
        frame.columns = [column[0] if isinstance(column, tuple) else column for column in frame.columns]
    return frame


def price(symbol):
    frame = yf.download(symbol, period="13mo", interval="1d", auto_adjust=True, progress=False, threads=False)
    frame = flatten_columns(frame)
    if frame is None or frame.empty or "Close" not in frame or "Volume" not in frame:
        return fail("yfinance 价格数据为空")

    frame = frame[["Close", "Volume"]].dropna()
    if len(frame) < 60:
        emit({"sufficient": False})
        return 0

    last = frame.iloc[-1]
    first = frame.iloc[0]
    volume_window = frame.iloc[-51:-1]
    avg_volume = clean_float(volume_window["Volume"].mean())
    if not avg_volume or clean_float(first["Close"]) in (None, 0):
        emit({"sufficient": False})
        return 0

    volume_ratio = round(float(last["Volume"]) / avg_volume * 100, 2)
    twelve_month_return = round((float(last["Close"]) - float(first["Close"])) / abs(float(first["Close"])) * 100, 2)
    emit(
        {
            "volume_ratio": volume_ratio,
            "twelve_month_return": twelve_month_return,
            "sufficient": True,
            "data_as_of": frame.index[-1].date().isoformat(),
        }
    )
    return 0


def candles(symbol, period):
    settings = {
        "daily": ("2y", "1d", 365),
        "weekly": ("3y", "1wk", 104),
        "monthly": ("7y", "1mo", 60),
    }
    if period not in settings:
        return fail("不支持的周期")
    download_period, interval, limit = settings[period]
    frame = yf.download(symbol, period=download_period, interval=interval, auto_adjust=False, progress=False, threads=False)
    frame = flatten_columns(frame)
    required = ["Open", "High", "Low", "Close", "Volume"]
    if frame is None or frame.empty or any(column not in frame for column in required):
        return fail("yfinance K线数据为空")

    frame = frame[required].dropna().tail(limit)
    items = []
    for date, row in frame.iterrows():
        items.append(
            {
                "date": date.date().isoformat(),
                "open": clean_float(row["Open"]),
                "high": clean_float(row["High"]),
                "low": clean_float(row["Low"]),
                "close": clean_float(row["Close"]),
                "volume": int(row["Volume"]) if clean_float(row["Volume"]) is not None else 0,
            }
        )
    emit({"candles": items})
    return 0


def pick_eps_row(frame):
    if frame is None or frame.empty:
        return None
    candidates = ["Diluted EPS", "Basic EPS", "Basic EPS from Continuing Operations", "Diluted EPS from Continuing Operations"]
    for key in candidates:
        if key in frame.index:
            return frame.loc[key].dropna()
    return None


def pick_statement_row(frame, candidates):
    if frame is None or frame.empty:
        return None
    for key in candidates:
        if key in frame.index:
            return frame.loc[key].dropna()
    return None


def info_number(info, key, digits=2):
    return clean_float(info.get(key))


def info_percent(info, key):
    value = clean_float(info.get(key))
    if value is None:
        return None
    return round(value * 100, 2)


def info_int(info, key):
    value = clean_float(info.get(key))
    if value is None:
        return None
    return int(value)


def date_text(value):
    try:
        if value is None or pd.isna(value):
            return None
    except Exception:
        if value is None:
            return None
    try:
        if isinstance(value, (int, float)):
            return pd.to_datetime(value, unit="s").date().isoformat()
        return pd.Timestamp(value).date().isoformat()
    except Exception:
        text = str(value).strip()
        return text or None


def pick_scalar(series, date, candidates):
    if series is None:
        return None
    for key in candidates:
        if key in series.index:
            try:
                return clean_float(series.loc[key, date])
            except Exception:
                try:
                    return clean_float(series.loc[key])
                except Exception:
                    continue
    return None


def frame_dates(*frames):
    dates = []
    seen = set()
    for frame in frames:
        if frame is None or frame.empty:
            continue
        for column in frame.columns:
            key = str(column)
            if key not in seen:
                seen.add(key)
                dates.append(column)
    dates.sort(key=lambda value: pd.Timestamp(value), reverse=True)
    return dates


def eps(symbol):
    ticker = yf.Ticker(symbol)
    quarterly = pick_eps_row(ticker.quarterly_income_stmt)
    annual = pick_eps_row(ticker.income_stmt)
    if quarterly is None and annual is None:
        return fail("yfinance EPS 数据为空")

    quarter_growth = None
    data_as_of = None
    quarter_sufficient = False
    if quarterly is not None and len(quarterly) > 0:
        quarter_growth = clean_float(quarterly.iloc[0])
        try:
            data_as_of = quarterly.index[0].date().isoformat()
        except Exception:
            data_as_of = None
        if len(quarterly) >= 5:
            value = growth(quarterly.iloc[0], quarterly.iloc[4])
            if value is not None:
                quarter_growth = value
                quarter_sufficient = True

    annual_growth = []
    annual_sufficient = annual is not None and len(annual) >= 4
    if annual is not None:
        for idx in range(min(3, len(annual))):
            if idx + 1 < len(annual):
                value = growth(annual.iloc[idx], annual.iloc[idx + 1])
                if value is None:
                    value = clean_float(annual.iloc[idx])
                    annual_sufficient = False
            else:
                value = clean_float(annual.iloc[idx])
                annual_sufficient = False
            if value is not None:
                annual_growth.append(value)
    if len(annual_growth) < 3:
        annual_sufficient = False

    emit(
        {
            "quarter_growth": quarter_growth,
            "quarter_sufficient": quarter_sufficient,
            "annual_growth": annual_growth,
            "years_available": len(annual_growth),
            "annual_sufficient": annual_sufficient,
            "sufficient": quarter_growth is not None and quarter_sufficient and annual_sufficient,
            "data_as_of": data_as_of,
        }
    )
    return 0


def period_label(value):
    try:
        timestamp = pd.Timestamp(value)
        quarter = int((timestamp.month - 1) / 3) + 1
        return f"{timestamp.year}Q{quarter}", timestamp.date().isoformat()
    except Exception:
        text = str(value)
        return text, text


def eps_quarterly(symbol):
    ticker = yf.Ticker(symbol)
    quarterly = pick_eps_row(ticker.quarterly_income_stmt)
    if quarterly is None or len(quarterly) == 0:
        return fail("yfinance EPS data is empty")

    items = []
    for idx in range(min(12, len(quarterly))):
        eps_value = clean_float(quarterly.iloc[idx])
        if eps_value is None:
            continue

        yoy_growth = eps_value
        sufficient = False
        if idx + 4 < len(quarterly):
            value = growth(quarterly.iloc[idx], quarterly.iloc[idx + 4])
            if value is not None:
                yoy_growth = value
                sufficient = True

        period, report_date = period_label(quarterly.index[idx])
        items.append(
            {
                "period": period,
                "eps": eps_value,
                "eps_yoy_growth": yoy_growth,
                "data_sufficient": sufficient,
                "report_date": report_date,
            }
        )

    if not items:
        return fail("yfinance EPS data is empty")

    emit({"quarters": items, "data_as_of": items[0]["report_date"]})
    return 0


def annual_revenue(symbol):
    ticker = yf.Ticker(symbol)
    revenue = pick_statement_row(ticker.income_stmt, ["Total Revenue", "Operating Revenue"])
    if revenue is None or len(revenue) == 0:
        return fail("yfinance annual revenue data is empty")

    items = []
    for idx in range(min(5, len(revenue))):
        current = clean_float(revenue.iloc[idx])
        if current is None:
            continue

        yoy_growth = 0.0
        sufficient = False
        if idx + 1 < len(revenue):
            value = growth(revenue.iloc[idx], revenue.iloc[idx + 1])
            if value is not None:
                yoy_growth = value
                sufficient = True

        try:
            year = int(pd.Timestamp(revenue.index[idx]).year)
            report_date = pd.Timestamp(revenue.index[idx]).date().isoformat()
        except Exception:
            text = str(revenue.index[idx])
            year = int(text[:4]) if len(text) >= 4 and text[:4].isdigit() else 0
            report_date = text

        items.append(
            {
                "year": year,
                "revenue": current,
                "revenue_yoy_growth": yoy_growth,
                "data_sufficient": sufficient,
                "report_date": report_date,
            }
        )

    if not items:
        return fail("yfinance annual revenue data is empty")

    emit({"years": items, "data_as_of": items[0]["report_date"]})
    return 0


def gross_margin_quarterly(symbol):
    ticker = yf.Ticker(symbol)
    frame = ticker.quarterly_income_stmt
    revenue = pick_statement_row(frame, ["Total Revenue", "Operating Revenue"])
    gross_profit = pick_statement_row(frame, ["Gross Profit"])
    if revenue is None or gross_profit is None or len(revenue) == 0 or len(gross_profit) == 0:
        return fail("yfinance gross margin data is empty")

    items = []
    dates = [date for date in revenue.index if date in gross_profit.index]
    for date in dates[:12]:
        revenue_value = clean_float(revenue.loc[date])
        gross_profit_value = clean_float(gross_profit.loc[date])
        if revenue_value in (None, 0) or gross_profit_value is None:
            continue
        period, report_date = period_label(date)
        items.append(
            {
                "period": period,
                "gross_margin": round(gross_profit_value / revenue_value * 100, 2),
                "report_date": report_date,
            }
        )

    if not items:
        return fail("yfinance gross margin data is empty")

    emit({"quarters": items, "data_as_of": items[0]["report_date"]})
    return 0


def profile(symbol):
    ticker = yf.Ticker(symbol)
    try:
        info = ticker.get_info()
    except AttributeError:
        info = ticker.info
    if not info:
        return fail("yfinance 公司简介为空")

    description = str(info.get("longBusinessSummary") or "").strip()
    if not description:
        return fail("yfinance 公司简介为空")

    emit(
        {
            "symbol": str(info.get("symbol") or symbol).strip().upper(),
            "name": str(info.get("longName") or info.get("shortName") or "").strip(),
            "description": " ".join(description.split()),
            "sector": str(info.get("sector") or "").strip(),
            "industry": str(info.get("industry") or "").strip(),
            "website": str(info.get("website") or "").strip(),
        }
    )
    return 0


def quarterly_financial_quality(ticker):
    income = ticker.quarterly_income_stmt
    cashflow = ticker.quarterly_cashflow
    balance = ticker.quarterly_balance_sheet
    dates = frame_dates(income, cashflow, balance)[:8]

    rows = []
    by_date = {}
    for date in dates:
        revenue = pick_scalar(income, date, ["Total Revenue", "Operating Revenue"])
        gross_profit = pick_scalar(income, date, ["Gross Profit"])
        operating_income = pick_scalar(income, date, ["Operating Income", "Operating Income Loss"])
        net_income = pick_scalar(income, date, ["Net Income", "Net Income Common Stockholders"])
        diluted_eps = pick_scalar(income, date, ["Diluted EPS", "Basic EPS"])
        unusual_items = pick_scalar(income, date, ["Total Unusual Items", "Total Unusual Items Excluding Goodwill"])
        unusual_tax_effect = pick_scalar(income, date, ["Tax Effect Of Unusual Items"])
        normalized_ebitda = pick_scalar(income, date, ["Normalized EBITDA"])
        operating_cash_flow = pick_scalar(
            cashflow,
            date,
            ["Operating Cash Flow", "Total Cash From Operating Activities", "Net Cash Provided By Operating Activities"],
        )
        free_cash_flow = pick_scalar(cashflow, date, ["Free Cash Flow"])
        capex = pick_scalar(cashflow, date, ["Capital Expenditure", "Capital Expenditures"])
        receivables = pick_scalar(balance, date, ["Accounts Receivable", "Net Receivables"])
        inventory = pick_scalar(balance, date, ["Inventory"])
        total_debt = pick_scalar(balance, date, ["Total Debt"])
        shares = pick_scalar(balance, date, ["Ordinary Shares Number", "Share Issued"])

        period, report_date = period_label(date)
        row = {
            "period": period,
            "report_date": report_date,
            "revenue": revenue,
            "gross_profit": gross_profit,
            "operating_income": operating_income,
            "net_income": net_income,
            "diluted_eps": diluted_eps,
            "unusual_items": unusual_items,
            "tax_effect_of_unusual_items": unusual_tax_effect,
            "normalized_ebitda": normalized_ebitda,
            "gross_margin": round(gross_profit / revenue * 100, 2) if revenue not in (None, 0) and gross_profit is not None else None,
            "operating_margin": round(operating_income / revenue * 100, 2) if revenue not in (None, 0) and operating_income is not None else None,
            "net_margin": round(net_income / revenue * 100, 2) if revenue not in (None, 0) and net_income is not None else None,
            "operating_cash_flow": operating_cash_flow,
            "free_cash_flow": free_cash_flow,
            "capital_expenditure": capex,
            "receivables": receivables,
            "inventory": inventory,
            "total_debt": total_debt,
            "shares_outstanding": shares,
        }
        by_date[date] = row
        rows.append(row)

    for idx, date in enumerate(dates[: len(rows)]):
        row = rows[idx]
        if idx + 4 < len(dates):
            previous = by_date.get(dates[idx + 4], {})
            row["revenue_yoy_growth"] = growth(row.get("revenue"), previous.get("revenue"))
            row["eps_yoy_growth"] = growth(row.get("diluted_eps"), previous.get("diluted_eps"))
            row["operating_cash_flow_yoy_growth"] = growth(row.get("operating_cash_flow"), previous.get("operating_cash_flow"))
            row["receivables_yoy_growth"] = growth(row.get("receivables"), previous.get("receivables"))
            row["inventory_yoy_growth"] = growth(row.get("inventory"), previous.get("inventory"))
            row["shares_yoy_change_pct"] = growth(row.get("shares_outstanding"), previous.get("shares_outstanding"))
        else:
            row["revenue_yoy_growth"] = None
            row["eps_yoy_growth"] = None
            row["operating_cash_flow_yoy_growth"] = None
            row["receivables_yoy_growth"] = None
            row["inventory_yoy_growth"] = None
            row["shares_yoy_change_pct"] = None
        if idx + 1 < len(rows):
            row["shares_qoq_change_pct"] = growth(row.get("shares_outstanding"), rows[idx + 1].get("shares_outstanding"))
        else:
            row["shares_qoq_change_pct"] = None

    return rows


def earnings_calendar(ticker, info):
    calendar = {}
    try:
        calendar = ticker.calendar or {}
    except Exception:
        calendar = {}

    earnings_date = None
    raw_earnings = calendar.get("Earnings Date") if isinstance(calendar, dict) else None
    if isinstance(raw_earnings, list) and raw_earnings:
        earnings_date = date_text(raw_earnings[0])
    else:
        earnings_date = date_text(raw_earnings)
    if not earnings_date:
        earnings_date = date_text(info.get("earningsTimestamp"))

    return {
        "next_earnings_date": earnings_date,
        "eps_estimate_low": clean_float(calendar.get("Earnings Low")) if isinstance(calendar, dict) else None,
        "eps_estimate_average": clean_float(calendar.get("Earnings Average")) if isinstance(calendar, dict) else None,
        "eps_estimate_high": clean_float(calendar.get("Earnings High")) if isinstance(calendar, dict) else None,
        "revenue_estimate_low": clean_float(calendar.get("Revenue Low")) if isinstance(calendar, dict) else None,
        "revenue_estimate_average": clean_float(calendar.get("Revenue Average")) if isinstance(calendar, dict) else None,
        "revenue_estimate_high": clean_float(calendar.get("Revenue High")) if isinstance(calendar, dict) else None,
    }


def earnings_surprises(ticker):
    try:
        frame = ticker.earnings_dates
    except Exception:
        return []
    if frame is None or frame.empty:
        return []
    rows = []
    for date, row in frame.head(8).iterrows():
        rows.append(
            {
                "earnings_date": date_text(date),
                "eps_estimate": clean_float(row.get("EPS Estimate")),
                "reported_eps": clean_float(row.get("Reported EPS")),
                "surprise_pct": clean_float(row.get("Surprise(%)")),
            }
        )
    return rows


def second_layer(symbol):
    ticker = yf.Ticker(symbol)
    try:
        info = ticker.get_info()
    except AttributeError:
        info = ticker.info
    if not info:
        return fail("yfinance 二层筛选数据为空")

    payload = {
        "symbol": str(info.get("symbol") or symbol).strip().upper(),
        "name": str(info.get("longName") or info.get("shortName") or "").strip(),
        "source": "yfinance",
        "data_as_of": date_text(info.get("regularMarketTime")),
        "company": {
            "sector": str(info.get("sector") or "").strip() or None,
            "industry": str(info.get("industry") or "").strip() or None,
            "market_cap": info_int(info, "marketCap"),
            "current_price": info_number(info, "currentPrice") or info_number(info, "regularMarketPrice"),
            "beta": info_number(info, "beta"),
        },
        "quote": {
            "bid": info_number(info, "bid"),
            "ask": info_number(info, "ask"),
            "bid_size": info_int(info, "bidSize"),
            "ask_size": info_int(info, "askSize"),
            "bid_ask_spread": round(info_number(info, "ask") - info_number(info, "bid"), 4)
            if info_number(info, "ask") is not None and info_number(info, "bid") is not None
            else None,
            "bid_ask_spread_pct": round((info_number(info, "ask") - info_number(info, "bid")) / info_number(info, "currentPrice") * 100, 4)
            if info_number(info, "ask") is not None and info_number(info, "bid") is not None and info_number(info, "currentPrice") not in (None, 0)
            else None,
            "regular_market_volume": info_int(info, "regularMarketVolume"),
            "average_volume": info_int(info, "averageVolume"),
            "average_volume_10d": info_int(info, "averageVolume10days") or info_int(info, "averageDailyVolume10Day"),
        },
        "supply": {
            "shares_outstanding": info_int(info, "sharesOutstanding"),
            "float_shares": info_int(info, "floatShares"),
            "held_percent_institutions": info_percent(info, "heldPercentInstitutions"),
            "held_percent_insiders": info_percent(info, "heldPercentInsiders"),
            "short_percent_of_float": info_percent(info, "shortPercentOfFloat"),
            "short_ratio": info_number(info, "shortRatio"),
            "shares_short": info_int(info, "sharesShort"),
            "short_date": date_text(info.get("dateShortInterest")),
        },
        "valuation": {
            "trailing_pe": info_number(info, "trailingPE"),
            "forward_pe": info_number(info, "forwardPE"),
            "price_to_sales_ttm": info_number(info, "priceToSalesTrailing12Months"),
            "enterprise_value": info_int(info, "enterpriseValue"),
            "ev_to_sales": info_number(info, "enterpriseToRevenue"),
            "ev_to_ebitda": info_number(info, "enterpriseToEbitda"),
            "peg_ratio": info_number(info, "pegRatio"),
            "price_to_book": info_number(info, "priceToBook"),
        },
        "analyst_expectations": {
            "recommendation_key": str(info.get("recommendationKey") or "").strip() or None,
            "recommendation_mean": info_number(info, "recommendationMean"),
            "target_mean_price": info_number(info, "targetMeanPrice"),
            "target_high_price": info_number(info, "targetHighPrice"),
            "target_low_price": info_number(info, "targetLowPrice"),
            "number_of_analyst_opinions": info_int(info, "numberOfAnalystOpinions"),
        },
        "financial_quality": {
            "quarterly": quarterly_financial_quality(ticker),
        },
        "events": earnings_calendar(ticker, info),
        "earnings_history": earnings_surprises(ticker),
    }
    emit(payload)
    return 0


def normalize_symbol(value):
    return str(value).strip().replace(".", "-").upper()


def constituents(market):
    market = market.upper()
    if market == "SP500":
        url = "https://en.wikipedia.org/wiki/List_of_S%26P_500_companies"
        tables = read_html_tables(url)
        for table in tables:
            if "Symbol" in table.columns and "Security" in table.columns:
                items = [
                    {"symbol": normalize_symbol(row["Symbol"]), "name": str(row["Security"]).strip()}
                    for _, row in table.iterrows()
                    if str(row.get("Symbol", "")).strip()
                ]
                emit({"items": items, "source": "wikipedia"})
                return 0
        return fail("无法从 Wikipedia 获取标普500成分股")

    if market == "NASDAQ100":
        url = "https://en.wikipedia.org/wiki/Nasdaq-100"
        tables = read_html_tables(url)
        for table in tables:
            columns = {str(column).strip().lower(): column for column in table.columns}
            symbol_column = columns.get("ticker") or columns.get("symbol")
            name_column = columns.get("company") or columns.get("security") or columns.get("company name")
            if symbol_column is None or name_column is None:
                continue
            items = [
                {"symbol": normalize_symbol(row[symbol_column]), "name": str(row[name_column]).strip()}
                for _, row in table.iterrows()
                if str(row.get(symbol_column, "")).strip()
            ]
            if len(items) >= 90:
                emit({"items": items[:100], "source": "wikipedia"})
                return 0
        return fail("无法从 Wikipedia 获取纳指100成分股")

    return fail(f"{market} 暂未配置真实成分股来源")


def main():
    if len(sys.argv) < 2:
        return fail("缺少 yfinance helper 命令")
    command = sys.argv[1]
    if command == "health":
        emit({"ok": True})
        return 0
    if len(sys.argv) < 3:
        return fail("缺少股票代码")
    symbol = sys.argv[2].upper()
    if command == "price":
        return price(symbol)
    if command == "eps":
        return eps(symbol)
    if command == "eps_quarterly":
        return eps_quarterly(symbol)
    if command == "annual_revenue":
        return annual_revenue(symbol)
    if command == "gross_margin_quarterly":
        return gross_margin_quarterly(symbol)
    if command == "profile":
        return profile(symbol)
    if command == "second_layer":
        return second_layer(symbol)
    if command == "candles":
        period = sys.argv[3].lower() if len(sys.argv) >= 4 else "daily"
        return candles(symbol, period)
    if command == "constituents":
        return constituents(symbol)
    return fail(f"未知命令：{command}")


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        raise SystemExit(fail(str(exc)))
