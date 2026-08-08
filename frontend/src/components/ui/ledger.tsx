import { useId, type HTMLAttributes, type ReactNode } from 'react';
import { cn } from '../../lib/utils';

interface LedgerSectionProps extends Omit<HTMLAttributes<HTMLElement>, 'title'> {
  title: ReactNode;
  meta?: ReactNode;
}

export function LedgerSection({ title, meta, children, className, ...props }: LedgerSectionProps) {
  const titleID = useId();
  return (
    <section aria-labelledby={titleID} className={cn('ledger-section', className)} {...props}>
      <header className="ledger-section-header">
        <h2 id={titleID} className="font-display text-lg font-bold text-ink sm:text-xl">
          {title}
        </h2>
        {meta ? <div className="font-data text-xs text-ink-tertiary">{meta}</div> : null}
      </header>
      {children}
    </section>
  );
}

interface DataValueProps extends HTMLAttributes<HTMLDivElement> {
  label: ReactNode;
  value: ReactNode;
  detail?: ReactNode;
}

export function DataValue({ label, value, detail, className, ...props }: DataValueProps) {
  return (
    <div className={cn('ledger-data-value', className)} {...props}>
      <span className="text-xs font-semibold text-ink-tertiary">{label}</span>
      <strong className="font-data text-lg font-semibold text-ink">{value}</strong>
      {detail ? <span className="text-xs text-ink-secondary">{detail}</span> : null}
    </div>
  );
}

interface SourceStampProps extends HTMLAttributes<HTMLSpanElement> {
  source: ReactNode;
  date?: ReactNode;
}

export function SourceStamp({ source, date, className, ...props }: SourceStampProps) {
  return (
    <span className={cn('source-stamp font-data', className)} {...props}>
      {source}
      {date ? <span aria-hidden="true"> · </span> : null}
      {date}
    </span>
  );
}
