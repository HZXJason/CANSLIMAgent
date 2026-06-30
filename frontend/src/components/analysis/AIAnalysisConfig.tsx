import { ChevronDown, ChevronUp, Loader2, Settings2, Sparkles } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { Button } from '../ui/button';
import { Card } from '../ui/card';
import { Checkbox } from '../ui/checkbox';
import { Input } from '../ui/input';
import { Label } from '../ui/label';
import { Select } from '../ui/select';
import { fetchAIAnalysisConfig } from '../../lib/api';
import type { AIAnalysisConfigResponse, AnalysisConfig } from '../../types/analysis';

interface AIAnalysisConfigProps {
  symbol: string;
  onStartAnalysis: (config: AnalysisConfig) => void;
  isAnalyzing: boolean;
}

const STORAGE_KEY = 'alphalens.aiAnalysisConfig.v1';

const DEFAULT_API_KEYS = {
  anthropic: '',
  openai: '',
  openai_compatible: '',
  google: ''
};

const FALLBACK_OPTIONS: AIAnalysisConfigResponse = {
  providers: [
    {
      id: 'anthropic',
      name: 'Anthropic',
      models: {
        deep: ['claude-opus-4-6', 'claude-sonnet-4-6'],
        quick: ['claude-sonnet-4-6', 'claude-haiku-4-5-20251001']
      }
    },
    {
      id: 'openai',
      name: 'OpenAI',
      models: {
        deep: ['gpt-5.4', 'o4-mini'],
        quick: ['gpt-5.4-mini', 'gpt-4.1-nano']
      }
    },
    {
      id: 'openai_compatible',
      name: 'OpenAI 兼容网关',
      custom_models: true,
      default_backend_url: 'https://ai-gateway.vercel.sh/v1',
      models: {
        deep: ['anthropic/claude-haiku-4.5', 'google/gemini-3.1-flash-lite-preview'],
        quick: ['anthropic/claude-haiku-4.5', 'google/gemini-3.1-flash-lite-preview']
      }
    },
    {
      id: 'google',
      name: 'Google Gemini',
      models: {
        deep: ['gemini-3.0-flash-thinking', 'gemini-3.0-pro'],
        quick: ['gemini-3.0-flash', 'gemini-3.1-flash']
      }
    }
  ],
  analysts: [
    { id: 'fundamental', name: '基本面分析师', description: '评估公司财务和业绩指标' },
    { id: 'sentiment', name: '情绪分析师', description: '聚合新闻和社交媒体情绪' },
    { id: 'news', name: '新闻分析师', description: '监控全球新闻和宏观指标' },
    { id: 'technical', name: '技术分析师', description: '基于技术指标检测交易信号' }
  ],
  debate_rounds: [1, 2, 3],
  risk_rounds: [1, 2, 3]
};

function defaultConfig(options: AIAnalysisConfigResponse): AnalysisConfig {
  const provider = options.providers[0];
  return {
    llm_provider: provider.id,
    deep_think_llm: provider.models.deep[1] ?? provider.models.deep[0] ?? '',
    quick_think_llm: provider.models.quick[1] ?? provider.models.quick[0] ?? '',
    backend_url: provider.default_backend_url ?? '',
    max_debate_rounds: 1,
    max_risk_rounds: 1,
    online_tools: true,
    analysts: options.analysts.map((analyst) => analyst.id),
    api_keys: { ...DEFAULT_API_KEYS }
  };
}

function readStoredConfig() {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as Partial<AnalysisConfig>) : null;
  } catch {
    return null;
  }
}

function normalizeConfig(config: Partial<AnalysisConfig> | null, options: AIAnalysisConfigResponse): AnalysisConfig {
  const fallback = defaultConfig(options);
  if (!config) return fallback;

  const provider = options.providers.find((item) => item.id === config.llm_provider) ?? options.providers[0];
  const deepModels = provider.models.deep;
  const quickModels = provider.models.quick;
  const customModels = Boolean(provider.custom_models);
  const analystIds = new Set(options.analysts.map((analyst) => analyst.id));
  const selectedAnalysts = (config.analysts ?? fallback.analysts).filter((analyst) => analystIds.has(analyst));
  const storedDeepModel = config.deep_think_llm ?? '';
  const storedQuickModel = config.quick_think_llm ?? '';
  const restrictedFreeTierModels = new Set(['anthropic/claude-sonnet-4.6', 'openai/gpt-5.5']);
  const deepThinkLLM = customModels && restrictedFreeTierModels.has(storedDeepModel) ? deepModels[0] ?? '' : storedDeepModel;
  const quickThinkLLM = customModels && restrictedFreeTierModels.has(storedQuickModel) ? quickModels[0] ?? '' : storedQuickModel;

  return {
    llm_provider: provider.id,
    deep_think_llm: customModels || deepModels.includes(deepThinkLLM) ? deepThinkLLM || deepModels[0] || '' : deepModels[0] ?? '',
    quick_think_llm: customModels || quickModels.includes(quickThinkLLM) ? quickThinkLLM || quickModels[0] || '' : quickModels[0] ?? '',
    backend_url: config.backend_url ?? provider.default_backend_url ?? '',
    max_debate_rounds: config.max_debate_rounds === 2 ? 2 : 1,
    max_risk_rounds: config.max_risk_rounds === 2 ? 2 : 1,
    online_tools: config.online_tools ?? true,
    analysts: selectedAnalysts.length > 0 ? selectedAnalysts : fallback.analysts,
    api_keys: { ...DEFAULT_API_KEYS, ...(config.api_keys ?? {}) }
  };
}

