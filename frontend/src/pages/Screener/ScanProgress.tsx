import { Progress } from '../../components/ui/progress';
import type { ScanProgress as ScanProgressState } from '../../types';

interface ScanProgressProps {
  progress: ScanProgressState;
  isScanning: boolean;
  statusMessage?: string | null;
}

export default function ScanProgress({ progress, isScanning, statusMessage }: ScanProgressProps) {
  const percent = progress.total > 0 ? (progress.analyzed / progress.total) * 100 : 0;
  const fallbackLabel = `${isScanning ? '正在分析' : '已分析'} ${progress.analyzed} / ${progress.total} 支`;

  return (
    <div className="min-w-0 flex-1">
      <div className="mb-3 flex items-center justify-between gap-3 text-sm text-slate-300">
        <span className="min-w-0 font-semibold text-slate-100">
          <span className="block truncate">{statusMessage || fallbackLabel}</span>
          {statusMessage ? <span className="mt-1 block text-xs font-medium text-slate-500">{fallbackLabel}</span> : null}
        </span>
        <span className="font-semibold text-cyan-300">{Math.round(percent)}%</span>
      </div>
      <Progress value={percent} />
    </div>
  );
}
