"""Structured weekly events from official calendars and AI research sources.

Yahoo Finance supplies company earnings and ex-dividend dates. BLS, the
Federal Reserve, and BEA supply no-key US macro calendars. FRED is an optional
supplement when ``FRED_API_KEY`` is configured. No LLM is involved: the
calendar only emits dated upstream records.
"""

from __future__ import annotations

import os
import re
from concurrent.futures import ThreadPoolExecutor
from datetime import date, datetime, time, timedelta
from typing import Any, Callable
from urllib.parse import urljoin
from zoneinfo import ZoneInfo

import requests
import yfinance as yf
from parsel import Selector


YAHOO_SOURCE = {"name": "Yahoo Finance", "url": "https://finance.yahoo.com"}
YAHOO_ECONOMIC_SOURCE = {"name": "Yahoo Economic Calendar", "url": "https://finance.yahoo.com/calendar/economic/"}
BLS_SOURCE = {"name": "BLS", "url": "https://www.bls.gov/schedule/news_release/bls.ics"}
FED_SOURCE = {"name": "Federal Reserve", "url": "https://www.federalreserve.gov/newsevents/calendar.htm"}
BEA_SOURCE = {"name": "BEA", "url": "https://www.bea.gov/news/schedule"}
FRED_SOURCE = {"name": "FRED", "url": "https://fred.stlouisfed.org/releases/calendar"}
_HIGH_IMPACT_RELEASES = (
    "consumer price index",
    "employment situation",
    "federal open market committee",
    "gross domestic product",
    "personal income and outlays",
    "producer price index",
    "retail and food services",
)
_BLS_HIGH_IMPACT_RELEASES = (
    "consumer price index",
    "producer price index",
    "employment situation",
    "job openings and labor turnover",
    "employment cost index",
)
_BEA_HIGH_IMPACT_RELEASES = (
    "gdp",
    "gross domestic product",
    "personal income and outlays",
    "personal consumption expenditures",
)
_YAHOO_RELEVANT_MACRO = (
    "cpi",
    "ppi",
    "nonfarm",
    "non-farm",
    "unemployment rate",
    "employment situation",
    "employment trends",
    "jobless claims",
    "retail sales",
    "gdp",
    "personal income",
    "pce",
    "fomc",
    "fed interest rate",
    "consumer confidence",
    "u mich sentiment",
    "existing home sales",
    "new home sales",
    "durable goods",
    "industrial production",
)
_HTTP_HEADERS = {
    "Accept": "text/html,application/xhtml+xml,text/calendar;q=0.9,*/*;q=0.8",
    "User-Agent": "AlphaLens/1.0 (weekly research calendar)",
}


def _date_text(value: Any) -> str | None:
    if value is None:
        return None
    if isinstance(value, (list, tuple)):
        value = value[0] if value else None
    if value is None:
        return None
    if isinstance(value, datetime):
        return value.date().isoformat()
    if isinstance(value, date):
        return value.isoformat()
    if isinstance(value, (int, float)):
        return datetime.fromtimestamp(value).date().isoformat()
    text = str(value).strip()
    try:
        return datetime.fromisoformat(text.replace("Z", "+00:00")).date().isoformat()
    except ValueError:
        return text[:10] if len(text) >= 10 else None


def _number(value: Any) -> float | None:
    try:
        if value is None:
            return None
        return round(float(value), 4)
    except (TypeError, ValueError):
        return None


def _yahoo_symbol(symbol: str) -> str:
    normalized = symbol.strip().upper()
    if normalized.endswith(".SH"):
        return normalized[:-3] + ".SS"
    if normalized.endswith(".US"):
        return normalized[:-3]
    return normalized


def _clock_text(value: str) -> str:
    normalized = re.sub(r"\s+", " ", value.strip()).upper().replace(".", "")
    for pattern in ("%I:%M %p", "%I %p", "%H:%M"):
        try:
            return datetime.strptime(normalized, pattern).strftime("%H:%M")
        except ValueError:
            continue
    return ""


