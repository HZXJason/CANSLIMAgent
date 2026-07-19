// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it } from 'vitest';
import type { StockSummary } from '../../types/analysis';
import StockHeader from './StockHeader';

const insufficientSummary: StockSummary = {
  symbol: 'MU',
  name: 'Micron',
  market: 'NASDAQ100',
  metrics: {
    c: { value: 0, sufficient: false },
    a: { value: [], sufficient: false },
    s: { value: 0, sufficient: false },
    l: { value: 0, sufficient: false },
    i: { value: 0, sufficient: false },
    n: null
  },
  ai_analyzed: false,
  data_as_of: '2026-07-17',
  n_analysis_at: null,
  n_analysis_summary: ''
};

describe('StockHeader', () => {
  afterEach(() => cleanup());

  it('shows an explicit question mark for every insufficient CANSLIM metric', () => {
    render(
      <MemoryRouter>
        <StockHeader symbol="MU" tabName="Micron" summary={insufficientSummary} assetMarket="US" />
      </MemoryRouter>
    );

    for (const tag of ['C', 'A', 'S', 'L', 'I']) {
      expect(screen.getByLabelText(tag + ' 数据不足')).toHaveTextContent('?');
    }
  });

  it('uses the canonical symbol market while summary metadata is still loading', () => {
    render(
      <MemoryRouter>
        <StockHeader symbol="600519.SH" tabName="贵州茅台" summary={null} assetMarket="CN" />
      </MemoryRouter>
    );

    expect(screen.getByRole('heading', { name: '600519' })).toBeInTheDocument();
    expect(screen.getByText('A股市场')).toBeInTheDocument();
  });
});
