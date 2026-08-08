// @vitest-environment jsdom

import '@testing-library/jest-dom/vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { acknowledgePreTradeFinding, fetchLatestPreTradeReview } from '../../lib/pretradeReviewApi';
import type { PreTradeRiskReview } from '../../types/pretradeReview';
import PreTradeRiskReviewCard from './PreTradeRiskReviewCard';

vi.mock('../../lib/pretradeReviewApi', () => ({
  acknowledgePreTradeFinding: vi.fn(),
  createPreTradeReview: vi.fn(),
  fetchLatestPreTradeReview: vi.fn()
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function review(overrides: Partial<PreTradeRiskReview> = {}): PreTradeRiskReview {
  return {
    id: 'review-2', created_at: '2026-08-04T10:00:00Z', trade_plan_id: 'plan-1', version: 2,
    plan_snapshot: {},
    hard_conflicts: [{ code: 'missing_stop_loss', severity: 'high', category: 'deterministic', message: '计划没有定义止损价。', field: 'stop_loss', evidence: { value: null } }],
    risk_warnings: [], behavior_reminders: [], unresolved_questions: [], changes_since_research: [],
    freshness: {}, sources: [], ai_status: 'unavailable', ai_provider: 'openai', ai_model: 'gpt-test', acknowledgements: [],
    ...overrides
  };
}

function renderCard() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={queryClient}><PreTradeRiskReviewCard planId="plan-1" /></QueryClientProvider>);
}

describe('PreTradeRiskReviewCard', () => {
  it('separates deterministic findings from an unavailable AI review', async () => {
    vi.mocked(fetchLatestPreTradeReview).mockResolvedValue(review());
    renderCard();

    expect(await screen.findByText('计划没有定义止损价。')).toBeInTheDocument();
    expect(screen.getByText(/AI 审查不可用/)).toBeInTheDocument();
    expect(screen.getByText('确定性冲突')).toBeInTheDocument();
    expect(screen.queryByText(/计划通过|批准交易|建议买入/)).not.toBeInTheDocument();
    expect(screen.queryByText(/先检查计划自洽性/)).not.toBeInTheDocument();
  });

  it('renders cited AI risks and data freshness', async () => {
    vi.mocked(fetchLatestPreTradeReview).mockResolvedValue(review({
      hard_conflicts: [], ai_status: 'completed',
      risk_warnings: [{ code: 'event_risk', severity: 'high', category: 'event', message: '财报窗口可能放大跳空风险。', field: 'valid_until', evidence: { source_ids: ['news-1'] } }],
      sources: [{ id: 'news-1', label: '公司公告', url: 'https://example.com/filing', data_as_of: '2026-08-04', status: 'available' }],
      freshness: { news: { status: 'fresh', data_as_of: '2026-08-04' } }
    }));
    renderCard();

    expect(await screen.findByText('财报窗口可能放大跳空风险。')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '公司公告' })).toHaveAttribute('href', 'https://example.com/filing');
    expect(screen.getByText(/版本 2/)).toBeInTheDocument();
    expect(screen.getByText(/2026-08-04/)).toBeInTheDocument();
  });

  it('records a high-risk acknowledgement without removing the finding', async () => {
    vi.mocked(fetchLatestPreTradeReview).mockResolvedValue(review());
    vi.mocked(acknowledgePreTradeFinding).mockResolvedValue({ id: 'ack-1' } as never);
    renderCard();

    fireEvent.click(await screen.findByRole('button', { name: '确认已知晓' }));
    fireEvent.change(screen.getByLabelText('确认理由（可选）'), { target: { value: '下单前会补充止损' } });
    fireEvent.click(screen.getByRole('button', { name: '保存确认' }));

    await waitFor(() => expect(acknowledgePreTradeFinding).toHaveBeenCalledWith('plan-1', 'review-2', { finding_code: 'missing_stop_loss', reason: '下单前会补充止损' }));
    expect(screen.getByText('计划没有定义止损价。')).toBeInTheDocument();
  });
});
