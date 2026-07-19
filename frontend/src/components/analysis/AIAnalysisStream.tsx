import { AlertCircle, CheckCircle2, CircleDashed, Loader2, PauseCircle, XCircle } from 'lucide-react';
import { Collapsible } from '../ui/collapsible';
import MarkdownContent from './MarkdownContent';
import type { AIAgentProgress, AIDecision, AIProgressStatus, AIStatus } from '../../types/analysis';

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
  liveNarrationByAgent: Record<string, string>;
  fallbackReport: string;
}

const AGENT_ORDER = [
  { id: 'VibeEvidence', label: 'Vibe 数据证据' },
  { id: 'DataAuditor', label: '数据审计员' },
  { id: 'FundamentalsAnalyst', label: '基本面分析师' },
  { id: 'SentimentAnalyst', label: '情绪分析师' },
  { id: 'NewsAnalyst', label: '新闻分析师' },
  { id: 'TechnicalAnalyst', label: '技术分析师' },
  { id: 'CANSLIMSecondLayerAnalyst', label: 'CANSLIM 二层筛选官' },
  { id: 'ResearchTeam', label: '研究团队' },
  { id: 'Trader', label: '交易员' },
  { id: 'RiskManagement', label: '风险管理团队' },
  { id: 'VibeEvidenceReconciler', label: 'Vibe 证据对账' },
  { id: 'PortfolioManager', label: '投资组合经理' }
];

const AGENT_LABELS = new Map(AGENT_ORDER.map((agent) => [agent.id, agent.label]));
const AGENT_INDEX = new Map(AGENT_ORDER.map((agent, index) => [agent.id, index]));
const REPORT_TITLE_TO_AGENT = new Map<string, string>();

for (const agent of AGENT_ORDER) {
  REPORT_TITLE_TO_AGENT.set(agent.id, agent.id);
  REPORT_TITLE_TO_AGENT.set(agent.label, agent.id);
}

REPORT_TITLE_TO_AGENT.set('风险管理', 'RiskManagement');
REPORT_TITLE_TO_AGENT.set('实时分析', 'AnalysisAgent');

interface ReportSection {
  key: string;
  title: string;
  content: string;
  agent?: string;
  source: 'report' | 'live' | 'placeholder';
}

function orderedProgress(progress: AIAgentProgress[]) {
  return [...progress].sort((left, right) => {
    const leftIndex = AGENT_INDEX.get(left.agent) ?? Number.MAX_SAFE_INTEGER;
    const rightIndex = AGENT_INDEX.get(right.agent) ?? Number.MAX_SAFE_INTEGER;
    if (leftIndex !== rightIndex) return leftIndex - rightIndex;
    return left.timestamp - right.timestamp;
  });
}

function normalizeHeading(value: string) {
  return value.replace(/[：:]\s*$/, '').trim();
}

function titleForAgent(agent: string) {
  if (agent === 'AnalysisAgent') return '实时分析';
  return AGENT_LABELS.get(agent) ?? agent;
}

