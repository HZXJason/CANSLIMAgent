// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import * as api from '../../lib/api';
import * as decisionSpineApi from '../../lib/decisionSpineApi';
import * as deskApi from '../../lib/deskApi';
import * as positionSizerApi from '../../lib/positionSizerApi';
import * as pretradeReviewApi from '../../lib/pretradeReviewApi';
import * as profileApi from '../../lib/profileApi';
import QuickPlanCockpit from './QuickPlanCockpit';

describe('QuickPlanCockpit', () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('sizes 1R position and atomically creates thesis + waiting trade plan', async () => {
    vi.spyOn(profileApi, 'fetchPlaybooks').mockResolvedValue([
      {
        id: 'pb-1',
        name: '箱体枢轴突破',
        setup_type: 'pivot_breakout',
        markets: ['US'],
        horizons: ['swing'],
        market_context: null,
        entry_rules: null,
        invalidation_rules: null,
        stop_rules: null,
        target_rules: null,
        position_rules: null,
        no_trade_rules: null,
        notes: null,
        default_risk_multiple: 1.0,
        default_stop_atr_multiple: 1.0,
        status: 'active',
        source: 'system',
        evidence_review_ids: [],
        evidence_count: 0,
        ai_enabled: true,
        created_at: '',
        updated_at: ''
      }
    ]);
    vi.spyOn(deskApi, 'fetchMarketRegime').mockResolvedValue({
      market: 'US',
      status: 'ok',
      regime: 'confirmed_uptrend',
      regime_label: '确认上升趋势',
      max_exposure_percent: 100,
      distribution_days: 1,
      guidance: 'ok',
      data_as_of: '2026-11-25',
      indices: []
    });
    vi.spyOn(positionSizerApi, 'calculatePositionSize').mockResolvedValue({
      symbol: 'NVDA',
      lot_size: 1,
      min_lot_size: 1,
      one_r_amount: 1000,
      target_risk_amount: 1000,
      per_share_risk: 5,
      suggested_quantity: 200,
      position_value: 20000,
      position_percent: 20,
      actual_risk_amount: 1000,
      actual_risk_percent: 1.0,
      actual_risk_r: 1.0,
      stop_distance_percent: 5.0,
      stop_atr_multiple: 1.25,
      reward_risk_ratio: 3.0,
      current_open_heat_percent: 0,
      projected_heat_amount: 1000,
      projected_heat_percent: 1.0,
      warnings: []
    });
    vi.spyOn(decisionSpineApi, 'createCandidateSnapshot').mockResolvedValue({ id: 'cand-1' } as never);
    vi.spyOn(decisionSpineApi, 'createResearchCase').mockResolvedValue({ id: 'case-1' } as never);
    vi.spyOn(decisionSpineApi, 'createUserThesis').mockResolvedValue({ id: 'thesis-1' } as never);
    const createPlanSpy = vi.spyOn(api, 'createTradePlan').mockResolvedValue({ id: 'plan-1' } as never);
    vi.spyOn(pretradeReviewApi, 'fetchLatestPreTradeReview').mockRejectedValue(new Error('not found'));

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <QuickPlanCockpit symbol="NVDA" market="NASDAQ100" />
      </QueryClientProvider>
    );

    fireEvent.click(await screen.findByRole('button', { name: '箱体枢轴突破' }));
    fireEvent.change(screen.getByLabelText('入场触发价'), { target: { value: '100' } });
    fireEvent.change(screen.getByLabelText('失效止损价'), { target: { value: '95' } });

    expect(await screen.findByText('200 股')).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText(/买入唯一核心理由/), { target: { value: 'EPS 加速放量突破' } });
    fireEvent.change(screen.getByLabelText(/证伪离场条件/), { target: { value: '跌破 95 止损' } });
    fireEvent.click(screen.getByRole('button', { name: /保存观点与计划并执行红队审查/ }));

    await waitFor(() => {
      expect(createPlanSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          ticker: 'NVDA',
          status: 'waiting',
          playbook_id: 'pb-1',
          planned_quantity: 200,
          risk_multiple: 1.0
        })
      );
    });
  });
});
