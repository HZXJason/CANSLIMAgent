import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ExternalLink, ShieldAlert, Sparkles } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { FALLBACK_AI_ANALYSIS_OPTIONS, normalizeAIAnalysisConfig, readStoredAIAnalysisConfig } from '../../lib/aiAnalysisConfig';
import { acknowledgePreTradeFinding, createPreTradeReview, fetchLatestPreTradeReview } from '../../lib/pretradeReviewApi';
import type { PreTradeFinding, PreTradeRiskReview, PreTradeSource } from '../../types/pretradeReview';

const severityLabel = { high: '高', medium: '中', low: '低' } as const;
const severityClass = {
  high: 'border-red-500/50 bg-red-950/35 text-red-100',
  medium: 'border-amber-500/45 bg-amber-950/30 text-amber-100',
  low: 'border-slate-700 bg-slate-950/30 text-slate-200'
} as const;

function sourceIDs(finding: PreTradeFinding) {
  const value = finding.evidence.source_ids;
  return Array.isArray(value) ? value.filter((id): id is string => typeof id === 'string') : [];
}

function FindingList({
  title,
  findings,
  sources,
  acknowledged,
  onAcknowledge
}: {
  title: string;
  findings: PreTradeFinding[];
  sources: Map<string, PreTradeSource>;
  acknowledged: Set<string>;
  onAcknowledge: (finding: PreTradeFinding) => void;
}) {
  if (findings.length === 0) return null;
  return (
    <section>
      <h3 className="text-sm font-black text-slate-200">{title}</h3>
      <div className="mt-2 space-y-2">
        {findings.map((finding) => (
          <div className={'rounded-xl border px-4 py-3 ' + severityClass[finding.severity]} key={finding.code}>
            <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
              <div>
                <p className="text-sm font-bold">{finding.message}</p>
                <p className="mt-1 text-xs opacity-70">风险等级：{severityLabel[finding.severity]}{finding.field ? ' · 字段：' + finding.field : ''}</p>
              </div>
              {finding.severity === 'high' ? (
                acknowledged.has(finding.code)
                  ? <span className="text-xs font-bold text-emerald-200">已确认</span>
                  : <Button className="shrink-0" variant="outline" onClick={() => onAcknowledge(finding)}>确认已知晓</Button>
              ) : null}
            </div>
            {sourceIDs(finding).length > 0 ? (
              <div className="mt-2 flex flex-wrap gap-2">
                {sourceIDs(finding).map((id) => {
                  const source = sources.get(id);
                  if (!source) return <span className="text-xs opacity-70" key={id}>来源 {id} 不可用</span>;
                  return source.url ? (
                    <a className="inline-flex items-center gap-1 text-xs font-bold underline underline-offset-2" href={source.url} key={id} rel="noreferrer" target="_blank">
                      {source.label}<ExternalLink className="h-3 w-3" />
                    </a>
                  ) : <span className="text-xs font-bold" key={id}>{source.label}</span>;
                })}
              </div>
            ) : null}
          </div>
        ))}
      </div>
    </section>
  );
}

