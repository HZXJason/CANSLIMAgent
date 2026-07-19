// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useMarketStore } from '../../stores/marketStore';
import { useTabStore } from '../../stores/tabStore';
import AnalysisPage from './index';

vi.mock('./AnalysisPage', () => ({
  default: ({ symbol }: { symbol: string }) => <div data-testid="analysis-content">分析 {symbol}</div>
}));

function LocationProbe() {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <>
      <output data-testid="location-path">{location.pathname}</output>
      <button type="button" onClick={() => navigate('/analysis/600519.SH')}>测试前往 A 股深链</button>
      <button type="button" onClick={() => navigate('/analysis/AAPL')}>测试前往美股深链</button>
    </>
  );
}

function renderAnalysis(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/analysis" element={<><AnalysisPage /><LocationProbe /></>} />
        <Route path="/analysis/:symbol" element={<><AnalysisPage /><LocationProbe /></>} />
      </Routes>
    </MemoryRouter>
  );
}

describe('Analysis market-specific workspace', () => {
  beforeEach(() => {
    useMarketStore.setState({ assetMarket: 'US' });
    useTabStore.setState({ tabs: [], activeTabIds: { US: null, CN: null } });
    window.localStorage.clear();
  });

  afterEach(() => cleanup());

  it('shows only current-market tabs and restores each market active tab', async () => {
    useTabStore.getState().openTab('AAPL', 'Apple');
    useTabStore.getState().openTab('600519.SH', '贵州茅台');
    useTabStore.getState().setActiveTab('AAPL');
    renderAnalysis('/analysis/AAPL');

    expect(screen.getByRole('tab', { name: /AAPL/ })).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: /600519/ })).not.toBeInTheDocument();
    expect(screen.getByTestId('analysis-content')).toHaveTextContent('AAPL');

    act(() => useMarketStore.getState().setAssetMarket('CN'));
    await waitFor(() => expect(screen.getByTestId('analysis-content')).toHaveTextContent('600519.SH'));
    expect(screen.getByRole('tab', { name: /600519/ })).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: /AAPL/ })).not.toBeInTheDocument();

    act(() => useMarketStore.getState().setAssetMarket('US'));
    await waitFor(() => expect(screen.getByTestId('analysis-content')).toHaveTextContent('AAPL'));
    expect(useTabStore.getState().tabs.map((tab) => tab.symbol)).toEqual(['AAPL', '600519.SH']);
    expect(useTabStore.getState().tabs.map((tab) => tab.name)).toEqual(['Apple', '贵州茅台']);
  });

  it('shows an empty A-share workspace without deleting US tabs when none are open', async () => {
    useTabStore.getState().openTab('AAPL', 'Apple');
    renderAnalysis('/analysis/AAPL');

    act(() => useMarketStore.getState().setAssetMarket('CN'));

    await waitFor(() => expect(screen.getByTestId('location-path')).toHaveTextContent('/analysis'));
    expect(screen.getByText('添加股票开始分析')).toBeInTheDocument();
    expect(screen.queryByTestId('analysis-content')).not.toBeInTheDocument();
    expect(useTabStore.getState().tabs.map((tab) => tab.symbol)).toEqual(['AAPL']);
  });

  it.each([
    { persistedMarket: 'US' as const, path: '/analysis/600519.SH', expectedMarket: 'CN' as const, symbol: '600519.SH', expectedPath: '/analysis/600519.SH' },
    { persistedMarket: 'US' as const, path: '/analysis/600519', expectedMarket: 'CN' as const, symbol: '600519.SH', expectedPath: '/analysis/600519.SH' },
    { persistedMarket: 'CN' as const, path: '/analysis/AAPL', expectedMarket: 'US' as const, symbol: 'AAPL', expectedPath: '/analysis/AAPL' }
  ])('lets a valid deep link override a stale $persistedMarket market selection', async ({ persistedMarket, path, expectedMarket, symbol, expectedPath }) => {
    useMarketStore.setState({ assetMarket: persistedMarket });
    renderAnalysis(path);

    await waitFor(() => expect(useMarketStore.getState().assetMarket).toBe(expectedMarket));
    await waitFor(() => expect(screen.getByTestId('analysis-content')).toHaveTextContent(symbol));
    expect(useTabStore.getState().activeTabIds[expectedMarket]).toBe(symbol);
    expect(screen.getByTestId('location-path')).toHaveTextContent(expectedPath);
  });

  it('reconciles every route change while the same analysis workspace remains mounted', async () => {
    renderAnalysis('/analysis/AAPL');
    await waitFor(() => expect(screen.getByTestId('analysis-content')).toHaveTextContent('AAPL'));

    fireEvent.click(screen.getByRole('button', { name: '测试前往 A 股深链' }));
    await waitFor(() => expect(useMarketStore.getState().assetMarket).toBe('CN'));
    await waitFor(() => expect(screen.getByTestId('analysis-content')).toHaveTextContent('600519.SH'));
    expect(screen.getByTestId('location-path')).toHaveTextContent('/analysis/600519.SH');

    fireEvent.click(screen.getByRole('button', { name: '测试前往美股深链' }));
    await waitFor(() => expect(useMarketStore.getState().assetMarket).toBe('US'));
    await waitFor(() => expect(screen.getByTestId('analysis-content')).toHaveTextContent('AAPL'));
    expect(screen.getByTestId('location-path')).toHaveTextContent('/analysis/AAPL');
  });

  it('keeps invalid manual input visible with an actionable message and creates no tab', () => {
    useMarketStore.setState({ assetMarket: 'CN' });
    renderAnalysis('/analysis');

    fireEvent.click(screen.getByRole('button', { name: '添加股票' }));
    fireEvent.change(screen.getByRole('textbox', { name: '股票代码' }), { target: { value: 'AAPL' } });
    fireEvent.click(screen.getByRole('button', { name: '打开股票' }));

    expect(screen.getByRole('alert')).toHaveTextContent('不是有效的 A 股代码');
    expect(screen.getByRole('textbox', { name: '股票代码' })).toHaveValue('AAPL');
    expect(useTabStore.getState().tabs).toEqual([]);
  });

  it('shows an invalid deep-link error without redirecting in a loop', () => {
    useMarketStore.setState({ assetMarket: 'CN' });
    renderAnalysis('/analysis/INVALID!');

    expect(screen.getByRole('alert')).toHaveTextContent('不是有效的 A 股代码');
    expect(screen.getByTestId('location-path')).toHaveTextContent('/analysis/INVALID!');
    expect(useTabStore.getState().tabs).toEqual([]);
  });
});