def _economic_event(*, event_date: date, title: str, source: str, time: str = "", importance: str = "high", subtitle: str = "美国 · 宏观发布") -> dict[str, Any]:
    return {
        "date": event_date.isoformat(),
        "time": time,
        "kind": "economic",
        "title": title.strip(),
        "subtitle": subtitle,
        "symbol": "",
        "importance": importance,
        "source": source,
    }


def _unfold_ics(payload: str) -> list[str]:
    unfolded: list[str] = []
    for raw_line in payload.replace("\r\n", "\n").split("\n"):
        if raw_line.startswith((" ", "\t")) and unfolded:
            unfolded[-1] += raw_line[1:]
        else:
            unfolded.append(raw_line)
    return unfolded


def parse_bls_ics(payload: str, week_start: date, week_end: date) -> list[dict[str, Any]]:
    events: list[dict[str, Any]] = []
    current: dict[str, str] | None = None
    for line in _unfold_ics(payload):
        if line == "BEGIN:VEVENT":
            current = {}
            continue
        if line == "END:VEVENT":
            if current:
                summary = current.get("SUMMARY", "").replace("\\,", ",").strip()
                start = current.get("DTSTART", "")
                if summary and start and any(keyword in summary.lower() for keyword in _BLS_HIGH_IMPACT_RELEASES):
                    try:
                        if start.endswith("Z"):
                            instant = datetime.strptime(start, "%Y%m%dT%H%M%SZ").replace(tzinfo=ZoneInfo("UTC")).astimezone(ZoneInfo("America/New_York"))
                        else:
                            instant = datetime.strptime(start[:15], "%Y%m%dT%H%M%S")
                        if week_start <= instant.date() <= week_end:
                            events.append(_economic_event(event_date=instant.date(), title=summary, source="BLS", time=instant.strftime("%H:%M"), subtitle="美国劳工统计局 · 宏观发布"))
                    except ValueError:
                        pass
            current = None
            continue
        if current is not None and ":" in line:
            key, value = line.split(":", 1)
            current[key.split(";", 1)[0]] = value
    return events


def fetch_bls_releases(week_start: date, week_end: date) -> list[dict[str, Any]]:
    response = requests.get(BLS_SOURCE["url"], headers=_HTTP_HEADERS, timeout=8)
    response.raise_for_status()
    if "BEGIN:VCALENDAR" not in response.text:
        raise RuntimeError("BLS calendar did not return iCalendar data")
    return parse_bls_ics(response.text, week_start, week_end)


def _yahoo_macro_title(title: str) -> str:
    lowered = title.lower()
    if re.search(r"\bcpi\b", lowered):
        return "美国消费者价格指数（CPI）"
    if re.search(r"\bppi\b", lowered):
        return "美国生产者价格指数（PPI）"
    if any(keyword in lowered for keyword in ("nonfarm", "non-farm", "unemployment rate", "employment situation")):
        return "美国就业报告"
    if "retail sales" in lowered:
        return "美国零售销售"
    if "u mich sentiment" in lowered:
        return "美国密歇根大学消费者信心指数"
    if "existing home sales" in lowered:
        return "美国成屋销售"
    if "new home sales" in lowered:
        return "美国新屋销售"
    if "employment trends" in lowered:
        return "美国就业趋势指数"
    return title.strip()


