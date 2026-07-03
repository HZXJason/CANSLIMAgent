import { AlertCircle } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import rehypeSanitize from 'rehype-sanitize';
import { Collapsible } from '../ui/collapsible';
import type { AIAgentProgress, AIDecision, AIStatus } from '../../types/analysis';

interface AIAnalysisStreamProps {
  status: AIStatus;
  progress: AIAgentProgress[];
  results: {
    agent: string;
    content: string;
    timestamp: number;
  }[];
  decision: AIDecision | null;
  error: string | null;
  liveNarration: string;
  fallbackReport: string;
}

const AGENT_ORDER = [
  { id: 'FundamentalsAnalyst', label: '基本面分析师' },
  { id: 'SentimentAnalyst', label: '情绪分析师' },
  { id: 'NewsAnalyst', label: '新闻分析师' },
  { id: 'TechnicalAnalyst', label: '技术分析师' },
  { id: 'ResearchTeam', label: '研究团队' },
  { id: 'Trader', label: '交易员' },
  { id: 'RiskManagement', label: '风险管理' },
  { id: 'PortfolioManager', label: '投资组合经理' }
];

const AGENT_LABELS = new Map(AGENT_ORDER.map((agent) => [agent.id, agent.label]));

function iconForStatus(status?: string) {
  if (status === 'done') return '✅';
  if (status === 'error') return '❌';
  return '⏳';
}

function messageForStatus(progress: AIAgentProgress | undefined, isStreaming: boolean) {
  if (progress?.message) return progress.message;
  if (progress?.status === 'done') return '分析完成';
  if (isStreaming) return '等待执行';
  return '尚未开始';
}

function decisionClass(action: string) {
  const normalized = action.toUpperCase();
  if (normalized === 'BUY') return 'border-emerald-400/40 bg-emerald-400/10 text-emerald-100';
  if (normalized === 'SELL') return 'border-red-400/40 bg-red-400/10 text-red-100';
  return 'border-amber-300/40 bg-amber-300/10 text-amber-100';
}

function MarkdownContent({ content }: { content: string }) {
  return (
    <div className="text-sm leading-7 text-slate-300">
      <ReactMarkdown
        rehypePlugins={[rehypeSanitize]}
        components={{
          h1: ({ children }) => <h1 className="mb-4 text-xl font-black text-white">{children}</h1>,
          h2: ({ children }) => <h2 className="mb-2 mt-5 text-base font-bold text-cyan-100">{children}</h2>,
          h3: ({ children }) => <h3 className="mb-2 mt-4 text-sm font-bold text-slate-100">{children}</h3>,
          p: ({ children }) => <p className="my-2 text-slate-300">{children}</p>,
          ul: ({ children }) => <ul className="my-3 space-y-2 pl-5 text-slate-300">{children}</ul>,
          ol: ({ children }) => <ol className="my-3 list-decimal space-y-2 pl-5 text-slate-300">{children}</ol>,
          li: ({ children }) => <li className="list-disc">{children}</li>,
          strong: ({ children }) => <strong className="font-bold text-white">{children}</strong>,
          code: ({ children }) => <code className="rounded bg-slate-900 px-1.5 py-0.5 text-cyan-100">{children}</code>
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}

export default function AIAnalysisStream({ status, progress, results, decision, error, liveNarration, fallbackReport }: AIAnalysisStreamProps) {
  const progressByAgent = new Map(progress.map((item) => [item.agent, item]));
  const isStreaming = status === 'streaming';
  const hasStructuredResults = results.length > 0;
  const showFallbackReport = !hasStructuredResults && fallbackReport;
  const showLiveNarration = !showFallbackReport && !hasStructuredResults && liveNarration;

  if (status === 'idle' && !hasStructuredResults && !fallbackReport && !liveNarration && !decision) {
    return (
      <div className="flex min-h-40 flex-col items-center justify-center rounded-xl border border-slate-800 bg-slate-900/45 text-center text-sm text-slate-500">
        等待启动 AI 分析
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {isStreaming || progress.length > 0 ? (
        <section className="rounded-xl border border-slate-800 bg-slate-950/35">
          <div className="border-b border-slate-800 px-4 py-3 text-sm font-bold text-slate-100">分析进度</div>
          <div className="grid gap-1 p-3 md:grid-cols-2">
            {AGENT_ORDER.map((agent) => {
              const item = progressByAgent.get(agent.id);
              const muted = !item && !isStreaming;
              return (
                <div
                  key={agent.id}
                  className="flex min-h-12 items-center gap-3 rounded-lg px-3 py-2 text-sm transition"
                >
                  <span className="w-6 text-center text-base" aria-hidden="true">
                    {iconForStatus(item?.status)}
                  </span>
                  <span className="min-w-0">
                    <span className={muted ? 'block font-bold text-slate-600' : 'block font-bold text-slate-100'}>{agent.label}</span>
                    <span className={muted ? 'block truncate text-xs text-slate-700' : 'block truncate text-xs text-slate-500'}>
                      {messageForStatus(item, isStreaming)}
                    </span>
                  </span>
                </div>
              );
            })}
          </div>
        </section>
      ) : null}

      {hasStructuredResults ? (
        <section className="space-y-3">
          {results.map((result) => (
            <Collapsible
              key={result.agent + '-' + result.timestamp}
              title={
                <span className="flex items-center justify-between gap-3">
                  <span>{AGENT_LABELS.get(result.agent) ?? result.agent}</span>
                  <span className="text-xs font-semibold text-cyan-300">已完成</span>
                </span>
              }
              defaultOpen={result.agent === results[results.length - 1]?.agent}
            >
              <MarkdownContent content={result.content} />
            </Collapsible>
          ))}
        </section>
      ) : null}

      {showLiveNarration ? (
        <Collapsible title="实时生成" defaultOpen>
          <MarkdownContent content={liveNarration} />
          {status === 'streaming' ? <span className="analysis-cursor ml-1 inline-block h-4 w-2 translate-y-0.5 bg-cyan-300" /> : null}
        </Collapsible>
      ) : null}

      {showFallbackReport ? (
        <Collapsible title="综合报告" defaultOpen>
          <MarkdownContent content={fallbackReport} />
          {status === 'streaming' ? <span className="analysis-cursor ml-1 inline-block h-4 w-2 translate-y-0.5 bg-cyan-300" /> : null}
        </Collapsible>
      ) : null}

      {decision ? (
        <section className={'rounded-xl border p-4 ' + decisionClass(decision.action)}>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <p className="text-xs font-bold uppercase opacity-80">最终操作建议</p>
              <h3 className="mt-1 text-3xl font-black">{decision.action}</h3>
            </div>
            <span className="inline-flex w-fit rounded-full border border-current/30 px-3 py-1 text-xs font-bold">
              置信度：{decision.confidence}
            </span>
          </div>
          {decision.summary ? <p className="mt-4 text-sm leading-7 opacity-90">{decision.summary}</p> : null}
        </section>
      ) : null}

      {error ? (
        <div className="flex items-center gap-2 rounded-xl border border-red-800 bg-red-950/60 px-4 py-3 text-sm text-red-100">
          <AlertCircle className="h-4 w-4" />
          {error}
        </div>
      ) : null}
    </div>
  );
}
