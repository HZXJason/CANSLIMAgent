import { Suspense, useLayoutEffect } from 'react';
import { BarChart3, ClipboardList, ShieldCheck, SlidersHorizontal } from 'lucide-react';
import { NavLink, Outlet } from 'react-router-dom';
import { cn } from '../lib/utils';
import { useMarketStore } from '../stores/marketStore';
import { MarketOnboarding } from './MarketOnboarding';

const navItems = [
  { to: '/desk', label: '作战室', icon: ShieldCheck },
  { to: '/research', label: '研究台', icon: BarChart3 },
  { to: '/positions', label: '头寸簿', icon: ClipboardList },
  { to: '/system', label: '交易系统', icon: SlidersHorizontal }
];

function MarketSwitchPill({ compact = false }: { compact?: boolean }) {
  const assetMarket = useMarketStore((state) => state.assetMarket);
  const setAssetMarket = useMarketStore((state) => state.setAssetMarket);

  return (
    <div
      role="group"
      aria-label="切换研究市场"
      className={cn(
        'grid grid-cols-2 rounded-sm border border-white/[0.1] bg-slate-950/70 p-0.5',
        compact ? 'w-[104px]' : 'w-full'
      )}
    >
      {(['US', 'CN'] as const).map((market) => {
        const active = assetMarket === market;
        return (
          <button
            key={market}
            type="button"
            onClick={() => setAssetMarket(market)}
            aria-pressed={active}
            className={cn(
              'min-h-8 rounded-sm px-2 text-xs font-bold transition-colors',
              active ? 'bg-accent text-paper-ink' : 'text-slate-400 hover:text-slate-100'
            )}
          >
            {market === 'US' ? (compact ? 'US' : '美股 US') : compact ? 'CN' : 'A股 CN'}
          </button>
        );
      })}
    </div>
  );
}

function DesktopNavItem({ to, label, icon: Icon }: { to: string; label: string; icon: typeof ShieldCheck }) {
  return (
    <NavLink
      to={to}
      aria-label={label}
      title={label}
      className={({ isActive }) =>
        cn(
          'group relative flex min-h-11 items-center gap-3 border-b-2 px-1 py-2 transition-colors duration-150',
          isActive ? 'border-accent text-ink' : 'border-transparent text-slate-400 hover:border-slate-700 hover:text-slate-200'
        )
      }
    >
      <Icon className="h-[18px] w-[18px]" strokeWidth={1.8} aria-hidden="true" />
      <span className="text-[15px] font-bold leading-tight">{label}</span>
    </NavLink>
  );
}

function RailNavItem({ to, label, icon: Icon }: { to: string; label: string; icon: typeof ShieldCheck }) {
  return (
    <NavLink
      to={to}
      aria-label={label}
      title={label}
      className={({ isActive }) =>
        cn(
          'interactive-press relative flex h-12 w-12 items-center justify-center rounded-sm border transition-colors',
          isActive
            ? 'border-accent/40 bg-accent/10 text-ink'
            : 'border-transparent text-slate-400 hover:border-slate-700 hover:bg-slate-900/70 hover:text-slate-100'
        )
      }
    >
      <Icon className="h-[19px] w-[19px]" strokeWidth={1.8} aria-hidden="true" />
    </NavLink>
  );
}

function MobileNavItem({ to, label, icon: Icon }: { to: string; label: string; icon: typeof ShieldCheck }) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        cn(
          'relative flex min-h-[64px] min-w-0 flex-col items-center justify-center gap-1.5 px-1 py-2',
          isActive ? 'text-ink' : 'text-slate-400'
        )
      }
    >
      {({ isActive }) => (
        <>
          <span className={cn('absolute top-0 left-1/2 h-0.5 w-8 -translate-x-1/2 bg-accent transition-opacity', isActive ? 'opacity-100' : 'opacity-0')} />
          <Icon className="h-[18px] w-[18px]" strokeWidth={isActive ? 2 : 1.7} aria-hidden="true" />
          <span className="text-xs font-bold leading-none">{label}</span>
        </>
      )}
    </NavLink>
  );
}

