export type TodaySession = 'premarket' | 'intraday' | 'postmarket' | 'non_trading';

export interface TodayItem {
  id: string;
  kind: 'candidate' | 'waiting_plan' | 'unreviewed_plan' | 'off_plan_execution' | 'pending_review';
  title: string;
  subtitle: string;
  href: string;
  priority: number;
}

export type WeekCalendarStatus = 'live' | 'partial' | 'local_only';
export type WeekCalendarEventKind = 'economic' | 'earnings' | 'dividend' | 'plan_deadline';

export interface WeekCalendarEvent {
  id: string;
  date: string;
  time: string | null;
  kind: WeekCalendarEventKind;
  title: string;
  subtitle: string;
  symbol: string | null;
  importance: 'high' | 'medium' | 'low';
  source: string;
  href: string | null;
}

export interface WeekCalendar {
  week_start: string;
  week_end: string;
  status: WeekCalendarStatus;
  sources: Array<{ name: string; url: string }>;
  fetched_at: string | null;
  events: WeekCalendarEvent[];
}

export interface TodayBrief {
  market: 'US' | 'CN';
  timezone: string;
  local_date: string;
  session: TodaySession;
  calendar_status: 'weekday_only_unverified';
  data_as_of: string | null;
  week_calendar: WeekCalendar;
  items: TodayItem[];
}
