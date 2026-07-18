import * as React from 'react';
import { cn } from '../../lib/utils';

type ButtonVariant = 'primary' | 'secondary' | 'outline' | 'ghost' | 'danger';

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
}

const variantClass: Record<ButtonVariant, string> = {
  primary: 'border-cyan-300 bg-cyan-400 text-slate-950 hover:border-cyan-200 hover:bg-cyan-300',
  secondary: 'border-slate-700 bg-slate-800 text-slate-100 hover:bg-slate-700',
  outline:
    'border-slate-700 bg-transparent text-slate-300 hover:border-slate-600 hover:bg-slate-800/70 hover:text-white',
  ghost: 'border-transparent bg-transparent text-slate-300 hover:bg-slate-800/70 hover:text-white',
  danger: 'border-red-700 bg-red-950/70 text-red-100 hover:bg-red-900/80'
};

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'primary', ...props }, ref) => (
    <button
      ref={ref}
      className={cn(
        'inline-flex h-10 items-center justify-center gap-2 rounded-xl border px-4 text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-300/60 focus-visible:ring-offset-2 focus-visible:ring-offset-slate-950 disabled:cursor-not-allowed disabled:opacity-50',
        variantClass[variant],
        className
      )}
      {...props}
    />
  )
);

Button.displayName = 'Button';
