import { ChevronDown, ChevronUp, Loader2, Settings2, Sparkles } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Button } from '../ui/button';
import { Card } from '../ui/card';
import { Checkbox } from '../ui/checkbox';
import { Label } from '../ui/label';
import { Select } from '../ui/select';
import { fetchAIAnalysisConfig } from '../../lib/api';
import {
  FALLBACK_AI_ANALYSIS_OPTIONS,
  normalizeAIAnalysisConfig,
  readStoredAIAnalysisConfig,
  writeStoredAIAnalysisConfig
} from '../../lib/aiAnalysisConfig';
import type { AIAnalysisConfigResponse, AnalysisConfig } from '../../types/analysis';

interface AIAnalysisConfigProps {
  onStartAnalysis: (config: AnalysisConfig) => void;
  isAnalyzing: boolean;
}

export default function AIAnalysisConfig({ onStartAnalysis, isAnalyzing }: AIAnalysisConfigProps) {
  const [expanded, setExpanded] = useState(false);
  const [options, setOptions] = useState<AIAnalysisConfigResponse>(FALLBACK_AI_ANALYSIS_OPTIONS);
  const [config, setConfig] = useState<AnalysisConfig>(() =>
    normalizeAIAnalysisConfig(readStoredAIAnalysisConfig(), FALLBACK_AI_ANALYSIS_OPTIONS)
  );
  const [loadError, setLoadError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetchAIAnalysisConfig()
      .then((response) => {
        if (cancelled) return;
        setOptions(response);
        setConfig((current) => normalizeAIAnalysisConfig(current, response));
        setLoadError(null);
      })
      .catch(() => {
        if (!cancelled) setLoadError('配置接口暂不可用，已使用默认配置');
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    writeStoredAIAnalysisConfig(config);
  }, [config]);

  const depthValue = config.max_debate_rounds >= 2 ? 'deep' : 'standard';
  const disabled = isAnalyzing;

  const updateDepth = (value: string) => {
    const rounds = value === 'deep' ? 2 : 1;
    setConfig((current) => ({
      ...current,
      max_debate_rounds: rounds,
      max_risk_rounds: rounds
    }));
  };

  const toggleAnalyst = (analystId: string, checked: boolean) => {
    setConfig((current) => ({
      ...current,
      analysts: checked
        ? Array.from(new Set([...current.analysts, analystId]))
        : current.analysts.filter((item) => item !== analystId)
    }));
  };

  const hasAnalyst = config.analysts.length > 0;

  return (
    <Card className="space-y-4 border-cyan-400/20 bg-slate-950/70 p-4 sm:p-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <span className="flex h-9 w-9 items-center justify-center rounded-xl border border-cyan-400/25 bg-cyan-400/10 text-cyan-200">
            <Sparkles className="h-4 w-4" />
          </span>
          <h2 className="text-lg font-black text-white">AI 分析</h2>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            className="h-10 rounded-xl px-3"
            onClick={() => setExpanded((value) => !value)}
            aria-expanded={expanded}
          >
            <Settings2 className="h-4 w-4" />
            参数
            {expanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
          </Button>
          <Button className="h-10 min-w-32 rounded-xl" disabled={disabled || !hasAnalyst} onClick={() => onStartAnalysis(config)}>
            {isAnalyzing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Sparkles className="h-4 w-4" />}
            {isAnalyzing ? '分析中' : '开始分析'}
          </Button>
        </div>
      </div>

      {expanded ? (
        <>
          <div className="grid gap-4 border-t border-slate-800 pt-4 lg:grid-cols-[minmax(0,1fr)_200px]">
            <div className="space-y-3">
              <h3 className="text-sm font-bold text-slate-100">分析角色</h3>
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                {options.analysts.map((analyst) => (
                  <label
                    key={analyst.id}
                    className="flex min-h-11 cursor-pointer items-center gap-3 rounded-xl border border-slate-800 bg-slate-900/45 px-3 py-2.5 transition hover:border-cyan-400/40"
                  >
                    <Checkbox
                      checked={config.analysts.includes(analyst.id)}
                      disabled={disabled}
                      onCheckedChange={(checked) => toggleAnalyst(analyst.id, checked)}
                    />
                    <span className="text-sm font-bold text-slate-100">{analyst.name}</span>
                  </label>
                ))}
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="ai-depth">分析深度</Label>
              <Select id="ai-depth" value={depthValue} disabled={disabled} onValueChange={updateDepth}>
                <option value="standard">标准</option>
                <option value="deep">深度</option>
              </Select>
            </div>
          </div>

          {!hasAnalyst ? <p className="text-sm font-medium text-amber-300">请至少选择一个分析角色</p> : null}
        </>
      ) : null}
      {loadError ? <p className="text-sm font-medium text-amber-300">{loadError}</p> : null}
    </Card>
  );
}

export type { AnalysisConfig };
