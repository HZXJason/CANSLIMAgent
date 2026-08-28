import { ArrowRight, Building2, Globe2 } from 'lucide-react';
import { useMarketStore } from '../stores/marketStore';
import type { AssetMarket } from '../types';

const choices: Array<{
  market: AssetMarket;
  title: string;
  icon: typeof Building2;
}> = [
  { market: 'CN', title: '我是 A 股投资者', icon: Building2 },
  { market: 'US', title: '我是美股投资者', icon: Globe2 }
];

export function MarketOnboarding() {
  const setAssetMarket = useMarketStore((state) => state.setAssetMarket);

  return (
    <main className="relative flex min-h-screen items-center overflow-hidden px-4 py-10 text-slate-100 sm:px-6">
      <div className="pointer-events-none absolute inset-x-0 top-0 h-px bg-brand-300/70" aria-hidden="true" />
      <div className="mx-auto w-full max-w-[920px]">
        <div className="border-b border-rule-strong pb-7 sm:pb-9">
          <p className="font-display text-xl font-bold text-white">AlphaLens</p>
          <h1 className="mt-12 max-w-3xl text-balance font-display text-[2.4rem] font-bold leading-[1.05] tracking-[-0.035em] text-ink sm:mt-16 sm:text-[3.8rem]">
            你主要关注哪个市场？
          </h1>
          <p className="mt-5 max-w-[68ch] text-sm leading-7 text-slate-400 sm:text-base">
            你的选择会决定“今日”、发现与个股分析的数据范围。之后可以在“我的 → 设置”中修改。
          </p>
        </div>

        <div className="grid border-b border-rule-strong sm:grid-cols-2">
          {choices.map(({ market, title, icon: Icon }, index) => (
            <button
              key={market}
              type="button"
              onClick={() => setAssetMarket(market)}
              className={`group flex min-h-44 items-center justify-between gap-5 py-7 text-left outline-none transition-[background-color,color] duration-150 hover:bg-slate-900/55 focus-visible:bg-slate-900/70 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand-300/70 sm:min-h-52 sm:px-7 ${index === 0 ? 'border-b border-rule sm:border-b-0 sm:border-r' : ''}`}
            >
              <span className="flex min-w-0 items-center gap-5">
                <span className="flex h-12 w-12 shrink-0 items-center justify-center rounded-sm border border-slate-700 bg-slate-950/70 text-brand-200 transition-colors group-hover:border-brand-300/50 group-hover:text-brand-100">
                  <Icon className="h-5 w-5" strokeWidth={1.7} aria-hidden="true" />
                </span>
                <span className="font-display text-xl font-bold text-white sm:text-2xl">{title}</span>
              </span>
              <ArrowRight className="h-5 w-5 shrink-0 text-slate-600 transition-[color,transform] duration-150 group-hover:translate-x-1 group-hover:text-brand-200" aria-hidden="true" />
            </button>
          ))}
        </div>
        <p className="mt-5 text-xs leading-5 text-slate-600">市场之间的数据、候选与分析状态始终分开。</p>
      </div>
    </main>
  );
}
