// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import MarketSelector from './MarketSelector';

describe('Screener MarketSelector', () => {
  afterEach(() => cleanup());

  it('shows the existing three non-destructive US market filters', () => {
    const onChange = vi.fn();
    render(<MarketSelector assetMarket="US" selectedMarket="SP500" onChange={onChange} />);

    expect(screen.getAllByRole('tab')).toHaveLength(3);
    fireEvent.click(screen.getByRole('tab', { name: '纳指 100' }));
    expect(onChange).toHaveBeenCalledWith('NASDAQ100');
  });

  it('shows five independently selectable A-share board filters', () => {
    const onChange = vi.fn();
    render(<MarketSelector assetMarket="CN" selectedMarket="STAR" onChange={onChange} />);

    expect(screen.getAllByRole('tab')).toHaveLength(5);
    expect(screen.getByRole('tab', { name: '科创板' })).toHaveAttribute('aria-selected', 'true');
    fireEvent.click(screen.getByRole('tab', { name: '创业板' }));
    expect(onChange).toHaveBeenCalledWith('CHINEXT');
    expect(screen.queryByRole('tab', { name: '标普 500' })).not.toBeInTheDocument();
  });
});
