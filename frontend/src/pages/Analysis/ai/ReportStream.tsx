import MarkdownContent from '../../../components/analysis/MarkdownContent';
import type { AIStatus } from '../../../types/analysis';

interface ReportStreamProps {
  content: string;
  status: AIStatus;
}

export default function ReportStream({ content, status }: ReportStreamProps) {
  if (!content && status === 'streaming') {
    return <div className="rounded-2xl border border-slate-800 bg-slate-950/30 p-4 text-sm text-slate-500">正在生成报告...</div>;
  }
  if (!content) return null;

  return (
    <article className="rounded-2xl border border-slate-800 bg-slate-950/35 p-5">
      <MarkdownContent content={content} />
      {status === 'streaming' ? <span className="analysis-cursor ml-1 inline-block h-4 w-2 translate-y-0.5 bg-brand-300" /> : null}
    </article>
  );
}
