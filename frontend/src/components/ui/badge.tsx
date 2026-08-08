import * as React from 'react';
import { cn } from '../../lib/utils';

interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  variant?: 'pass' | 'fail' | 'insufficient' | 'neutral';
}

const variantClass = {
  pass: 'border-emerald-300/50 bg-emerald-300 text-emerald-950 shadow-sm shadow-emerald-500/15',
  fail: 'border-slate-700/80 bg-slate-800/65 text-slate-400',
  insufficient: 'border-amber-300/50 bg-amber-300 text-amber-950 shadow-sm shadow-amber-400/15',
  neutral: 'border-slate-700/80 bg-slate-800/75 text-slate-300'
};

export function Badge({ className, variant = 'neutral', ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        'font-data inline-flex h-7 min-w-7 items-center justify-center rounded-sm border px-2 text-xs font-extrabold tabular-nums',
        variantClass[variant],
        className
      )}
      {...props}
    />
  );
}
