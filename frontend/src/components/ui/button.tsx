import * as React from 'react';
import { cn } from '../../lib/utils';

type ButtonVariant = 'primary' | 'secondary' | 'outline' | 'ghost' | 'danger';

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
}

const variantClass: Record<ButtonVariant, string> = {
  primary: 'border-cyan-200/80 bg-cyan-300 text-slate-950 shadow-[0_8px_24px_rgba(34,184,221,0.13)] hover:border-cyan-100 hover:bg-cyan-200',
  secondary: 'border-slate-700/80 bg-slate-800/90 text-slate-100 hover:border-slate-600 hover:bg-slate-700',
  outline:
    'border-slate-700/80 bg-slate-950/20 text-slate-300 hover:border-slate-600 hover:bg-slate-800/70 hover:text-white',
  ghost: 'border-transparent bg-transparent text-slate-300 hover:bg-slate-800/65 hover:text-white',
  danger: 'border-red-500/35 bg-red-500/10 text-red-200 hover:border-red-400/50 hover:bg-red-500/20'
};

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'primary', ...props }, ref) => (
    <button
      ref={ref}
      className={cn(
        'interactive-press inline-flex min-h-11 items-center justify-center gap-2 rounded-xl border px-4 text-sm font-bold tracking-[-0.01em] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-300/60 focus-visible:ring-offset-2 focus-visible:ring-offset-slate-950 disabled:cursor-not-allowed disabled:opacity-45 disabled:active:scale-100 sm:min-h-10',
        variantClass[variant],
        className
      )}
      {...props}
    />
  )
);

Button.displayName = 'Button';