export default function PreTradeRiskReviewCard({ planId }: { planId: string }) {
  const queryClient = useQueryClient();
  const [pendingFinding, setPendingFinding] = useState<PreTradeFinding | null>(null);
  const [reason, setReason] = useState('');
  const latestQuery = useQuery({
    queryKey: ['pretrade-review', planId, 'latest'],
    queryFn: () => fetchLatestPreTradeReview(planId),
    retry: false
  });
  const createMutation = useMutation({
    mutationFn: (runAI: boolean) => {
      const config = normalizeAIAnalysisConfig(readStoredAIAnalysisConfig(), FALLBACK_AI_ANALYSIS_OPTIONS);
      return createPreTradeReview(planId, {
        run_ai: runAI,
        llm_provider: runAI ? config.llm_provider : undefined,
        model: runAI ? config.quick_think_llm : undefined,
        backend_url: runAI ? config.backend_url : undefined,
        current_evidence: {}
      });
    },
    onSuccess: (saved) => queryClient.setQueryData(['pretrade-review', planId, 'latest'], saved)
  });
  const acknowledgeMutation = useMutation({
    mutationFn: ({ review, finding, acknowledgementReason }: { review: PreTradeRiskReview; finding: PreTradeFinding; acknowledgementReason: string }) =>
      acknowledgePreTradeFinding(planId, review.id, { finding_code: finding.code, reason: acknowledgementReason.trim() || null }),
    onSuccess: (saved, variables) => {
      queryClient.setQueryData<PreTradeRiskReview>(['pretrade-review', planId, 'latest'], (current) => current ? {
        ...current,
        acknowledgements: [...current.acknowledgements, saved]
      } : current);
      setPendingFinding(null);
      setReason('');
    }
  });

  const review = latestQuery.data;
  const sourceMap = new Map((review?.sources ?? []).map((source) => [source.id, source]));
  const acknowledged = new Set((review?.acknowledgements ?? []).map((item) => item.finding_code));
  const runButtons = (
    <div className="flex flex-col gap-2 sm:flex-row">
      <Button variant="outline" disabled={createMutation.isPending} onClick={() => createMutation.mutate(false)}>
        <ShieldAlert className="h-4 w-4" />仅检查计划冲突
      </Button>
      <Button disabled={createMutation.isPending} onClick={() => createMutation.mutate(true)}>
        <Sparkles className="h-4 w-4" />{createMutation.isPending ? '审查中' : '运行快速风险审查'}
      </Button>
    </div>
  );

  return (
    <Card className="p-5 sm:p-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-lg font-bold text-white">入场前风险审查</h2>
          {review ? <p className="mt-2 text-xs font-semibold text-slate-500">版本 {review.version} · {new Date(review.created_at).toLocaleString()}</p> : null}
        </div>
        {runButtons}
      </div>

      {latestQuery.isLoading ? <div className="mt-5 h-24 animate-pulse rounded-xl bg-slate-800/60" /> : null}
      {!latestQuery.isLoading && !review ? <p className="mt-5 text-sm text-slate-400">尚未运行风险审查。</p> : null}
      {createMutation.isError ? <p className="mt-4 text-sm text-red-300">{createMutation.error instanceof Error ? createMutation.error.message : '风险审查失败'}</p> : null}

      {review ? (
        <div className="mt-5 space-y-5">
          <div className={review.ai_status === 'completed' ? 'rounded-xl border border-emerald-500/35 bg-emerald-950/25 px-4 py-3 text-sm text-emerald-100' : 'rounded-xl border border-amber-500/40 bg-amber-950/25 px-4 py-3 text-sm text-amber-100'}>
            {review.ai_status === 'completed'
              ? `AI 风险补充已完成 · ${review.ai_provider ?? '-'} / ${review.ai_model ?? '-'}`
              : review.ai_status === 'not_requested'
                ? '本版本仅包含确定性校验，未请求 AI。'
                : 'AI 审查不可用；下方确定性结果仍然有效，未生成替代内容。'}
          </div>
          <FindingList title="确定性冲突" findings={review.hard_conflicts} sources={sourceMap} acknowledged={acknowledged} onAcknowledge={setPendingFinding} />
          <FindingList title="风险提醒" findings={review.risk_warnings} sources={sourceMap} acknowledged={acknowledged} onAcknowledge={setPendingFinding} />
          <FindingList title="行为提醒" findings={review.behavior_reminders} sources={sourceMap} acknowledged={acknowledged} onAcknowledge={setPendingFinding} />
          <FindingList title="待确认问题" findings={review.unresolved_questions} sources={sourceMap} acknowledged={acknowledged} onAcknowledge={setPendingFinding} />
          <FindingList title="研究后变化" findings={review.changes_since_research} sources={sourceMap} acknowledged={acknowledged} onAcknowledge={setPendingFinding} />
          {Object.keys(review.freshness).length > 0 ? (
            <div><h3 className="text-sm font-black text-slate-200">数据新鲜度</h3><pre className="mt-2 overflow-x-auto rounded-xl bg-slate-950/50 p-3 text-xs text-slate-400">{JSON.stringify(review.freshness, null, 2)}</pre></div>
          ) : null}
        </div>
      ) : null}

      {pendingFinding && review ? (
        <div className="mt-5 rounded-xl border border-slate-700 bg-slate-950/45 p-4">
          <p className="text-sm font-bold text-white">确认你已看到：{pendingFinding.message}</p>
          <label className="mt-3 block text-sm font-semibold text-slate-300">
            确认理由（可选）
            <textarea aria-label="确认理由（可选）" className="mt-2 min-h-20 w-full rounded-xl border border-slate-700 bg-slate-950/60 p-3 text-sm text-white" value={reason} onChange={(event) => setReason(event.target.value)} />
          </label>
          <div className="mt-3 flex justify-end gap-2">
            <Button variant="outline" onClick={() => setPendingFinding(null)}>取消</Button>
            <Button disabled={acknowledgeMutation.isPending} onClick={() => acknowledgeMutation.mutate({ review, finding: pendingFinding, acknowledgementReason: reason })}>保存确认</Button>
          </div>
        </div>
      ) : null}
    </Card>
  );
}
