import { ReactNode, useId } from 'react';
import { cn } from '../../lib/utils';

interface TooltipProps {
  content: string;
  children: ReactNode;
  singleLine?: boolean;
}

export function Tooltip({ content, children, singleLine = false }: TooltipProps) {
  const id = useId();

  return (
    <span className="group/tooltip relative inline-flex" aria-label={content}>
      {children}
      <span
        id={id}
        role="tooltip"
        className={cn(
          'pointer-events-none absolute bottom-full left-1/2 z-50 mb-2 -translate-x-1/2 scale-[0.97] rounded-xl border border-slate-700/90 bg-slate-950/98 px-3 py-2 text-xs leading-5 text-slate-200 opacity-0 shadow-float transition-[opacity,transform] duration-150 ease-[cubic-bezier(0.23,1,0.32,1)] group-hover/tooltip:scale-100 group-hover/tooltip:opacity-100 group-focus-within/tooltip:scale-100 group-focus-within/tooltip:opacity-100',
          singleLine
            ? 'w-max max-w-[calc(100vw-2rem)] overflow-hidden whitespace-nowrap text-ellipsis'
            : 'max-w-[min(18rem,calc(100vw-2rem))] whitespace-normal'
        )}
      >
        {content}
      </span>
    </span>
  );
}
