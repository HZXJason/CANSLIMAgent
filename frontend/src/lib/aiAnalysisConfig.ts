import type { AIAnalysisConfigResponse, AIProviderConfig, AnalysisConfig } from '../types/analysis';

export const AI_ANALYSIS_CONFIG_STORAGE_KEY = 'alphalens.aiAnalysisConfig.v1';

export const DEFAULT_AI_API_KEYS: Record<string, string> = {
  anthropic: '',
  openai: '',
  openai_compatible: '',
  google: ''
};

export const FALLBACK_AI_ANALYSIS_OPTIONS: AIAnalysisConfigResponse = {
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
    { id: 'technical', name: '技术分析师', description: '基于技术指标检测交易信号' },
    {
      id: 'canslim_second_layer',
      name: 'CANSLIM 二层筛选官',
      description: '严格复核假成长、盈利质量、技术形态、估值和风险收益比',
      default_enabled: false
    }
  ],
  debate_rounds: [1, 2, 3],
  risk_rounds: [1, 2, 3]
};

export function createDefaultAIAnalysisConfig(options: AIAnalysisConfigResponse): AnalysisConfig {
  const provider = options.providers[0];
  return {
    llm_provider: provider.id,
    deep_think_llm: provider.models.deep[1] ?? provider.models.deep[0] ?? '',
    quick_think_llm: provider.models.quick[1] ?? provider.models.quick[0] ?? '',
    backend_url: provider.default_backend_url ?? '',
    max_debate_rounds: 1,
    max_risk_rounds: 1,
    online_tools: true,
    analysts: options.analysts.filter((analyst) => analyst.default_enabled !== false).map((analyst) => analyst.id),
    api_keys: { ...DEFAULT_AI_API_KEYS }
  };
}

export function readStoredAIAnalysisConfig() {
  if (typeof window === 'undefined') return null;

  try {
    const raw = window.localStorage.getItem(AI_ANALYSIS_CONFIG_STORAGE_KEY);
    return raw ? (JSON.parse(raw) as Partial<AnalysisConfig>) : null;
  } catch {
    return null;
  }
}

export function writeStoredAIAnalysisConfig(config: AnalysisConfig) {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(AI_ANALYSIS_CONFIG_STORAGE_KEY, JSON.stringify(config));
}

export function getAIAnalysisProvider(options: AIAnalysisConfigResponse, providerId: string): AIProviderConfig {
  return options.providers.find((item) => item.id === providerId) ?? options.providers[0];
}

export function normalizeAIAnalysisConfig(
  config: Partial<AnalysisConfig> | null,
  options: AIAnalysisConfigResponse
): AnalysisConfig {
  const fallback = createDefaultAIAnalysisConfig(options);
  if (!config) return fallback;

  const provider = getAIAnalysisProvider(options, config.llm_provider ?? options.providers[0].id);
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
    quick_think_llm: customModels || quickModels.includes(quickThinkLLM)
      ? quickThinkLLM || quickModels[0] || ''
      : quickModels[0] ?? '',
    backend_url: config.backend_url ?? provider.default_backend_url ?? '',
    max_debate_rounds: config.max_debate_rounds === 2 ? 2 : 1,
    max_risk_rounds: config.max_risk_rounds === 2 ? 2 : 1,
    online_tools: config.online_tools ?? true,
    analysts: selectedAnalysts.length > 0 ? selectedAnalysts : fallback.analysts,
    api_keys: { ...DEFAULT_AI_API_KEYS, ...(config.api_keys ?? {}) }
  };
}
