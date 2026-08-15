import { useEffect, useState } from 'react';
import type { TodaySession } from '../../types/today';

type MarketStatus = '休市' | '盘前' | '盘中' | '盘后' | '夜盘';

const sessionLabels: Partial<Record<TodaySession, MarketStatus>> = {
  premarket: '盘前',
  intraday: '盘中',
  postmarket: '盘后'
};

const statusStyles: Record<MarketStatus, string> = {
  休市: 'text-slate-300',
  盘前: 'text-sky-200',
  盘中: 'text-emerald-300',
  盘后: 'text-amber-200',
  夜盘: 'text-violet-200'
};

function dateTimeParts(date: Date, timeZone: string) {
  const parts = new Intl.DateTimeFormat('zh-CN', {
    timeZone,
    month: 'numeric',
    day: 'numeric',
    weekday: 'long',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23'
  }).formatToParts(date);
  return Object.fromEntries(parts.map(part => [part.type, part.value]));
}

export function formatMarketDateTime(date: Date, timeZone: string) {
  const parts = dateTimeParts(date, timeZone);
  return `${parts.month}月${parts.day}日 ${parts.weekday} ${parts.hour}:${parts.minute}:${parts.second}`;
}

export function resolveMarketStatus(date: Date, timeZone: string, session: TodaySession): MarketStatus {
  const sessionStatus = sessionLabels[session];
  if (sessionStatus) return sessionStatus;

  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    weekday: 'short',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23'
  }).formatToParts(date);
  const values = Object.fromEntries(parts.map(part => [part.type, part.value]));
  const weekday = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'].indexOf(values.weekday);
  const minutes = Number(values.hour) * 60 + Number(values.minute);
  const nightTrading = (weekday === 0 && minutes >= 20 * 60)
    || (weekday >= 1 && weekday <= 4 && (minutes < 4 * 60 || minutes >= 20 * 60))
    || (weekday === 5 && minutes < 4 * 60);
  return nightTrading ? '夜盘' : '休市';
}

export function MarketClock({ timeZone, session }: { timeZone: string; session: TodaySession }) {
  const [now, setNow] = useState(() => new Date());
  const status = resolveMarketStatus(now, timeZone, session);

  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 1_000);
    return () => window.clearInterval(timer);
  }, []);

  return <div className="flex w-full items-stretch text-[11px] sm:text-xs">
    <div className="min-w-0 flex-1 space-y-0.5">
      <div className="grid min-h-7 grid-cols-[5.75rem_minmax(0,1fr)] items-center gap-2 px-2.5 sm:grid-cols-[6.75rem_minmax(0,1fr)] sm:gap-3 sm:px-3">
        <span className="font-sans font-semibold text-ink-secondary">美国东部时间</span>
        <time className="whitespace-nowrap font-data tabular-nums text-ink" dateTime={now.toISOString()}>{formatMarketDateTime(now, timeZone)}</time>
      </div>
      <div className="grid min-h-7 grid-cols-[5.75rem_minmax(0,1fr)] items-center gap-2 px-2.5 sm:grid-cols-[6.75rem_minmax(0,1fr)] sm:gap-3 sm:px-3">
        <span className="font-sans font-semibold text-ink-secondary">北京时间</span>
        <time className="whitespace-nowrap font-data tabular-nums text-ink" dateTime={now.toISOString()}>{formatMarketDateTime(now, 'Asia/Shanghai')}</time>
      </div>
    </div>
    <div
      role="status"
      aria-label="美股时段状态"
      aria-live="polite"
      className="ml-auto flex shrink-0 items-center justify-end pl-4 pr-2.5 sm:pl-6 sm:pr-3"
    >
      <span className={`font-sans text-[22px] font-bold tracking-wide sm:text-2xl ${statusStyles[status]}`}>{status}</span>
    </div>
  </div>;
}
