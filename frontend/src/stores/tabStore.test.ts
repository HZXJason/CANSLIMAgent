// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest';
import { useTabStore } from './tabStore';

describe('analysis tab store market isolation', () => {
  beforeEach(() => {
    useTabStore.setState({ tabs: [], activeTabIds: { US: null, CN: null } });
    window.localStorage.clear();
  });

  it('remembers an active tab for each asset market without deleting the other market', () => {
    const store = useTabStore.getState();
    store.openTab('AAPL', 'Apple');
    store.openTab('600519.SS', '贵州茅台');

    expect(useTabStore.getState().tabs.map((tab) => tab.symbol)).toEqual(['AAPL', '600519.SH']);
    expect(useTabStore.getState().activeTabIds).toEqual({ US: 'AAPL', CN: '600519.SH' });

    useTabStore.getState().setActiveTab('AAPL');
    expect(useTabStore.getState().activeTabIds).toEqual({ US: 'AAPL', CN: '600519.SH' });
  });

  it('selects an adjacent tab only within the market whose active tab closed', () => {
    const store = useTabStore.getState();
    store.openTab('AAPL', 'Apple');
    store.openTab('MSFT', 'Microsoft');
    store.openTab('600519.SH', '贵州茅台');

    useTabStore.getState().closeTab('MSFT');

    expect(useTabStore.getState().activeTabIds).toEqual({ US: 'AAPL', CN: '600519.SH' });
    expect(useTabStore.getState().tabs.map((tab) => tab.symbol)).toEqual(['AAPL', '600519.SH']);
  });

  it('does not create tabs for malformed or cross-market-unclassifiable symbols', () => {
    useTabStore.getState().openTab('not valid!', 'Invalid');
    expect(useTabStore.getState().tabs).toEqual([]);
    expect(useTabStore.getState().activeTabIds).toEqual({ US: null, CN: null });
  });
});
