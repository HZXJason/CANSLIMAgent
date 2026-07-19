import { AlertCircle } from 'lucide-react';
import { ReactNode } from 'react';
import { Card } from '../../../components/ui/card';
import { Skeleton } from '../../../components/ui/skeleton';
import { cn } from '../../../lib/utils';
import type { ChartStatus } from '../../../types/analysis';

interface ChartFrameProps {
  title: string;
  status: ChartStatus;
  height: number;
  heightClassName?: string;
  source?: string;
  dataAsOf?: string;
  action?: ReactNode;
  children: ReactNode;
}

function CenterState({ height, heightClassName, type }: { height: number; heightClassName?: string; type: 'error' | 'insufficient' }) {
  return (
    <div
      className={cn('flex items-center justify-center rounded-xl border border-white/[0.065] bg-slate-950/25', heightClassName)}
      style={heightClassName ? undefined : { height }}
      role={type === 'error' ? 'alert' : 'status'}
    >
      <div className="flex items-center gap-2 text-center text-sm text-slate-400">
        {type === 'error' ? <AlertCircle className="h-5 w-5 text-red-300" /> : null}
        <span>{type === 'error' ? '图表加载失败' : '数据不足'}</span>
      </div>
    </div>
  );
}

export default function ChartFrame({ title, status, height, heightClassName, source, dataAsOf, action, children }: ChartFrameProps) {
  const note = source || dataAsOf ? [source || '数据源未知', dataAsOf ? '截止 ' + dataAsOf : ''].filter(Boolean).join(' · ') : '';
  const fixedHeightStyle = heightClassName ? undefined : { height };

  return (
    <Card className="h-full overflow-hidden p-0" aria-busy={status === 'loading'}>
      <div className="flex min-h-14 items-center justify-between gap-3 border-b border-white/[0.065] px-4 py-3 sm:px-5">
        <div className="min-w-0">
          <h2 className="truncate text-[15px] font-bold tracking-[-0.012em] text-white sm:text-base">{title}</h2>
          {note ? <p className="mt-1 truncate text-xs font-medium text-slate-400">{note}</p> : null}
        </div>
        {action ? <div className="shrink-0">{action}</div> : null}
      </div>
      <div className="p-3 sm:p-4">
        {status === 'loading' ? (
          <div role="status" aria-label={`${title}加载中`}>
            <Skeleton className={cn('w-full', heightClassName)} style={fixedHeightStyle} />
          </div>
        ) : null}
        {status === 'error' ? <CenterState height={height} heightClassName={heightClassName} type="error" /> : null}
        {status === 'insufficient' ? <CenterState height={height} heightClassName={heightClassName} type="insufficient" /> : null}
        {status === 'success' ? (
          <div className={cn('min-w-0', heightClassName)} style={fixedHeightStyle} aria-label={`${title}图表`}>
            {children}
          </div>
        ) : null}
      </div>
    </Card>
  );
}
