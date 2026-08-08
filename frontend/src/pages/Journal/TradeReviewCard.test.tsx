// @vitest-environment jsdom

import '@testing-library/jest-dom/vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { fetchJournalPlanCandidates, fetchTradeReview, linkJournalToPlan, updateTradeReview } from '../../lib/tradeReviewApi';
import type { TradeReview } from '../../types/tradeReview';
import TradeReviewCard from './TradeReviewCard';

vi.mock('../../lib/tradeReviewApi', () => ({
  createTradeReview: vi.fn(), fetchJournalPlanCandidates: vi.fn(), fetchTradeReview: vi.fn(), linkJournalToPlan: vi.fn(), updateTradeReview: vi.fn()
}));

afterEach(() => { cleanup(); vi.clearAllMocks(); });

function review(overrides: Partial<TradeReview> = {}): TradeReview {
  return {
    id: 'review-1', created_at: '2026-08-04T10:00:00Z', updated_at: '2026-08-04T10:00:00Z', journal_entry_id: 'journal-1', version: 1,
    trade_plan_id: 'plan-1', plan_snapshot: { entry_price: 100 }, execution_snapshot: { price: 102 },
    diff: { plan_link_status: 'linked', symbol_match: true, direction_match: true, entry_slippage_percent: 2, timing_status: 'within_plan_window', known_deviations: ['adverse_entry_slippage_over_1_percent'], unknown_comparisons: ['actual_exit', 'stop_execution'] },
    process_quality: 'unreviewed', deviation_reason: null, emotion_before: null, emotion_after: null, outcome_notes: null, lesson: null, completed_at: null,
    ...overrides
  };
}

function renderCard() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<MemoryRouter><QueryClientProvider client={client}><TradeReviewCard journalId="journal-1" /></QueryClientProvider></MemoryRouter>);
}

describe('TradeReviewCard', () => {
  it('separates execution diff, process quality, outcome, and unknown facts', async () => {
    vi.mocked(fetchTradeReview).mockResolvedValue(review());
    renderCard();
    expect(await screen.findByText('计划内成交')).toBeInTheDocument();
    expect(screen.getByText(/入场不利偏差 2/)).toBeInTheDocument();
    expect(screen.getByText('过程复盘')).toBeInTheDocument();
    expect(screen.getAllByText('结果记录')).toHaveLength(2);
    expect(screen.getByText(/真实退出.*未知/)).toBeInTheDocument();
    expect(screen.getByText(/止损执行.*未知/)).toBeInTheDocument();
    expect(screen.queryByText(/不会改写成交事实/)).not.toBeInTheDocument();
  });

  it('keeps broker executions off-plan until explicit user confirmation', async () => {
    vi.mocked(fetchTradeReview).mockResolvedValue(review({ trade_plan_id: null, plan_snapshot: {}, diff: { plan_link_status: 'off_plan', symbol_match: null, direction_match: null, entry_slippage_percent: null, timing_status: 'unknown', known_deviations: [], unknown_comparisons: ['planned_quantity'] } }));
    vi.mocked(fetchJournalPlanCandidates).mockResolvedValue([{ id: 'plan-2', ticker: 'NVDA', direction: 'buy', plan_date: '2026-08-04', status: 'waiting' } as never]);
    vi.mocked(linkJournalToPlan).mockResolvedValue(review({ id: 'review-2', version: 2, trade_plan_id: 'plan-2' }));
    renderCard();
    expect(await screen.findByText('计划外成交')).toBeInTheDocument();
    fireEvent.change(await screen.findByLabelText('关联交易计划'), { target: { value: 'plan-2' } });
    fireEvent.click(screen.getByRole('button', { name: '确认关联' }));
    await waitFor(() => expect(linkJournalToPlan).toHaveBeenCalledWith('journal-1', 'plan-2'));
  });

  it('saves process and outcome fields independently', async () => {
    vi.mocked(fetchTradeReview).mockResolvedValue(review());
    vi.mocked(updateTradeReview).mockResolvedValue(review({ process_quality: 'deviated', outcome_notes: '结果尚未闭环' }));
    renderCard();
    fireEvent.change(await screen.findByLabelText('过程质量'), { target: { value: 'deviated' } });
    fireEvent.change(screen.getByLabelText('结果记录'), { target: { value: '结果尚未闭环' } });
    fireEvent.click(screen.getByRole('button', { name: '保存复盘' }));
    await waitFor(() => expect(updateTradeReview).toHaveBeenCalledWith('journal-1', expect.objectContaining({ process_quality: 'deviated', outcome_notes: '结果尚未闭环' })));
  });
});
