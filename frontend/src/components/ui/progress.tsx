import { cn } from '../../lib/utils';

interface ProgressProps {
  value: number;
  className?: string;
}

export function Progress({ value, className }: ProgressProps) {
  const safeValue = Math.max(0, Math.min(100, value));
  return (
    <div
      className={cn('h-2.5 overflow-hidden rounded-full bg-slate-950/70', className)}
      role="progressbar"
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(safeValue)}
    >
      <div
        className="h-full w-full origin-left rounded-full bg-gradient-to-r from-brand-400 to-brand-200 transition-transform duration-150 ease-linear"
        style={{ transform: `scaleX(${safeValue / 100})` }}
      />
    </div>
  );
}
