import { Suspense, useLayoutEffect } from 'react';
import { BarChart3, BookOpenCheck, CandlestickChart, ClipboardList, Filter, Settings } from 'lucide-react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import GlobalMarketSelector from './GlobalMarketSelector';
import { cn } from '../lib/utils';
import { useMarketStore } from '../stores/marketStore';

const topItems = [
  { to: '/screener', label: '筛选器', icon: Filter },
  { to: '/analysis', label: '个股分析', icon: BarChart3 },
  { to: '/trade-plans', label: '交易计划', icon: ClipboardList },
  { to: '/journal', label: '交易日志', icon: BookOpenCheck }
];

const bottomItems = [{ to: '/settings', label: '设置', icon: Settings }];
function ShellNavItem({ to, label, icon: Icon }: { to: string; label: string; icon: typeof Filter }) {
  return (
    <NavLink
      to={to}
      aria-label={label}
      title={label}
      className={({ isActive }) =>
        cn(
          'interactive-press group relative flex min-h-11 items-center justify-center gap-3 rounded-xl border px-2.5 text-sm font-semibold xl:justify-start xl:px-3.5',
          isActive
            ? 'border-brand-300/20 bg-brand-300/[0.09] text-brand-100 shadow-[inset_3px_0_0_var(--accent)]'
            : 'border-transparent text-slate-400 hover:border-slate-800 hover:bg-slate-800/55 hover:text-slate-100'
        )
      }
    >
      <Icon className="h-[19px] w-[19px] shrink-0 transition-colors group-hover:text-brand-200" strokeWidth={1.8} />
      <span className="hidden xl:inline">{label}</span>
    </NavLink>
  );
}

function MobileNavItem({ to, label, icon: Icon }: { to: string; label: string; icon: typeof Filter }) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        cn(
          'interactive-press relative flex min-h-[64px] min-w-0 flex-col items-center justify-center gap-1.5 rounded-xl px-1 py-2 text-xs font-semibold',
          isActive ? 'text-brand-200' : 'text-slate-400'
        )
      }
    >
      {({ isActive }) => (
        <>
          <span className={cn('absolute top-1.5 h-1 w-4 rounded-full bg-brand-300 transition-opacity', isActive ? 'opacity-100' : 'opacity-0')} />
          <Icon className="h-5 w-5" strokeWidth={isActive ? 2.1 : 1.75} />
          <span className="truncate">{label}</span>
        </>
      )}
    </NavLink>
  );
}

export default function AppShell() {
  const location = useLocation();
  const assetMarket = useMarketStore((state) => state.assetMarket);
  const currentItem = [...topItems, ...bottomItems].find((item) => location.pathname.startsWith(item.to)) ?? topItems[0];

  useLayoutEffect(() => {
    document.documentElement.dataset.market = assetMarket;
  }, [assetMarket]);

  return (
    <div className="flex min-h-screen bg-transparent text-slate-100">
      <a href="#main-content" className="fixed left-3 top-3 z-[100] -translate-y-20 rounded-xl bg-brand-300 px-4 py-2 text-sm font-bold text-slate-950 transition-transform focus:translate-y-0">
        跳到主要内容
      </a>

      <aside className="glass-chrome sticky top-0 hidden h-screen w-[76px] shrink-0 flex-col border-r border-white/[0.07] px-3 py-4 md:flex xl:w-[240px] xl:px-4">
        <div>
          <div className="flex min-h-12 items-center justify-center gap-3 xl:justify-start xl:px-1">
            <NavLink to="/screener" aria-label="AlphaLens 首页" className="interactive-press flex shrink-0 rounded-[13px]">
              <span className="flex h-10 w-10 items-center justify-center rounded-[13px] bg-gradient-to-br from-brand-200 to-brand-400 text-slate-950 shadow-glow">
                <CandlestickChart className="h-5 w-5" strokeWidth={2.2} />
              </span>
            </NavLink>
            <div className="hidden min-w-0 xl:block">
              <NavLink to="/screener" className="interactive-press block rounded-md text-[15px] font-extrabold tracking-[-0.02em] text-white">
                AlphaLens
              </NavLink>
              <GlobalMarketSelector className="-mt-0.5" />
            </div>
          </div>
          <GlobalMarketSelector compact className="mt-1 flex justify-center xl:hidden" />
        </div>

        <nav aria-label="主导航" className="mt-5 space-y-1.5 xl:mt-7">
          {topItems.map((item) => (
            <ShellNavItem key={item.to} {...item} />
          ))}
        </nav>

        <div className="mt-auto">
          <div className="mx-2 mb-3 hidden border-t border-white/[0.06] pt-4 text-[11px] leading-5 text-slate-600 xl:block">
            研究先于决策<br />风险始终可见
          </div>
          <nav aria-label="设置" className="space-y-1">
          {bottomItems.map((item) => (
            <ShellNavItem key={item.to} {...item} />
          ))}
          </nav>
        </div>
      </aside>

      <div className="min-w-0 flex-1">
        <header className="glass-chrome sticky top-0 z-40 flex min-h-16 items-center justify-between border-b border-white/[0.07] px-4 md:hidden">
          <div className="flex min-w-0 items-center gap-2.5">
            <NavLink to="/screener" className="interactive-press flex min-w-0 items-center gap-2.5 rounded-xl" aria-label="AlphaLens 首页">
              <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-brand-200 to-brand-400 text-slate-950 shadow-glow">
                <CandlestickChart className="h-[18px] w-[18px]" strokeWidth={2.2} />
              </span>
              <span className="min-w-0">
                <span className="block truncate text-sm font-extrabold tracking-tight text-white">AlphaLens</span>
                <span className="block truncate text-[11px] font-medium text-slate-500">{currentItem.label}</span>
              </span>
            </NavLink>
            <GlobalMarketSelector compact className="shrink-0" />
          </div>
          <NavLink
            to="/settings"
            aria-label="设置"
            className={({ isActive }) => cn('interactive-press flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border', isActive ? 'border-brand-300/30 bg-brand-300/10 text-brand-200' : 'border-slate-800 bg-slate-900/70 text-slate-400')}
          >
            <Settings className="h-5 w-5" />
          </NavLink>
        </header>

        <main
          id="main-content"
          className="min-w-0 pb-[calc(var(--mobile-nav-height)+env(safe-area-inset-bottom)+1rem)] md:pb-0"
        >
          <Suspense
            fallback={
              <div className="mx-auto flex min-h-[60vh] w-full max-w-[1440px] items-center justify-center px-4 text-sm font-semibold text-slate-400" role="status">
                <span className="mr-3 h-2 w-2 rounded-full bg-brand-300 shadow-glow" />
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
        className="glass-chrome fixed inset-x-0 bottom-0 z-40 grid grid-cols-4 border-t border-white/[0.08] px-[max(0.5rem,env(safe-area-inset-left))] pb-[env(safe-area-inset-bottom)] pr-[max(0.5rem,env(safe-area-inset-right))] md:hidden"
      >
        {topItems.map((item) => (
          <MobileNavItem key={item.to} {...item} />
        ))}
      </nav>
    </div>
  );
}
