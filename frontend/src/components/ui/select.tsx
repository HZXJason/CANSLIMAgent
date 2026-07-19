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
          'min-h-11 w-full appearance-none rounded-xl border border-slate-700/80 bg-slate-950/55 px-3.5 pr-10 text-sm font-semibold text-slate-100 outline-none transition-[border-color,background-color,box-shadow] duration-150 ease-out hover:bg-slate-950/75 focus:border-brand-300/60 focus:bg-slate-950/80 focus:ring-2 focus:ring-brand-300/15 disabled:cursor-not-allowed disabled:opacity-50 sm:min-h-10',
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
