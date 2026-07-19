import * as React from 'react';
import { Check } from 'lucide-react';
import { cn } from '../../lib/utils';

interface CheckboxProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type' | 'onChange'> {
  checked?: boolean;
  onCheckedChange?: (checked: boolean) => void;
}

export const Checkbox = React.forwardRef<HTMLInputElement, CheckboxProps>(
  ({ className, checked, onCheckedChange, disabled, ...props }, ref) => (
    <span className="relative inline-flex h-6 w-6 shrink-0 items-center justify-center">
      <input
        ref={ref}
        type="checkbox"
        checked={checked}
        disabled={disabled}
        className={cn(
          'peer h-6 w-6 cursor-pointer appearance-none rounded-[7px] border border-slate-600 bg-slate-950/70 transition-[border-color,background-color,box-shadow,transform] duration-150 ease-out checked:border-cyan-300 checked:bg-cyan-300 focus-visible:ring-2 focus-visible:ring-cyan-300/30 disabled:cursor-not-allowed disabled:opacity-50 active:scale-95',
          className
        )}
        onChange={(event) => onCheckedChange?.(event.target.checked)}
        {...props}
      />
      <Check className="pointer-events-none absolute h-4 w-4 text-slate-950 opacity-0 transition-opacity duration-150 peer-checked:opacity-100" strokeWidth={2.7} />
    </span>
  )
);

Checkbox.displayName = 'Checkbox';
