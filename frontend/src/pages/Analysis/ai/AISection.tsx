import { CheckCircle2, PauseCircle, PlayCircle } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import AIAnalysisConfig, { type AnalysisConfig } from '../../../components/analysis/AIAnalysisConfig';
import AIAnalysisStream from '../../../components/analysis/AIAnalysisStream';
import { Button } from '../../../components/ui/button';
import { Card } from '../../../components/ui/card';
import { ConfirmDialog } from '../../../components/ui/confirm-dialog';
import { useAIStream } from '../../../hooks/useAIStream';
import { buildAIAnalysisContext } from '../../../lib/aiAnalysisContext';
import { readStoredAIAnalysisConfig } from '../../../lib/aiAnalysisConfig';
import { markAIAnalysisCompleted } from '../../../lib/api';
import { useAnalysisStore } from '../../../stores/analysisStore';
import type { SymbolAnalysisState } from '../../../types/analysis';
import AIActions from './AIActions';

interface AISectionProps {
  symbol: string;
  state: SymbolAnalysisState;
}

export default function AISection({ symbol, state }: AISectionProps) {
	const { start, resume, abort } = useAIStream(symbol);
	const queryClient = useQueryClient();
	const updateState = useAnalysisStore((store) => store.updateState);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [pendingConfig, setPendingConfig] = useState<AnalysisConfig | null>(null);
	const lastConfigRef = useRef<AnalysisConfig | null>(readStoredAIAnalysisConfig() as AnalysisConfig | null);
	const markedRunRef = useRef<string | null>(null);
  const isAnalyzing = state.aiStatus === 'streaming';
  const hasOutput =
    state.thoughtLines.length > 0 ||
    state.liveNarration.length > 0 ||
    Object.keys(state.liveNarrationByAgent).length > 0 ||
    state.reportContent.length > 0 ||
    state.aiProgress.length > 0 ||
    state.aiResults.length > 0 ||
    Boolean(state.aiDecision);
  const canResume = Boolean(state.aiRunId) && (state.aiStatus === 'paused' || state.aiStatus === 'error');
  const showResults = hasOutput || state.aiStatus !== 'idle';

  const statusBadge = (() => {
    if (state.aiStatus === 'streaming') {
      return (
        <span className="inline-flex items-center gap-2 rounded-full border border-cyan-400/30 bg-cyan-400/10 px-3 py-1.5 text-xs font-bold text-cyan-200">
          <span className="h-2 w-2 animate-pulse rounded-full bg-cyan-300" />
          生成中
        </span>
      );
    }
    if (state.aiStatus === 'done') {
      return (
        <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-400/30 bg-emerald-400/10 px-3 py-1.5 text-xs font-bold text-emerald-200">
          <CheckCircle2 className="h-3.5 w-3.5" />
          已完成
        </span>
      );
    }
    if (state.aiStatus === 'paused') return <span className="rounded-full border border-amber-300/30 bg-amber-300/10 px-3 py-1.5 text-xs font-bold text-amber-100">已暂停</span>;
    if (state.aiStatus === 'error') return <span className="rounded-full border border-red-400/30 bg-red-400/10 px-3 py-1.5 text-xs font-bold text-red-200">已中断</span>;
    return null;
  })();

	useEffect(() => {
		if (state.aiStatus !== 'done' || !state.aiRunId || state.stockSummary?.ai_analyzed || markedRunRef.current === state.aiRunId) return;
		const runID = state.aiRunId;
		markedRunRef.current = runID;
		void markAIAnalysisCompleted(symbol)
			.then(() => {
				if (state.stockSummary) {
					updateState(symbol, { stockSummary: { ...state.stockSummary, ai_analyzed: true } });
				}
				return queryClient.invalidateQueries({ queryKey: ['screener-results'] });
			})
			.catch(() => {
				if (markedRunRef.current === runID) markedRunRef.current = null;
			});
	}, [queryClient, state.aiRunId, state.aiStatus, state.stockSummary, symbol, updateState]);

  const runAnalysis = (config: AnalysisConfig) => {
    lastConfigRef.current = config;
    start(config, buildAIAnalysisContext(state));
  };

  const requestAnalysis = (config: AnalysisConfig) => {
    if (hasOutput && !isAnalyzing) {
      setPendingConfig(config);
      setConfirmOpen(true);
      return;
    }
    runAnalysis(config);
  };

  const requestRegenerate = () => {
    if (!lastConfigRef.current) return;
    setPendingConfig(lastConfigRef.current);
    setConfirmOpen(true);
  };

  const resumeAnalysis = () => {
    const config = lastConfigRef.current ?? (readStoredAIAnalysisConfig() as AnalysisConfig | null);
    if (!config || !state.aiRunId) return;
    lastConfigRef.current = config;
    resume(config, buildAIAnalysisContext(state));
  };

  return (
    <section className="space-y-4">
      <AIAnalysisConfig onStartAnalysis={requestAnalysis} isAnalyzing={isAnalyzing} />

      {showResults ? <Card className="space-y-4 border-cyan-400/20 bg-slate-950/70 p-4 sm:p-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <h2 className="text-lg font-black text-white">分析结果</h2>
          <div className="flex flex-wrap items-center gap-2">
            {statusBadge}
            {isAnalyzing ? (
              <Button variant="outline" className="h-9 rounded-xl px-3 text-xs" onClick={abort}>
                <PauseCircle className="h-4 w-4" />
                暂停
              </Button>
            ) : canResume ? (
              <Button variant="outline" className="h-9 rounded-xl px-3 text-xs" onClick={resumeAnalysis}>
                <PlayCircle className="h-4 w-4" />
                继续分析
              </Button>
            ) : null}
          </div>
        </div>

        <AIAnalysisStream
          status={state.aiStatus}
          progress={state.aiProgress}
          results={state.aiResults}
          decision={state.aiDecision}
          error={state.aiError}
          liveNarration={state.liveNarration}
          liveNarrationByAgent={state.liveNarrationByAgent}
          fallbackReport={state.reportContent}
        />
        {state.aiStatus === 'done' || state.aiStatus === 'error' || state.aiStatus === 'paused' ? (
          <div className="border-t border-slate-800 pt-4">
            <AIActions
              symbol={symbol}
              reportContent={state.reportContent}
              nLabelWritten={state.nLabelWritten}
              analysisComplete={state.aiStatus === 'done'}
              onRegenerate={requestRegenerate}
            />
          </div>
        ) : null}
      </Card> : null}

      <ConfirmDialog
        open={confirmOpen}
        title="确认重新分析"
        description="重新分析会覆盖当前 AI 分析进度、报告和最终建议，是否继续？"
        cancelText="取消"
        confirmText="确认重新分析"
        onConfirm={() => {
          if (pendingConfig) runAnalysis(pendingConfig);
          setPendingConfig(null);
        }}
        onOpenChange={(open) => {
          setConfirmOpen(open);
          if (!open) setPendingConfig(null);
        }}
      />
    </section>
  );
}
