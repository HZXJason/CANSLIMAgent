// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it } from 'vitest';
import { useMarketStore } from '../stores/marketStore';
import AppShell from './AppShell';

function renderShell(path = '/screener') {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/screener" element={<div>筛选页面</div>} />
        </Route>
      </Routes>
    </MemoryRouter>
  );
}

describe('AppShell market theme', () => {
  afterEach(() => {
    cleanup();
    useMarketStore.getState().setAssetMarket('US');
    document.documentElement.removeAttribute('data-market');
    window.localStorage.clear();
  });

  it('syncs the selected global market to the html theme attribute', () => {
    useMarketStore.getState().setAssetMarket('CN');
    renderShell();

    expect(document.documentElement).toHaveAttribute('data-market', 'CN');
    fireEvent.change(screen.getAllByRole('combobox', { name: '股票市场' })[0], { target: { value: 'US' } });
    expect(document.documentElement).toHaveAttribute('data-market', 'US');
  });

  it('keeps the active route readable on dark navigation surfaces', () => {
    renderShell();

    for (const link of screen.getAllByRole('link', { name: '发现' })) {
      expect(link.className).not.toContain('text-paper-ink');
    }
  });

  it('uses a vertical navigation rail between tablet and desktop breakpoints', () => {
    const { container } = renderShell();

    expect(container.querySelector('[data-testid="rail-navigation"]')).toHaveClass('flex-col');
  });

  it('keeps stock analysis and exposes the personal workspace without adding a sixth mobile tab', () => {
    renderShell();
    expect(screen.getAllByRole('link', { name: '个股分析' }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole('link', { name: '我的' }).length).toBeGreaterThan(0);
    expect(screen.getByRole('navigation', { name: '移动端主导航' }).querySelectorAll('a')).toHaveLength(5);
  });

  it('uses concise navigation without decorative English labels or slogans', () => {
    renderShell();

    expect(screen.queryByText('DISCOVERY')).not.toBeInTheDocument();
    expect(screen.queryByText('研究先于决策')).not.toBeInTheDocument();
    expect(screen.queryByText('风险始终可见')).not.toBeInTheDocument();
    expect(screen.queryByText('让 AI 展示证据')).not.toBeInTheDocument();
  });
});
