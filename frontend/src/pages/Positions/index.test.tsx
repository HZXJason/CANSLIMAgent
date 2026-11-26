// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../../lib/api';
import * as tradesApi from '../../lib/tradesApi';
import { useMarketStore } from '../../stores/marketStore';
import PositionsPage from './index';

describe('PositionsPage', () => {
  beforeEach(() => {
    useMarketStore.setState({ assetMarket: 'US', hasSelectedMarket: true });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('displays closed trades with Realized R, 4-quadrant badges, and submits inline review', async () => {
    vi.spyOn(tradesApi, 'fetchTrades').mockImplementation(async (params) => {
      if (params?.status === 'open') return [];
      return [
        {
          id: 'tr-1',
          user_id: 'u1',
          created_at: '',
          updated_at: '',
          trade_plan_id: 'plan-1',
          playbook_id: null,
          symbol: 'NVDA',
          name: 'NVIDIA',
          market: 'us',
          currency: 'USD',
          direction: 'long',
          status: 'closed',
          opened_at: '2026-11-20T00:00:00Z',
          closed_at: '2026-11-22T00:00:00Z',
          open_quantity: 0,
          total_buy_quantity: 100,
          total_sell_quantity: 100,
          avg_entry_price: 100,
          avg_exit_price: 95,
          current_stop_loss: 95,
          total_fees: 0,
          planned_entry_price: 100,
          planned_stop_loss: 95,
          initial_risk_amount: 500,
          open_risk_amount: 0,
          realized_pnl: -500,
          realized_pnl_percent: -5,
          realized_r: -1.0,
          entry_slippage_r: 0,
          holding_days: 2,
          quadrant: 'good_loss',
          auto_violations: [],
          fills: []
        }
      ];
    });
    vi.spyOn(api, 'fetchTradePlans').mockResolvedValue([]);
    vi.spyOn(api, 'fetchJournalEntries').mockResolvedValue([]);
    vi.spyOn(api, 'fetchBrokerConnections').mockResolvedValue([]);
    const reviewSpy = vi.spyOn(tradesApi, 'upsertTradeReview').mockResolvedValue({ trade: {} as never });

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/positions?tab=closed']}>
          <PositionsPage />
        </MemoryRouter>
      </QueryClientProvider>
    );

    expect(await screen.findByText('-1.00R')).toBeInTheDocument();
    expect(screen.getByText('合规止损 Good Loss')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /15秒复盘/ }));
    fireEvent.change(screen.getByLabelText('一句话复盘教训'), { target: { value: '止损执行果断' } });
    fireEvent.click(screen.getByRole('button', { name: '保存复盘' }));

    await waitFor(() => {
      expect(reviewSpy).toHaveBeenCalledWith('tr-1', { process_quality: 'followed', lesson: '止损执行果断' });
    });
  });
});
