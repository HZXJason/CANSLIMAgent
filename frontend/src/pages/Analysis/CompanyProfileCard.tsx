import { AlertCircle, Building2, ExternalLink } from 'lucide-react';
import { useCallback, useLayoutEffect, useRef, useState } from 'react';
import { Card } from '../../components/ui/card';
import { Skeleton } from '../../components/ui/skeleton';
import { formatDateCN } from '../../lib/utils';
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
  const [expanded, setExpanded] = useState(false);
  const [canExpand, setCanExpand] = useState(false);
  const descriptionRef = useRef<HTMLParagraphElement | null>(null);

  const measureOverflow = useCallback(() => {
    const element = descriptionRef.current;
    if (!element || expanded) return;
    setCanExpand(element.scrollHeight > element.clientHeight + 1);
  }, [expanded]);

  useLayoutEffect(() => {
    setExpanded(false);
    setCanExpand(false);
  }, [data?.symbol]);

  useLayoutEffect(() => {
    if (finalStatus !== 'success' || !data) {
      setCanExpand(false);
      return;
    }

    measureOverflow();
    window.addEventListener('resize', measureOverflow);
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(measureOverflow);
    if (descriptionRef.current) observer?.observe(descriptionRef.current);

    return () => {
      window.removeEventListener('resize', measureOverflow);
      observer?.disconnect();
    };
  }, [data, finalStatus, measureOverflow]);

  return (
    <Card className="h-full space-y-4 rounded-2xl p-4 sm:p-5">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2.5">
          <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-brand-400/20 bg-brand-400/10 text-brand-200">
            <Building2 className="h-4 w-4" />
          </span>
          <h2 className="text-base font-bold text-white">公司简介</h2>
        </div>
        {data?.website ? (
          <a
            className="inline-flex h-9 items-center gap-1.5 rounded-xl px-2.5 text-sm font-semibold text-brand-200 transition hover:bg-brand-400/10 hover:text-brand-100"
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
        <div className="flex min-h-28 items-center justify-center rounded-xl border border-slate-800 bg-slate-950/25 text-sm text-slate-400">
          <div className="flex items-center gap-2 text-center">
            <AlertCircle className="h-5 w-5 text-red-300" />
            <span>公司简介加载失败</span>
          </div>
        </div>
      ) : null}

      {finalStatus === 'insufficient' ? (
        <div className="flex min-h-28 items-center justify-center rounded-xl border border-slate-800 bg-slate-950/25 text-sm text-slate-400">
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
          <div>
            <p ref={descriptionRef} className={expanded ? 'text-sm leading-7 text-slate-300' : 'line-clamp-5 text-sm leading-7 text-slate-300'}>{data.description}</p>
            {canExpand ? (
              <button
                type="button"
                className="mt-2 text-sm font-semibold text-brand-300 transition hover:text-brand-200"
                onClick={() => setExpanded((value) => !value)}
              >
                {expanded ? '收起' : '展开'}
              </button>
            ) : null}
          </div>
        </div>
      ) : null}

      {data?.source ? (
        <div className="text-xs font-medium text-slate-500">
          {[data.source, data.updated_at ? '更新 ' + formatDateCN(data.updated_at) : ''].filter(Boolean).join(' · ')}
        </div>
      ) : null}
    </Card>
  );
}
