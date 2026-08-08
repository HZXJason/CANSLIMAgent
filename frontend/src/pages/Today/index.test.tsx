// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as todayApi from '../../lib/todayApi';
import TodayPage from '.';

vi.mock('../../lib/todayApi');

describe('TodayPage task hierarchy', () => {
  beforeEach(() => {
    vi.mocked(todayApi.fetchTodayBrief).mockResolvedValue({
      market: 'US',
      timezone: 'America/New_York',
      local_date: '2026-08-07',
      session: 'premarket',
      calendar_status: 'weekday_only_unverified',
      data_as_of: '2026-08-07T08:00:00Z',
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
});
