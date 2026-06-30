import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Activity, DatabaseZap, RefreshCw, ShieldCheck } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Card, CardHeader, CardTitle } from '../../components/ui/card';
import { API_BASE_URL, createPreset, deletePreset, fetchCachedResults, fetchPresets, fetchRefreshJob, refreshMarket } from '../../lib/api';
import { ACTIVE_LABEL_KEYS, computeStock } from '../../lib/computeLabels';
import { sortStocks } from '../../lib/sortStocks';
import { formatDateCN } from '../../lib/utils';
import { useScreenerStore } from '../../stores/screenerStore';
import type { Market, ScanEvent, ThresholdPreset } from '../../types';
import MarketSelector from './MarketSelector';
import ScanProgress from './ScanProgress';
import StockList from './StockList';
import TagFilter from './TagFilter';
import ThresholdPanel from './ThresholdPanel';

export default function ScreenerPage() {
  const queryClient = useQueryClient();
  const eventSourceRef = useRef<EventSource | null>(null);
  const retryRef = useRef(0);
  const defaultPresetAppliedRef = useRef(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const {
    selectedMarket,
    setMarket,
    thresholds,
    setThresholds,
    activePresetId,
    presets,
    setPresets,
    applyPreset,
    thresholdPanelOpen,
    toggleThresholdPanel,
    rawStocks,
    setRawStocks,
    addStockResult,
    clearResults,
    scanProgress,
    setScanProgress,
    dataAsOf,
    isScanning,
    setIsScanning,
    activeTagFilters,
    toggleTagFilter
  } = useScreenerStore();

  const cachedQuery = useQuery({
    queryKey: ['screener-results', selectedMarket],
    queryFn: () => fetchCachedResults(selectedMarket)
  });

  const presetsQuery = useQuery({
    queryKey: ['threshold-presets'],
    queryFn: fetchPresets
  });

  useEffect(() => {
    if (cachedQuery.data && !isScanning) {
      setRawStocks(cachedQuery.data);
      setErrorMessage(null);
    }
  }, [cachedQuery.data, isScanning, setRawStocks]);

  useEffect(() => {
    if (cachedQuery.isError && rawStocks.length === 0) {
      setErrorMessage('数据获取失败，请检查网络连接');
    }
  }, [cachedQuery.isError, rawStocks.length]);

  useEffect(() => {
    if (presetsQuery.data) {
      setPresets(presetsQuery.data);
      const defaultPreset = presetsQuery.data.find((preset) => preset.is_default);
      if (defaultPreset && !defaultPresetAppliedRef.current) {
        applyPreset(defaultPreset);
        defaultPresetAppliedRef.current = true;
      }
    }
  }, [applyPreset, presetsQuery.data, setPresets]);

  useEffect(() => {
    return () => eventSourceRef.current?.close();
  }, []);

  const computedStocks = useMemo(() => {
    return sortStocks(rawStocks.map((stock) => computeStock(stock, thresholds)));
  }, [rawStocks, thresholds]);

  const filteredStocks = useMemo(() => {
    if (activeTagFilters.length === 0) return computedStocks;
    return computedStocks.filter((stock) => activeTagFilters.every((tag) => stock.labels[tag].status === 'pass'));
  }, [activeTagFilters, computedStocks]);

  const labelResults = useMemo(() => computedStocks.flatMap((stock) => ACTIVE_LABEL_KEYS.map((key) => stock.labels[key])), [computedStocks]);
  const dataCompleteness = labelResults.length
    ? Math.round((labelResults.filter((label) => label.sufficient).length / labelResults.length) * 100)
    : 0;
  const filteredRate = computedStocks.length ? Math.round((filteredStocks.length / computedStocks.length) * 100) : 0;

  const marketLabel: Record<Market, string> = {
    SP500: '标普 500',
    NASDAQ100: '纳指 100',
    RUSSELL2000: '罗素 2000'
  };

  const startScan = useCallback(() => {
    eventSourceRef.current?.close();
    retryRef.current = 0;
    clearResults();
    setErrorMessage(null);
    setNotice(null);
    setIsScanning(true);
    setScanProgress({ analyzed: 0, total: 0 });

    const connect = () => {
      const source = new EventSource(API_BASE_URL + '/screener/scan?market=' + selectedMarket);
      eventSourceRef.current = source;

      source.onmessage = (message) => {
        try {
          const event = JSON.parse(message.data) as ScanEvent;
          if (event.type === 'stock_result') {
            addStockResult(event.data);
          }
          if (event.type === 'scan_progress') {
            setScanProgress(event.data);
          }
          if (event.type === 'scan_complete') {
            setScanProgress(event.data);
            setIsScanning(false);
            source.close();
            void queryClient.invalidateQueries({ queryKey: ['screener-results', selectedMarket] });
          }
          if (event.type === 'scan_error') {
            setIsScanning(false);
            setErrorMessage(event.data.message || '数据获取失败，请检查网络连接');
            source.close();
          }
        } catch {
          setErrorMessage('数据格式异常，请稍后重试');
        }
      };

      source.onerror = () => {
        source.close();
        if (retryRef.current < 3) {
          retryRef.current += 1;
          setNotice('连接中断，正在第 ' + retryRef.current + ' 次自动重连');
          window.setTimeout(connect, 700 * retryRef.current);
          return;
        }
        setIsScanning(false);
        setErrorMessage('数据获取失败，请检查网络连接');
        void cachedQuery.refetch();
      };
    };

    connect();
  }, [
    addStockResult,
    cachedQuery,
    clearResults,
    queryClient,
    selectedMarket,
    setIsScanning,
    setScanProgress
  ]);

  const handleMarketChange = (market: Market) => {
    if (market === selectedMarket) return;
    if (rawStocks.length > 0 && !window.confirm('切换市场将清空当前结果，是否继续？')) {
      return;
    }
    eventSourceRef.current?.close();
    setIsScanning(false);
    clearResults();
    setErrorMessage(null);
    setNotice(null);
    setMarket(market);
  };

  const handleSavePreset = async () => {
    const name = window.prompt('请输入方案名称', '自定义方案');
    if (!name?.trim()) return;
    try {
      const saved = await createPreset(name.trim(), thresholds);
      setNotice('阈值方案已保存：' + saved.name);
      applyPreset(saved);
      await queryClient.invalidateQueries({ queryKey: ['threshold-presets'] });
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : '阈值方案保存失败');
    }
  };

  const handleDeletePreset = async (preset: ThresholdPreset) => {
    if (preset.is_default) {
      setErrorMessage('奥尼尔默认方案不可删除');
      return;
    }
    if (!window.confirm('确认删除阈值方案「' + preset.name + '」？')) return;
    try {
      await deletePreset(preset.id);
      setNotice('阈值方案已删除');
      await queryClient.invalidateQueries({ queryKey: ['threshold-presets'] });
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : '阈值方案删除失败');
    }
  };

  const pollRefreshJob = async (jobID: string) => {
    for (let i = 0; i < 30; i += 1) {
      await new Promise((resolve) => window.setTimeout(resolve, 1000));
      const job = await fetchRefreshJob(jobID);
      if (job.status === 'completed') {
        setNotice('数据刷新完成，可以重新开始筛选');
        await queryClient.invalidateQueries({ queryKey: ['screener-results', selectedMarket] });
        return;
      }
      if (job.status === 'failed') {
        throw new Error(job.error ?? '数据刷新失败');
      }
    }
    setNotice('刷新任务仍在后台执行，可稍后重新获取缓存结果');
  };

  const handleRefresh = async () => {
    setErrorMessage(null);
    try {
      const job = await refreshMarket(selectedMarket);
      setNotice('刷新任务已创建：' + job.job_id.slice(0, 8));
      void pollRefreshJob(job.job_id).catch((error) => {
        setErrorMessage(error instanceof Error ? error.message : '数据获取失败，请检查网络连接');
      });
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : '数据获取失败，请检查网络连接');
    }
  };

  return (
    <section className="mx-auto w-full max-w-[1440px] px-4 py-6 sm:px-6 lg:px-8">
          <header className="flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
            <div>
              <p className="text-sm font-semibold text-cyan-300/80">个人投资研究工具</p>
              <h1 className="mt-2 text-3xl font-black tracking-tight text-white sm:text-4xl">CANSLIM 选股工作台</h1>
              <p className="mt-3 max-w-2xl text-sm text-slate-400">
                从市场池、阈值策略到候选股排序的一屏式研究流程。
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-3">
              <span className="rounded-full bg-emerald-400 px-4 py-2 text-sm font-bold text-slate-950">{marketLabel[selectedMarket]}</span>
              <span className="rounded-full border border-slate-700 bg-slate-950/30 px-4 py-2 text-sm text-slate-400">
                数据截止 {formatDateCN(dataAsOf)}
              </span>
              <Button variant="outline" onClick={handleRefresh}>
                <RefreshCw className="h-4 w-4" />
                刷新数据
              </Button>
            </div>
          </header>

          <section className="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <Card>
              <div className="flex items-center gap-2 text-sm text-slate-400">
                <span className="h-2.5 w-2.5 rounded-full bg-cyan-300" />
                候选股票
              </div>
              <p className="mt-4 text-3xl font-black text-white">{computedStocks.length}</p>
              <p className="mt-3 text-sm text-slate-500">来自缓存与实时扫描结果</p>
            </Card>
            <Card>
              <div className="flex items-center gap-2 text-sm text-slate-400">
                <span className="h-2.5 w-2.5 rounded-full bg-emerald-400" />
                当前入选率
              </div>
              <p className="mt-4 text-3xl font-black text-white">{filteredRate}%</p>
              <p className="mt-3 text-sm text-slate-500">受标签 AND 条件影响</p>
            </Card>
            <Card>
              <div className="flex items-center gap-2 text-sm text-slate-400">
                <span className="h-2.5 w-2.5 rounded-full bg-amber-300" />
                数据完整度
              </div>
              <p className="mt-4 text-3xl font-black text-white">{dataCompleteness}%</p>
              <p className="mt-3 text-sm text-slate-500">黄色 ? 表示指标不足</p>
            </Card>
            <Card>
              <div className="flex items-center gap-2 text-sm text-slate-400">
                <span className="h-2.5 w-2.5 rounded-full bg-finance-blue" />
                扫描进度
              </div>
              <p className="mt-4 text-3xl font-black text-white">
                {scanProgress.analyzed}/{scanProgress.total}
              </p>
              <p className="mt-3 text-sm text-slate-500">SSE 流式返回结果</p>
            </Card>
          </section>

          <Card className="mt-6">
            <CardHeader>
              <CardTitle className="flex items-center gap-3">
                <span className="flex h-10 w-10 items-center justify-center rounded-2xl bg-finance-blue/80">
                  <ShieldCheck className="h-5 w-5 text-cyan-100" />
                </span>
                <span>
                  <span className="block text-lg">市场选择</span>
                  <span className="mt-1 block text-sm font-normal text-slate-400">把三类股票池升级为可解释的市场卡片</span>
                </span>
              </CardTitle>
            </CardHeader>
            <MarketSelector selectedMarket={selectedMarket} onChange={handleMarketChange} />
          </Card>

          <section className="mt-6 grid gap-6 xl:grid-cols-2">
            <ThresholdPanel
              open={thresholdPanelOpen}
              thresholds={thresholds}
              presets={presets}
              activePresetId={activePresetId}
              onToggle={toggleThresholdPanel}
              onChange={setThresholds}
              onApplyPreset={applyPreset}
              onSavePreset={handleSavePreset}
              onDeletePreset={handleDeletePreset}
            />

            <Card className="h-full">
              <CardHeader>
                <CardTitle className="flex items-center gap-3">
                  <span className="flex h-10 w-10 items-center justify-center rounded-2xl bg-cyan-400 text-slate-950">
                    <Activity className="h-5 w-5" />
                  </span>
                  <span>
                    <span className="block text-lg">扫描控制台</span>
                    <span className="mt-1 block text-sm font-normal text-slate-400">实时进度、缓存信息和主要操作集中到同一卡片</span>
                  </span>
                </CardTitle>
              </CardHeader>
              <div className="space-y-5">
                <ScanProgress progress={scanProgress} isScanning={isScanning} />
                <div className="flex flex-wrap gap-3">
                  <Button onClick={startScan} disabled={isScanning}>
                    <Activity className="h-4 w-4" />
                    {isScanning ? '筛选中' : '开始筛选'}
                  </Button>
                  <Button variant="outline" onClick={handleRefresh}>
                    <RefreshCw className="h-4 w-4" />
                    刷新数据
                  </Button>
                </div>
                <div className="rounded-2xl border border-slate-800 bg-slate-950/35 p-4 text-sm text-slate-400">
                  建议：扫描结果进入表格前先进行 C/A/L 权重排序，再允许用标签 AND 过滤。
                </div>
                <div className="flex items-center gap-2 text-sm text-slate-400">
                  <DatabaseZap className="h-4 w-4 text-cyan-300" />
                  当前 API：{API_BASE_URL}
                </div>
              </div>
            </Card>
          </section>

          {errorMessage ? (
            <div className="mt-6 rounded-2xl border border-red-800 bg-red-950/60 px-4 py-3 text-sm text-red-100">{errorMessage}</div>
          ) : null}
          {notice ? (
            <div className="mt-6 rounded-2xl border border-cyan-400/40 bg-finance-navy/60 px-4 py-3 text-sm text-blue-100">{notice}</div>
          ) : null}

          <Card className="mt-6">
            <TagFilter activeTags={activeTagFilters} onToggle={toggleTagFilter} />
          </Card>

          <div className="mt-6">
            <StockList stocks={filteredStocks} hasRawData={rawStocks.length > 0} filtered={activeTagFilters.length > 0} />
          </div>
    </section>
  );
}
