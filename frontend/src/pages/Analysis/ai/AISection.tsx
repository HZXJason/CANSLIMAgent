import { Sparkles } from 'lucide-react';
import { useRef, useState } from 'react';
import AIAnalysisConfig, { type AnalysisConfig } from '../../../components/analysis/AIAnalysisConfig';
import AIAnalysisStream from '../../../components/analysis/AIAnalysisStream';
import { Card } from '../../../components/ui/card';
import { ConfirmDialog } from '../../../components/ui/confirm-dialog';
import { useAIStream } from '../../../hooks/useAIStream';
import type { SymbolAnalysisState } from '../../../types/analysis';
import AIActions from './AIActions';

interface AISectionProps {
  symbol: string;
  state: SymbolAnalysisState;
}

export default function AISection({ symbol, state }: AISectionProps) {
  const { start } = useAIStream(symbol);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [pendingConfig, setPendingConfig] = useState<AnalysisConfig | null>(null);
  const lastConfigRef = useRef<AnalysisConfig | null>(null);
  const isAnalyzing = state.aiStatus === 'streaming';
  const hasOutput =
    state.thoughtLines.length > 0 ||
    state.reportContent.length > 0 ||
    state.aiProgress.length > 0 ||
    state.aiResults.length > 0 ||
    Boolean(state.aiDecision);

  const runAnalysis = (config: AnalysisConfig) => {
    lastConfigRef.current = config;
    start(config);
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

  return (
    <section className="space-y-5">
      <AIAnalysisConfig symbol={symbol} onStartAnalysis={requestAnalysis} isAnalyzing={isAnalyzing} />

      <Card className="space-y-5 border-cyan-400/20 bg-slate-950/70">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 className="text-lg font-black text-white">多智能体 AI 分析</h2>
            <p className="mt-1 text-sm text-slate-500">流式输出研究过程、分角色报告和最终建议</p>
          </div>
          {isAnalyzing ? (
            <span className="inline-flex items-center gap-2 rounded-full border border-cyan-400/30 bg-cyan-400/10 px-3 py-1 text-xs font-bold text-cyan-200">
              <span className="h-2 w-2 rounded-full bg-cyan-300" />
              生成中
            </span>
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
              fallbackReport={state.reportContent}
            />
            {state.aiStatus === 'done' || state.aiStatus === 'error' ? (
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
