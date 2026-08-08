export type TodaySession = 'premarket' | 'intraday' | 'postmarket' | 'non_trading';

export interface TodayItem {
  id: string;
  kind: 'candidate' | 'waiting_plan' | 'unreviewed_plan' | 'off_plan_execution' | 'pending_review';
  title: string;
  subtitle: string;
  href: string;
  priority: number;
}

export interface TodayBrief {
  market: 'US' | 'CN';
  timezone: string;
  local_date: string;
  session: TodaySession;
  calendar_status: 'weekday_only_unverified';
  data_as_of: string | null;
  items: TodayItem[];
}
