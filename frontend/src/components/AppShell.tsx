import { Suspense } from 'react';
import { BarChart3, BookOpenCheck, ClipboardList, Filter, Settings } from 'lucide-react';
import { NavLink, Outlet } from 'react-router-dom';
import { cn } from '../lib/utils';

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
      className={({ isActive }) =>
        cn(
          'flex h-11 items-center gap-3 rounded-lg border px-3 text-sm font-semibold transition',
          isActive
            ? 'border-cyan-400/50 bg-finance-blue text-white shadow-glow'
            : 'border-transparent text-slate-400 hover:border-slate-700 hover:bg-slate-900/80 hover:text-white'
        )
      }
    >
      <Icon className="h-4 w-4 shrink-0" />
      <span className="hidden lg:inline">{label}</span>
    </NavLink>
  );
}

export default function AppShell() {
  return (
    <div className="flex min-h-screen bg-transparent text-slate-100">
      <aside className="sticky top-0 flex h-screen w-20 shrink-0 flex-col border-r border-slate-800/90 bg-slate-950/70 px-3 py-5 backdrop-blur lg:w-64 lg:px-5">
        <div className="flex items-center justify-center gap-3 lg:justify-start">
          <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg bg-cyan-400 text-lg font-black text-slate-950 shadow-glow">
            A
          </div>
          <div className="hidden lg:block">
            <p className="text-lg font-bold text-white">AlphaLens</p>
            <p className="text-xs text-slate-500">CANSLIM Research OS</p>
          </div>
        </div>

        <nav className="mt-8 space-y-2">
          {topItems.map((item) => (
            <ShellNavItem key={item.to} {...item} />
          ))}
        </nav>

        <nav className="mt-auto space-y-2">
          {bottomItems.map((item) => (
            <ShellNavItem key={item.to} {...item} />
          ))}
        </nav>
      </aside>

      <main className="min-w-0 flex-1 overflow-x-hidden">
        <Suspense
          fallback={
            <div className="mx-auto flex min-h-screen w-full max-w-[1440px] items-center justify-center px-4 text-sm font-semibold text-slate-400">
              加载中...
            </div>
          }
        >
          <Outlet />
        </Suspense>
      </main>
    </div>
  );
}
