import unittest
from datetime import date
from threading import Barrier

from calendar_sources import (
    BEA_SOURCE,
    BLS_SOURCE,
    FED_SOURCE,
    YAHOO_ECONOMIC_SOURCE,
    build_week_calendar,
    parse_bea_schedule_html,
    parse_bls_ics,
    parse_fed_calendar_html,
    parse_yahoo_economic_calendar,
)


class WeekCalendarSourcesTest(unittest.TestCase):
    def test_parses_high_impact_bls_ics_events_in_requested_week(self):
        payload = """BEGIN:VCALENDAR
BEGIN:VEVENT
DTSTART:20260812T123000Z
SUMMARY:Consumer Price Index
END:VEVENT
BEGIN:VEVENT
DTSTART:20260813T123000Z
SUMMARY:Producer Price Index
END:VEVENT
BEGIN:VEVENT
DTSTART:20260818T123000Z
SUMMARY:Employment Situation
END:VEVENT
END:VCALENDAR
"""

        events = parse_bls_ics(payload, date(2026, 8, 10), date(2026, 8, 14))

        self.assertEqual([event["title"] for event in events], ["Consumer Price Index", "Producer Price Index"])
        self.assertEqual(events[0]["date"], "2026-08-12")
        self.assertEqual(events[0]["time"], "08:30")
        self.assertEqual(events[0]["source"], "BLS")

    def test_parses_fed_fomc_and_speech_rows(self):
        payload = """
        <div id="article" class="cal-nojs">
          <div class="row cal-nojs__rowTitle"><h4>Speeches</h4></div>
          <div class="row"><div class="panel"><div class="panel-body"><div class="row">
            <div class="col-xs-2"><p>4:05 p.m.</p></div>
            <div class="col-xs-7"><p>Speech - Governor Lisa D. Cook</p><p class="calendar__title"><em>Economic Outlook</em></p></div>
            <div class="col-xs-3"><p>11</p></div>
          </div></div></div></div>
          <div class="row cal-nojs__rowTitle"><h4>FOMC Meetings</h4></div>
          <div class="row"><div class="panel"><div class="panel-body"><div class="row">
            <div class="col-xs-2"><p>2:00 p.m.</p></div>
            <div class="col-xs-7"><p>FOMC Minutes</p><p>Meeting of July 28-29</p></div>
            <div class="col-xs-3"><p>12</p></div>
          </div></div></div></div>
        </div>
        """

        events = parse_fed_calendar_html(payload, 2026, 8, date(2026, 8, 10), date(2026, 8, 14))

        self.assertEqual([event["title"] for event in events], ["Speech - Governor Lisa D. Cook · Economic Outlook", "FOMC Minutes"])
        self.assertEqual(events[0]["importance"], "medium")
        self.assertEqual(events[1]["importance"], "high")
        self.assertEqual(events[1]["source"], "Federal Reserve")

    def test_parses_bea_gdp_and_pce_schedule_rows(self):
        payload = """
        <table id="release-schedule-table"><thead><tr><th>Year 2026</th></tr></thead><tbody>
          <tr><td class="scheduled-date"><div class="release-date">August 12</div><small>8:30 AM</small></td><td></td><td class="release-title">GDP (Second Estimate), 2nd Quarter 2026</td></tr>
          <tr><td class="scheduled-date"><div class="release-date">August 14</div><small>8:30 AM</small></td><td></td><td class="release-title">Personal Income and Outlays, July 2026</td></tr>
          <tr><td class="scheduled-date"><div class="release-date">August 18</div><small>8:30 AM</small></td><td></td><td class="release-title">International Trade</td></tr>
        </tbody></table>
        """

        events = parse_bea_schedule_html(payload, date(2026, 8, 10), date(2026, 8, 14))

        self.assertEqual([event["title"] for event in events], ["GDP (Second Estimate), 2nd Quarter 2026", "Personal Income and Outlays, July 2026"])
        self.assertEqual(events[1]["time"], "08:30")
        self.assertEqual(events[1]["source"], "BEA")

    def test_parses_and_deduplicates_yahoo_high_impact_us_events(self):
        payload = {"finance": {"result": {"economicEvents": [{
            "timestampString": "2026-08-12",
            "records": [
                {"event": "Core CPI MM, SA", "countryCode": "US", "eventTime": 1786537800000, "period": "Jul"},
                {"event": "CPI YY, NSA", "countryCode": "US", "eventTime": 1786537800000, "period": "Jul"},
                {"event": "CPI YY, NSA", "countryCode": "CA", "eventTime": 1786537800000, "period": "Jul"},
                {"event": "WSD-Corn Output 26/27", "countryCode": "US", "eventTime": 1786537800000, "period": "Jul"},
                {"event": "Cleveland Fed CPI*", "countryCode": "US", "eventTime": 1786546800000, "period": "Jul"},
            ],
        }, {
            "timestampString": "2026-08-14",
            "records": [
                {"event": "Retail Sales MM", "countryCode": "US", "eventTime": 1786710600000, "period": "Jul"},
                {"event": "Retail Sales Ex-Autos MM", "countryCode": "US", "eventTime": 1786710600000, "period": "Jul"},
                {"event": "U Mich Sentiment Prelim", "countryCode": "US", "eventTime": 1786716000000, "period": "Aug"},
            ],
        }]}, "error": None}}

        events = parse_yahoo_economic_calendar(payload, date(2026, 8, 10), date(2026, 8, 14))

        self.assertEqual([event["title"] for event in events], ["美国消费者价格指数（CPI）", "美国零售销售", "美国密歇根大学消费者信心指数"])
        self.assertEqual(events[0]["time"], "08:30")
        self.assertEqual(events[0]["source"], "Yahoo Economic Calendar")

    def test_completed_yahoo_event_exposes_result_and_initial_stock_impact(self):
        payload = {"finance": {"result": {"economicEvents": [{
            "records": [{
                "event": "CPI YY, NSA", "countryCode": "US", "eventTime": 1786537800000,
                "period": "Jul", "actual": "3.4", "prior": "3.5",
            }],
        }]}, "error": None}}

        events = parse_yahoo_economic_calendar(payload, date(2026, 8, 1), date(2026, 8, 31))

        self.assertEqual(events[0]["result"], "实际 3.4% · 前值 3.5%")
        self.assertEqual(events[0]["market_impact"], "bullish")
        self.assertIn("通胀", events[0]["impact_reason"])

    def test_fomc_event_exposes_official_live_page(self):
        payload = """
        <div id="article" class="cal-nojs">
          <div class="row cal-nojs__rowTitle"><h4>FOMC Meetings</h4></div>
          <div class="row"><div class="panel"><div class="panel-body"><div class="row">
            <div class="col-xs-2"><p>2:00 p.m.</p></div>
            <div class="col-xs-7"><p>FOMC Press Conference</p></div>
            <div class="col-xs-3"><p>12</p></div>
          </div></div></div></div>
        </div>
        """

        events = parse_fed_calendar_html(payload, 2026, 8, date(2026, 8, 1), date(2026, 8, 31))

        self.assertEqual(events[0]["live_url"], "https://www.federalreserve.gov/live-broadcast.htm")

    def test_uses_no_key_official_sources_and_keeps_fred_optional(self):
        def source_event(name):
            return [{
                "date": "2026-08-12",
                "kind": "economic",
                "title": f"{name} event",
                "subtitle": "美国 · 宏观发布",
                "symbol": "",
                "importance": "high",
                "source": name,
            }]

        result = build_week_calendar(
            market="US",
            week_start=date(2026, 8, 10),
            week_end=date(2026, 8, 14),
            symbols=[],
            macro_fetchers=[
                (BLS_SOURCE, lambda _start, _end: source_event("BLS")),
                (FED_SOURCE, lambda _start, _end: source_event("Federal Reserve")),
                (BEA_SOURCE, lambda _start, _end: source_event("BEA")),
            ],
        )

        self.assertEqual([source["name"] for source in result["sources"]], ["BLS", "Federal Reserve", "BEA"])
        self.assertEqual(len(result["events"]), 3)
        self.assertEqual(result["errors"], [])

    def test_fetches_independent_official_sources_concurrently(self):
        started = Barrier(3, timeout=0.5)

        def fetch(_start, _end):
            started.wait()
            return []

        result = build_week_calendar(
            market="US",
            week_start=date(2026, 8, 10),
            week_end=date(2026, 8, 14),
            symbols=[],
            macro_fetchers=[(BLS_SOURCE, fetch), (FED_SOURCE, fetch), (BEA_SOURCE, fetch)],
        )

        self.assertEqual(result["errors"], [])
        self.assertEqual(result["sources"], [BLS_SOURCE, FED_SOURCE, BEA_SOURCE])

    def test_reuses_yahoo_and_official_macro_sources(self):
        def yahoo(symbol, _start, _end):
            self.assertEqual(symbol, "NVDA")
            return [
                {
                    "date": "2026-08-05",
                    "kind": "earnings",
                    "title": "NVDA 财报",
                    "subtitle": "预估 EPS 1.23",
                    "symbol": symbol,
                    "importance": "high",
                    "source": "Yahoo Finance",
                },
                {
                    "date": "2026-08-06",
                    "kind": "dividend",
                    "title": "NVDA 除息",
                    "subtitle": "除息日",
                    "symbol": symbol,
                    "importance": "medium",
                    "source": "Yahoo Finance",
                },
            ]

        def bls(_start, _end):
            return [
                {
                    "date": "2026-08-07",
                    "kind": "economic",
                    "title": "Employment Situation",
                    "subtitle": "美国 · 高影响宏观发布",
                    "symbol": "",
                    "importance": "high",
                    "source": "BLS",
                }
            ]

        result = build_week_calendar(
            market="US",
            week_start=date(2026, 8, 3),
            week_end=date(2026, 8, 7),
            symbols=["NVDA"],
            yahoo_fetcher=yahoo,
            macro_fetchers=[(BLS_SOURCE, bls)],
        )

        self.assertEqual([event["kind"] for event in result["events"]], ["earnings", "dividend", "economic"])
        self.assertEqual([source["name"] for source in result["sources"]], ["Yahoo Finance", "BLS"])
        self.assertEqual(result["errors"], [])

    def test_source_failure_keeps_other_research_events(self):
        def yahoo(_symbol, _start, _end):
            raise RuntimeError("Yahoo unavailable")

        result = build_week_calendar(
            market="US",
            week_start=date(2026, 8, 3),
            week_end=date(2026, 8, 7),
            symbols=["NVDA"],
            yahoo_fetcher=yahoo,
            macro_fetchers=[(BLS_SOURCE, lambda _start, _end: [{
                "date": "2026-08-07",
                "kind": "economic",
                "title": "Employment Situation",
                "subtitle": "美国 · 高影响宏观发布",
                "symbol": "",
                "importance": "high",
                "source": "BLS",
            }])],
        )

        self.assertEqual(len(result["events"]), 1)
        self.assertEqual(result["sources"], [BLS_SOURCE])
        self.assertEqual(len(result["errors"]), 1)


if __name__ == "__main__":
    unittest.main()
