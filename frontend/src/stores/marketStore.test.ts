// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest';
import { useMarketStore } from './marketStore';

describe('market store', () => {
  afterEach(() => {
    useMarketStore.setState({ assetMarket: 'US', hasSelectedMarket: false });
    window.localStorage.clear();
  });

  it('records and persists the first explicit market choice', () => {
    useMarketStore.getState().setAssetMarket('CN');

    expect(useMarketStore.getState()).toMatchObject({ assetMarket: 'CN', hasSelectedMarket: true });
    const persisted = JSON.parse(window.localStorage.getItem('alphalens-asset-market') ?? '{}') as {
      state?: { assetMarket?: string; hasSelectedMarket?: boolean };
    };
    expect(persisted.state?.assetMarket).toBe('CN');
    expect(persisted.state?.hasSelectedMarket).toBe(true);
  });
});
