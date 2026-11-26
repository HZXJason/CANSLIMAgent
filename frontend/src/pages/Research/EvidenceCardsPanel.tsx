import { useMutation, useQuery } from '@tanstack/react-query';
import { AlertTriangle, CheckCircle2, HelpCircle, Sparkles } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { FALLBACK_AI_ANALYSIS_OPTIONS, normalizeAIAnalysisConfig, readStoredAIAnalysisConfig } from '../../lib/aiAnalysisConfig';
import { compileEvidenceCards } from '../../lib/evidenceCardsApi';
import { cn } from '../../lib/utils';
import type { EvidenceCardsPack } from '../../types/evidenceCards';

export default function EvidenceCardsPanel({
  symbol,
  onPackLoaded
}: {
  symbol: string;
  onPackLoaded?: (pack: EvidenceCardsPack) => void;
}) {
  const query = useQuery({
    queryKey: ['evidence-cards', symbol],
    queryFn: async () => {
      const pack = await compileEvidenceCards(symbol, { include_ai: false });
      onPackLoaded?.(pack);
      return pack;
    }
  });

  const aiMutation = useMutation({
    mutationFn: () => {
      const config = normalizeAIAnalysisConfig(readStoredAIAnalysisConfig(), FALLBACK_AI_ANALYSIS_OPTIONS);
      return compileEvidenceCards(symbol, {
        include_ai: true,
        llm_provider: config.llm_provider,
        model: config.quick_think_llm,
        backend_url: config.backend_url
      });
    },
    onSuccess: (updated) => {
      onPackLoaded?.(updated);
    }
  });

  const pack = aiMutation.data ?? query.data;

  return (
    <Card className="p-5">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-rule pb-3">
        <div>
          <h2 className="text-base font-bold text-white">结构化审计证据卡片</h2>
          <p className="mt-0.5 text-xs text-slate-400">
            先于 AI 结论核验来源与期间；缺失字段诚实显示为未知
            {pack?.data_as_of ? ` · 截至 ${pack.data_as_of}` : ''}
          </p>
        </div>
        <Button
          variant="outline"
          className="h-9 px-3 text-xs"
          disabled={aiMutation.isPending || query.isLoading}
          onClick={() => aiMutation.mutate()}
        >
          <Sparkles className="h-3.5 w-3.5" />
          {aiMutation.isPending ? '提炼中...' : 'AI 提炼多空反证 (<5s)'}
        </Button>
      </div>

      {query.isLoading ? (
        <div className="mt-4 grid gap-3 sm:grid-cols-2">
          {Array.from({ length: 4 }).map((_, idx) => (
            <div key={idx} className="h-28 animate-pulse rounded-sm bg-slate-900/60" />
          ))}
        </div>
      ) : null}

      {pack ? (
        <div className="mt-4 space-y-4">
          <div className="grid gap-3 sm:grid-cols-2">
            {pack.cards.map((card) => (
              <div key={card.id} className="rounded-sm border border-rule bg-slate-950/55 p-3.5">
                <h3 className="text-xs font-black tracking-wide text-brand-200">{card.title}</h3>
                <dl className="mt-2.5 space-y-1.5 text-xs">
                  {card.items.map((item, idx) => (
                    <div key={idx} className="flex items-start justify-between gap-2">
                      <dt className="text-slate-400">{item.label}</dt>
                      <dd
                        className={cn(
                          'font-data text-right font-semibold',
                          item.status === 'ok' && 'text-slate-100',
                          item.status === 'unknown' && 'text-slate-500',
                          item.status === 'warning' && 'text-amber-300'
                        )}
                      >
                        {item.value}
                      </dd>
                    </div>
                  ))}
                </dl>
              </div>
            ))}
          </div>

          {pack.ai_status === 'unavailable' && pack.ai_error ? (
            <div className="rounded-sm border border-amber-500/40 bg-amber-500/10 px-3.5 py-2.5 text-xs text-amber-100">
              {pack.ai_error}
            </div>
          ) : null}

          {pack.counter_thesis ? (
            <div className="grid gap-3 border-t border-rule pt-4 md:grid-cols-3">
              <div className="rounded-sm border border-emerald-500/30 bg-emerald-950/20 p-3">
                <h4 className="flex items-center gap-1.5 text-xs font-bold text-emerald-200">
                  <CheckCircle2 className="h-3.5 w-3.5" />看多核心锚点
                </h4>
                <ul className="mt-2 space-y-2 text-xs text-slate-300">
                  {pack.counter_thesis.bull_anchors.map((item, i) => (
                    <li key={i}>
                      <p className="font-bold text-white">{item.title}</p>
                      <p className="mt-0.5 text-slate-400">{item.detail}</p>
                    </li>
                  ))}
                </ul>
              </div>
              <div className="rounded-sm border border-rose-500/30 bg-rose-950/20 p-3">
                <h4 className="flex items-center gap-1.5 text-xs font-bold text-rose-200">
                  <AlertTriangle className="h-3.5 w-3.5" />反证 / 证伪条件
                </h4>
                <ul className="mt-2 space-y-2 text-xs text-slate-300">
                  {pack.counter_thesis.bear_falsifiers.map((item, i) => (
                    <li key={i}>
                      <p className="font-bold text-white">{item.title}</p>
                      <p className="mt-0.5 text-slate-400">{item.detail}</p>
                    </li>
                  ))}
                </ul>
              </div>
              <div className="rounded-sm border border-amber-500/30 bg-amber-950/20 p-3">
                <h4 className="flex items-center gap-1.5 text-xs font-bold text-amber-200">
                  <HelpCircle className="h-3.5 w-3.5" />信息盲点
                </h4>
                <ul className="mt-2 space-y-2 text-xs text-slate-300">
                  {pack.counter_thesis.blind_spots.map((item, i) => (
                    <li key={i}>
                      <p className="font-bold text-white">{item.title}</p>
                      <p className="mt-0.5 text-slate-400">{item.detail}</p>
                    </li>
                  ))}
                </ul>
              </div>
            </div>
          ) : null}
        </div>
      ) : null}
    </Card>
  );
}
