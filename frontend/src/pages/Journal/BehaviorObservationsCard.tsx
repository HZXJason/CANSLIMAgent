import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { fetchBehaviorObservations, updateBehaviorObservationFeedback } from '../../lib/tradeReviewApi';
import type { BehaviorObservation, BehaviorObservationFeedback } from '../../types/tradeReview';

export default function BehaviorObservationsCard() {
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: ['behavior-observations'], queryFn: fetchBehaviorObservations, retry: false });
  const mutation = useMutation({
    mutationFn: ({ id, feedback }: { id: string; feedback: BehaviorObservationFeedback }) => updateBehaviorObservationFeedback(id, feedback),
    onSuccess: (saved) => queryClient.setQueryData<BehaviorObservation[]>(['behavior-observations'], (current) => current?.map((item) => item.id === saved.id ? saved : item))
  });
  if (query.isLoading) return <Card className="h-28 animate-pulse bg-slate-900/60" />;
  if (!query.data || query.data.length === 0) return null;
  return (
    <Card className="p-5 sm:p-6">
      <h2 className="text-lg font-bold text-white">行为观察</h2>
      <div className="mt-4 space-y-3">
        {query.data.map((observation) => (
          <div className="rounded-xl border border-slate-800 bg-slate-950/35 p-4" key={observation.id}>
            <p className="text-sm font-bold text-slate-100">{observation.message}</p>
            <p className="mt-2 text-xs text-slate-500">样本 {observation.sample_count} · 置信度 {Math.round(observation.confidence * 100)}% · 证据 {observation.evidence_review_ids?.length ?? 0} 条</p>
            {observation.status === 'insufficient' ? <p className="mt-2 text-xs font-bold text-amber-200">样本不足，暂不形成稳定观察。</p> : null}
            {observation.status !== 'insufficient' ? <div className="mt-3 flex flex-wrap gap-2">
              <Button variant="outline" onClick={() => mutation.mutate({ id: observation.id, feedback: 'accepted' })}>认可</Button>
              <Button variant="outline" onClick={() => mutation.mutate({ id: observation.id, feedback: 'not_applicable' })}>不适用于我</Button>
              <Button variant="outline" onClick={() => mutation.mutate({ id: observation.id, feedback: 'needs_more_evidence' })}>需要更多证据</Button>
            </div> : null}
          </div>
        ))}
      </div>
    </Card>
  );
}
