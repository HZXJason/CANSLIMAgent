import { KeyRound, Loader2, RefreshCw, Save, Settings, SlidersHorizontal, Trash2 } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Checkbox } from '../../components/ui/checkbox';
import { Input } from '../../components/ui/input';
import { Label } from '../../components/ui/label';
import { Select } from '../../components/ui/select';
import {
  API_BASE_URL,
  deleteLLMCredential,
  discoverAIModels,
  fetchAIAnalysisConfig,
  fetchLLMCredentialStatus,
  saveLLMCredential
} from '../../lib/api';
import {
  FALLBACK_AI_ANALYSIS_OPTIONS,
  getAIAnalysisProvider,
  normalizeAIAnalysisConfig,
  readStoredAIAnalysisConfig,
  writeStoredAIAnalysisConfig
} from '../../lib/aiAnalysisConfig';
import type { AIAnalysisConfigResponse, AnalysisConfig, LLMCredentialStatus } from '../../types/analysis';
import BrokerConnections from '../../components/settings/BrokerConnections';

export default function SettingsPage() {
  const [options, setOptions] = useState<AIAnalysisConfigResponse>(FALLBACK_AI_ANALYSIS_OPTIONS);
  const [config, setConfig] = useState<AnalysisConfig>(() =>
    normalizeAIAnalysisConfig(readStoredAIAnalysisConfig(), FALLBACK_AI_ANALYSIS_OPTIONS)
  );
  const [loadError, setLoadError] = useState<string | null>(null);
  const [configReady, setConfigReady] = useState(false);
  const [discoveredModels, setDiscoveredModels] = useState<string[]>([]);
  const [modelSource, setModelSource] = useState('');
  const [modelError, setModelError] = useState<string | null>(null);
  const [modelLoading, setModelLoading] = useState(false);
  const [modelRefreshVersion, setModelRefreshVersion] = useState(0);
  const [apiKey, setAPIKey] = useState('');
  const [credentialStatus, setCredentialStatus] = useState<LLMCredentialStatus | null>(null);
  const [credentialLoading, setCredentialLoading] = useState(false);
  const [credentialSaving, setCredentialSaving] = useState(false);
  const [credentialError, setCredentialError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetchAIAnalysisConfig()
      .then((response) => {
        if (cancelled) return;
        setOptions(response);
        setConfig((current) => normalizeAIAnalysisConfig(current, response));
        setLoadError(null);
        setConfigReady(true);
      })
      .catch(() => {
        if (!cancelled) {
          setLoadError('配置接口暂不可用，已使用默认配置');
          setConfigReady(true);
        }
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
  const credentialBackendURL = customProvider ? config.backend_url.trim() : '';

  useEffect(() => {
    setAPIKey('');
    setCredentialError(null);
    if (!configReady || (customProvider && !credentialBackendURL)) {
      setCredentialStatus(null);
      setCredentialLoading(false);
      return;
    }
    let cancelled = false;
    setCredentialLoading(true);
    const timeoutId = window.setTimeout(() => {
      void fetchLLMCredentialStatus(provider.id, credentialBackendURL)
        .then((status) => {
          if (!cancelled) setCredentialStatus(status);
        })
        .catch((error) => {
          if (!cancelled) {
            setCredentialStatus(null);
            setCredentialError(error instanceof Error ? error.message : '读取 Key 状态失败');
          }
        })
        .finally(() => {
          if (!cancelled) setCredentialLoading(false);
        });
    }, 300);
    return () => {
      cancelled = true;
      window.clearTimeout(timeoutId);
    };
  }, [configReady, credentialBackendURL, customProvider, provider.id]);

  useEffect(() => {
    if (!configReady) return;
    const backendURL = config.backend_url.trim();
    if (customProvider && !backendURL) {
      setDiscoveredModels([]);
      setModelSource('');
      setModelLoading(false);
      setModelError('填写自定义 API 地址后会自动获取模型列表');
      return;
    }

    let cancelled = false;
    const timeoutId = window.setTimeout(() => {
      setModelLoading(true);
      setModelError(null);
      void discoverAIModels({
        provider: provider.id,
        ...(backendURL ? { backend_url: backendURL } : {})
      })
        .then((result) => {
          if (cancelled) return;
          setDiscoveredModels(result.models);
          setModelSource(result.source);
          setConfig((current) => ({
            ...current,
            deep_think_llm: result.models.includes(current.deep_think_llm)
              ? current.deep_think_llm
              : result.models[0] ?? current.deep_think_llm,
            quick_think_llm: result.models.includes(current.quick_think_llm)
              ? current.quick_think_llm
              : result.models[0] ?? current.quick_think_llm
          }));
        })
        .catch((error) => {
          if (cancelled) return;
          setDiscoveredModels([]);
          setModelSource('');
          setModelError(error instanceof Error ? error.message : '获取模型列表失败');
        })
        .finally(() => {
          if (!cancelled) setModelLoading(false);
        });
    }, 500);

    return () => {
      cancelled = true;
      window.clearTimeout(timeoutId);
    };
  }, [config.backend_url, configReady, customProvider, modelRefreshVersion, provider.id]);

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

  const updateBackendURL = (value: string) => {
    setConfig((current) => ({ ...current, backend_url: value }));
    setDiscoveredModels([]);
    setModelSource('');
    setModelError(null);
  };

  const availableDeepModels = discoveredModels.length > 0 ? discoveredModels : provider.models.deep;
  const availableQuickModels = discoveredModels.length > 0 ? discoveredModels : provider.models.quick;

  const saveAPIKey = async () => {
    if (!apiKey.trim()) {
      setCredentialError('请输入 API Key');
      return;
    }
    if (customProvider && !credentialBackendURL) {
      setCredentialError('请先填写自定义 API 地址');
      return;
    }
    setCredentialSaving(true);
    setCredentialError(null);
    try {
      const status = await saveLLMCredential(provider.id, apiKey, credentialBackendURL);
      setCredentialStatus(status);
      setAPIKey('');
      setModelRefreshVersion((current) => current + 1);
    } catch (error) {
      setCredentialError(error instanceof Error ? error.message : '保存 API Key 失败');
    } finally {
      setCredentialSaving(false);
    }
  };

  const deleteAPIKey = async () => {
    setCredentialSaving(true);
    setCredentialError(null);
    try {
      await deleteLLMCredential(provider.id, credentialBackendURL);
      setCredentialStatus((current) => current ? { ...current, configured: false, updated_at: undefined } : current);
      setAPIKey('');
      setModelRefreshVersion((current) => current + 1);
    } catch (error) {
      setCredentialError(error instanceof Error ? error.message : '删除 API Key 失败');
    } finally {
      setCredentialSaving(false);
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
              <p className="mt-2 text-sm leading-6 text-slate-500">模型与网关选择会应用到个股 AI 分析；密钥可加密保存在后端，并仅在本次分析时转发给 sidecar。</p>
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
                  autoComplete="new-password"
                  value={apiKey}
                  className="pl-10"
                  placeholder={credentialStatus?.configured ? '已安全保存；输入新 Key 可覆盖' : '输入 API Key 后保存到后端'}
                  onChange={(event) => setAPIKey(event.target.value)}
                />
                <KeyRound className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  disabled={credentialSaving || credentialLoading || !apiKey.trim() || (customProvider && !credentialBackendURL)}
                  onClick={() => void saveAPIKey()}
                >
                  {credentialSaving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
                  {credentialStatus?.configured ? '更新 Key' : '保存 Key'}
                </Button>
                {credentialStatus?.configured ? (
                  <Button type="button" variant="outline" disabled={credentialSaving} onClick={() => void deleteAPIKey()}>
                    <Trash2 className="h-4 w-4" />
                    删除 Key
                  </Button>
                ) : null}
                <span className={credentialStatus?.configured ? 'text-xs text-emerald-300' : 'text-xs text-slate-500'}>
                  {credentialLoading
                    ? '正在读取状态...'
                    : credentialStatus?.configured
                      ? '已加密保存在后端'
                      : credentialStatus?.storage_ready === false
                        ? '后端安全存储尚未配置'
                        : '尚未保存'}
                </span>
              </div>
              <p className="text-xs text-slate-500">保存后前端会立即清空输入框，后端不会回显原始 Key。</p>
              {credentialError ? <p className="text-xs text-red-300">{credentialError}</p> : null}
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
                  {availableDeepModels.map((model) => (
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
                  {availableQuickModels.map((model) => (
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

          <div className="flex flex-col gap-3 rounded-lg border border-slate-800 bg-slate-950/35 p-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <p className="text-sm font-semibold text-slate-200">可用模型列表</p>
              <p className="mt-1 text-xs text-slate-500">
                {modelLoading
                  ? '正在从模型服务自动获取...'
                  : discoveredModels.length > 0
                    ? `已从 ${modelSource} 获取 ${discoveredModels.length} 个可用模型`
                    : '会使用已保存的 Key 或 sidecar 服务端密钥自动获取；失败时保留内置模型列表'}
              </p>
              {modelError ? <p className="mt-2 text-xs text-amber-300">{modelError}</p> : null}
            </div>
            <Button
              type="button"
              variant="outline"
              disabled={modelLoading || (customProvider && !config.backend_url.trim())}
              onClick={() => setModelRefreshVersion((current) => current + 1)}
            >
              {modelLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
              {modelLoading ? '获取中' : '重新获取'}
            </Button>
          </div>

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