def parse_yahoo_economic_calendar(payload: dict[str, Any], week_start: date, week_end: date) -> list[dict[str, Any]]:
    days = payload.get("finance", {}).get("result", {}).get("economicEvents", [])
    events: list[dict[str, Any]] = []
    seen: set[tuple[str, str, str]] = set()
    for day in days if isinstance(days, list) else []:
        for row in day.get("records", []) if isinstance(day, dict) else []:
            if str(row.get("countryCode") or "").upper() != "US":
                continue
            raw_title = str(row.get("event") or "").strip()
            if "cleveland fed cpi" in raw_title.lower():
                continue
            if not any(keyword in raw_title.lower() for keyword in _YAHOO_RELEVANT_MACRO):
                continue
            title = _yahoo_macro_title(raw_title)
            try:
                instant = datetime.fromtimestamp(float(row.get("eventTime")) / 1000, tz=ZoneInfo("UTC")).astimezone(ZoneInfo("America/New_York"))
            except (TypeError, ValueError, OSError):
                continue
            if not title or not (week_start <= instant.date() <= week_end):
                continue
            event_time = instant.strftime("%H:%M")
            identity = (instant.date().isoformat(), event_time, title)
            if identity in seen:
                continue
            seen.add(identity)
            period = str(row.get("period") or "").strip()
            subtitle = "美国 · Yahoo 高影响宏观事件" + (f" · {period}" if period else "")
            events.append(_economic_event(event_date=instant.date(), title=title, source="Yahoo Economic Calendar", time=event_time, subtitle=subtitle))
    return events


def fetch_yahoo_economic_events(week_start: date, week_end: date) -> list[dict[str, Any]]:
    eastern = ZoneInfo("America/New_York")
    start = datetime.combine(week_start, time.min, tzinfo=eastern)
    end = datetime.combine(week_end + timedelta(days=1), time.min, tzinfo=eastern)
    response = requests.get(
        "https://query1.finance.yahoo.com/ws/screeners/v1/finance/calendar-events",
        params={
            "countPerDay": 50,
            "economicEventsHighImportanceOnly": "false",
            "economicEventsRegionFilter": "US",
            "startDate": int(start.timestamp() * 1000),
            "endDate": int(end.timestamp() * 1000),
            "modules": "economicEvents",
            "lang": "en-US",
            "region": "US",
        },
        headers=_HTTP_HEADERS,
        timeout=8,
    )
    response.raise_for_status()
    payload = response.json()
    if payload.get("finance", {}).get("error"):
        raise RuntimeError("Yahoo economic calendar returned an error")
    return parse_yahoo_economic_calendar(payload, week_start, week_end)


def _selector_text(node: Selector, xpath: str) -> str:
    return " ".join(part.strip() for part in node.xpath(xpath).getall() if part.strip())


def parse_fed_calendar_html(payload: str, year: int, month: int, week_start: date, week_end: date) -> list[dict[str, Any]]:
    selector = Selector(text=payload)
    events: list[dict[str, Any]] = []
    for panel in selector.xpath('//div[@id="article"]//div[contains(concat(" ", normalize-space(@class), " "), " panel ")]'):
        category = _selector_text(panel, 'ancestor::div[contains(concat(" ", normalize-space(@class), " "), " row ")][1]/preceding-sibling::div[contains(@class, "cal-nojs__rowTitle")][1]//h4//text()')
        if category not in {"Speeches", "FOMC Meetings", "Beige Book"}:
            continue
        columns = panel.xpath('./div[contains(@class, "panel-body")]/div[contains(concat(" ", normalize-space(@class), " "), " row ")]/div')
        if len(columns) < 3:
            continue
        time_text = _selector_text(columns[0], ".//text()")
        title_parts = [" ".join(item.xpath(".//text()").getall()).strip() for item in columns[1].xpath("./p")]
        title_parts = [re.sub(r"\s+", " ", item) for item in title_parts if item]
        day_match = re.search(r"\b(\d{1,2})\b", _selector_text(columns[-1], ".//text()"))
        if not title_parts or not day_match:
            continue
        try:
            event_date = date(year, month, int(day_match.group(1)))
        except ValueError:
            continue
        if not (week_start <= event_date <= week_end):
            continue
        title = title_parts[0]
        if category == "Speeches" and len(title_parts) > 1:
            title += " · " + title_parts[1]
        events.append(_economic_event(
            event_date=event_date,
            title=title,
            source="Federal Reserve",
            time=_clock_text(time_text),
            importance="medium" if category == "Speeches" else "high",
            subtitle=f"美联储 · {category}",
        ))
    return events


