import { Progress } from '../../components/ui/progress';
import type { ScanProgress as ScanProgressState } from '../../types';

interface ScanProgressProps {
  progress: ScanProgressState;
  isScanning: boolean;
  isPaused: boolean;
  isComplete: boolean;
  showProgress: boolean;
  statusMessage?: string | null;
}

export default function ScanProgress({ progress, isScanning, isPaused, isComplete, showProgress, statusMessage }: ScanProgressProps) {
  const percent = progress.total > 0 ? (progress.analyzed / progress.total) * 100 : 0;
  const label = statusMessage
    || (isScanning
      ? '扫描中'
      : isPaused
        ? '扫描已暂停'
        : isComplete
          ? '扫描完成'
          : progress.analyzed > 0
            ? '已加载缓存'
            : '准备扫描');

  if (!showProgress) {
    return (
      <div className="text-sm font-medium text-slate-400">
        {statusMessage || (progress.analyzed > 0 ? `已加载 ${progress.analyzed} 条缓存` : '准备扫描')}
      </div>
    );
  }

  return (
    <div className="min-w-0 flex-1">
      <div className="mb-2 flex items-center justify-between gap-3 text-sm">
        <span className="min-w-0 truncate font-medium text-slate-200">{label}</span>
        <span className="shrink-0 tabular-nums text-slate-400">
          {progress.total > 0 ? `${progress.analyzed} / ${progress.total}` : '准备中'}
        </span>
      </div>
      <Progress value={percent} className="h-2" />
    </div>
  );
}
