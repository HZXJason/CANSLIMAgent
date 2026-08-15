import { Building2, KeyRound, Loader2, RefreshCw, Save, SlidersHorizontal, Trash2 } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { PageHeader } from '../../components/PageHeader';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Checkbox } from '../../components/ui/checkbox';
import { ConfirmDialog } from '../../components/ui/confirm-dialog';
import { Input } from '../../components/ui/input';
import { Label } from '../../components/ui/label';
import { Select } from '../../components/ui/select';
import {
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
import { ASSET_MARKET_OPTIONS } from '../../lib/markets';
import { useMarketStore } from '../../stores/marketStore';
import type { AssetMarket } from '../../types';

export default function SettingsPage() {
  const assetMarket = useMarketStore((state) => state.assetMarket);
  const setAssetMarket = useMarketStore((state) => state.setAssetMarket);
  const [options, setOptions] = useState<AIAnalysisConfigResponse>(FALLBACK_AI_ANALYSIS_OPTIONS);
  const [config, setConfig] = useState<AnalysisConfig>(() =>
    normalizeAIAnalysisConfig(readStoredAIAnalysisConfig(), FALLBACK_AI_ANALYSIS_OPTIONS)
  );
  const [loadError, setLoadError] = useState<string | null>(null);
  const [configReady, setConfigReady] = useState(false);
  const [discoveredModels, setDiscoveredModels] = useState<string[]>([]);
  const [modelError, setModelError] = useState<string | null>(null);
  const [modelLoading, setModelLoading] = useState(false);
  const [modelRefreshVersion, setModelRefreshVersion] = useState(0);
  const [apiKey, setAPIKey] = useState('');
  const [credentialStatus, setCredentialStatus] = useState<LLMCredentialStatus | null>(null);
  const [credentialLoading, setCredentialLoading] = useState(false);
  const [credentialSaving, setCredentialSaving] = useState(false);
  const [credentialError, setCredentialError] = useState<string | null>(null);
  const [deleteKeyConfirmOpen, setDeleteKeyConfirmOpen] = useState(false);
  const credentialWriteVersionRef = useRef(0);

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
  const credentialTarget = `${provider.id}\n${credentialBackendURL}`;
  const credentialTargetRef = useRef(credentialTarget);

  useEffect(() => {
    if (credentialTargetRef.current !== credentialTarget) {
      credentialTargetRef.current = credentialTarget;
      setAPIKey('');
    }
    setCredentialError(null);
    if (!configReady || (customProvider && !credentialBackendURL)) {
      setCredentialStatus(null);
      setCredentialLoading(false);
      return;
    }
    let cancelled = false;
    const writeVersion = credentialWriteVersionRef.current;
    setCredentialLoading(true);
    const timeoutId = window.setTimeout(() => {
      void fetchLLMCredentialStatus(provider.id, credentialBackendURL)
        .then((status) => {
          if (!cancelled && writeVersion === credentialWriteVersionRef.current) setCredentialStatus(status);
        })
        .catch((error) => {
          if (!cancelled && writeVersion === credentialWriteVersionRef.current) {
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
  }, [configReady, credentialBackendURL, credentialTarget, customProvider, provider.id]);

  useEffect(() => {
    if (!configReady) return;
    const backendURL = config.backend_url.trim();
    if (customProvider && !backendURL) {
      setDiscoveredModels([]);
      setModelLoading(false);
      setModelError(null);
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
    setModelError(null);
  };

  const updateBackendURL = (value: string) => {
    setConfig((current) => ({ ...current, backend_url: value }));
    setDiscoveredModels([]);
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
    credentialWriteVersionRef.current += 1;
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
    credentialWriteVersionRef.current += 1;
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
    <section className="page-ledger mx-auto flex w-full max-w-[1100px] flex-col gap-6 px-4 py-6 sm:px-6 sm:py-8 lg:px-8 lg:py-10">
      <PageHeader title="设置" />

      <Card className="overflow-hidden p-0">
        <div className="flex items-center gap-3 border-b border-slate-800 px-5 py-4 sm:px-6">
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-sm border border-brand-400/20 bg-brand-400/10 text-brand-300">
            <Building2 className="h-5 w-5" aria-hidden="true" />
          </div>
          <div>
            <h2 className="font-display text-xl font-bold text-ink">市场偏好</h2>
          </div>
        </div>
        <div className="p-5 sm:p-6">
          <div className="max-w-md space-y-2">
            <Label htmlFor="settings-asset-market">主要投资市场</Label>
            <Select id="settings-asset-market" value={assetMarket} onValueChange={(value) => setAssetMarket(value as AssetMarket)}>
              {ASSET_MARKET_OPTIONS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </Select>
          </div>
        </div>
      </Card>

      <Card className="overflow-hidden p-0">
        <div className="flex items-center gap-3 border-b border-slate-800 px-5 py-4 sm:px-6">
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-sm border border-brand-400/20 bg-brand-400/10 text-brand-300">
            <SlidersHorizontal className="h-5 w-5" />
          </div>
          <h2 className="font-display text-xl font-bold text-ink">AI 分析配置</h2>
        </div>

        <div className="space-y-6 p-5 sm:p-6">
          {loadError ? <p className="rounded-xl border border-amber-700/60 bg-amber-950/30 px-4 py-3 text-sm text-amber-200">{loadError}</p> : null}

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="settings-ai-provider">模型服务</Label>
              <Select id="settings-ai-provider" value={config.llm_provider} onValueChange={updateProvider}>
                {options.providers.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </Select>
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
              </div>
            ) : null}
          </div>

          <div className="rounded-sm border border-rule bg-slate-950/25 p-4">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex min-w-0 items-center gap-3">
                <h3 className="font-bold text-slate-100">API Key</h3>
                <span className={credentialStatus?.configured ? 'rounded-full bg-emerald-400/10 px-2.5 py-1 text-xs font-semibold text-emerald-100' : 'rounded-full bg-slate-800 px-2.5 py-1 text-xs font-semibold text-slate-400'}>
                  {credentialLoading
                    ? '读取中'
                    : credentialStatus?.configured
                      ? '已加密保存在后端'
                      : credentialStatus?.storage_ready === false
                        ? '安全存储未配置'
                        : '未配置'}
                </span>
              </div>
              {credentialStatus?.configured ? (
                <Button className="self-start px-3 py-2 sm:self-auto" type="button" variant="danger" disabled={credentialSaving} onClick={() => setDeleteKeyConfirmOpen(true)}>
                  <Trash2 className="h-4 w-4" />
                  删除 Key
                </Button>
              ) : null}
            </div>
            <Label className="sr-only" htmlFor="settings-ai-provider-key">{provider.name} API Key</Label>
            <div className="mt-3 flex flex-col gap-2 sm:flex-row">
              <div className="relative min-w-0 flex-1">
                <Input
                  id="settings-ai-provider-key"
                  type="password"
                  autoComplete="new-password"
                  value={apiKey}
                  className="pl-10"
                  placeholder={credentialStatus?.configured ? '输入新 Key 进行更新' : `输入 ${provider.name} API Key`}
                  onChange={(event) => setAPIKey(event.target.value)}
                />
                <KeyRound className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
              </div>
              <Button
                className="shrink-0"
                type="button"
                disabled={credentialSaving || credentialStatus?.storage_ready === false || !apiKey.trim() || (customProvider && !credentialBackendURL)}
                onClick={() => void saveAPIKey()}
              >
                {credentialSaving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
                {credentialStatus?.configured ? '更新 Key' : '保存 Key'}
              </Button>
            </div>
            {credentialError ? <p className="mt-3 text-sm text-red-300">{credentialError}</p> : null}
          </div>

          <div>
            <div className="mb-3 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex items-center gap-3">
                <h3 className="font-bold text-slate-100">模型</h3>
                {modelLoading ? <span className="text-sm text-slate-400">获取中</span> : null}
                {!modelLoading && discoveredModels.length > 0 ? <span className="text-sm text-slate-400">{discoveredModels.length} 个可用模型</span> : null}
              </div>
              <Button
                className="self-start px-3 py-2 sm:self-auto"
                type="button"
                variant="outline"
                disabled={modelLoading || (customProvider && !config.backend_url.trim())}
                onClick={() => setModelRefreshVersion((current) => current + 1)}
              >
                {modelLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
                刷新模型
              </Button>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="settings-ai-deep-model">深度思考模型</Label>
                {customProvider && discoveredModels.length === 0 ? (
                  <Input
                    id="settings-ai-deep-model"
                    value={config.deep_think_llm}
                    placeholder="输入模型名称"
                    onChange={(event) => setConfig((current) => ({ ...current, deep_think_llm: event.target.value }))}
                  />
                ) : (
                  <Select id="settings-ai-deep-model" value={config.deep_think_llm} onValueChange={(value) => setConfig((current) => ({ ...current, deep_think_llm: value }))}>
                    {availableDeepModels.map((model) => <option key={model} value={model}>{model}</option>)}
                  </Select>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="settings-ai-quick-model">快速响应模型</Label>
                {customProvider && discoveredModels.length === 0 ? (
                  <Input
                    id="settings-ai-quick-model"
                    value={config.quick_think_llm}
                    placeholder="输入模型名称"
                    onChange={(event) => setConfig((current) => ({ ...current, quick_think_llm: event.target.value }))}
                  />
                ) : (
                  <Select id="settings-ai-quick-model" value={config.quick_think_llm} onValueChange={(value) => setConfig((current) => ({ ...current, quick_think_llm: value }))}>
                    {availableQuickModels.map((model) => <option key={model} value={model}>{model}</option>)}
                  </Select>
                )}
              </div>
            </div>
            {modelError ? <p className="mt-3 text-sm text-amber-300">{modelError}</p> : null}
          </div>

          <label className="flex cursor-pointer items-center gap-3 rounded-sm border border-rule bg-slate-950/25 px-4 py-3 transition hover:border-brand-400/40">
            <Checkbox
              checked={config.online_tools}
              onCheckedChange={(checked) => setConfig((current) => ({ ...current, online_tools: checked }))}
            />
            <span className="text-sm font-bold text-slate-100">启用联网工具</span>
          </label>
        </div>
      </Card>
      <BrokerConnections />
      <ConfirmDialog
        open={deleteKeyConfirmOpen}
        title="删除 API Key"
        description={`删除 ${provider.name} API Key 后，AI 分析将无法使用该密钥。`}
        confirmText="删除 Key"
        onConfirm={() => void deleteAPIKey()}
        onOpenChange={setDeleteKeyConfirmOpen}
      />
    </section>
  );
}