def fetch_fed_events(week_start: date, week_end: date) -> list[dict[str, Any]]:
    index_response = requests.get(FED_SOURCE["url"], headers=_HTTP_HEADERS, timeout=8)
    index_response.raise_for_status()
    index = Selector(text=index_response.text)
    month_links: dict[tuple[int, int], str] = {}
    for anchor in index.xpath('//a[@href]'):
        label = _selector_text(anchor, ".//text()")
        try:
            parsed = datetime.strptime(label, "%B %Y")
        except ValueError:
            continue
        month_links[(parsed.year, parsed.month)] = urljoin(FED_SOURCE["url"], anchor.attrib["href"])

    months = {(week_start.year, week_start.month), (week_end.year, week_end.month)}
    events: list[dict[str, Any]] = []
    for year, month in sorted(months):
        url = month_links.get((year, month))
        if not url:
            raise RuntimeError(f"Federal Reserve calendar month is unavailable: {year}-{month:02d}")
        response = requests.get(url, headers=_HTTP_HEADERS, timeout=8)
        response.raise_for_status()
        events.extend(parse_fed_calendar_html(response.text, year, month, week_start, week_end))
    return events


def parse_bea_schedule_html(payload: str, week_start: date, week_end: date) -> list[dict[str, Any]]:
    selector = Selector(text=payload)
    year_text = _selector_text(selector, '//table[@id="release-schedule-table"]//thead//th[1]//text()')
    year_match = re.search(r"\b(20\d{2})\b", year_text)
    if not year_match:
        raise ValueError("BEA schedule year is missing")
    year = int(year_match.group(1))
    events: list[dict[str, Any]] = []
    for row in selector.xpath('//table[@id="release-schedule-table"]//tbody/tr'):
        release_date = _selector_text(row, './/*[contains(@class, "release-date")]//text()')
        title = _selector_text(row, './/*[contains(@class, "release-title")]//text()')
        if not release_date or not title or not any(keyword in title.lower() for keyword in _BEA_HIGH_IMPACT_RELEASES):
            continue
        try:
            event_date = datetime.strptime(f"{release_date} {year}", "%B %d %Y").date()
        except ValueError:
            continue
        if week_start <= event_date <= week_end:
            time_text = _selector_text(row, './/*[contains(@class, "scheduled-date")]//small//text()')
            events.append(_economic_event(event_date=event_date, title=title, source="BEA", time=_clock_text(time_text), subtitle="美国经济分析局 · 宏观发布"))
    return events


def fetch_bea_releases(week_start: date, week_end: date) -> list[dict[str, Any]]:
    response = requests.get(BEA_SOURCE["url"], headers=_HTTP_HEADERS, timeout=8)
    response.raise_for_status()
    return parse_bea_schedule_html(response.text, week_start, week_end)


def fetch_yahoo_symbol(symbol: str, week_start: date, week_end: date) -> list[dict[str, Any]]:
    calendar = yf.Ticker(_yahoo_symbol(symbol)).calendar or {}
    if not isinstance(calendar, dict):
        return []

    events: list[dict[str, Any]] = []
    earnings_date = _date_text(calendar.get("Earnings Date"))
    if earnings_date and week_start.isoformat() <= earnings_date <= week_end.isoformat():
        estimate = _number(calendar.get("Earnings Average"))
        subtitle = "财报日期" if estimate is None else f"预估 EPS {estimate:g}"
        events.append({
            "date": earnings_date,
            "kind": "earnings",
            "title": f"{symbol} 财报",
            "subtitle": subtitle,
            "symbol": symbol,
            "importance": "high",
            "source": "Yahoo Finance",
        })

    dividend_date = _date_text(calendar.get("Ex-Dividend Date"))
    if dividend_date and week_start.isoformat() <= dividend_date <= week_end.isoformat():
        events.append({
            "date": dividend_date,
            "kind": "dividend",
            "title": f"{symbol} 除息",
            "subtitle": "除息日",
            "symbol": symbol,
            "importance": "medium",
            "source": "Yahoo Finance",
        })
    return events


