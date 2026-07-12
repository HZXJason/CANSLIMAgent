import { KeyRound, Loader2, RefreshCw, Settings, SlidersHorizontal } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { Card } from '../../components/ui/card';
import { Button } from '../../components/ui/button';
import { Checkbox } from '../../components/ui/checkbox';
import { Input } from '../../components/ui/input';
import { Label } from '../../components/ui/label';
import { Select } from '../../components/ui/select';
import { API_BASE_URL, discoverAIModels, fetchAIAnalysisConfig } from '../../lib/api';
import {
  FALLBACK_AI_ANALYSIS_OPTIONS,
  getAIAnalysisProvider,
  normalizeAIAnalysisConfig,
  readStoredAIAnalysisConfig,
  writeStoredAIAnalysisConfig
} from '../../lib/aiAnalysisConfig';
import type { AIAnalysisConfigResponse, AnalysisConfig } from '../../types/analysis';
import BrokerConnections from '../../components/settings/BrokerConnections';

export default function SettingsPage() {
  const [options, setOptions] = useState<AIAnalysisConfigResponse>(FALLBACK_AI_ANALYSIS_OPTIONS);
  const [config, setConfig] = useState<AnalysisConfig>(() =>
    normalizeAIAnalysisConfig(readStoredAIAnalysisConfig(), FALLBACK_AI_ANALYSIS_OPTIONS)
  );
  const [loadError, setLoadError] = useState<string | null>(null);
  const [discoveredModels, setDiscoveredModels] = useState<string[]>([]);
  const [modelSource, setModelSource] = useState('');
  const [modelError, setModelError] = useState<string | null>(null);
  const [modelLoading, setModelLoading] = useState(false);

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

  const provider = useMemo(
    () => getAIAnalysisProvider(options, config.llm_provider),
    [config.llm_provider, options]
  );
  const customProvider = Boolean(provider.custom_models);
  const providerApiKey = config.api_keys[config.llm_provider] ?? '';

  const updateProvider = (providerId: string) => {
    const nextProvider = getAIAnalysisProvider(options, providerId);
    setConfig((current) => ({
      ...current,
      llm_provider: nextProvider.id,
      deep_think_llm: nextProvider.models.deep[0] ?? '',
      quick_think_llm: nextProvider.models.quick[0] ?? '',
      backend_url: nextProvider.default_backend_url ?? ''
    }));
    setDiscoveredModels([]);
    setModelSource('');
    setModelError(null);
  };

  const updateProviderApiKey = (value: string) => {
    setConfig((current) => ({
      ...current,
      api_keys: {
        ...current.api_keys,
        [current.llm_provider]: value
      }
    }));
    setDiscoveredModels([]);
    setModelSource('');
    setModelError(null);
  };

  const updateBackendURL = (value: string) => {
    setConfig((current) => ({ ...current, backend_url: value }));
    setDiscoveredModels([]);
    setModelSource('');
    setModelError(null);
  };

  const fetchModels = async () => {
    if (customProvider && !config.backend_url.trim()) {
      setModelError('请先填写 OpenAI 兼容 API 地址');
      return;
    }
    setModelLoading(true);
    setModelError(null);
    try {
      const result = await discoverAIModels({
        provider: config.llm_provider,
        backend_url: config.backend_url.trim(),
        api_key: providerApiKey.trim()
      });
      setDiscoveredModels(result.models);
      setModelSource(result.source);
      setConfig((current) => ({
        ...current,
        deep_think_llm: result.models.includes(current.deep_think_llm) ? current.deep_think_llm : result.models[0] ?? '',
        quick_think_llm: result.models.includes(current.quick_think_llm) ? current.quick_think_llm : result.models[0] ?? ''
      }));
    } catch (error) {
      setDiscoveredModels([]);
      setModelSource('');
      setModelError(error instanceof Error ? error.message : '获取模型列表失败');
    } finally {
      setModelLoading(false);
    }
  };

  return (
    <section className="mx-auto w-full max-w-[1180px] px-4 py-6 sm:px-6 lg:px-8">
      <header className="rounded-lg border border-slate-800/90 bg-slate-900/70 p-5 shadow-2xl shadow-slate-950/25 backdrop-blur">
        <p className="text-sm font-semibold text-cyan-300/80">AlphaLens</p>
        <h1 className="mt-2 text-3xl font-black tracking-tight text-white">设置</h1>
      </header>

      <Card className="mt-6 rounded-lg">
        <div className="flex items-start gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-slate-950/70 text-cyan-300">
            <Settings className="h-5 w-5" />
          </div>
          <div>
            <h2 className="text-lg font-bold text-white">本地运行配置</h2>
            <p className="mt-3 text-sm leading-6 text-slate-400">当前 API 地址：{API_BASE_URL}</p>
          </div>
        </div>
      </Card>

      <Card className="mt-6 rounded-lg">
        <div className="flex flex-col gap-5">
          <div className="flex items-start gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-slate-950/70 text-cyan-300">
              <SlidersHorizontal className="h-5 w-5" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-white">AI 分析连接</h2>
              <p className="mt-2 text-sm leading-6 text-slate-500">模型、Key 与网关配置会应用到个股 AI 分析。</p>
              {loadError ? <p className="mt-2 text-sm text-amber-300">{loadError}</p> : null}
            </div>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="settings-ai-provider">LLM 提供商</Label>
              <Select id="settings-ai-provider" value={config.llm_provider} onValueChange={updateProvider}>
                {options.providers.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </Select>
            </div>

            <div className="space-y-2">
              <Label htmlFor="settings-ai-provider-key">{provider.name} API Key</Label>
              <div className="relative">
                <Input
                  id="settings-ai-provider-key"
                  type="password"
                  autoComplete="off"
                  value={providerApiKey}
                  className="pl-10"
                  placeholder="可留空使用 sidecar/.env 中的环境变量"
                  onChange={(event) => updateProviderApiKey(event.target.value)}
                />
                <KeyRound className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
              </div>
              <p className="text-xs text-slate-500">保存在本机浏览器，并随分析请求发送到本地 sidecar</p>
            </div>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="settings-ai-deep-model">深度思考模型</Label>
              {customProvider && discoveredModels.length === 0 ? (
                <Input
                  id="settings-ai-deep-model"
                  value={config.deep_think_llm}
                  placeholder="例如 anthropic/claude-haiku-4.5"
                  onChange={(event) => setConfig((current) => ({ ...current, deep_think_llm: event.target.value }))}
                />
              ) : (
                <Select
                  id="settings-ai-deep-model"
                  value={config.deep_think_llm}
                  onValueChange={(value) => setConfig((current) => ({ ...current, deep_think_llm: value }))}
                >
                  {(discoveredModels.length > 0 ? discoveredModels : provider.models.deep).map((model) => (
                    <option key={model} value={model}>
                      {model}
                    </option>
                  ))}
                </Select>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="settings-ai-quick-model">快速响应模型</Label>
              {customProvider && discoveredModels.length === 0 ? (
                <Input
                  id="settings-ai-quick-model"
                  value={config.quick_think_llm}
                  placeholder="例如 google/gemini-3.1-flash-lite-preview"
                  onChange={(event) => setConfig((current) => ({ ...current, quick_think_llm: event.target.value }))}
                />
              ) : (
                <Select
                  id="settings-ai-quick-model"
                  value={config.quick_think_llm}
                  onValueChange={(value) => setConfig((current) => ({ ...current, quick_think_llm: value }))}
                >
                  {(discoveredModels.length > 0 ? discoveredModels : provider.models.quick).map((model) => (
                    <option key={model} value={model}>
                      {model}
                    </option>
                  ))}
                </Select>
              )}
            </div>
          </div>

          {customProvider ? (
            <div className="space-y-2">
              <Label htmlFor="settings-ai-backend-url">自定义 API 地址</Label>
              <Input
                id="settings-ai-backend-url"
                value={config.backend_url}
                placeholder="https://ai-gateway.vercel.sh/v1"
                onChange={(event) => updateBackendURL(event.target.value)}
              />
              <p className="text-xs text-slate-500">支持 OpenAI Chat Completions 兼容的 /v1 网关</p>
              <p className="text-xs text-amber-300">Vercel 免费层建议使用 anthropic/claude-haiku-4.5；Sonnet/Opus 常会返回 403</p>
            </div>
          ) : null}

          {customProvider ? (
            <div className="flex flex-col gap-3 rounded-lg border border-slate-800 bg-slate-950/35 p-3 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <p className="text-sm font-semibold text-slate-200">可用模型列表</p>
                <p className="mt-1 text-xs text-slate-500">
                  {discoveredModels.length > 0
                    ? `已从 ${modelSource} 获取 ${discoveredModels.length} 个模型，可在上方下拉选择`
                    : '填写 API 地址和 Key 后，从供应商的 /models 接口读取'}
                </p>
                {modelError ? <p className="mt-2 text-xs text-red-300">{modelError}</p> : null}
              </div>
              <Button type="button" variant="outline" disabled={modelLoading} onClick={() => void fetchModels()}>
                {modelLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
                {modelLoading ? '获取中' : '获取可用模型'}
              </Button>
            </div>
          ) : null}

          <label className="flex cursor-pointer items-start gap-3 rounded-lg border border-slate-800 bg-slate-950/35 p-3 transition hover:border-cyan-400/40">
            <Checkbox
              checked={config.online_tools}
              onCheckedChange={(checked) => setConfig((current) => ({ ...current, online_tools: checked }))}
            />
            <span>
              <span className="block text-sm font-bold text-slate-100">启用在线工具</span>
              <span className="mt-1 block text-xs leading-5 text-slate-500">允许分析流程使用联网检索和外部数据工具</span>
            </span>
          </label>
        </div>
      </Card>
      <BrokerConnections />
    </section>
  );
}
