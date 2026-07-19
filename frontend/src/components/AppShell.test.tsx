// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it } from 'vitest';
import { useMarketStore } from '../stores/marketStore';
import AppShell from './AppShell';

describe('AppShell market theme', () => {
  afterEach(() => {
    cleanup();
    useMarketStore.getState().setAssetMarket('US');
    document.documentElement.removeAttribute('data-market');
    window.localStorage.clear();
  });

  it('syncs the selected global market to the html theme attribute', () => {
    useMarketStore.getState().setAssetMarket('CN');
    render(
      <MemoryRouter initialEntries={['/screener']}>
        <Routes>
          <Route element={<AppShell />}>
            <Route path="/screener" element={<div>筛选页面</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    );

    expect(document.documentElement).toHaveAttribute('data-market', 'CN');
    fireEvent.change(screen.getAllByRole('combobox', { name: '股票市场' })[0], { target: { value: 'US' } });
    expect(document.documentElement).toHaveAttribute('data-market', 'US');
  });
});
