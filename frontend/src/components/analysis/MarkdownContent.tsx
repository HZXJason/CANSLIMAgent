import ReactMarkdown from 'react-markdown';
import rehypeSanitize from 'rehype-sanitize';
import remarkGfm from 'remark-gfm';

interface MarkdownContentProps {
  content: string;
  className?: string;
}

export default function MarkdownContent({ content, className = '' }: MarkdownContentProps) {
  const wrapperClassName = ['text-[15px] leading-7 text-slate-300', className].filter(Boolean).join(' ');

  return (
    <div className={wrapperClassName}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeSanitize]}
        components={{
          h1: ({ children }) => <h1 className="mb-4 text-xl font-black text-white">{children}</h1>,
          h2: ({ children }) => <h2 className="mb-2 mt-5 text-base font-bold text-brand-100">{children}</h2>,
          h3: ({ children }) => <h3 className="mb-2 mt-4 text-sm font-bold text-slate-100">{children}</h3>,
          p: ({ children }) => <p className="my-2 text-slate-300">{children}</p>,
          ul: ({ children }) => <ul className="my-3 list-disc space-y-2 pl-5 text-slate-300">{children}</ul>,
          ol: ({ children }) => <ol className="my-3 list-decimal space-y-2 pl-5 text-slate-300">{children}</ol>,
          li: ({ children }) => <li className="pl-1">{children}</li>,
          strong: ({ children }) => <strong className="font-bold text-white">{children}</strong>,
          a: ({ children, href }) => (
            <a className="font-semibold text-brand-300 underline decoration-brand-300/40 underline-offset-4" href={href}>
              {children}
            </a>
          ),
          blockquote: ({ children }) => (
            <blockquote className="my-4 border-l-2 border-brand-400/50 pl-4 text-slate-400">{children}</blockquote>
          ),
          table: ({ children }) => (
            <div className="my-4 overflow-x-auto rounded-lg border border-slate-800">
              <table className="min-w-full border-collapse text-left text-sm">{children}</table>
            </div>
          ),
          thead: ({ children }) => <thead className="bg-slate-900/80 text-slate-100">{children}</thead>,
          tbody: ({ children }) => <tbody className="divide-y divide-slate-800 bg-slate-950/35">{children}</tbody>,
          tr: ({ children }) => <tr className="align-top">{children}</tr>,
          th: ({ children }) => <th className="border-r border-slate-800 px-3 py-2 font-bold last:border-r-0">{children}</th>,
          td: ({ children }) => <td className="border-r border-slate-800 px-3 py-2 text-slate-300 last:border-r-0">{children}</td>,
          pre: ({ children }) => <pre className="my-4 overflow-x-auto rounded-lg bg-slate-950 p-3 text-xs leading-6 text-slate-200">{children}</pre>,
          code: ({ children }) => <code className="rounded bg-slate-900 px-1.5 py-0.5 text-brand-100">{children}</code>
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}
