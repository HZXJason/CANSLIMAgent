import * as React from 'react';
import { cn } from '../../lib/utils';

type ButtonVariant = 'primary' | 'secondary' | 'outline' | 'ghost' | 'danger';

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
}

const variantClass: Record<ButtonVariant, string> = {
  primary:
    'border-cyan-400/40 bg-gradient-to-r from-finance-blue to-cyan-500 text-white shadow-glow hover:brightness-110',
  secondary: 'border-slate-700/80 bg-slate-800/80 text-slate-100 hover:bg-slate-700',
  outline:
    'border-slate-700/80 bg-slate-950/20 text-slate-300 hover:border-cyan-400/50 hover:bg-slate-800/80 hover:text-white',
  ghost: 'border-transparent bg-transparent text-slate-300 hover:bg-slate-800/70 hover:text-white',
  danger: 'border-red-800/80 bg-red-950/70 text-red-100 hover:bg-red-900/80'
};

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'primary', ...props }, ref) => (
    <button
      ref={ref}
      className={cn(
        'inline-flex items-center justify-center gap-2 rounded-2xl border px-4 py-2.5 text-sm font-semibold transition duration-200 disabled:cursor-not-allowed disabled:opacity-50',
        variantClass[variant],
        className
      )}
      {...props}
    />
  )
);

Button.displayName = 'Button';
