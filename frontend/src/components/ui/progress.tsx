import { cn } from '../../lib/utils';

interface ProgressProps {
  value: number;
  className?: string;
}

export function Progress({ value, className }: ProgressProps) {
  const safeValue = Math.max(0, Math.min(100, value));
  return (
    <div className={cn('h-3.5 overflow-hidden rounded-full bg-slate-950/70', className)}>
      <div
        className="h-full rounded-full bg-gradient-to-r from-finance-blue via-sky-400 to-cyan-300 transition-all duration-300"
        style={{ width: safeValue + '%' }}
      />
    </div>
  );
}