def fetch_fred_releases(week_start: date, week_end: date) -> list[dict[str, Any]]:
    api_key = os.getenv("FRED_API_KEY", "").strip()
    if not api_key:
        raise RuntimeError("FRED_API_KEY is not configured")
    response = requests.get(
        "https://api.stlouisfed.org/fred/releases/dates",
        params={
            "api_key": api_key,
            "file_type": "json",
            "realtime_start": week_start.isoformat(),
            "realtime_end": week_end.isoformat(),
            "include_release_dates_with_no_data": "true",
            "order_by": "release_date",
            "sort_order": "asc",
            "limit": 1000,
        },
        timeout=8,
    )
    response.raise_for_status()
    events = []
    for row in response.json().get("release_dates", []):
        release_date = str(row.get("date") or "")[:10]
        name = str(row.get("release_name") or "").strip()
        if not (week_start.isoformat() <= release_date <= week_end.isoformat()):
            continue
        if not any(keyword in name.lower() for keyword in _HIGH_IMPACT_RELEASES):
            continue
        events.append({
            "date": release_date,
            "kind": "economic",
            "title": name,
            "subtitle": "美国 · 高影响宏观发布",
            "symbol": "",
            "importance": "high",
            "source": "FRED",
        })
    return events


def build_week_calendar(
    *,
    market: str,
    week_start: date,
    week_end: date,
    symbols: list[str],
    yahoo_fetcher: Callable[[str, date, date], list[dict[str, Any]]] = fetch_yahoo_symbol,
    macro_fetchers: list[tuple[dict[str, str], Callable[[date, date], list[dict[str, Any]]]]] | None = None,
    fred_fetcher: Callable[[date, date], list[dict[str, Any]]] | None = None,
) -> dict[str, Any]:
    events: list[dict[str, Any]] = []
    sources: list[dict[str, str]] = []
    errors: list[str] = []
    normalized_symbols = sorted({item.strip().upper() for item in symbols if item.strip()})
    official_fetchers: list[tuple[dict[str, str], Callable[[date, date], list[dict[str, Any]]]]] = []
    optional_fred: Callable[[date, date], list[dict[str, Any]]] | None = None
    if market.strip().upper() == "US":
        official_fetchers = macro_fetchers if macro_fetchers is not None else [
            (YAHOO_ECONOMIC_SOURCE, fetch_yahoo_economic_events),
            (BLS_SOURCE, fetch_bls_releases),
            (FED_SOURCE, fetch_fed_events),
            (BEA_SOURCE, fetch_bea_releases),
        ]
        optional_fred = fred_fetcher
        if optional_fred is None and os.getenv("FRED_API_KEY", "").strip():
            optional_fred = fetch_fred_releases

    worker_count = len(normalized_symbols) + len(official_fetchers) + (1 if optional_fred else 0)
    if worker_count:
        with ThreadPoolExecutor(max_workers=min(worker_count, 8)) as executor:
            yahoo_futures = [
                (symbol, executor.submit(yahoo_fetcher, symbol, week_start, week_end))
                for symbol in normalized_symbols
            ]
            macro_futures = [
                (source, executor.submit(fetcher, week_start, week_end))
                for source, fetcher in official_fetchers
            ]
            fred_future = executor.submit(optional_fred, week_start, week_end) if optional_fred else None

            yahoo_succeeded = False
            for symbol, future in yahoo_futures:
                try:
                    events.extend(future.result())
                    yahoo_succeeded = True
                except Exception as exc:
                    errors.append(f"Yahoo Finance ({symbol}): {type(exc).__name__}")
            if yahoo_succeeded:
                sources.append(YAHOO_SOURCE)

            for source, future in macro_futures:
                try:
                    events.extend(future.result())
                    sources.append(source)
                except Exception as exc:
                    errors.append(f"{source['name']}: {type(exc).__name__}")

            if fred_future is not None:
                try:
                    events.extend(fred_future.result())
                    sources.append(FRED_SOURCE)
                except Exception as exc:
                    errors.append(f"FRED: {type(exc).__name__}")

    events.sort(key=lambda event: (event.get("date", ""), event.get("kind", ""), event.get("symbol", "")))
    return {"events": events, "sources": sources, "errors": errors}
