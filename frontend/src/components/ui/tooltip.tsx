import { ReactNode } from 'react';

interface TooltipProps {
  content: string;
  children: ReactNode;
}

export function Tooltip({ content, children }: TooltipProps) {
  return (
    <span className="group relative inline-flex">
      {children}
      <span className="pointer-events-none absolute bottom-full left-1/2 z-20 mb-2 w-56 -translate-x-1/2 rounded-xl border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-slate-200 opacity-0 shadow-xl transition group-hover:opacity-100">
        {content}
      </span>
    </span>
  );
}
