import { ReactNode, useId } from 'react';

interface TooltipProps {
  content: string;
  children: ReactNode;
}

export function Tooltip({ content, children }: TooltipProps) {
  const id = useId();

  return (
    <span className="group relative inline-flex" aria-label={content}>
      {children}
      <span
        id={id}
        role="tooltip"
        className="pointer-events-none absolute bottom-full left-1/2 z-50 mb-2 max-w-[min(18rem,calc(100vw-2rem))] -translate-x-1/2 scale-[0.97] whitespace-normal rounded-xl border border-slate-700/90 bg-slate-950/98 px-3 py-2 text-xs leading-5 text-slate-200 opacity-0 shadow-float transition-[opacity,transform] duration-150 ease-[cubic-bezier(0.23,1,0.32,1)] group-hover:scale-100 group-hover:opacity-100 group-focus-within:scale-100 group-focus-within:opacity-100"
      >
        {content}
      </span>
    </span>
  );
}
