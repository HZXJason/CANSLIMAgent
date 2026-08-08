import { useQuery } from '@tanstack/react-query';
import { ArrowRight, CalendarClock, CircleAlert } from 'lucide-react';
import { Link } from 'react-router-dom';
import { PageHeader } from '../../components/PageHeader';
import { Card } from '../../components/ui/card';
import { fetchTodayBrief } from '../../lib/todayApi';
import { useMarketStore } from '../../stores/marketStore';

const sessionLabels = { premarket: '盘前准备', intraday: '盘中检查', postmarket: '盘后复盘', non_trading: '非交易时段' } as const;
const kindLabels = { candidate: '研究候选', waiting_plan: '等待中的计划', unreviewed_plan: '计划待审查', off_plan_execution: '计划外成交', pending_review: '成交待复盘' } as const;

export default function TodayPage() {
  const market = useMarketStore((state) => state.assetMarket);
  const query = useQuery({ queryKey: ['today', market], queryFn: () => fetchTodayBrief(market), refetchInterval: 60_000 });
  const brief = query.data;
  const nextItem = brief?.items[0];
  const laterItems = brief?.items.slice(1) ?? [];
  return (
    <div className="mx-auto w-full max-w-[1240px] px-4 py-6 sm:px-6 lg:py-9">
      <PageHeader title="今日" meta={brief ? <><span>{sessionLabels[brief.session]}</span><span>·</span><span>{brief.market} {brief.local_date}</span>{brief.data_as_of ? <><span>·</span><span>数据截至 {brief.data_as_of.slice(0, 10)}</span></> : null}</> : null} />
      {query.isError ? <Card className="mt-6 border-rose-500/30 p-5 text-sm text-rose-200">今日信息读取失败，请稍后重试。</Card> : null}
      {query.isLoading ? <Card className="mt-6 h-40 animate-pulse bg-slate-900/60" /> : null}
      {brief ? <>
        <div className="mt-5 flex flex-wrap items-center gap-x-4 gap-y-2 border-y border-rule py-3 text-xs text-slate-500">
          <span title={brief.timezone}>{brief.timezone}</span>
          <span className="flex items-center gap-1 text-amber-200"><CircleAlert className="h-3.5 w-3.5" />节假日状态待确认</span>
        </div>
        {nextItem ? <section aria-labelledby="next-action-title" className="mt-8">
          <h2 id="next-action-title" className="font-display text-xl font-bold text-white">下一步</h2>
          <Link to={nextItem.href} className="group mt-3 flex min-h-36 items-center justify-between gap-5 border-y border-rule-strong py-6 transition hover:border-brand-300/60">
            <div><p className="text-sm font-bold text-brand-200">{kindLabels[nextItem.kind]}</p><p className="mt-2 font-display text-2xl font-bold text-white sm:text-3xl">{nextItem.title}</p>{nextItem.subtitle ? <p className="mt-2 text-sm text-slate-400">{nextItem.subtitle}</p> : null}</div>
            <span className="flex h-11 w-11 shrink-0 items-center justify-center border border-rule text-slate-400 transition group-hover:border-brand-300 group-hover:text-white"><ArrowRight className="h-5 w-5 transition group-hover:translate-x-0.5" /></span>
          </Link>
        </section> : <section aria-labelledby="next-action-title" className="mt-8"><h2 id="next-action-title" className="font-display text-xl font-bold text-white">下一步</h2><div className="mt-3 border-y border-rule py-8"><CalendarClock className="h-6 w-6 text-slate-500" /><p className="mt-3 font-bold text-white">当前没有待办</p><div className="mt-4 flex gap-5"><Link className="text-sm font-bold text-brand-200" to="/screener">发现候选</Link><Link className="text-sm font-bold text-brand-200" to="/journal/new">记录成交</Link></div></div></section>}
        {laterItems.length ? <section aria-labelledby="later-actions-title" className="mt-10">
          <h2 id="later-actions-title" className="font-display text-lg font-bold text-white">稍后处理</h2>
          <div className="mt-3 divide-y divide-rule border-y border-rule">{laterItems.map(item => <Link to={item.href} key={`${item.kind}-${item.id}`} className="group flex min-h-20 items-center justify-between gap-4 py-4"><div className="min-w-0"><p className="truncate font-bold text-white">{item.title}</p><p className="mt-1 text-xs text-slate-500">{kindLabels[item.kind]}{item.subtitle ? ` · ${item.subtitle}` : ''}</p></div><ArrowRight className="h-4 w-4 shrink-0 text-slate-600 transition group-hover:translate-x-0.5 group-hover:text-slate-300" /></Link>)}</div>
        </section> : null}
      </> : null}
    </div>
  );
}
