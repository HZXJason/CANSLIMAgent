import * as React from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';
import { cn } from '../../lib/utils';

interface CollapsibleProps {
  title: React.ReactNode;
  children: React.ReactNode;
  defaultOpen?: boolean;
  disabled?: boolean;
  className?: string;
}

export function Collapsible({ title, children, defaultOpen = true, disabled = false, className }: CollapsibleProps) {
  const [open, setOpen] = React.useState(defaultOpen);

  return (
    <div className={cn('rounded-xl border border-slate-800 bg-slate-950/35', className)}>
      <button
        type="button"
        disabled={disabled}
        onClick={() => setOpen((value) => !value)}
        className="flex w-full items-center justify-between gap-3 px-4 py-3 text-left text-sm font-bold text-slate-100 transition hover:bg-slate-800/40 disabled:cursor-not-allowed disabled:opacity-60"
      >
        <span className="min-w-0 flex-1">{title}</span>
        {open ? <ChevronDown className="h-4 w-4 text-slate-400" /> : <ChevronRight className="h-4 w-4 text-slate-400" />}
      </button>
      {open ? <div className="border-t border-slate-800 px-4 py-4">{children}</div> : null}
    </div>
  );
}