function parseReportSections(report: string): ReportSection[] {
  const sections: ReportSection[] = [];
  const introLines: string[] = [];
  let current: { agent?: string; title: string; lines: string[] } | null = null;

  const flushCurrent = () => {
    if (!current) return;
    const content = current.lines.join('\n').trim();
    if (content) {
      sections.push({
        key: current.agent ?? 'section-' + sections.length,
        title: current.title,
        content,
        agent: current.agent,
        source: 'report'
      });
    }
    current = null;
  };

  for (const line of report.split(/\r?\n/)) {
    const headingMatch = line.match(/^##\s+(.+?)\s*$/);
    const heading = headingMatch ? normalizeHeading(headingMatch[1]) : '';
    const agent = heading ? REPORT_TITLE_TO_AGENT.get(heading) : undefined;

    if (agent) {
      flushCurrent();
      current = { agent, title: titleForAgent(agent), lines: [] };
      continue;
    }

    if (current) {
      current.lines.push(line);
    } else {
      introLines.push(line);
    }
  }

  flushCurrent();

  const intro = introLines.join('\n').trim();
  if (intro) {
    sections.unshift({
      key: 'summary',
      title: '综合报告',
      content: intro,
      source: 'report'
    });
  }

  return sections;
}

function statusStyles(status?: AIProgressStatus) {
  if (status === 'done') return 'border-emerald-400/30 bg-emerald-400/10 text-emerald-200';
  if (status === 'error') return 'border-red-400/30 bg-red-400/10 text-red-200';
  if (status === 'running') return 'border-brand-400/30 bg-brand-400/10 text-brand-200';
  if (status === 'paused') return 'border-amber-300/30 bg-amber-300/10 text-amber-100';
  return 'border-slate-700 bg-slate-900/80 text-slate-400';
}

function StatusIcon({ status }: { status?: AIProgressStatus }) {
  if (status === 'done') return <CheckCircle2 className="h-4 w-4" />;
  if (status === 'error') return <XCircle className="h-4 w-4" />;
  if (status === 'paused') return <PauseCircle className="h-4 w-4" />;
  if (status === 'running') return <Loader2 className="h-4 w-4 animate-spin" />;
  return <CircleDashed className="h-4 w-4" />;
}

function resultSections(results: AIAnalysisStreamProps['results']): ReportSection[] {
  return results.map((result, index) => ({
    key: result.agent + '-' + result.timestamp + '-' + index,
    title: titleForAgent(result.agent),
    content: result.content,
    agent: result.agent,
    source: 'report'
  }));
}

function liveSections(liveNarrationByAgent: Record<string, string>): ReportSection[] {
  return Object.entries(liveNarrationByAgent)
    .filter(([, content]) => content.trim())
    .map(([agent, content]) => ({
      key: 'live-' + agent,
      title: titleForAgent(agent),
      content,
      agent,
      source: 'live' as const
    }));
}

function orderedAgentSections(sections: ReportSection[]) {
  return [...sections].sort((left, right) => {
    const leftIndex = left.agent ? AGENT_INDEX.get(left.agent) ?? Number.MAX_SAFE_INTEGER : Number.MAX_SAFE_INTEGER;
    const rightIndex = right.agent ? AGENT_INDEX.get(right.agent) ?? Number.MAX_SAFE_INTEGER : Number.MAX_SAFE_INTEGER;
    if (leftIndex !== rightIndex) return leftIndex - rightIndex;
    return left.title.localeCompare(right.title);
  });
}

function roleOutputSections(reportSections: ReportSection[], liveNarrationByAgent: Record<string, string>) {
  const byAgent = new Map<string, ReportSection>();
  const unassigned: ReportSection[] = [];

  for (const section of reportSections) {
    if (section.agent) {
      byAgent.set(section.agent, section);
    } else {
      unassigned.push(section);
    }
  }

  for (const section of liveSections(liveNarrationByAgent)) {
    if (section.agent && !byAgent.has(section.agent)) {
      byAgent.set(section.agent, section);
    }
  }

  return [...orderedAgentSections([...byAgent.values()]), ...unassigned];
}

function balanceStreamingMarkdown(content: string) {
  const boldMarkers = content.split('**').length - 1;
  return boldMarkers % 2 === 0 ? content : content + '**';
}

function sectionStatusLabel(section: ReportSection, progress: AIAgentProgress | undefined, status: AIStatus) {
  if (status === 'paused' && section.source !== 'report') return '已暂停';
  if (status === 'error' && section.source !== 'report') return '已中断';
  if (section.source === 'live') return status === 'done' ? '已生成' : '生成中';
  if (section.content) return status === 'streaming' && progress?.status === 'running' ? '生成中' : '已生成';
  if (progress?.status === 'pending') return '等待中';
  if (progress?.status === 'running') return '生成中';
  if (progress?.status === 'paused') return '已暂停';
  if (progress?.status === 'error') return '已中断';
  return '等待输出';
}

function emptySectionText(status: AIStatus, progress: AIAgentProgress | undefined) {
  if (status === 'paused') return '已暂停，当前角色暂无输出。';
  if (progress?.status === 'running') return '正在生成该角色输出...';
  if (progress?.status === 'pending') return '等待该角色执行。';
  return '暂无输出。';
}

function decisionClass(action: string) {
  const normalized = action.toUpperCase();
  if (normalized === 'BUY') return 'border-emerald-400/40 bg-emerald-400/10 text-emerald-100';
  if (normalized === 'SELL') return 'border-red-400/40 bg-red-400/10 text-red-100';
  return 'border-amber-300/40 bg-amber-300/10 text-amber-100';
}

function decisionLabel(action: string) {
  const normalized = action.toUpperCase();
  if (normalized === 'BUY') return '买入';
  if (normalized === 'SELL') return '卖出';
  if (normalized === 'HOLD') return '观望';
  return action;
}

function confidenceLabel(confidence: string) {
  const normalized = confidence.toUpperCase();
  if (normalized === 'HIGH') return '高';
  if (normalized === 'MEDIUM') return '中';
  if (normalized === 'LOW') return '低';
  return confidence;
}

export default function AIAnalysisStream({
  status,
  progress,
  results,
  decision,
  error,
  liveNarration,
  liveNarrationByAgent,
  fallbackReport
}: AIAnalysisStreamProps) {
  const isStreaming = status === 'streaming';
  const progressItems = orderedProgress(progress);
  const progressByAgent = new Map(progressItems.map((item) => [item.agent, item]));
  const hasStructuredResults = results.length > 0;
  const reportSections = [...resultSections(results), ...parseReportSections(fallbackReport)];
  const liveOutputByAgent = Object.keys(liveNarrationByAgent).length > 0
    ? liveNarrationByAgent
    : liveNarration
      ? { AnalysisAgent: liveNarration }
      : {};
  const outputSections = roleOutputSections(reportSections, liveOutputByAgent);
  const showOutputSections = outputSections.length > 0;
  const showFallbackReport = !showOutputSections && !hasStructuredResults && fallbackReport;
  const displayError = error || (status === 'error' ? '分析中断' : null);
  const completedProgress = progressItems.filter((item) => item.status === 'done').length;
  const showProgress = isStreaming || status === 'paused' || (status === 'error' && progressItems.length > 0);
  const progressSummary =
    status === 'paused'
      ? '已暂停'
      : status === 'error'
        ? '已中断'
        : progressItems.length > 0
          ? completedProgress + '/' + progressItems.length + ' 已完成'
          : '等待连接';

  if (status === 'idle' && !hasStructuredResults && !fallbackReport && !liveNarration && !decision) {
    return (
      <div className="flex min-h-40 flex-col items-center justify-center rounded-xl border border-slate-800 bg-slate-900/45 text-center text-sm text-slate-500">
        等待启动 AI 分析
      </div>
    );
  }

  if (status === 'done' && !showOutputSections && !decision && !displayError) {
    return (
      <div className="flex items-center gap-2 rounded-xl border border-amber-700/60 bg-amber-950/30 px-4 py-3 text-sm text-amber-100">
        <AlertCircle className="h-4 w-4 shrink-0" />
        未收到分析结果
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {displayError ? (
        <div className="flex items-center gap-2 rounded-xl border border-red-800 bg-red-950/60 px-4 py-3 text-sm text-red-100">
          <AlertCircle className="h-4 w-4 shrink-0" />
          {displayError}
        </div>
      ) : null}

      {decision ? (
        <section className={'rounded-xl border p-4 sm:p-5 ' + decisionClass(decision.action)}>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-baseline gap-3">
              <span className="text-sm font-bold opacity-80">操作建议</span>
              <h3 className="text-3xl font-black">{decisionLabel(decision.action)}</h3>
            </div>
            <span className="inline-flex rounded-full border border-current/30 px-3 py-1 text-sm font-bold">
              置信度 {confidenceLabel(decision.confidence)}
            </span>
          </div>
          {decision.summary ? <p className="mt-3 text-sm leading-7 opacity-90">{decision.summary}</p> : null}
        </section>
      ) : null}

      {showProgress ? (
        <section className="rounded-xl border border-slate-800 bg-slate-950/35">
          <div className="flex items-center justify-between gap-3 border-b border-slate-800 px-4 py-3">
            <span className="text-sm font-bold text-slate-100">分析进度</span>
            <span className="shrink-0 rounded-full border border-slate-700 px-2.5 py-1 text-xs font-semibold text-slate-400">
              {progressSummary}
            </span>
          </div>
          <div className="grid gap-2 p-3 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
            {progressItems.length > 0 ? (
              progressItems.map((item) => {
                return (
                  <div
                    key={item.agent}
                    className={'flex min-h-11 items-center gap-2.5 rounded-lg border px-3 py-2 text-sm transition ' + statusStyles(item.status)}
                    title={item.message || undefined}
                  >
                    <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-current/10" aria-hidden="true">
                      <StatusIcon status={item.status} />
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-bold text-slate-100">{titleForAgent(item.agent)}</span>
                      {item.status === 'error' && item.message ? <span className="mt-0.5 block text-xs text-red-100">{item.message}</span> : null}
                    </span>
                  </div>
                );
              })
            ) : (
              <div className="rounded-lg border border-slate-800 bg-slate-900/45 px-3 py-3 text-sm text-slate-500">
                正在连接分析服务...
              </div>
            )}
          </div>
        </section>
      ) : null}

      {showOutputSections ? (
        <section className="grid gap-3">
          {outputSections.map((section, index) => {
            const progressItem = section.agent ? progressByAgent.get(section.agent) : undefined;
            const isActiveSection = status === 'streaming' && section.source !== 'report' && progressItem?.status === 'running';
            const content = status === 'streaming' ? balanceStreamingMarkdown(section.content) : section.content;
            const showSectionStatus = isActiveSection || (!content && progressItem?.status !== 'done');
            return (
              <Collapsible
                key={section.key}
                className={isActiveSection ? 'border-brand-400/35 bg-brand-400/5' : undefined}
                title={
                  <span className="flex items-center justify-between gap-3">
                    <span className="min-w-0 break-words">{section.title}</span>
                    {showSectionStatus ? (
                      <span className="shrink-0 text-xs font-semibold text-brand-300">
                        {sectionStatusLabel(section, progressItem, status)}
                      </span>
                    ) : null}
                  </span>
                }
                defaultOpen={isActiveSection || index === 0}
              >
                {content ? (
                  <MarkdownContent content={content} className="leading-8" />
                ) : (
                  <p className="text-sm leading-7 text-slate-500">{emptySectionText(status, progressItem)}</p>
                )}
                {isActiveSection ? (
                  <span className="analysis-cursor ml-1 inline-block h-4 w-2 translate-y-0.5 bg-brand-300" />
                ) : null}
              </Collapsible>
            );
          })}
        </section>
      ) : null}

      {showFallbackReport ? (
        <Collapsible title="综合报告" defaultOpen>
          <MarkdownContent content={fallbackReport} />
          {status === 'streaming' ? <span className="analysis-cursor ml-1 inline-block h-4 w-2 translate-y-0.5 bg-brand-300" /> : null}
        </Collapsible>
      ) : null}
    </div>
  );
}
