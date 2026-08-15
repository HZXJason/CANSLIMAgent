// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as todayApi from '../../lib/todayApi';
import type { WeekCalendar } from '../../types/today';
import TodayPage from '.';
import { MarketClock } from './MarketClock';
import { WeeklyCalendar } from './WeeklyCalendar';

vi.mock('../../lib/todayApi');

const weekCalendar = {
  week_start: '2026-08-03',
  week_end: '2026-08-07',
  status: 'live',
  sources: [
    { name: 'Yahoo Finance', url: 'https://finance.yahoo.com' },
    { name: 'Yahoo Economic Calendar', url: 'https://finance.yahoo.com/calendar/economic/' },
    { name: 'BLS', url: 'https://www.bls.gov/schedule/news_release/bls.ics' },
    { name: 'Federal Reserve', url: 'https://www.federalreserve.gov/newsevents/calendar.htm' },
    { name: 'BEA', url: 'https://www.bea.gov/news/schedule' }
  ],
  fetched_at: '2026-08-05T08:00:00Z',
  events: [
    { id: 'macro-1', date: '2026-08-03', time: '10:00', kind: 'economic', title: '美国制造业 PMI', subtitle: '美国 · 高影响', symbol: null, importance: 'high', source: 'BLS', href: null },
    { id: 'earnings-1', date: '2026-08-05', time: null, kind: 'earnings', title: 'NVDA 财报', subtitle: '预估 EPS 1.05', symbol: 'NVDA', importance: 'high', source: 'Yahoo Finance', href: '/analysis/NVDA' },
    { id: 'plan-1', date: '2026-08-07', time: null, kind: 'plan_deadline', title: '检查 MU 计划', subtitle: '交易计划到期', symbol: 'MU', importance: 'medium', source: 'AlphaLens', href: '/trade-plans/plan-1' }
  ]
} satisfies WeekCalendar;

