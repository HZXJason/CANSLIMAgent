// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useMarketStore } from '../../stores/marketStore';
import * as deskApi from '../../lib/deskApi';
import DeskPage from './index';

describe('DeskPage', () => {
  beforeEach(() => {
    useMarketStore.setState({ assetMarket: 'US', hasSelectedMarket: true });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('renders Market Regime M gate, Portfolio Heat, and North Star rate', async () => {
    vi.spyOn(deskApi, 'fetchDesk').mockResolvedValue({
      market: 'US',
      timezone: 'America/New_York',
      local_date: '2026-11-26',
      session: 'premarket',
      market_regime: {
        market: 'US',
        status: 'ok',
        regime: 'uptrend_under_pressure',
        regime_label: '上升趋势受压',
        max_exposure_percent: 50,
        distribution_days: 5,
        guidance: '派发日累积，建议压缩总仓位至 50% 以内。',
        data_as_of: '2026-11-25',
        indices: [
          {
            symbol: 'SPY',
            name: '标普 500 ETF',
            status: 'ok',
            regime: 'uptrend_under_pressure',
            regime_label: '上升趋势受压',
            max_exposure_percent: 50,
            close: 590,
            ema_21: 588,
            sma_50: 580,
            sma_200: 540,
            atr_14_percent: 1.1,
            distribution_days: 5,
            above_21ema: true,
            above_50sma: true,
            above_200sma: true,
            data_as_of: '2026-11-25',
            source: 'stooq'
          }
        ]
      },
      summary: {
        total_trades: 4,
        open_trades: 1,
        closed_trades: 3,
        model_wins: 1,
        good_losses: 1,
        unforced_errors: 0,
        toxic_wins: 1,
        expectancy_r: 0.45,
        avg_win_r: 1.8,
        avg_loss_r: -1.0,
        win_rate: 66.7,
        cost_of_indiscipline_r: 0,
        disciplined_trades: 2,
        disciplined_r_loop_rate: 66.7,
        open_portfolio_risk: 1000,
        open_portfolio_risk_percent: 1.0,
        account_equity: 100000,
        one_r_amount: 1000,
        max_portfolio_heat_percent: 5.0,
        unlinked_fills_count: 1,
        pending_review_trades: 0
      },
      open_trades: [],
      waiting_plans: [],
      pending_reviews: []
    });

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <DeskPage />
        </MemoryRouter>
      </QueryClientProvider>
    );

    expect((await screen.findAllByText('上升趋势受压')).length).toBeGreaterThan(0);
    expect(screen.getByText('50%')).toBeInTheDocument();
    expect(screen.getByText('66.7%')).toBeInTheDocument();
    expect(screen.getByText(/1\.00% \/ 5\.0%/)).toBeInTheDocument();
    expect(screen.getByText(/毒性盈利 Toxic Win/)).toBeInTheDocument();
    expect(screen.getByText(/发现 1 笔计划外成交/)).toBeInTheDocument();
  });
});