export default function AIAnalysisConfig({ symbol, onStartAnalysis, isAnalyzing }: AIAnalysisConfigProps) {
  const [expanded, setExpanded] = useState(true);
  const [options, setOptions] = useState<AIAnalysisConfigResponse>(FALLBACK_OPTIONS);
  const [config, setConfig] = useState<AnalysisConfig>(() => normalizeConfig(readStoredConfig(), FALLBACK_OPTIONS));
  const [loadError, setLoadError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetchAIAnalysisConfig()
      .then((response) => {
        if (cancelled) return;
        setOptions(response);
        setConfig((current) => normalizeConfig(current, response));
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
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(config));
  }, [config]);

  const provider = useMemo(
    () => options.providers.find((item) => item.id === config.llm_provider) ?? options.providers[0],
    [config.llm_provider, options.providers]
  );
  const depthValue = config.max_debate_rounds >= 2 ? 'deep' : 'standard';
  const providerApiKey = config.api_keys[config.llm_provider] ?? '';
  const customProvider = Boolean(provider.custom_models);
  const disabled = isAnalyzing;

  const updateProvider = (providerId: string) => {
    const nextProvider = options.providers.find((item) => item.id === providerId) ?? options.providers[0];
    setConfig((current) => ({
      ...current,
      llm_provider: nextProvider.id,
      deep_think_llm: nextProvider.models.deep[0] ?? '',
      quick_think_llm: nextProvider.models.quick[0] ?? '',
      backend_url: nextProvider.default_backend_url ?? ''
    }));
  };

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
      analysts: checked ? [...current.analysts, analystId] : current.analysts.filter((item) => item !== analystId)
    }));
  };

  const updateProviderApiKey = (value: string) => {
    setConfig((current) => ({
      ...current,
      api_keys: {
        ...current.api_keys,
        [current.llm_provider]: value
      }
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
            <h2 className="text-lg font-black text-white">AI 多智能体分析配置</h2>
            <p className="mt-1 text-sm text-slate-500">{symbol} · TradingAgents 参数</p>
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
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="ai-provider">LLM 提供商</Label>
              <Select id="ai-provider" value={config.llm_provider} disabled={disabled} onValueChange={updateProvider}>
                {options.providers.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="ai-depth">分析深度</Label>
              <Select id="ai-depth" value={depthValue} disabled={disabled} onValueChange={updateDepth}>
                <option value="standard">标准</option>
                <option value="deep">深度</option>
              </Select>
            </div>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="ai-deep-model">深度思考模型</Label>
              {customProvider ? (
                <Input
                  id="ai-deep-model"
                  value={config.deep_think_llm}
                  disabled={disabled}
                  placeholder="例如 anthropic/claude-sonnet-4.6"
                  onChange={(event) => setConfig((current) => ({ ...current, deep_think_llm: event.target.value }))}
                />
              ) : (
                <Select
                  id="ai-deep-model"
                  value={config.deep_think_llm}
                  disabled={disabled}
                  onValueChange={(value) => setConfig((current) => ({ ...current, deep_think_llm: value }))}
                >
                  {provider.models.deep.map((model) => (
                    <option key={model} value={model}>
                      {model}
                    </option>
                  ))}
                </Select>
              )}
              <p className="text-xs text-slate-500">复杂推理角色使用</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="ai-quick-model">快速响应模型</Label>
              {customProvider ? (
                <Input
                  id="ai-quick-model"
                  value={config.quick_think_llm}
                  disabled={disabled}
                  placeholder="例如 openai/gpt-5-mini"
                  onChange={(event) => setConfig((current) => ({ ...current, quick_think_llm: event.target.value }))}
                />
              ) : (
                <Select
                  id="ai-quick-model"
                  value={config.quick_think_llm}
                  disabled={disabled}
                  onValueChange={(value) => setConfig((current) => ({ ...current, quick_think_llm: value }))}
                >
                  {provider.models.quick.map((model) => (
                    <option key={model} value={model}>
                      {model}
                    </option>
                  ))}
                </Select>
              )}
              <p className="text-xs text-slate-500">简单任务使用</p>
            </div>
          </div>

          {customProvider ? (
            <div className="space-y-2">
              <Label htmlFor="ai-backend-url">自定义 API 地址</Label>
              <Input
                id="ai-backend-url"
                value={config.backend_url}
                disabled={disabled}
                placeholder="https://ai-gateway.vercel.sh/v1"
                onChange={(event) => setConfig((current) => ({ ...current, backend_url: event.target.value }))}
              />
            <p className="text-xs text-slate-500">支持 OpenAI Chat Completions 兼容的 /v1 网关</p>
            <p className="text-xs text-amber-300">Vercel 免费层建议使用 anthropic/claude-haiku-4.5；Sonnet/Opus 常会返回 403</p>
          </div>
          ) : null}

          <div className="space-y-2">
            <Label htmlFor="ai-provider-key">{provider.name} API Key</Label>
            <Input
              id="ai-provider-key"
              type="password"
              autoComplete="off"
              value={providerApiKey}
              disabled={disabled}
              placeholder="可留空使用 sidecar/.env 中的环境变量"
              onChange={(event) => updateProviderApiKey(event.target.value)}
            />
            <p className="text-xs text-slate-500">保存在本机浏览器，并随本次分析请求发送到本地 sidecar</p>
          </div>

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