describe('TodayPage task hierarchy', () => {
  beforeEach(() => {
    vi.mocked(todayApi.fetchTodayBrief).mockResolvedValue({
      market: 'US',
      timezone: 'America/New_York',
      local_date: '2026-08-05',
      session: 'non_trading',
      calendar_status: 'weekday_only_unverified',
      data_as_of: '2026-08-05T08:00:00Z',
      week_calendar: weekCalendar,
      items: [
        { id: 'plan-1', kind: 'unreviewed_plan', title: '检查 NVDA 计划', subtitle: '等待盘前检查', href: '/trade-plans/plan-1', priority: 100 },
        { id: 'review-1', kind: 'pending_review', title: '复盘 MU', subtitle: '昨日已平仓', href: '/journal/review-1', priority: 80 }
      ]
    });
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it('shows dual market clocks and removes redundant page metadata and task queues', async () => {
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><TodayPage /></QueryClientProvider></MemoryRouter>);

    await screen.findByRole('region', { name: '本周大事' });
    expect(screen.getByText('美国东部时间')).toBeInTheDocument();
    expect(screen.getByText('北京时间')).toBeInTheDocument();
    expect(screen.getByRole('status', { name: '美股时段状态' })).toHaveTextContent('休市');
    expect(screen.getByRole('heading', { name: '今日' }).parentElement).toHaveClass('w-full', 'max-w-none');
    expect(screen.queryByText(/数据截至/)).not.toBeInTheDocument();
    expect(screen.queryByText('America/New_York')).not.toBeInTheDocument();
    expect(screen.queryByText('节假日状态待确认')).not.toBeInTheDocument();
    expect(screen.queryByRole('region', { name: '下一步' })).not.toBeInTheDocument();
    expect(screen.queryByRole('region', { name: '稍后处理' })).not.toBeInTheDocument();
    expect(screen.queryByText('检查 NVDA 计划')).not.toBeInTheDocument();
    expect(screen.queryByText('复盘 MU')).not.toBeInTheDocument();
  });

  it('formats Eastern and Beijing date-time rows from the same instant', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-08-05T14:23:45Z'));

    render(<MarketClock timeZone="America/New_York" session="non_trading" />);

    expect(screen.getByText('美国东部时间')).toBeInTheDocument();
    expect(screen.getByText('8月5日 星期三 10:23:45')).toBeInTheDocument();
    expect(screen.getByText('北京时间')).toBeInTheDocument();
    expect(screen.getByText('8月5日 星期三 22:23:45')).toBeInTheDocument();
    expect(screen.getAllByRole('time')).toHaveLength(2);
    expect(screen.getByRole('status', { name: '美股时段状态' })).toHaveTextContent('休市');
  });

  it('labels an eligible Eastern overnight window as night trading', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-08-06T01:15:00Z'));

    render(<MarketClock timeZone="America/New_York" session="non_trading" />);

    expect(screen.getByRole('status', { name: '美股时段状态' })).toHaveTextContent('夜盘');
  });

  it('keeps the dual-time group frameless', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-08-05T14:23:45Z'));

    const { container } = render(<MarketClock timeZone="America/New_York" session="non_trading" />);
    const clock = container.firstElementChild;
    const timeRows = screen.getByText('美国东部时间').parentElement?.parentElement;
    const status = screen.getByRole('status', { name: '美股时段状态' });

    expect(clock).not.toHaveClass('border-y');
    expect(timeRows).not.toHaveClass('divide-y');
    expect(status).not.toHaveClass('border-l');
    expect(status).not.toHaveClass('bg-slate-900/45');
  });

  it('uses compact rows and pushes a double-size status to the far right', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-08-05T14:23:45Z'));

    const { container } = render(<MarketClock timeZone="America/New_York" session="non_trading" />);
    const clock = container.firstElementChild;
    const timeRows = screen.getByText('美国东部时间').parentElement?.parentElement;
    const status = screen.getByRole('status', { name: '美股时段状态' });

    expect(clock).not.toHaveClass('max-w-[42rem]');
    expect(timeRows).toHaveClass('space-y-0.5');
    expect(status).toHaveClass('ml-auto');
    expect(within(status).getByText('休市')).toHaveClass('text-[22px]', 'sm:text-2xl');
  });

  it('shows this week and lets the user inspect events by day', async () => {
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><TodayPage /></QueryClientProvider></MemoryRouter>);

    const calendar = await screen.findByRole('region', { name: '本周大事' });
    expect(within(calendar).getByText('NVDA 财报')).toBeInTheDocument();
    expect(within(calendar).queryByText('检查 MU 计划')).not.toBeInTheDocument();
    expect(within(calendar).getByText('8月3日—8月7日')).toBeInTheDocument();
    expect(within(calendar).queryByText(/只显示当前市场与研究范围内的事件/)).not.toBeInTheDocument();
    expect(within(calendar).queryByRole('link', { name: 'Yahoo Finance' })).not.toBeInTheDocument();
    expect(within(calendar).queryByRole('link', { name: 'Yahoo Economic Calendar' })).not.toBeInTheDocument();
    expect(within(calendar).queryByRole('link', { name: 'BLS' })).not.toBeInTheDocument();
    expect(within(calendar).queryByRole('link', { name: 'Federal Reserve' })).not.toBeInTheDocument();
    expect(within(calendar).queryByRole('link', { name: 'BEA' })).not.toBeInTheDocument();
    expect(within(calendar).queryByText('预估 EPS 1.05')).not.toBeInTheDocument();
    expect(within(calendar).queryByText('Yahoo Finance')).not.toBeInTheDocument();
    expect(within(calendar).queryByText(/EVENTS/)).not.toBeInTheDocument();

    fireEvent.click(within(calendar).getByRole('tab', { name: '周五 8月7日，1 项事件' }));
    expect(within(calendar).getByText('检查 MU 计划')).toBeInTheDocument();
    expect(within(calendar).queryByText('NVDA 财报')).not.toBeInTheDocument();
  });

  it('defaults to Friday when viewed over the weekend', () => {
    render(<MemoryRouter><WeeklyCalendar calendar={weekCalendar} localDate="2026-08-08" /></MemoryRouter>);

    expect(screen.getByRole('tab', { name: '周五 8月7日，1 项事件' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByText('检查 MU 计划')).toBeInTheDocument();
  });

  it('does not show external-source failure copy in a partial calendar', () => {
    render(<MemoryRouter><WeeklyCalendar calendar={{ ...weekCalendar, status: 'partial' }} localDate="2026-08-05" /></MemoryRouter>);

    expect(screen.queryByText('部分外部日历读取失败，已显示其余可用事件。')).not.toBeInTheDocument();
  });
});
