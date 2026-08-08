import * as React from 'react';
import { cn } from '../../lib/utils';

export interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  invalid?: boolean;
}

export const Input = React.forwardRef<HTMLInputElement, InputProps>(({ className, invalid, ...props }, ref) => (
  <input
    ref={ref}
    aria-invalid={invalid || undefined}
    className={cn(
      'min-h-11 w-full rounded-sm border bg-slate-950/55 px-3.5 text-sm text-slate-100 outline-none transition-[border-color,background-color,box-shadow] duration-150 ease-out placeholder:text-slate-500 hover:bg-slate-950/75 focus:border-brand-300/60 focus:bg-slate-950/80 focus:ring-2 focus:ring-brand-300/15 disabled:cursor-not-allowed disabled:bg-slate-900/55 disabled:text-slate-500 sm:min-h-10',
      invalid ? 'border-red-400/80 focus:border-red-400 focus:ring-red-400/15' : 'border-slate-700/80',
      className
    )}
    {...props}
  />
));

Input.displayName = 'Input';
