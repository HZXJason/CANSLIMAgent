import type { ReactNode } from 'react';
import { cn } from '../lib/utils';

interface PageHeaderProps {
  title: ReactNode;
  description?: ReactNode;
  meta?: ReactNode;
  actions?: ReactNode;
  className?: string;
  contentClassName?: string;
}

export function PageHeader({ title, description, meta, actions, className, contentClassName }: PageHeaderProps) {
  return (
    <header className={cn('border-b border-rule-strong pb-5 sm:pb-6', className)}>
      <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between lg:gap-8">
        <div className={cn('min-w-0 max-w-3xl', contentClassName)}>
          <h1 className="font-display text-balance text-[1.75rem] font-bold leading-[1.1] tracking-[-0.025em] text-ink sm:text-[2.15rem] lg:text-[2.5rem]">
            {title}
          </h1>
          {description ? (
            <p className="mt-3 max-w-[70ch] text-sm leading-6 text-ink-secondary sm:text-[15px] sm:leading-7">{description}</p>
          ) : null}
          {meta ? (
            <div className="font-data mt-3 flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs text-ink-tertiary">
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
