import { Progress } from '../../components/ui/progress';
import type { ScanProgress as ScanProgressState } from '../../types';

interface ScanProgressProps {
  progress: ScanProgressState;
  isScanning: boolean;
}

export default function ScanProgress({ progress, isScanning }: ScanProgressProps) {
  const percent = progress.total > 0 ? (progress.analyzed / progress.total) * 100 : 0;

  return (
    <div className="min-w-0 flex-1">
      <div className="mb-3 flex items-center justify-between gap-3 text-sm text-slate-300">
        <span className="font-semibold text-slate-100">
          {isScanning ? '正在分析' : '已分析'} {progress.analyzed} / {progress.total} 支
        </span>
        <span className="font-semibold text-cyan-300">{Math.round(percent)}%</span>
      </div>
      <Progress value={percent} />
    </div>
  );
}
