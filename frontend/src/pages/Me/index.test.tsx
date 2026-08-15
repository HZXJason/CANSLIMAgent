// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as profileApi from '../../lib/profileApi';
import * as tradeReviewApi from '../../lib/tradeReviewApi';
import MePage from '.';
import type { ProfileDashboard } from '../../types/profile';

vi.mock('../../lib/profileApi');
vi.mock('../../lib/tradeReviewApi');

const dashboard: ProfileDashboard = { range: '30d', from: null, to: '2026-08-05T00:00:00Z', as_of: '2026-08-05T00:00:00Z', sample_size: 0, process: { executions: 0, planned_executions: 0, planned_execution_rate: 0, pretrade_reviewed: 0, pretrade_review_rate: 0, completed_reviews: 0, review_completion_rate: 0, followed_reviews: 0, plan_follow_rate: 0, deviated_reviews: 0, off_plan_executions: 0 }, trends: [], outcomes: [] };

describe('MePage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(profileApi.fetchTradingProfile).mockResolvedValue({ configured: false, primary_markets: [], trading_horizons: [], max_risk_per_trade_percent: null, max_daily_loss_percent: null, max_open_positions: null, created_at: '', updated_at: '' });
    vi.mocked(profileApi.fetchProfileDashboard).mockResolvedValue(dashboard);
    vi.mocked(profileApi.fetchPrinciples).mockResolvedValue([]);
    vi.mocked(profileApi.fetchPlaybooks).mockResolvedValue([]);
    vi.mocked(profileApi.fetchAIMemory).mockResolvedValue({ principles_enabled: true, observations_enabled: true, playbooks_enabled: true, created_at: '', updated_at: '' });
    vi.mocked(tradeReviewApi.fetchBehaviorObservations).mockResolvedValue([]);
  });
  afterEach(cleanup);

  it('shows a focused overview and reveals each workspace on demand', async () => {
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MePage /></QueryClientProvider></MemoryRouter>);
    expect(await screen.findByRole('heading', { name: '当前状态' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '设置' })).toHaveAttribute('href', '/settings');
    expect(screen.getByRole('heading', { name: '执行趋势' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: '交易规则' })).not.toBeInTheDocument();
    expect(profileApi.fetchPrinciples).not.toHaveBeenCalled();
    expect(profileApi.fetchPlaybooks).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('tab', { name: '规则' }));
    expect(screen.getByRole('heading', { name: '交易规则' })).toBeInTheDocument();
    expect(screen.getByText('尚未建立个人交易规则')).toBeInTheDocument();
    expect(screen.queryByText(/系统不会替你生成默认画像/)).not.toBeInTheDocument();
    expect(screen.queryByText(/确定市场与交易周期/)).not.toBeInTheDocument();

    expect(profileApi.fetchPrinciples).toHaveBeenCalled();

    fireEvent.keyDown(screen.getByRole('tab', { name: '规则' }), { key: 'ArrowRight' });
    expect(screen.getByRole('tab', { name: '剧本' })).toHaveAttribute('aria-selected', 'true');

    fireEvent.click(screen.getByRole('tab', { name: '记忆与数据' }));
    expect(screen.getByRole('heading', { name: 'AI 记忆与数据' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: '交易规则' })).not.toBeInTheDocument();
  });

  it('reloads dynamic evidence when the time range changes', async () => {
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MePage /></QueryClientProvider></MemoryRouter>);
    await screen.findByRole('heading', { name: '当前状态' });
    fireEvent.click(screen.getByRole('button', { name: '最近 90 天' }));
    await waitFor(() => expect(profileApi.fetchProfileDashboard).toHaveBeenCalledWith('90d'));
  });
});
