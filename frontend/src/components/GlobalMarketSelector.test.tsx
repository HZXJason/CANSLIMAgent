// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { useMarketStore } from '../stores/marketStore';
import GlobalMarketSelector from './GlobalMarketSelector';

describe('GlobalMarketSelector', () => {
  afterEach(() => {
    cleanup();
    useMarketStore.getState().setAssetMarket('US');
    window.localStorage.clear();
  });

  it('renders a borderless, transparent native select and changes the global market', () => {
    render(<GlobalMarketSelector />);
    const select = screen.getByRole('combobox', { name: '股票市场' });

    expect(select).toHaveClass('h-11', 'border-0', 'bg-transparent', 'opacity-100');
    expect(screen.getByRole('option', { name: '美股市场' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'A股市场' })).toBeInTheDocument();

    fireEvent.change(select, { target: { value: 'CN' } });
    expect(useMarketStore.getState().assetMarket).toBe('CN');
  });
});
