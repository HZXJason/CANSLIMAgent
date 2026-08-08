// @vitest-environment jsdom

import '@testing-library/jest-dom/vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { fetchBehaviorObservations, updateBehaviorObservationFeedback } from '../../lib/tradeReviewApi';
import BehaviorObservationsCard from './BehaviorObservationsCard';

vi.mock('../../lib/tradeReviewApi', () => ({ fetchBehaviorObservations: vi.fn(), updateBehaviorObservationFeedback: vi.fn() }));
afterEach(() => { cleanup(); vi.clearAllMocks(); });

describe('BehaviorObservationsCard', () => {
  it('shows sample count and confidence without personality labels', async () => {
    vi.mocked(fetchBehaviorObservations).mockResolvedValue([{
      id: 'observation-1', created_at: '', updated_at: '', key: 'adverse_entry_slippage', scenario: 'complete_linked_trade_reviews', status: 'eligible',
      sample_count: 10, confidence: 0.6, message: '在 10 个完整样本中，有 4 次入场价格出现不利偏差。', evidence_review_ids: ['r1', 'r2', 'r3', 'r4'], feedback: null, feedback_at: null
    }]);
    vi.mocked(updateBehaviorObservationFeedback).mockResolvedValue({ id: 'observation-1', feedback: 'not_applicable' } as never);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={client}><BehaviorObservationsCard /></QueryClientProvider>);
    expect(await screen.findByText(/样本 10/)).toBeInTheDocument();
    expect(screen.getByText(/置信度 60%/)).toBeInTheDocument();
    expect(screen.queryByText(/冲动型交易者|固定交易风格/)).not.toBeInTheDocument();
    expect(screen.queryByText(/不定义你的性格/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '不适用于我' }));
    await waitFor(() => expect(updateBehaviorObservationFeedback).toHaveBeenCalledWith('observation-1', 'not_applicable'));
  });

  it('does not ask for feedback when the sample is insufficient', async () => {
    vi.mocked(fetchBehaviorObservations).mockResolvedValue([{
      id: 'insufficient', created_at: '', updated_at: '', key: 'sample_threshold', scenario: 'complete_linked_trade_reviews', status: 'insufficient',
      sample_count: 0, confidence: 0, message: '当前样本不足。', evidence_review_ids: [], feedback: null, feedback_at: null
    }]);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={client}><BehaviorObservationsCard /></QueryClientProvider>);
    expect(await screen.findByText('当前样本不足。')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '认可' })).not.toBeInTheDocument();
  });
});
