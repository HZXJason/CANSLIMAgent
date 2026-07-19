// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest';
import { useMarketStore } from './marketStore';

describe('market store', () => {
  afterEach(() => {
    useMarketStore.getState().setAssetMarket('US');
    window.localStorage.clear();
  });

  it('switches and persists the global asset market', () => {
    useMarketStore.getState().setAssetMarket('CN');

    expect(useMarketStore.getState().assetMarket).toBe('CN');
    const persisted = JSON.parse(window.localStorage.getItem('alphalens-asset-market') ?? '{}') as {
      state?: { assetMarket?: string };
    };
    expect(persisted.state?.assetMarket).toBe('CN');
  });
});
