// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as todayApi from '../../lib/todayApi';
import type { WeekCalendar } from '../../types/today';
import TodayPage from '.';
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
      session: 'premarket',
      calendar_status: 'weekday_only_unverified',
      data_as_of: '2026-08-05T08:00:00Z',
      week_calendar: weekCalendar,
      items: [
        { id: 'plan-1', kind: 'unreviewed_plan', title: '检查 NVDA 计划', subtitle: '等待盘前检查', href: '/trade-plans/plan-1', priority: 100 },
        { id: 'review-1', kind: 'pending_review', title: '复盘 MU', subtitle: '昨日已平仓', href: '/journal/review-1', priority: 80 }
      ]
    });
  });

  afterEach(cleanup);

  it('promotes one next action and keeps the rest in a quieter queue', async () => {
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><TodayPage /></QueryClientProvider></MemoryRouter>);

    const next = await screen.findByRole('region', { name: '下一步' });
    expect(within(next).getByText('检查 NVDA 计划')).toBeInTheDocument();
    expect(within(next).queryByText('复盘 MU')).not.toBeInTheDocument();

    const later = screen.getByRole('region', { name: '稍后处理' });
    expect(within(later).getByText('复盘 MU')).toBeInTheDocument();
    expect(screen.queryByText(/不替你制造交易理由/)).not.toBeInTheDocument();
  });

  it('shows this week and lets the user inspect events by day', async () => {
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><TodayPage /></QueryClientProvider></MemoryRouter>);

    const calendar = await screen.findByRole('region', { name: '本周大事' });
    expect(within(calendar).getByText('NVDA 财报')).toBeInTheDocument();
    expect(within(calendar).queryByText('检查 MU 计划')).not.toBeInTheDocument();
    expect(within(calendar).getByRole('link', { name: 'Yahoo Finance' })).toBeInTheDocument();
    expect(within(calendar).getByRole('link', { name: 'Yahoo Economic Calendar' })).toBeInTheDocument();
    expect(within(calendar).getByRole('link', { name: 'BLS' })).toBeInTheDocument();
    expect(within(calendar).getByRole('link', { name: 'Federal Reserve' })).toBeInTheDocument();
    expect(within(calendar).getByRole('link', { name: 'BEA' })).toBeInTheDocument();

    fireEvent.click(within(calendar).getByRole('tab', { name: '周五 8月7日，1 项事件' }));
    expect(within(calendar).getByText('检查 MU 计划')).toBeInTheDocument();
    expect(within(calendar).queryByText('NVDA 财报')).not.toBeInTheDocument();
  });

  it('defaults to Friday when viewed over the weekend', () => {
    render(<MemoryRouter><WeeklyCalendar calendar={weekCalendar} localDate="2026-08-08" /></MemoryRouter>);

    expect(screen.getByRole('tab', { name: '周五 8月7日，1 项事件' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByText('检查 MU 计划')).toBeInTheDocument();
  });
});
