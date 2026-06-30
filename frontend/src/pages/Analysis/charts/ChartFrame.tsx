import { AlertCircle } from 'lucide-react';
import { ReactNode } from 'react';
import { Card } from '../../../components/ui/card';
import { Skeleton } from '../../../components/ui/skeleton';
import type { ChartStatus } from '../../../types/analysis';

interface ChartFrameProps {
  title: string;
  subtitle?: string;
  status: ChartStatus;
  height: number;
  source?: string;
  dataAsOf?: string;
  action?: ReactNode;
  children: ReactNode;
}

function CenterState({ height, type }: { height: number; type: 'error' | 'insufficient' }) {
  return (
    <div className="flex items-center justify-center rounded-2xl border border-slate-800 bg-slate-950/25" style={{ height }}>
      <div className="flex flex-col items-center gap-3 text-center text-sm text-slate-400">
        {type === 'error' ? <AlertCircle className="h-9 w-9 text-red-300" /> : <span className="text-3xl text-amber-300">?</span>}
        <span>{type === 'error' ? '数据加载失败，请检查数据源后重试' : '历史数据不足，无法生成完整图表'}</span>
      </div>
    </div>
  );
}

export default function ChartFrame({ title, subtitle, status, height, source, dataAsOf, action, children }: ChartFrameProps) {
  const note = source || dataAsOf ? [source || '数据源未知', dataAsOf ? '截止 ' + dataAsOf : ''].filter(Boolean).join(' · ') : '';

  return (
    <Card className="space-y-4 rounded-[1rem] p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-base font-bold text-white">{title}</h2>
          {subtitle ? <p className="mt-1 text-xs text-slate-500">{subtitle}</p> : null}
        </div>
        {action}
      </div>
      {status === 'loading' ? <Skeleton style={{ height }} /> : null}
      {status === 'error' ? <CenterState height={height} type="error" /> : null}
      {status === 'insufficient' ? <CenterState height={height} type="insufficient" /> : null}
      {status === 'success' ? <div style={{ height }}>{children}</div> : null}
      <div className="min-h-4 text-right text-xs text-slate-500">{note}</div>
    </Card>
  );
}
