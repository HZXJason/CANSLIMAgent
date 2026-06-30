import * as React from 'react';
import { ChevronDown } from 'lucide-react';
import { cn } from '../../lib/utils';

interface SelectProps extends Omit<React.SelectHTMLAttributes<HTMLSelectElement>, 'onChange'> {
  onValueChange?: (value: string) => void;
}

export const Select = React.forwardRef<HTMLSelectElement, SelectProps>(
  ({ className, children, onValueChange, ...props }, ref) => (
    <div className="relative">
      <select
        ref={ref}
        className={cn(
          'h-11 w-full appearance-none rounded-xl border border-slate-700/80 bg-slate-950/60 px-3 pr-10 text-sm font-semibold text-slate-100 outline-none transition focus:border-cyan-400/70 disabled:cursor-not-allowed disabled:opacity-50',
          className
        )}
        onChange={(event) => onValueChange?.(event.target.value)}
        {...props}
      >
        {children}
      </select>
      <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
    </div>
  )
);

Select.displayName = 'Select';
