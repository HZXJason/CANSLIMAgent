import { useMemo, useState } from 'react';
import { CalendarRange, ExternalLink, Radio } from 'lucide-react';
import { Link } from 'react-router-dom';
import { cn } from '../../lib/utils';
import type { MonthCalendar, WeekCalendarEvent, WeekCalendarEventKind } from '../../types/today';

const kindLabels: Record<WeekCalendarEventKind, string> = { economic: '经济事件', earnings: '财报', dividend: '分红', plan_deadline: '计划到期' };
const kindDots: Record<WeekCalendarEventKind, string> = { economic: 'bg-amber-300', earnings: 'bg-sky-300', dividend: 'bg-emerald-300', plan_deadline: 'bg-violet-300' };
const weekDays = ['一', '二', '三', '四', '五', '六', '日'];

function eventLink(event: WeekCalendarEvent, children: React.ReactNode) {
  if (!event.href) return children;
  return event.href.startsWith('/') ? <Link to={event.href} className="hover:text-brand-200">{children}</Link> : <a href={event.href} className="hover:text-brand-200">{children}</a>;
}

function EventRow({ event, localDate }: { event: WeekCalendarEvent; localDate: string }) {
  const impact = event.market_impact === 'bullish' ? '利好股市' : event.market_impact === 'bearish' ? '利空股市' : event.market_impact === 'neutral' ? '影响中性' : null;
  let impactClassName = 'bg-slate-800 text-slate-200';
  if (event.market_impact === 'bullish') impactClassName = 'bg-emerald-400/10 text-emerald-100';
  if (event.market_impact === 'bearish') impactClassName = 'bg-rose-400/10 text-rose-100';
  return <article className="grid gap-3 py-4 sm:grid-cols-[5rem_minmax(0,1fr)_auto] sm:items-start">
    <div className="font-data text-sm text-slate-400">{event.time ?? '待定'}</div>
    <div className="min-w-0">
      <div className="flex flex-wrap items-center gap-2">
        <span className={cn('h-2 w-2 rounded-full', kindDots[event.kind])} aria-hidden="true" />
        <p className="font-bold text-slate-100">{eventLink(event, event.title)}</p>
        <span className="text-xs text-slate-500">{kindLabels[event.kind]}</span>
      </div>
      {event.result ? <p className="mt-2 text-sm font-semibold text-slate-200">{event.result}</p> : null}
      {impact ? <div className="mt-2 flex flex-wrap items-center gap-2 text-xs">
        <span className={cn('rounded-full px-2.5 py-1 font-bold', impactClassName)}>{impact}</span>
        {event.impact_reason ? <span className="text-slate-400">{event.impact_reason}</span> : null}
      </div> : null}
    </div>
    {event.date === localDate && event.live_url ? <a href={event.live_url} target="_blank" rel="noreferrer" className="inline-flex min-h-10 items-center gap-2 self-start rounded-sm border border-brand-300/35 px-3 text-sm font-bold text-brand-200 transition-colors hover:bg-brand-300/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-300/60"><Radio className="h-4 w-4" />观看直播<ExternalLink className="h-3.5 w-3.5" /></a> : null}
  </article>;
}

function dateRange(start: string, end: string) {
  const dates: string[] = [];
  const cursor = new Date(`${start}T00:00:00Z`);
  const last = new Date(`${end}T00:00:00Z`);
  while (cursor <= last) {
    dates.push(cursor.toISOString().slice(0, 10));
    cursor.setUTCDate(cursor.getUTCDate() + 1);
  }
  return dates;
}

export function MonthlyCalendar({ calendar, localDate }: { calendar: MonthCalendar; localDate: string }) {
  const dates = useMemo(() => dateRange(calendar.month_start, calendar.month_end), [calendar.month_end, calendar.month_start]);
  const initialDate = localDate >= calendar.month_start && localDate <= calendar.month_end ? localDate : calendar.events[0]?.date ?? calendar.month_start;
  const [selectedDate, setSelectedDate] = useState(initialDate);
  const selectedEvents = calendar.events.filter((event) => event.date === selectedDate);
  const startWeekDay = (new Date(`${calendar.month_start}T00:00:00Z`).getUTCDay() + 6) % 7;
  const [year, month] = calendar.month_start.split('-').map(Number);

  return <section aria-labelledby="month-calendar-title" aria-label="本月大事" className="mt-8 border-y border-rule py-5 sm:py-6">
    <div className="mb-5 flex items-end justify-between gap-4">
      <h2 id="month-calendar-title" className="font-display text-xl font-bold text-white sm:text-2xl">本月大事</h2>
      <p className="font-data text-sm text-slate-500">{year}年{month}月</p>
    </div>
    <div className="grid grid-cols-7 border-x border-t border-rule text-center text-xs font-bold text-slate-500" aria-hidden="true">
      {weekDays.map((day) => <span key={day} className="border-b border-r border-rule py-2 last:border-r-0">{day}</span>)}
    </div>
    <div className="grid grid-cols-7 border-l border-rule" role="tablist" aria-label="本月日期">
      {Array.from({ length: startWeekDay }).map((_, index) => <span key={`empty-${index}`} className="min-h-14 border-b border-r border-rule bg-slate-950/20 sm:min-h-20" />)}
      {dates.map((date) => {
        const events = calendar.events.filter((event) => event.date === date);
        const active = date === selectedDate;
        const today = date === localDate;
        return <button key={date} type="button" role="tab" aria-selected={active} aria-label={`${Number(date.slice(5, 7))}月${Number(date.slice(8))}日，${events.length} 项事件`} onClick={() => setSelectedDate(date)} className={cn('relative min-h-14 border-b border-r border-rule p-2 text-left transition-colors focus-visible:z-10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand-300/70 sm:min-h-20 sm:p-3', active ? 'bg-slate-800/80' : 'hover:bg-slate-900/45')}>
          <span className={cn('font-data text-sm', active || today ? 'font-bold text-white' : 'text-slate-400')}>{Number(date.slice(-2))}</span>
          <span className="mt-3 flex flex-wrap gap-1" aria-hidden="true">{Array.from(new Set(events.map((event) => event.kind))).map((kind) => <span key={kind} className={cn('h-1.5 w-1.5 rounded-full', kindDots[kind])} />)}</span>
          {today ? <span className="absolute inset-x-2 bottom-0 h-0.5 bg-brand-300" aria-hidden="true" /> : null}
        </button>;
      })}
    </div>
    <div className="mt-5" aria-live="polite">
      <p className="mb-2 text-sm font-bold text-white">{Number(selectedDate.slice(5, 7))}月{Number(selectedDate.slice(8))}日</p>
      {selectedEvents.length ? <div className="divide-y divide-rule border-y border-rule">{selectedEvents.map((event) => <EventRow key={event.id} event={event} localDate={localDate} />)}</div> : <div className="flex min-h-24 items-center gap-3 border-y border-rule py-5 text-sm text-slate-500"><CalendarRange className="h-5 w-5" />这一天暂无大事。</div>}
    </div>
  </section>;
}
