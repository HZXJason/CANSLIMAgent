import { useMemo, useState } from 'react';
import { ArrowUpRight, CalendarRange, Clock3 } from 'lucide-react';
import { Link } from 'react-router-dom';
import { cn } from '../../lib/utils';
import type { WeekCalendar, WeekCalendarEvent, WeekCalendarEventKind } from '../../types/today';

const weekdays = ['周一', '周二', '周三', '周四', '周五'];
const kindLabels: Record<WeekCalendarEventKind, string> = { economic: '经济事件', earnings: '财报', dividend: '分红', plan_deadline: '计划到期' };
const kindDots: Record<WeekCalendarEventKind, string> = { economic: 'bg-amber-300', earnings: 'bg-sky-300', dividend: 'bg-emerald-300', plan_deadline: 'bg-violet-300' };
const kindText: Record<WeekCalendarEventKind, string> = { economic: 'text-amber-200', earnings: 'text-sky-200', dividend: 'text-emerald-200', plan_deadline: 'text-violet-200' };

function addDays(date: string, days: number) {
  const value = new Date(`${date}T12:00:00Z`);
  value.setUTCDate(value.getUTCDate() + days);
  return value.toISOString().slice(0, 10);
}

function monthDay(date: string) {
  const [, month, day] = date.split('-').map(Number);
  return `${month}月${day}日`;
}

function EventRow({ event }: { event: WeekCalendarEvent }) {
  const content = <>
    <div className="flex min-w-0 items-start gap-3 sm:gap-4">
      <span className={cn('mt-1.5 h-2.5 w-2.5 shrink-0 rounded-full', kindDots[event.kind])} aria-hidden="true" />
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span className={cn('text-xs font-bold', kindText[event.kind])}>{kindLabels[event.kind]}</span>
          {event.time ? <span className="font-data flex items-center gap-1 text-xs text-slate-500"><Clock3 className="h-3 w-3" aria-hidden="true" />{event.time}</span> : null}
          {event.importance === 'high' ? <span className="text-[11px] font-bold text-amber-200">高影响</span> : null}
        </div>
        <p className="mt-1.5 text-sm font-bold text-white sm:text-base">{event.title}</p>
        <p className="mt-1 text-xs leading-5 text-slate-500">{event.subtitle} · {event.source}</p>
      </div>
    </div>
    {event.href ? <ArrowUpRight className="h-4 w-4 shrink-0 text-slate-600 transition group-hover:text-slate-200" aria-hidden="true" /> : null}
  </>;
  return event.href
    ? <Link to={event.href} className="group flex min-h-20 items-start justify-between gap-4 py-4 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-300/70">{content}</Link>
    : <article className="flex min-h-20 items-start justify-between gap-4 py-4">{content}</article>;
}

export function WeeklyCalendar({ calendar, localDate }: { calendar: WeekCalendar; localDate: string }) {
  const days = useMemo(() => weekdays.map((label, index) => ({ label, date: addDays(calendar.week_start, index) })), [calendar.week_start]);
  const initialDay = localDate < calendar.week_start
    ? calendar.week_start
    : localDate > calendar.week_end
      ? calendar.week_end
      : localDate;
  const [selectedDate, setSelectedDate] = useState(initialDay);
  const selectedEvents = calendar.events.filter(event => event.date === selectedDate);
  const selectedDay = days.find(day => day.date === selectedDate) ?? days[0];
  const eventKinds = (date: string) => Array.from(new Set(calendar.events.filter(event => event.date === date).map(event => event.kind))).slice(0, 4);
  const statusMessage = calendar.status === 'partial'
    ? '部分外部日历读取失败，已显示其余可用事件。'
    : calendar.status === 'local_only'
      ? '外部事件日历当前不可用，当前仅显示本地交易计划。'
      : null;

  return <section aria-labelledby="week-calendar-title" className="mt-8">
    <div className="flex flex-col gap-3 border-b border-rule-strong pb-4 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <h2 id="week-calendar-title" className="font-display text-xl font-bold text-white sm:text-2xl">本周大事</h2>
        <p className="mt-1.5 text-xs text-slate-500">{monthDay(calendar.week_start)}—{monthDay(calendar.week_end)} · 只显示当前市场与研究范围内的事件</p>
      </div>
      {calendar.sources.length ? <div className="flex flex-wrap gap-x-3 gap-y-1">
        {calendar.sources.map(source => <a key={source.name} href={source.url} target="_blank" rel="noreferrer" className="w-fit text-xs font-bold text-slate-400 underline decoration-slate-700 underline-offset-4 transition hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-300/70">
          {source.name}
        </a>)}
      </div> : null}
    </div>

    <div className="grid grid-cols-5 border-b border-rule" role="tablist" aria-label="本周交易日">
      {days.map(day => {
        const events = calendar.events.filter(event => event.date === day.date);
        const active = day.date === selectedDate;
        const today = day.date === localDate;
        return <button
          key={day.date}
          type="button"
          role="tab"
          aria-selected={active}
          aria-label={`${day.label} ${monthDay(day.date)}，${events.length} 项事件`}
          onClick={() => setSelectedDate(day.date)}
          className={cn('relative min-h-28 border-r border-rule px-1 py-3 text-center transition-colors last:border-r-0 focus-visible:z-10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand-300/70 sm:min-h-32 sm:px-3', active ? 'bg-slate-900/70' : 'hover:bg-slate-900/35')}
        >
          <span className={cn('text-xs font-bold', active ? 'text-white' : 'text-slate-500')}>{day.label}</span>
          <span className={cn('font-data mt-3 block text-2xl font-semibold sm:text-3xl', active ? 'text-white' : 'text-slate-300')}>{Number(day.date.slice(-2))}</span>
          <span className="mt-3 flex min-h-4 flex-wrap items-center justify-center gap-1.5" aria-hidden="true">
            {eventKinds(day.date).map(kind => <span key={kind} className={cn('h-2 w-2 rounded-full', kindDots[kind])} />)}
          </span>
          {today ? <span className="absolute inset-x-3 bottom-0 h-0.5 bg-brand-300" aria-hidden="true" /> : null}
        </button>;
      })}
    </div>

    <div className="flex flex-wrap gap-x-5 gap-y-2 border-b border-rule py-3 text-xs text-slate-400" aria-label="事件类型图例">
      {(Object.keys(kindLabels) as WeekCalendarEventKind[]).map(kind => <span key={kind} className="flex items-center gap-2"><span className={cn('h-2 w-2 rounded-full', kindDots[kind])} aria-hidden="true" />{kindLabels[kind]}</span>)}
    </div>

    {statusMessage ? <div className="border-b border-rule py-3 text-xs leading-5 text-amber-200">{statusMessage}</div> : null}
    <div aria-live="polite">
      <div className="flex items-center justify-between gap-4 py-4">
        <p className="text-sm font-bold text-white">{selectedDay.label} · {monthDay(selectedDate)}</p>
        <p className="font-data text-xs text-slate-500">{selectedEvents.length} EVENTS</p>
      </div>
      {selectedEvents.length
        ? <div className="divide-y divide-rule border-y border-rule">{selectedEvents.map(event => <EventRow key={event.id} event={event} />)}</div>
        : <div className="flex min-h-24 items-center gap-3 border-y border-rule py-5 text-sm text-slate-500"><CalendarRange className="h-5 w-5 shrink-0" aria-hidden="true" />这一天暂无与你当前研究相关的大事。</div>}
    </div>
  </section>;
}
