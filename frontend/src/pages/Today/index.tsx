import { useQuery } from '@tanstack/react-query';
import { PageHeader } from '../../components/PageHeader';
import { Card } from '../../components/ui/card';
import { fetchTodayBrief } from '../../lib/todayApi';
import { useMarketStore } from '../../stores/marketStore';
import { MarketClock } from './MarketClock';
import { WeeklyCalendar } from './WeeklyCalendar';

export default function TodayPage() {
  const market = useMarketStore((state) => state.assetMarket);
  const query = useQuery({ queryKey: ['today', market], queryFn: () => fetchTodayBrief(market), refetchInterval: 60_000 });
  const brief = query.data;
  return (
    <div className="mx-auto w-full max-w-[1240px] px-4 py-6 sm:px-6 lg:py-9">
      <PageHeader title="今日" contentClassName="w-full max-w-none" meta={brief ? <MarketClock timeZone={brief.timezone} session={brief.session} /> : null} />
      {query.isError ? <Card className="mt-6 border-rose-500/30 p-5 text-sm text-rose-200">今日信息读取失败，请稍后重试。</Card> : null}
      {query.isLoading ? <Card className="mt-6 h-40 animate-pulse bg-slate-900/60" /> : null}
      {brief ? <>
        <WeeklyCalendar key={`${brief.market}-${brief.week_calendar.week_start}`} calendar={brief.week_calendar} localDate={brief.local_date} />
      </> : null}
    </div>
  );
}
