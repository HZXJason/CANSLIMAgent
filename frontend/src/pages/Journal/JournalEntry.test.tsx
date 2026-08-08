// @vitest-environment jsdom

import '@testing-library/jest-dom/vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';

import JournalEntry from './JournalEntry';

vi.mock('../../lib/api', () => ({
  deleteJournalEntry: vi.fn(),
  fetchJournalEntry: vi.fn().mockResolvedValue({
    id: 'journal-1',
    datetime: '2026-08-04T09:30:00Z',
    symbol: 'NVDA',
    name: 'NVIDIA',
    side: 'buy',
    quantity: 10,
    price: 120,
    amount: 1200,
    fee: 1,
    market: 'us',
    currency: 'USD',
    order_id: null,
    trade_plan_id: 'plan-1',
    source: 'trade_plan',
    notes: null,
    reflection: null,
    created_at: '2026-08-04T09:30:00Z',
    updated_at: '2026-08-04T09:30:00Z'
  }),
  updateJournalReflection: vi.fn(),
  createJournalEntry: vi.fn()
}));

describe('JournalEntry provenance', () => {
  it('links a plan-created execution back to its original plan', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/journal/journal-1']}>
          <Routes>
            <Route path="/journal/:id" element={<JournalEntry />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    );

    const link = await screen.findByRole('link', { name: '查看原交易计划' });
    expect(link).toHaveAttribute('href', '/trade-plans/plan-1');
  });
});
