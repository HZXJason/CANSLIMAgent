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
  symbol: string;
  onStartAnalysis: (config: AnalysisConfig) => void;
  isAnalyzing: boolean;
}

export default function AIAnalysisConfig({ symbol, onStartAnalysis, isAnalyzing }: AIAnalysisConfigProps) {
  const [expanded, setExpanded] = useState(true);
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
    <Card className="space-y-5 border-cyan-400/20 bg-slate-950/70">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <span className="flex h-10 w-10 items-center justify-center rounded-xl border border-cyan-400/25 bg-cyan-400/10 text-cyan-200">
            <Settings2 className="h-5 w-5" />
          </span>
          <div>
            <h2 className="text-lg font-black text-white">AI 多智能体分析</h2>
            <p className="mt-1 text-sm text-slate-500">{symbol} · 分析师与深度</p>
          </div>
        </div>
        <Button
          variant="outline"
          className="h-10 w-10 rounded-xl px-0"
          disabled={disabled}
          onClick={() => setExpanded((value) => !value)}
          aria-label={expanded ? '折叠配置' : '展开配置'}
        >
          {expanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
        </Button>
      </div>

      {expanded ? (
        <>
          <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_220px]">
            <div className="space-y-3">
              <div>
                <h3 className="text-sm font-bold text-slate-100">启用的分析师角色</h3>
                <p className="mt-1 text-xs text-slate-500">至少选择一名分析师</p>
              </div>
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                {options.analysts.map((analyst) => (
                  <label
                    key={analyst.id}
                    className="flex min-h-16 cursor-pointer items-start gap-3 rounded-xl border border-slate-800 bg-slate-900/45 px-3 py-3 transition hover:border-cyan-400/40"
                  >
                    <Checkbox
                      checked={config.analysts.includes(analyst.id)}
                      disabled={disabled}
                      onCheckedChange={(checked) => toggleAnalyst(analyst.id, checked)}
                    />
                    <span>
                      <span className="block text-sm font-bold text-slate-100">{analyst.name}</span>
                      <span className="mt-1 block text-xs leading-5 text-slate-500">{analyst.description}</span>
                    </span>
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

          <div className="flex flex-col gap-3 border-t border-slate-800 pt-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="text-sm text-slate-500">
              {depthValue === 'deep' ? '深度模式约需 5-15 分钟' : '标准模式约需 3-8 分钟'}
              {loadError ? <span className="ml-2 text-amber-300">{loadError}</span> : null}
            </div>
            <Button className="w-full sm:w-40" disabled={disabled || !hasAnalyst} onClick={() => onStartAnalysis(config)}>
              {isAnalyzing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Sparkles className="h-4 w-4" />}
              {isAnalyzing ? '分析中' : '开始 AI 分析'}
            </Button>
          </div>
        </>
      ) : null}
    </Card>
  );
}

export type { AnalysisConfig };
