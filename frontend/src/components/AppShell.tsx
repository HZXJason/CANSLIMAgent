import { Suspense } from 'react';
import { BarChart3, BookOpenCheck, CandlestickChart, ClipboardList, Filter, Settings } from 'lucide-react';
import { NavLink, Outlet } from 'react-router-dom';
import { cn } from '../lib/utils';

const topItems = [
  { to: '/screener', label: '筛选器', icon: Filter },
  { to: '/analysis', label: '个股分析', icon: BarChart3 },
  { to: '/trade-plans', label: '交易计划', icon: ClipboardList },
  { to: '/journal', label: '交易日志', icon: BookOpenCheck }
];

const bottomItems = [{ to: '/settings', label: '设置', icon: Settings }];
const mobileItems = [...topItems, ...bottomItems];

function ShellNavItem({ to, label, icon: Icon }: { to: string; label: string; icon: typeof Filter }) {
  return (
    <NavLink
      to={to}
      aria-label={label}
      title={label}
      className={({ isActive }) =>
        cn(
          'flex h-10 items-center justify-center gap-3 rounded-xl border px-2.5 text-sm font-semibold transition-colors xl:justify-start xl:px-3',
          isActive
            ? 'border-cyan-400/25 bg-cyan-400/10 text-cyan-200'
            : 'border-transparent text-slate-400 hover:bg-slate-800/70 hover:text-white'
        )
      }
    >
      <Icon className="h-[18px] w-[18px] shrink-0" />
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
          'flex min-w-0 flex-col items-center justify-center gap-1 px-1 py-2 text-[11px] font-semibold transition-colors',
          isActive ? 'text-cyan-300' : 'text-slate-500'
        )
      }
    >
      <Icon className="h-5 w-5" />
      <span className="truncate">{label}</span>
    </NavLink>
  );
}

export default function AppShell() {
  return (
    <div className="flex min-h-screen bg-transparent text-slate-100">
      <aside className="sticky top-0 hidden h-screen w-[72px] shrink-0 flex-col border-r border-slate-800/80 bg-slate-950/65 px-3 py-4 backdrop-blur-xl md:flex xl:w-56 xl:px-4">
        <NavLink to="/screener" aria-label="AlphaLens" className="flex h-11 items-center justify-center gap-3 xl:justify-start">
          <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-cyan-400 text-slate-950">
            <CandlestickChart className="h-5 w-5" />
          </span>
          <span className="hidden text-base font-bold tracking-tight text-white xl:inline">AlphaLens</span>
        </NavLink>

        <nav aria-label="主导航" className="mt-6 space-y-1">
          {topItems.map((item) => (
            <ShellNavItem key={item.to} {...item} />
          ))}
        </nav>

        <nav aria-label="设置" className="mt-auto space-y-1">
          {bottomItems.map((item) => (
            <ShellNavItem key={item.to} {...item} />
          ))}
        </nav>
      </aside>

      <main className="min-w-0 flex-1 overflow-x-hidden pb-20 md:pb-0">
        <Suspense
          fallback={
            <div className="mx-auto flex min-h-[60vh] w-full max-w-[1440px] items-center justify-center px-4 text-sm font-semibold text-slate-400">
              加载中...
            </div>
          }
        >
          <Outlet />
        </Suspense>
      </main>

      <nav
        aria-label="移动端主导航"
        className="fixed inset-x-0 bottom-0 z-40 grid grid-cols-5 border-t border-slate-800 bg-slate-950/95 pb-[env(safe-area-inset-bottom)] backdrop-blur-xl md:hidden"
      >
        {mobileItems.map((item) => (
          <MobileNavItem key={item.to} {...item} />
        ))}
      </nav>
    </div>
  );
}
