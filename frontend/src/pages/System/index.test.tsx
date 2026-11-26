// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as profileApi from '../../lib/profileApi';
import * as tradeReviewApi from '../../lib/tradeReviewApi';
import * as tradesApi from '../../lib/tradesApi';
import { useMarketStore } from '../../stores/marketStore';
import SystemPage from './index';

describe('SystemPage', () => {
  beforeEach(() => {
    useMarketStore.setState({ assetMarket: 'US', hasSelectedMarket: true });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('renders 4-quadrant scorecard with Good Loss & Toxic Win and saves 1R risk profile', async () => {
    vi.spyOn(tradesApi, 'fetchTradesSummary').mockResolvedValue({
      total_trades: 5,
      open_trades: 0,
      closed_trades: 5,
      model_wins: 2,
      good_losses: 2,
      unforced_errors: 0,
      toxic_wins: 1,
      expectancy_r: 0.6,
      avg_win_r: 2.0,
      avg_loss_r: -1.0,
      win_rate: 60,
      cost_of_indiscipline_r: 0,
      disciplined_trades: 4,
      disciplined_r_loop_rate: 80,
      open_portfolio_risk: 0,
      open_portfolio_risk_percent: 0,
      account_equity: 100000,
      one_r_amount: 1000,
      max_portfolio_heat_percent: 5,
      unlinked_fills_count: 0,
      pending_review_trades: 0
    });
    vi.spyOn(tradeReviewApi, 'fetchBehaviorObservations').mockResolvedValue([]);
    vi.spyOn(profileApi, 'fetchTradingProfile').mockResolvedValue({
      configured: true,
      primary_markets: ['US'],
      trading_horizons: ['swing'],
      max_risk_per_trade_percent: 1,
      max_daily_loss_percent: 3,
      max_open_positions: 6,
      account_equity: 100000,
      account_currency: 'USD',
      standard_risk_percent: 1.0,
      max_portfolio_heat_percent: 5.0,
      created_at: '',
      updated_at: ''
    });
    vi.spyOn(profileApi, 'fetchPlaybooks').mockResolvedValue([]);
    vi.spyOn(profileApi, 'fetchPolicies').mockResolvedValue([]);
    const saveSpy = vi.spyOn(profileApi, 'saveTradingProfile').mockResolvedValue({} as never);

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/system']}>
          <SystemPage />
        </MemoryRouter>
      </QueryClientProvider>
    );

    expect(await screen.findByText('80.0%')).toBeInTheDocument();
    expect(screen.getByText(/合规止损 \(Good Loss\)|合规止损/)).toBeInTheDocument();
    expect(screen.getByText(/毒性盈利 \(Toxic Win\)/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('tab', { name: '账户净值与 1R 底线' }));
    fireEvent.change(await screen.findByLabelText('账户总净值 (Account Equity)'), { target: { value: '200000' } });
    fireEvent.click(screen.getByRole('button', { name: '保存风控参数' }));

    await waitFor(() => {
      expect(saveSpy).toHaveBeenCalledWith(expect.objectContaining({ account_equity: 200000, standard_risk_percent: 1 }));
    });
  });
});
