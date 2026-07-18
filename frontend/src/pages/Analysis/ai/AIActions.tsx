import { Check, Download, RefreshCw, Tag } from 'lucide-react';
import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Button } from '../../../components/ui/button';
import { updateNLabel } from '../../../lib/api';
import { useAnalysisStore } from '../../../stores/analysisStore';

interface AIActionsProps {
  symbol: string;
  reportContent: string;
  nLabelWritten: boolean;
  analysisComplete: boolean;
  onRegenerate: () => void;
}

export default function AIActions({ symbol, reportContent, nLabelWritten, analysisComplete, onRegenerate }: AIActionsProps) {
  const queryClient = useQueryClient();
  const [writing, setWriting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const setNLabelWritten = useAnalysisStore((store) => store.setNLabelWritten);
  const updateState = useAnalysisStore((store) => store.updateState);

  const handleExportMarkdown = () => {
    if (!reportContent.trim()) return;
    const generatedAt = new Date().toISOString();
    const markdown = [
      '# ' + symbol.trim().toUpperCase() + ' AI 分析报告',
      '',
      '> 生成时间：' + generatedAt,
      '> 来源：CANSLIMAgent 多智能体 AI 分析',
      '',
      reportContent.trim(),
      ''
    ].join('\n');
    const blob = new Blob([markdown], { type: 'text/markdown;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    const stamp = generatedAt.replace(/[:.]/g, '-');
    link.href = url;
    link.download = symbol.trim().toUpperCase() + '-ai-analysis-' + stamp + '.md';
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 0);
  };

  const handleWriteNLabel = async () => {
    setWriting(true);
    setError(null);
    try {
      await updateNLabel(symbol, true, reportContent.slice(0, 1800));
      setNLabelWritten(symbol, true);
      const current = useAnalysisStore.getState().getStateForSymbol(symbol);
      if (current.stockSummary) {
        updateState(symbol, {
          stockSummary: {
            ...current.stockSummary,
            metrics: { ...current.stockSummary.metrics, n: true }
          }
        });
      }
      void queryClient.invalidateQueries({ queryKey: ['screener-results'] });
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'N 标签写回失败');
    } finally {
      setWriting(false);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap justify-end gap-2">
        <Button variant="outline" className="h-9 rounded-xl px-3" onClick={onRegenerate}>
          <RefreshCw className="h-4 w-4" />
          重新生成
        </Button>
        <Button variant="outline" className="h-9 rounded-xl px-3" disabled={!reportContent.trim()} onClick={handleExportMarkdown}>
          <Download className="h-4 w-4" />
          导出报告
        </Button>
        {nLabelWritten || (analysisComplete && Boolean(reportContent.trim())) ? (
          <Button
            variant="outline"
            disabled={writing || nLabelWritten}
            onClick={handleWriteNLabel}
            className={'h-9 rounded-xl px-3 ' + (nLabelWritten ? 'border-emerald-400 bg-emerald-400 text-slate-950 opacity-100 hover:bg-emerald-400 disabled:opacity-100' : '')}
          >
            {nLabelWritten ? <Check className="h-4 w-4" /> : <Tag className="h-4 w-4" />}
            {nLabelWritten ? 'N 已写回' : writing ? '写回中' : '写回 N 标签'}
          </Button>
        ) : null}
      </div>
      {error ? <div className="rounded-xl border border-red-800 bg-red-950/60 px-3 py-2 text-sm text-red-100">{error}</div> : null}
    </div>
  );
}
