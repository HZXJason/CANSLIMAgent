import { PauseCircle, PlayCircle, Sparkles } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
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
import AICalibrationCard from './AICalibrationCard';

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
	const [currentConfig, setCurrentConfig] = useState<AnalysisConfig | null>(lastConfigRef.current);
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
		setCurrentConfig(config);
    start(config, buildAIAnalysisContext(state));
  };

	const handleConfigChange = useCallback((config: AnalysisConfig) => {
		lastConfigRef.current = config;
		setCurrentConfig(config);
	}, []);

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
    <section className="space-y-5">
      <AIAnalysisConfig symbol={symbol} onStartAnalysis={requestAnalysis} isAnalyzing={isAnalyzing} onConfigChange={handleConfigChange} />

			{currentConfig ? <AICalibrationCard symbol={symbol} config={currentConfig} disabled={isAnalyzing} /> : null}

      <Card className="space-y-5 border-cyan-400/20 bg-slate-950/70">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 className="text-lg font-black text-white">多智能体 AI 分析</h2>
            <p className="mt-1 text-sm text-slate-500">流式输出研究过程、分角色报告和最终建议</p>
          </div>
          {isAnalyzing ? (
            <div className="flex flex-wrap items-center gap-2">
              <span className="inline-flex items-center gap-2 rounded-full border border-cyan-400/30 bg-cyan-400/10 px-3 py-1 text-xs font-bold text-cyan-200">
                <span className="h-2 w-2 rounded-full bg-cyan-300" />
                生成中
              </span>
              <Button variant="outline" className="h-9 rounded-xl px-3 text-xs" onClick={abort}>
                <PauseCircle className="h-4 w-4" />
                暂停接收
              </Button>
            </div>
          ) : canResume ? (
            <Button variant="outline" className="h-9 rounded-xl px-3 text-xs" onClick={resumeAnalysis}>
              <PlayCircle className="h-4 w-4" />
              继续上次分析
            </Button>
          ) : null}
        </div>

        {!hasOutput && state.aiStatus === 'idle' ? (
          <div className="flex min-h-40 flex-col items-center justify-center gap-4 rounded-xl border border-slate-800 bg-slate-900/45 text-center">
            <Sparkles className="h-10 w-10 text-cyan-300" />
            <p className="text-sm text-slate-500">调整上方参数后启动分析</p>
          </div>
        ) : (
          <>
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
            {state.aiStatus === 'paused' && state.aiRunId ? (
              <p className="rounded-xl border border-amber-300/20 bg-amber-400/5 px-3 py-2 text-xs text-amber-100">
                流式接收已暂停；sidecar 会保留运行和事件游标，点击“继续上次分析”只补发缺失内容。
              </p>
            ) : null}
            {state.aiStatus === 'done' || state.aiStatus === 'error' || state.aiStatus === 'paused' ? (
              <AIActions symbol={symbol} reportContent={state.reportContent} nLabelWritten={state.nLabelWritten} onRegenerate={requestRegenerate} />
            ) : null}
          </>
        )}
      </Card>

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
