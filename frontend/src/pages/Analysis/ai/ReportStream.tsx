import ReactMarkdown from 'react-markdown';
import rehypeSanitize from 'rehype-sanitize';
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
    <article className="rounded-2xl border border-slate-800 bg-slate-950/35 p-5 text-sm leading-7 text-slate-300">
      <ReactMarkdown
        rehypePlugins={[rehypeSanitize]}
        components={{
          h1: ({ children }) => <h1 className="mb-4 text-xl font-black text-white">{children}</h1>,
          h2: ({ children }) => <h2 className="mb-2 mt-5 text-base font-bold text-cyan-100">{children}</h2>,
          p: ({ children }) => <p className="my-2 text-slate-300">{children}</p>,
          ul: ({ children }) => <ul className="my-3 space-y-2 pl-5 text-slate-300">{children}</ul>,
          li: ({ children }) => <li className="list-disc">{children}</li>,
          strong: ({ children }) => <strong className="font-bold text-white">{children}</strong>
        }}
      >
        {content}
      </ReactMarkdown>
      {status === 'streaming' ? <span className="analysis-cursor ml-1 inline-block h-4 w-2 translate-y-0.5 bg-cyan-300" /> : null}
    </article>
  );
}
