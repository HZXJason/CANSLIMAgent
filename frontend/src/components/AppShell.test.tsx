// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
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
  beforeEach(() => {
    useMarketStore.setState({ assetMarket: 'US', hasSelectedMarket: true });
  });

  afterEach(() => {
    cleanup();
    useMarketStore.setState({ assetMarket: 'US', hasSelectedMarket: true });
    document.documentElement.removeAttribute('data-market');
    window.localStorage.clear();
  });

  it('syncs the selected global market to the html theme attribute', () => {
    useMarketStore.getState().setAssetMarket('CN');
    renderShell();

    expect(document.documentElement).toHaveAttribute('data-market', 'CN');
    act(() => useMarketStore.getState().setAssetMarket('US'));
    expect(document.documentElement).toHaveAttribute('data-market', 'US');
  });

  it('asks first-time users to choose their investor market before showing the workspace', () => {
    useMarketStore.setState({ assetMarket: 'US', hasSelectedMarket: false });
    renderShell();

    expect(screen.getByRole('heading', { name: '你主要关注哪个市场？' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /我是 A 股投资者/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /我是美股投资者/ })).toBeInTheDocument();
    expect(screen.queryByText('沪深北市场 · 人民币口径')).not.toBeInTheDocument();
    expect(screen.queryByText('美国市场 · 美元口径')).not.toBeInTheDocument();
    expect(screen.queryByText('筛选页面')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /我是 A 股投资者/ }));

    expect(useMarketStore.getState()).toMatchObject({ assetMarket: 'CN', hasSelectedMarket: true });
    expect(screen.getByText('筛选页面')).toBeInTheDocument();
  });

  it('does not expose market switching in desktop or mobile app chrome', () => {
    renderShell();

    expect(screen.queryByRole('combobox', { name: '股票市场' })).not.toBeInTheDocument();
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

  it('keeps settings inside the personal workspace instead of the sidebar', () => {
    renderShell();

    const personalNavigation = screen.getByRole('navigation', { name: '个人导航' });
    expect(within(personalNavigation).getAllByRole('link')).toHaveLength(2);
    expect(within(personalNavigation).getAllByRole('link', { name: '我的' })).toHaveLength(2);
    expect(within(personalNavigation).queryByRole('link', { name: '设置' })).not.toBeInTheDocument();
  });

  it('uses concise navigation without decorative English labels or slogans', () => {
    renderShell();

    const mobileHeader = screen.getByRole('banner');
    expect(within(mobileHeader).getByText('AlphaLens')).toBeInTheDocument();
    expect(within(mobileHeader).queryByText('发现')).not.toBeInTheDocument();
    expect(screen.queryByText('DISCOVERY')).not.toBeInTheDocument();
    expect(screen.queryByText('研究先于决策')).not.toBeInTheDocument();
    expect(screen.queryByText('风险始终可见')).not.toBeInTheDocument();
    expect(screen.queryByText('让 AI 展示证据')).not.toBeInTheDocument();
  });
});
