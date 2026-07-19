import * as React from 'react';
import { cn } from '../../lib/utils';

export function Skeleton({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('animate-pulse rounded-2xl bg-slate-800/60', className)} role="status" aria-label="内容加载中" {...props} />;
}
