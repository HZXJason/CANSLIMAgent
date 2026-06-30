import * as React from 'react';
import { cn } from '../../lib/utils';

interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  variant?: 'pass' | 'fail' | 'insufficient' | 'neutral';
}

const variantClass = {
  pass: 'border-emerald-400 bg-emerald-400 text-slate-950 shadow-sm shadow-emerald-500/25',
  fail: 'border-slate-700 bg-slate-800/70 text-slate-500',
  insufficient: 'border-amber-300 bg-amber-300 text-slate-950 shadow-sm shadow-amber-400/20',
  neutral: 'border-slate-700 bg-slate-800/80 text-slate-300'
};

export function Badge({ className, variant = 'neutral', ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex h-8 min-w-8 items-center justify-center rounded-full border px-2 text-xs font-bold',
        variantClass[variant],
        className
      )}
      {...props}
    />
  );
}
