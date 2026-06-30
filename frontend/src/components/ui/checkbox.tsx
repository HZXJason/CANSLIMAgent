import * as React from 'react';
import { Check } from 'lucide-react';
import { cn } from '../../lib/utils';

interface CheckboxProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type' | 'onChange'> {
  checked?: boolean;
  onCheckedChange?: (checked: boolean) => void;
}

export const Checkbox = React.forwardRef<HTMLInputElement, CheckboxProps>(
  ({ className, checked, onCheckedChange, disabled, ...props }, ref) => (
    <span className="relative inline-flex h-5 w-5 shrink-0 items-center justify-center">
      <input
        ref={ref}
        type="checkbox"
        checked={checked}
        disabled={disabled}
        className={cn(
          'peer h-5 w-5 cursor-pointer appearance-none rounded-md border border-slate-600 bg-slate-950/70 transition checked:border-cyan-300 checked:bg-cyan-300 disabled:cursor-not-allowed disabled:opacity-50',
          className
        )}
        onChange={(event) => onCheckedChange?.(event.target.checked)}
        {...props}
      />
      <Check className="pointer-events-none absolute h-3.5 w-3.5 text-slate-950 opacity-0 transition peer-checked:opacity-100" />
    </span>
  )
);

Checkbox.displayName = 'Checkbox';
