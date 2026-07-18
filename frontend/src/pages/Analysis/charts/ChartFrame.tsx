import { AlertCircle } from 'lucide-react';
import { ReactNode } from 'react';
import { Card } from '../../../components/ui/card';
import { Skeleton } from '../../../components/ui/skeleton';
import type { ChartStatus } from '../../../types/analysis';

interface ChartFrameProps {
  title: string;
  status: ChartStatus;
  height: number;
  source?: string;
  dataAsOf?: string;
  action?: ReactNode;
  children: ReactNode;
}

function CenterState({ height, type }: { height: number; type: 'error' | 'insufficient' }) {
  return (
    <div className="flex items-center justify-center rounded-xl border border-slate-800 bg-slate-950/25" style={{ height }}>
      <div className="flex items-center gap-2 text-center text-sm text-slate-400">
        {type === 'error' ? <AlertCircle className="h-5 w-5 text-red-300" /> : null}
        <span>{type === 'error' ? '图表加载失败' : '数据不足'}</span>
      </div>
    </div>
  );
}

export default function ChartFrame({ title, status, height, source, dataAsOf, action, children }: ChartFrameProps) {
  const note = source || dataAsOf ? [source || '数据源未知', dataAsOf ? '截止 ' + dataAsOf : ''].filter(Boolean).join(' · ') : '';

  return (
    <Card className="h-full space-y-3 rounded-2xl p-4 sm:p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-base font-bold text-white">{title}</h2>
        <div className="flex flex-wrap items-center justify-end gap-3">
          {note ? <span className="text-xs font-medium text-slate-500">{note}</span> : null}
          {action}
        </div>
      </div>
      {status === 'loading' ? <Skeleton style={{ height }} /> : null}
      {status === 'error' ? <CenterState height={height} type="error" /> : null}
      {status === 'insufficient' ? <CenterState height={height} type="insufficient" /> : null}
      {status === 'success' ? <div style={{ height }}>{children}</div> : null}
    </Card>
  );
}
