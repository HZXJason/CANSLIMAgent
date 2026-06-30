import type { ComputedStock } from '../types';

export function sortStocks(stocks: ComputedStock[]) {
  return [...stocks].sort((a, b) => {
    if (a.passCount !== b.passCount) return b.passCount - a.passCount;
    if (a.weightSum !== b.weightSum) return b.weightSum - a.weightSum;
    return a.symbol.localeCompare(b.symbol);
  });
}
