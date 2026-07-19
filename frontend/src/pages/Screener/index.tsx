import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Activity, LoaderCircle, Pause, Play, RefreshCw, X } from 'lucide-react';
import { PageHeader } from '../../components/PageHeader';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { API_BASE_URL, createPreset, deletePreset, fetchCachedResults, fetchPresets, fetchRefreshJob, refreshMarket } from '../../lib/api';
import { computeStock, stockPassesFilter } from '../../lib/computeLabels';
import { SCREENER_MARKETS, assetMarketForScreenerMarket, screenerMarketLabel } from '../../lib/markets';
import { sortStocks } from '../../lib/sortStocks';
import { formatDateCN } from '../../lib/utils';
import { useMarketStore } from '../../stores/marketStore';
import { useScreenerStore } from '../../stores/screenerStore';
import type { Market, ScanEvent, ThresholdPreset } from '../../types';
import MarketSelector from './MarketSelector';
import ScanProgress from './ScanProgress';
import StockList from './StockList';
import TagFilter from './TagFilter';
import ThresholdPanel from './ThresholdPanel';

const SCAN_STALL_TIMEOUT_MS = 45_000;
const EMPTY_PROGRESS = { analyzed: 0, total: 0 };

export default function ScreenerPage() {
  const queryClient = useQueryClient();
  const assetMarket = useMarketStore((state) => state.assetMarket);
  const eventSourceRef = useRef<EventSource | null>(null);
  const retryRef = useRef(0);
  const scanRunRef = useRef(0);
  const scanTimeoutRef = useRef<number | null>(null);
  const activeScanProgressRef = useRef({ analyzed: 0, total: 0 });
  const marketProgressRef = useRef<Partial<Record<Market, { analyzed: number; total: number }>>>({});
  const defaultPresetAppliedRef = useRef(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [isScanPaused, setIsScanPaused] = useState(false);
  const [hasReceivedScanProgress, setHasReceivedScanProgress] = useState(false);
  const [scanCompletedMarkets, setScanCompletedMarkets] = useState<Set<Market>>(() => new Set());
  const [refreshingMarket, setRefreshingMarket] = useState<Market | null>(null);
  const isRefreshing = refreshingMarket !== null;

  const {
    selectedMarkets,
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
    setMarketStocks,
    addStockResult,
    scanProgress,
    setScanProgress,
    scanStatus,
    setScanStatus,
    activeScanMarket,
    setActiveScanMarket,
    isScanning,
    setIsScanning,
    activeTagFilters,
    toggleTagFilter
  } = useScreenerStore();
  const selectedMarket = selectedMarkets[assetMarket];
  const selectedMarketRef = useRef(selectedMarket);
  selectedMarketRef.current = selectedMarket;

  const cachedQuery = useQuery({
    queryKey: ['screener-results', assetMarket, selectedMarket],
    queryFn: () => fetchCachedResults(selectedMarket)
  });

  const presetsQuery = useQuery({
    queryKey: ['threshold-presets'],
    queryFn: fetchPresets
  });

  const appliedCacheAtRef = useRef<Partial<Record<Market, number>>>({});

  useEffect(() => {
    if (
      cachedQuery.data &&
      activeScanMarket !== selectedMarket &&
      appliedCacheAtRef.current[selectedMarket] !== cachedQuery.dataUpdatedAt
    ) {
      appliedCacheAtRef.current[selectedMarket] = cachedQuery.dataUpdatedAt;
      marketProgressRef.current[selectedMarket] = {
        analyzed: cachedQuery.data.length,
        total: cachedQuery.data.length
      };
      setMarketStocks(selectedMarket, cachedQuery.data);
      if (activeScanMarket !== null) {
        setScanProgress(activeScanProgressRef.current);
      }
      setErrorMessage(null);
    }
  }, [activeScanMarket, cachedQuery.data, cachedQuery.dataUpdatedAt, selectedMarket, setMarketStocks, setScanProgress]);

  useEffect(() => {
    if (cachedQuery.isError && !rawStocks.some((stock) => stock.market === selectedMarket)) {
      setErrorMessage('数据获取失败，请检查网络连接');
    }
  }, [cachedQuery.isError, rawStocks, selectedMarket]);

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
    return () => {
      eventSourceRef.current?.close();
      if (scanTimeoutRef.current !== null) {
        window.clearTimeout(scanTimeoutRef.current);
      }
      eventSourceRef.current = null;
    };
  }, []);

  const computedStocks = useMemo(() => {
    return sortStocks(rawStocks.map((stock) => computeStock(stock, thresholds)));
  }, [rawStocks, thresholds]);

  const selectedMarketStocks = useMemo(() => {
    return computedStocks.filter((stock) => stock.market === selectedMarket);
  }, [computedStocks, selectedMarket]);

  const filteredStocks = useMemo(() => {
    if (activeTagFilters.length === 0) return selectedMarketStocks;
    return selectedMarketStocks.filter((stock) => activeTagFilters.every((tag) => stockPassesFilter(stock, tag)));
  }, [activeTagFilters, selectedMarketStocks]);

  const selectedMarketDataAsOf = selectedMarketStocks[0]?.data_as_of ?? null;

  const progressForMarket = useCallback(
    (market: Market) => marketProgressRef.current[market] ?? EMPTY_PROGRESS,
    []
  );

  useEffect(() => {
    if (useScreenerStore.getState().activeScanMarket === null) {
      setScanProgress(progressForMarket(selectedMarket));
    }
    setErrorMessage(null);
    setNotice(null);
  }, [progressForMarket, selectedMarket, setScanProgress]);

  const clearScanTimeout = useCallback(() => {
    if (scanTimeoutRef.current !== null) {
      window.clearTimeout(scanTimeoutRef.current);
      scanTimeoutRef.current = null;
    }
  }, []);

  const stopScanWithError = useCallback(
    (message: string) => {
      eventSourceRef.current?.close();
      eventSourceRef.current = null;
      clearScanTimeout();
      setActiveScanMarket(null);
      setIsScanning(false);
      setIsScanPaused(false);
      setHasReceivedScanProgress(false);
      setScanProgress(progressForMarket(selectedMarketRef.current));
      setScanStatus(null);
      setNotice(null);
      setErrorMessage(message);
    },
    [clearScanTimeout, progressForMarket, setActiveScanMarket, setIsScanning, setScanProgress, setScanStatus]
  );

  const armScanTimeout = useCallback(
    (runID: number, market: Market) => {
      clearScanTimeout();
      scanTimeoutRef.current = window.setTimeout(() => {
        if (scanRunRef.current !== runID) return;
        stopScanWithError('数据源连接超时，请检查网络连接、代理或后端服务状态后重试');
        void queryClient.invalidateQueries({ queryKey: ['screener-results', assetMarketForScreenerMarket(market), market] });
      }, SCAN_STALL_TIMEOUT_MS);
    },
    [clearScanTimeout, queryClient, stopScanWithError]
  );

  const connectToScan = useCallback(
    function connectToScan(runID: number, market: Market) {
      if (scanRunRef.current !== runID) return;
      const source = new EventSource(API_BASE_URL + '/screener/scan?market=' + encodeURIComponent(market));
      eventSourceRef.current = source;

      source.onopen = () => {
        if (scanRunRef.current !== runID) return;
        setScanStatus('读取市场数据');
        armScanTimeout(runID, market);
      };

      source.onmessage = (message) => {
        if (scanRunRef.current !== runID) return;
        armScanTimeout(runID, market);
        try {
          const event = JSON.parse(message.data) as ScanEvent;
          if (event.type === 'stock_result') {
            addStockResult(event.data);
          }
          if (event.type === 'scan_status') {
            setScanStatus(event.data.message);
          }
          if (event.type === 'scan_progress') {
            activeScanProgressRef.current = event.data;
            marketProgressRef.current[market] = event.data;
            setHasReceivedScanProgress(true);
            setScanProgress(event.data);
            if (event.data.total > 0) {
              setScanStatus(null);
            }
          }
          if (event.type === 'scan_complete') {
            clearScanTimeout();
            activeScanProgressRef.current = event.data;
            marketProgressRef.current[market] = event.data;
            setHasReceivedScanProgress(true);
            const visibleMarket = selectedMarketRef.current;
            setScanProgress(visibleMarket === market ? event.data : progressForMarket(visibleMarket));
            setIsScanning(false);
            setIsScanPaused(false);
            setScanCompletedMarkets((current) => {
              const next = new Set(current);
              next.add(market);
              return next;
            });
            setScanStatus(null);
            setActiveScanMarket(null);
            source.close();
            eventSourceRef.current = null;
            void queryClient.invalidateQueries({ queryKey: ['screener-results', assetMarketForScreenerMarket(market), market] });
          }
          if (event.type === 'scan_error') {
            stopScanWithError(event.data.message || '数据获取失败，请检查网络连接或后端服务状态');
            source.close();
          }
        } catch {
          stopScanWithError('数据格式异常，请稍后重试');
        }
      };

      source.onerror = () => {
        if (scanRunRef.current !== runID) return;
        source.close();
        if (retryRef.current < 3) {
          retryRef.current += 1;
          setScanStatus('自动重连 ' + retryRef.current + ' / 3');
          armScanTimeout(runID, market);
          window.setTimeout(() => connectToScan(runID, market), 700 * retryRef.current);
          return;
        }
        stopScanWithError('数据获取失败，请检查网络连接或后端服务状态');
        void queryClient.invalidateQueries({ queryKey: ['screener-results', assetMarketForScreenerMarket(market), market] });
      };
    },
    [
      addStockResult,
      armScanTimeout,
      clearScanTimeout,
      queryClient,
      progressForMarket,
      setActiveScanMarket,
      setIsScanning,
      setScanProgress,
      setScanStatus,
      stopScanWithError
    ]
  );

  const startScan = useCallback(() => {
    eventSourceRef.current?.close();
    clearScanTimeout();
    retryRef.current = 0;
    scanRunRef.current += 1;
    const runID = scanRunRef.current;
    setErrorMessage(null);
    setNotice(null);
    setIsScanPaused(false);
    setHasReceivedScanProgress(false);
    setScanStatus(null);

    if (typeof navigator !== 'undefined' && !navigator.onLine) {
      setErrorMessage('当前浏览器处于离线状态，请检查网络连接后重试');
      return;
    }

    setActiveScanMarket(selectedMarket);
    setIsScanning(true);
    const initialProgress = {
      analyzed: selectedMarketStocks.length,
      total: Math.max(scanProgress.total, selectedMarketStocks.length)
    };
    activeScanProgressRef.current = initialProgress;
    marketProgressRef.current[selectedMarket] = initialProgress;
    setScanProgress(initialProgress);
    setScanStatus('连接数据源');
    armScanTimeout(runID, selectedMarket);
    connectToScan(runID, selectedMarket);
  }, [
    armScanTimeout,
    clearScanTimeout,
    connectToScan,
    scanProgress.total,
    selectedMarket,
    selectedMarketStocks.length,
    setActiveScanMarket,
    setIsScanning,
    setScanProgress,
    setScanStatus
  ]);

  const pauseScan = useCallback(() => {
    if (!isScanning) return;
    scanRunRef.current += 1;
    eventSourceRef.current?.close();
    eventSourceRef.current = null;
    clearScanTimeout();
    setIsScanning(false);
    setIsScanPaused(true);
    setScanStatus('扫描已暂停');
    setNotice(null);
  }, [clearScanTimeout, isScanning, setIsScanning, setScanStatus]);

  const resumeScan = useCallback(() => {
    const market = activeScanMarket ?? selectedMarket;
    if (!isScanPaused) {
      startScan();
      return;
    }
    retryRef.current = 0;
    scanRunRef.current += 1;
    const runID = scanRunRef.current;
    setErrorMessage(null);
    setNotice(null);
    setIsScanPaused(false);
    setActiveScanMarket(market);
    setIsScanning(true);
    setScanStatus('继续扫描');
    armScanTimeout(runID, market);
    connectToScan(runID, market);
  }, [activeScanMarket, armScanTimeout, connectToScan, isScanPaused, selectedMarket, setActiveScanMarket, setIsScanning, setScanStatus, startScan]);

  useEffect(() => {
    if (!isScanning || eventSourceRef.current) return;
    const market = activeScanMarket ?? selectedMarket;
    retryRef.current = 0;
    scanRunRef.current += 1;
    const runID = scanRunRef.current;
    setActiveScanMarket(market);
    setScanStatus('恢复扫描');
    armScanTimeout(runID, market);
    connectToScan(runID, market);
  }, [activeScanMarket, armScanTimeout, connectToScan, isScanning, selectedMarket, setActiveScanMarket, setScanStatus]);

  const handleMarketChange = (market: Market) => {
    if (market === selectedMarket) return;
    setErrorMessage(null);
    setNotice(null);
    if (activeScanMarket === null) {
      setScanProgress(progressForMarket(market));
    }
    setMarket(assetMarket, market);
  };

  const handleSavePreset = async (name: string) => {
    try {
      const saved = await createPreset(name, thresholds);
      setNotice('已保存「' + saved.name + '」');
      applyPreset(saved);
      await queryClient.invalidateQueries({ queryKey: ['threshold-presets'] });
      return true;
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : '阈值方案保存失败');
      return false;
    }
  };

  const handleDeletePreset = async (preset: ThresholdPreset) => {
    if (preset.is_default) {
      setErrorMessage('欧奈尔默认方案不可删除');
      return false;
    }
    try {
      await deletePreset(preset.id);
      const defaultPreset = presets.find((item) => item.is_default);
      if (defaultPreset) applyPreset(defaultPreset);
      setNotice('已删除「' + preset.name.replace(/奥尼尔/g, '欧奈尔') + '」');
      await queryClient.invalidateQueries({ queryKey: ['threshold-presets'] });
      return true;
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : '阈值方案删除失败');
      return false;
    }
  };

  const pollRefreshJob = async (jobID: string, market: Market) => {
    for (let i = 0; i < 30; i += 1) {
      await new Promise((resolve) => window.setTimeout(resolve, 1000));
      const job = await fetchRefreshJob(jobID);
      if (job.status === 'completed') {
        setNotice(screenerMarketLabel(market) + ' 刷新完成');
        setRefreshingMarket(null);
        setScanCompletedMarkets((current) => {
          const next = new Set(current);
          next.add(market);
          return next;
        });
        await queryClient.invalidateQueries({ queryKey: ['screener-results', assetMarketForScreenerMarket(market), market] });
        return;
      }
      if (job.status === 'failed') {
        setRefreshingMarket(null);
        throw new Error(job.error ?? '数据刷新失败');
      }
    }
    setNotice(screenerMarketLabel(market) + ' 仍在后台刷新');
    setRefreshingMarket(null);
  };

  const handleRefresh = async () => {
    if (isScanning || isScanPaused || isRefreshing) return;
    const market = selectedMarket;
    setErrorMessage(null);
    setNotice(null);
    setRefreshingMarket(market);
    try {
      const job = await refreshMarket(market);
      void pollRefreshJob(job.job_id, market).catch((error) => {
        setRefreshingMarket(null);
        setErrorMessage('刷新失败：' + (error instanceof Error ? error.message : '后端任务异常'));
      });
    } catch (error) {
      setRefreshingMarket(null);
      setErrorMessage('刷新请求失败：' + (error instanceof Error ? error.message : '无法创建后台任务'));
    }
  };

  const selectedMarketScanComplete = scanCompletedMarkets.has(selectedMarket);
  const operationMarket = activeScanMarket ?? refreshingMarket;
  const primaryActionLabel = isScanning
    ? '暂停扫描'
    : isScanPaused
      ? '继续扫描'
      : isRefreshing
        ? '刷新中'
        : selectedMarketScanComplete
          ? '覆盖刷新'
          : selectedMarketStocks.length > 0
            ? '继续扫描'
            : '开始扫描';
  const handlePrimaryAction = () => {
    if (isScanning) {
      pauseScan();
      return;
    }
    if (isScanPaused) {
      resumeScan();
      return;
    }
    if (selectedMarketScanComplete) {
      void handleRefresh();
      return;
    }
    startScan();
  };

  const clearTagFilters = () => {
    activeTagFilters.forEach(toggleTagFilter);
  };

  return (
    <section className="mx-auto w-full max-w-[1520px] px-4 py-6 sm:px-6 sm:py-8 lg:px-8 lg:py-10">
      <PageHeader
        eyebrow="市场研究"
        title="CANSLIM 选股"
        description="先用一致的增长与相对强度标准缩小范围，再进入个股证据与 AI 分析。"
        meta={
          <span className="inline-flex items-center gap-2 rounded-full border border-slate-800 bg-slate-950/35 px-3 py-1.5">
            <span className="h-1.5 w-1.5 rounded-full bg-brand-300" />
            数据日期 <span className="font-bold text-slate-200">{formatDateCN(selectedMarketDataAsOf)}</span>
          </span>
        }
      />

      <Card className="mt-6 overflow-hidden p-0">
        <div className="grid xl:grid-cols-[minmax(0,1.25fr)_minmax(360px,0.75fr)]">
          <section className="border-b border-slate-800/80 p-4 sm:p-5 xl:border-b-0 xl:border-r" aria-labelledby="screener-market-title">
            <div className="mb-3 flex items-center justify-between gap-3">
              <div>
                <h2 id="screener-market-title" className="text-base font-bold text-white">选择市场</h2>
                <p className="mt-1 text-xs leading-5 text-slate-500">切换不会中断其他市场正在进行的扫描</p>
              </div>
              <span className="hidden rounded-full border border-slate-800 px-2.5 py-1 text-xs font-semibold text-slate-500 sm:inline-flex">{SCREENER_MARKETS[assetMarket].length} 个市场</span>
            </div>
            <MarketSelector assetMarket={assetMarket} selectedMarket={selectedMarket} onChange={handleMarketChange} />
          </section>

          <section className="p-4 sm:p-5" aria-labelledby="screener-scan-title">
            <div className="flex h-full flex-col gap-4 sm:flex-row sm:items-end xl:flex-col xl:items-stretch">
            <div className="min-w-0 flex-1">
              <div className="mb-3 flex flex-wrap items-center gap-2.5">
                <h2 id="screener-scan-title" className="text-base font-bold text-white">市场扫描</h2>
                {operationMarket ? (
                  <span className="rounded-full border border-brand-300/15 bg-brand-300/[0.07] px-2.5 py-1 text-xs font-semibold text-brand-100">
                    {screenerMarketLabel(operationMarket)}
                  </span>
                ) : null}
              </div>
              <ScanProgress
                progress={scanProgress}
                isScanning={isScanning}
                isPaused={isScanPaused}
                isComplete={selectedMarketScanComplete}
                showProgress={!isRefreshing && ((hasReceivedScanProgress && (isScanning || isScanPaused)) || selectedMarketScanComplete)}
                statusMessage={isRefreshing ? '覆盖刷新中' : scanStatus}
              />
            </div>
            <Button
              type="button"
              className="w-full min-w-32 sm:w-auto xl:w-full"
              onClick={handlePrimaryAction}
              disabled={isRefreshing}
            >
              {isRefreshing ? (
                <LoaderCircle className="h-4 w-4 animate-spin" />
              ) : isScanning ? (
                <Pause className="h-4 w-4" />
              ) : isScanPaused ? (
                <Play className="h-4 w-4" />
              ) : selectedMarketScanComplete ? (
                <RefreshCw className="h-4 w-4" />
              ) : (
                <Activity className="h-4 w-4" />
              )}
              {primaryActionLabel}
            </Button>
          </div>
          </section>
        </div>
      </Card>

      <section className="mt-4">
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
      </section>

      {errorMessage ? (
        <div className="mt-4 flex items-start justify-between gap-3 rounded-2xl border border-red-500/30 bg-red-500/[0.08] px-4 py-3 text-sm text-red-100" role="alert">
          <span>{errorMessage}</span>
          <button type="button" className="interactive-press -m-2 flex h-10 w-10 shrink-0 items-center justify-center rounded-xl text-red-300 hover:bg-red-500/10 hover:text-white" onClick={() => setErrorMessage(null)} aria-label="关闭错误提示">
            <X className="h-4 w-4" />
          </button>
        </div>
      ) : null}
      {notice ? (
        <div className="mt-4 flex items-start justify-between gap-3 rounded-2xl border border-brand-300/25 bg-brand-300/[0.07] px-4 py-3 text-sm text-brand-50" role="status">
          <span>{notice}</span>
          <button type="button" className="interactive-press -m-2 flex h-10 w-10 shrink-0 items-center justify-center rounded-xl text-brand-300 hover:bg-brand-300/10 hover:text-white" onClick={() => setNotice(null)} aria-label="关闭提示">
            <X className="h-4 w-4" />
          </button>
        </div>
      ) : null}

      <Card className="mt-4 p-4 sm:p-5">
        <TagFilter activeTags={activeTagFilters} onToggle={toggleTagFilter} onClear={clearTagFilters} />
      </Card>

      <div className="mt-4">
        <StockList stocks={filteredStocks} hasRawData={selectedMarketStocks.length > 0} filtered={activeTagFilters.length > 0} />
      </div>
    </section>
  );
}
