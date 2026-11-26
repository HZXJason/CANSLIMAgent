// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useMarketStore } from '../stores/marketStore';
import AppShell from './AppShell';

function renderShell(path = '/desk') {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/desk" element={<div>作战室页面</div>} />
          <Route path="/research" element={<div>研究台页面</div>} />
          <Route path="/positions" element={<div>头寸簿页面</div>} />
          <Route path="/system" element={<div>交易系统页面</div>} />
        </Route>
      </Routes>
    </MemoryRouter>
  );
}

describe('AppShell 4-workspace layout', () => {
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
    fireEvent.click(screen.getByRole('button', { name: /我是 A 股投资者/ }));

    expect(useMarketStore.getState()).toMatchObject({ assetMarket: 'CN', hasSelectedMarket: true });
    expect(screen.getByText('作战室页面')).toBeInTheDocument();
  });

  it('provides 1-click US/CN market switching and 4 temporal workspaces', () => {
    renderShell('/research');

    const mobileNav = screen.getByRole('navigation', { name: '移动端主导航' });
    expect(mobileNav.querySelectorAll('a')).toHaveLength(4);
    expect(screen.getAllByRole('link', { name: '作战室' }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole('link', { name: '研究台' }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole('link', { name: '头寸簿' }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole('link', { name: '交易系统' }).length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole('button', { name: 'A股 CN' }));
    expect(useMarketStore.getState().assetMarket).toBe('CN');
  });

  it('keeps the active route readable on dark navigation surfaces', () => {
    renderShell('/research');
    for (const link of screen.getAllByRole('link', { name: '研究台' })) {
      expect(link.className).not.toContain('text-paper-ink');
    }
  });

  it('uses a vertical navigation rail between tablet and desktop breakpoints', () => {
    const { container } = renderShell();
    expect(container.querySelector('[data-testid="rail-navigation"]')).toHaveClass('flex-col');
  });

  it('keeps mobile header clean with brand and market switch only', () => {
    renderShell();
    const mobileHeader = screen.getByRole('banner');
    expect(within(mobileHeader).getByText('AlphaLens')).toBeInTheDocument();
    expect(within(mobileHeader).queryByText('作战室')).not.toBeInTheDocument();
  });
});
