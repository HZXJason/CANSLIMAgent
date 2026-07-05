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
