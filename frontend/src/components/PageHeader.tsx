import type { ReactNode } from 'react';
import { cn } from '../lib/utils';

interface PageHeaderProps {
  eyebrow?: ReactNode;
  title: ReactNode;
  description?: ReactNode;
  meta?: ReactNode;
  actions?: ReactNode;
  className?: string;
}

export function PageHeader({ eyebrow, title, description, meta, actions, className }: PageHeaderProps) {
  return (
    <header className={cn('border-b border-slate-800/80 pb-5 sm:pb-6', className)}>
      <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between lg:gap-8">
        <div className="min-w-0 max-w-3xl">
          {eyebrow ? (
            <p className="mb-2 text-xs font-bold uppercase tracking-[0.18em] text-cyan-300">{eyebrow}</p>
          ) : null}
          <h1 className="text-balance text-2xl font-black tracking-[-0.025em] text-white sm:text-3xl lg:text-[2rem]">
            {title}
          </h1>
          {description ? (
            <p className="mt-2 max-w-2xl text-sm leading-6 text-slate-400 sm:text-base sm:leading-7">{description}</p>
          ) : null}
          {meta ? (
            <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs font-semibold text-slate-400 sm:text-sm">
              {meta}
            </div>
          ) : null}
        </div>
        {actions ? (
          <div className="flex w-full shrink-0 flex-col gap-2 sm:w-auto sm:flex-row sm:flex-wrap sm:items-center lg:max-w-[48%] lg:justify-end">
            {actions}
          </div>
        ) : null}
      </div>
    </header>
  );
}
