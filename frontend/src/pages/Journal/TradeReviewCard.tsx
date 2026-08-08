import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { Link2, Save } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Select } from '../../components/ui/select';
import { createTradeReview, fetchJournalPlanCandidates, fetchTradeReview, linkJournalToPlan, updateTradeReview } from '../../lib/tradeReviewApi';
import type { ProcessQuality, TradeReview } from '../../types/tradeReview';

const unknownLabel: Record<string, string> = {
  planned_quantity: '计划数量',
  actual_exit: '真实退出',
  stop_execution: '止损执行',
  planned_entry_price: '计划入场价'
};

export default function TradeReviewCard({ journalId }: { journalId: string }) {
  const queryClient = useQueryClient();
  const reviewQuery = useQuery({ queryKey: ['trade-review', journalId], queryFn: () => fetchTradeReview(journalId), retry: false });
  const review = reviewQuery.data;
  const [selectedPlan, setSelectedPlan] = useState('');
  const [processQuality, setProcessQuality] = useState<ProcessQuality>('unreviewed');
  const [deviationReason, setDeviationReason] = useState('');
  const [emotionBefore, setEmotionBefore] = useState('');
  const [emotionAfter, setEmotionAfter] = useState('');
  const [outcomeNotes, setOutcomeNotes] = useState('');
  const [lesson, setLesson] = useState('');
  const [complete, setComplete] = useState(false);

  useEffect(() => {
    if (!review) return;
    setProcessQuality(review.process_quality);
    setDeviationReason(review.deviation_reason ?? '');
    setEmotionBefore(review.emotion_before ?? '');
    setEmotionAfter(review.emotion_after ?? '');
    setOutcomeNotes(review.outcome_notes ?? '');
    setLesson(review.lesson ?? '');
    setComplete(Boolean(review.completed_at));
  }, [review]);

  const candidatesQuery = useQuery({
    queryKey: ['trade-review-plan-candidates', journalId],
    queryFn: () => fetchJournalPlanCandidates(journalId),
    enabled: review?.diff.plan_link_status === 'off_plan'
  });
  const createMutation = useMutation({
    mutationFn: () => createTradeReview(journalId),
    onSuccess: (saved) => queryClient.setQueryData(['trade-review', journalId], saved)
  });
  const linkMutation = useMutation({
    mutationFn: () => linkJournalToPlan(journalId, selectedPlan),
    onSuccess: (saved) => {
      queryClient.setQueryData(['trade-review', journalId], saved);
      queryClient.invalidateQueries({ queryKey: ['journal-entry', journalId] });
    }
  });
  const saveMutation = useMutation({
    mutationFn: () => updateTradeReview(journalId, {
      process_quality: processQuality,
      deviation_reason: deviationReason.trim() || null,
      emotion_before: emotionBefore.trim() || null,
      emotion_after: emotionAfter.trim() || null,
      outcome_notes: outcomeNotes.trim() || null,
      lesson: lesson.trim() || null,
      complete
    }),
    onSuccess: (saved) => queryClient.setQueryData(['trade-review', journalId], saved)
  });

  if (reviewQuery.isLoading) return <Card className="h-40 animate-pulse bg-slate-900/60" />;
  if (!review) {
    return (
      <Card className="p-5 sm:p-6">
        <h2 className="text-lg font-bold text-white">计划与执行复盘</h2>
        <Button className="mt-4" onClick={() => createMutation.mutate()} disabled={createMutation.isPending}>{createMutation.isPending ? '生成中' : '开始复盘'}</Button>
      </Card>
    );
  }

  const slippage = review.diff.entry_slippage_percent;
  return (
    <Card className="overflow-hidden p-0">
      <div className="border-b border-slate-800 px-5 py-4 sm:px-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div><h2 className="text-lg font-bold text-white">计划与执行复盘</h2><p className="mt-1 text-xs text-slate-500">版本 {review.version} · 快照不会随当前数据改写</p></div>
          <span className={review.diff.plan_link_status === 'linked' ? 'text-sm font-bold text-emerald-200' : 'text-sm font-bold text-amber-200'}>
            {review.diff.plan_link_status === 'linked' ? '计划内成交' : '计划外成交'}
          </span>
        </div>
      </div>
      <div className="space-y-6 p-5 sm:p-6">
        {review.diff.plan_link_status === 'off_plan' ? (
          <div className="rounded-xl border border-amber-500/35 bg-amber-950/25 p-4">
            <p className="text-sm font-bold text-amber-100">这条成交尚未关联计划，系统不会自动猜测。</p>
            <div className="mt-3 flex flex-col gap-2 sm:flex-row">
              <label className="flex-1 text-sm font-semibold text-slate-300">关联交易计划
                <select aria-label="关联交易计划" className="mt-2 min-h-10 w-full rounded-xl border border-slate-700 bg-slate-950 px-3 text-sm" value={selectedPlan} onChange={(event) => setSelectedPlan(event.target.value)}>
                  <option value="">选择同标的同方向计划</option>
                  {(candidatesQuery.data ?? []).map((plan) => <option key={plan.id} value={plan.id}>{plan.ticker} · {plan.plan_date} · {plan.status}</option>)}
                </select>
              </label>
              <Button className="self-end" disabled={!selectedPlan || linkMutation.isPending} onClick={() => linkMutation.mutate()}><Link2 className="h-4 w-4" />确认关联</Button>
            </div>
          </div>
        ) : review.trade_plan_id ? <Link className="text-sm font-bold text-brand-200" to={'/trade-plans/' + review.trade_plan_id}>查看原交易计划</Link> : null}

        <section>
          <h3 className="text-sm font-black text-slate-200">计划—执行差异</h3>
          <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <div className="rounded-xl bg-slate-950/40 p-3 text-sm text-slate-300">方向：{review.diff.direction_match === null ? '未知' : review.diff.direction_match ? '一致' : '不一致'}</div>
            <div className="rounded-xl bg-slate-950/40 p-3 text-sm text-slate-300">{slippage === null ? '入场偏差：未知' : `入场不利偏差 ${slippage.toFixed(2)}%`}</div>
            <div className="rounded-xl bg-slate-950/40 p-3 text-sm text-slate-300">时间：{review.diff.timing_status === 'within_plan_window' ? '计划窗口内' : review.diff.timing_status}</div>
            {review.diff.unknown_comparisons.map((item) => <div className="rounded-xl border border-dashed border-slate-700 p-3 text-sm text-slate-500" key={item}>{unknownLabel[item] ?? item}：未知</div>)}
          </div>
        </section>

        <section className="grid gap-4 border-t border-slate-800 pt-5 lg:grid-cols-2">
          <div className="space-y-4">
            <h3 className="text-sm font-black text-slate-200">过程复盘</h3>
            <label className="block text-sm font-semibold text-slate-300">过程质量<Select aria-label="过程质量" className="mt-2" value={processQuality} onValueChange={(value) => setProcessQuality(value as ProcessQuality)}><option value="unreviewed">未评价</option><option value="followed">按计划执行</option><option value="adjusted">有依据地调整</option><option value="deviated">偏离计划</option></Select></label>
            <label className="block text-sm font-semibold text-slate-300">偏离或调整原因<textarea className="mt-2 min-h-20 w-full rounded-xl border border-slate-700 bg-slate-950/55 p-3" value={deviationReason} onChange={(event) => setDeviationReason(event.target.value)} /></label>
            <div className="grid gap-3 sm:grid-cols-2"><label className="text-sm font-semibold text-slate-300">入场前情绪<input className="mt-2 min-h-10 w-full rounded-xl border border-slate-700 bg-slate-950 px-3" value={emotionBefore} onChange={(event) => setEmotionBefore(event.target.value)} /></label><label className="text-sm font-semibold text-slate-300">执行后情绪<input className="mt-2 min-h-10 w-full rounded-xl border border-slate-700 bg-slate-950 px-3" value={emotionAfter} onChange={(event) => setEmotionAfter(event.target.value)} /></label></div>
          </div>
          <div className="space-y-4">
            <h3 className="text-sm font-black text-slate-200">结果记录</h3>
            <label className="block text-sm font-semibold text-slate-300">结果记录<textarea aria-label="结果记录" className="mt-2 min-h-24 w-full rounded-xl border border-slate-700 bg-slate-950/55 p-3" value={outcomeNotes} onChange={(event) => setOutcomeNotes(event.target.value)} /></label>
            <label className="block text-sm font-semibold text-slate-300">下一次的具体改进<textarea className="mt-2 min-h-24 w-full rounded-xl border border-slate-700 bg-slate-950/55 p-3" value={lesson} onChange={(event) => setLesson(event.target.value)} /></label>
            <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={complete} onChange={(event) => setComplete(event.target.checked)} />标记为完整复盘</label>
          </div>
        </section>
        <div className="flex justify-end"><Button onClick={() => saveMutation.mutate()} disabled={saveMutation.isPending || processQuality === 'unreviewed'}><Save className="h-4 w-4" />保存复盘</Button></div>
      </div>
    </Card>
  );
}
