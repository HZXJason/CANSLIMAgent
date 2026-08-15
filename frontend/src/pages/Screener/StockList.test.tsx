// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it } from 'vitest';
import type { ComputedStock } from '../../types';
import StockList from './StockList';

const stock: ComputedStock = {
  symbol: 'TEST',
  name: 'Test Company',
  market: 'SP500',
  ai_analyzed: false,
  data_as_of: '2026-08-15',
  metrics: {
    c: { value: 30.5, sufficient: true },
    a: { value: [31, 29.5, 27], sufficient: true },
    s: { value: 165, sufficient: true },
    l: { value: 88, sufficient: true },
    i: { value: 12, sufficient: true },
    n: false
  },
  labels: {
    C: { key: 'C', status: 'pass', value: 30.5, sufficient: true },
    A: { key: 'A', status: 'pass', value: [31, 29.5, 27], sufficient: true },
    S: { key: 'S', status: 'pass', value: 165, sufficient: true },
    L: { key: 'L', status: 'pass', value: 88, sufficient: true },
    I: { key: 'I', status: 'pass', value: 12, sufficient: true }
  },
  passCount: 4,
  weightSum: 9
};

describe('Screener StockList label tooltips', () => {
  afterEach(() => cleanup());

  it('shows metric names and values without repeating color-coded status', () => {
    render(
      <MemoryRouter>
        <StockList stocks={[stock]} hasRawData filtered={false} />
      </MemoryRouter>
    );

    const tooltipText = screen.getAllByRole('tooltip').map((tooltip) => tooltip.textContent);
    expect(tooltipText).toEqual(expect.arrayContaining([
      'EPS（当季增长）：30.5%',
      'EPS（年度增长）：31.0% / 29.5% / 27.0%',
      'New 标签',
      '成交量比率：165%',
      'RS：88',
      '机构持仓变化：12'
    ]));
    expect(tooltipText.join(' ')).not.toMatch(/达标|未达标|原始值/);
  });

  it('names the metric when its value is unavailable', () => {
    const insufficientStock: ComputedStock = {
      ...stock,
      labels: {
        ...stock.labels,
        C: { key: 'C', status: 'insufficient', value: 0, sufficient: false }
      }
    };

    render(
      <MemoryRouter>
        <StockList stocks={[insufficientStock]} hasRawData filtered={false} />
      </MemoryRouter>
    );

    expect(screen.getByRole('tooltip', { name: 'EPS（当季增长）：暂无数据' })).toBeInTheDocument();
    expect(screen.queryByText('数据不足，谨慎参考，基准期以 0 计算')).not.toBeInTheDocument();
  });

  it('removes the redundant market column and keeps the stock code untruncated', () => {
    render(
      <MemoryRouter>
        <StockList stocks={[stock]} hasRawData filtered={false} />
      </MemoryRouter>
    );

    expect(screen.queryByText('市场')).not.toBeInTheDocument();
    expect(screen.queryAllByText(/标普500/)).toHaveLength(0);
    expect(screen.getByText('TEST')).toHaveClass('whitespace-nowrap');
    expect(screen.getByText('TEST')).not.toHaveClass('truncate');
  });
});
