import { AlertCircle, Building2, ExternalLink } from 'lucide-react';
import { Card } from '../../components/ui/card';
import { Skeleton } from '../../components/ui/skeleton';
import type { ChartStatus, CompanyProfile } from '../../types/analysis';

interface CompanyProfileCardProps {
  data: CompanyProfile | null;
  status: ChartStatus;
}

function metaItems(data: CompanyProfile) {
  return [data.sector, data.industry].filter(Boolean);
}

export default function CompanyProfileCard({ data, status }: CompanyProfileCardProps) {
  const items = data ? metaItems(data) : [];
  const finalStatus = status === 'success' && !data?.description ? 'insufficient' : status;

  return (
    <Card className="space-y-4 rounded-[1rem] p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-cyan-400/25 bg-cyan-400/10 text-cyan-200">
            <Building2 className="h-5 w-5" />
          </span>
          <div>
            <h2 className="text-base font-bold text-white">公司简介</h2>
            <p className="mt-1 text-xs text-slate-500">{data?.name || data?.symbol || '基础业务概况'}</p>
          </div>
        </div>
        {data?.website ? (
          <a
            className="inline-flex items-center gap-1 text-xs font-semibold text-cyan-200 transition hover:text-cyan-100"
            href={data.website}
            target="_blank"
            rel="noreferrer"
          >
            官网
            <ExternalLink className="h-3.5 w-3.5" />
          </a>
        ) : null}
      </div>

      {finalStatus === 'loading' ? (
        <div className="space-y-3">
          <Skeleton className="h-5 w-56" />
          <Skeleton className="h-20 w-full" />
        </div>
      ) : null}

      {finalStatus === 'error' ? (
        <div className="flex min-h-28 items-center justify-center rounded-2xl border border-slate-800 bg-slate-950/25 text-sm text-slate-400">
          <div className="flex flex-col items-center gap-3 text-center">
            <AlertCircle className="h-8 w-8 text-red-300" />
            <span>公司简介加载失败，请检查 Alpha Vantage key 或本地 yfinance 环境</span>
          </div>
        </div>
      ) : null}

      {finalStatus === 'insufficient' ? (
        <div className="flex min-h-28 items-center justify-center rounded-2xl border border-slate-800 bg-slate-950/25 text-sm text-slate-400">
          暂无可用公司简介
        </div>
      ) : null}

      {finalStatus === 'success' && data ? (
        <div className="space-y-4">
          {items.length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {items.map((item) => (
                <span key={item} className="rounded-full border border-slate-700 bg-slate-950/45 px-3 py-1 text-xs font-semibold text-slate-300">
                  {item}
                </span>
              ))}
            </div>
          ) : null}
          <p className="text-sm leading-7 text-slate-300">{data.description}</p>
        </div>
      ) : null}

      <div className="min-h-4 text-right text-xs text-slate-500">
        {data?.source ? [data.source, data.updated_at ? '更新 ' + data.updated_at : ''].filter(Boolean).join(' · ') : ''}
      </div>
    </Card>
  );
}
