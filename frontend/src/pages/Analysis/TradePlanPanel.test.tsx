// @vitest-environment jsdom

import '@testing-library/jest-dom/vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';

import TradePlanPanel from './TradePlanPanel';

vi.mock('../../lib/api', () => ({ fetchTradePlans: vi.fn().mockResolvedValue([]) }));

function Destination() {
  const location = useLocation();
  const state = location.state as { userThesisId?: string; aiSummary?: string } | null;
  return <div>{location.pathname}|{location.search}|{state?.userThesisId}|{state?.aiSummary}</div>;
}

describe('TradePlanPanel decision provenance', () => {
  it('carries a saved thesis into a new trade plan', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/analysis/NVDA']}>
          <Routes>
            <Route path="/analysis/:symbol" element={<TradePlanPanel symbol="NVDA" aiSummary="风险仍待确认" userThesisId="thesis-1" />} />
            <Route path="/trade-plans/new" element={<Destination />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    );

    fireEvent.click(await screen.findByRole('button', { name: /新建 NVDA 交易计划/ }));

    expect(screen.getByText('/trade-plans/new|?symbol=NVDA|thesis-1|风险仍待确认')).toBeInTheDocument();
  });
});