export default function AppShell() {
  const assetMarket = useMarketStore((state) => state.assetMarket);
  const hasSelectedMarket = useMarketStore((state) => state.hasSelectedMarket);

  useLayoutEffect(() => {
    document.documentElement.dataset.market = assetMarket;
  }, [assetMarket]);

  if (!hasSelectedMarket) return <MarketOnboarding />;

  return (
    <div className="flex min-h-screen bg-transparent text-slate-100">
      <a href="#main-content" className="fixed left-3 top-3 z-[100] -translate-y-20 rounded-sm bg-accent px-4 py-2 text-sm font-bold text-paper-ink transition-transform focus:translate-y-0">
        跳到主要内容
      </a>

      <aside className="glass-chrome sticky top-0 hidden h-screen w-[76px] shrink-0 flex-col border-r border-white/[0.07] px-3 py-4 md:flex xl:w-[248px] xl:px-4">
        <div className="border-b border-white/[0.08] pb-3">
          <div className="flex min-h-12 items-center justify-center gap-3 xl:justify-start xl:px-0.5">
            <NavLink
              to="/desk"
              aria-label="AlphaLens 首页"
              className="interactive-press flex flex-col gap-1 rounded-sm xl:items-start xl:gap-1.5"
            >
              <span className="font-display hidden text-[22px] leading-none text-white xl:block">
                AlphaLens
              </span>
              <span className="font-display text-lg leading-none text-white xl:hidden">AL</span>
            </NavLink>
          </div>
          <div className="mt-3 hidden xl:block">
            <MarketSwitchPill />
          </div>
        </div>

        <nav aria-label="主导航" className="mt-5 space-y-2 xl:mt-6">
          {navItems.map((item) => (
            <div key={item.to} className="hidden xl:block">
              <DesktopNavItem to={item.to} label={item.label} icon={item.icon} />
            </div>
          ))}
          <div data-testid="rail-navigation" className="hidden flex-col items-center gap-2 md:flex xl:hidden">
            {navItems.map((item) => (
              <RailNavItem key={item.to} to={item.to} label={item.label} icon={item.icon} />
            ))}
          </div>
        </nav>

        <div className="mt-auto border-t border-white/[0.06] pt-3 text-center xl:text-left">
          <p className="hidden text-[11px] leading-relaxed text-slate-500 xl:block">
            1R 风控 · 证据先于结论
          </p>
        </div>
      </aside>

      <div className="min-w-0 flex-1">
        <header className="glass-chrome sticky top-0 z-40 flex min-h-14 items-center justify-between border-b border-white/[0.07] px-4 md:hidden">
          <div className="flex min-w-0 items-center gap-2.5">
            <NavLink to="/desk" className="interactive-press flex min-w-0 items-center gap-2.5 rounded-sm" aria-label="AlphaLens 首页">
              <span className="font-display text-lg leading-none text-white">AlphaLens</span>
            </NavLink>
          </div>
          <MarketSwitchPill compact />
        </header>

        <main
          id="main-content"
          className="min-w-0 pb-[calc(var(--mobile-nav-height)+env(safe-area-inset-bottom)+1rem)] md:pb-0"
        >
          <Suspense
            fallback={
              <div className="mx-auto flex min-h-[60vh] w-full max-w-[1440px] items-center justify-center px-4 text-sm font-semibold text-slate-400" role="status">
                <span className="mr-3 h-2 w-2 rounded-full bg-accent shadow-glow" />
                正在载入工作区
              </div>
            }
          >
            <Outlet />
          </Suspense>
        </main>
      </div>

      <nav
        aria-label="移动端主导航"
        className="glass-chrome fixed inset-x-0 bottom-0 z-40 grid grid-cols-4 border-t border-white/[0.08] px-1 pb-[env(safe-area-inset-bottom)] md:hidden"
      >
        {navItems.map((item) => (
          <MobileNavItem key={item.to} to={item.to} label={item.label} icon={item.icon} />
        ))}
      </nav>
    </div>
  );
}
