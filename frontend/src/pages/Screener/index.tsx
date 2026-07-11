import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Activity, DatabaseZap, Pause, Play, RefreshCw, ShieldCheck } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Card, CardHeader, CardTitle } from '../../components/ui/card';
import { API_BASE_URL, createPreset, deletePreset, fetchCachedResults, fetchPresets, fetchRefreshJob, refreshMarket } from '../../lib/api';
import { computeStock, stockPassesFilter } from '../../lib/computeLabels';
import { sortStocks } from '../../lib/sortStocks';
import { formatDateCN } from '../../lib/utils';
import { useScreenerStore } from '../../stores/screenerStore';
import type { Market, ScanEvent, ThresholdPreset } from '../../types';
import MarketSelector from './MarketSelector';
import ScanProgress from './ScanProgress';
import StockList from './StockList';
import TagFilter from './TagFilter';
import ThresholdPanel from './ThresholdPanel';

const SCAN_STALL_TIMEOUT_MS = 45_000;

export default function ScreenerPage() {
  const queryClient = useQueryClient();
  const eventSourceRef = useRef<EventSource | null>(null);
  const retryRef = useRef(0);
  const scanRunRef = useRef(0);
  const scanTimeoutRef = useRef<number | null>(null);
  const defaultPresetAppliedRef = useRef(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [isScanPaused, setIsScanPaused] = useState(false);
  const [scanCompletedMarket, setScanCompletedMarket] = useState<Market | null>(null);
  const [isRefreshing, setIsRefreshing] = useState(false);

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

  const cachedQuery = useQuery({
    queryKey: ['screener-results', selectedMarket],
    queryFn: () => fetchCachedResults(selectedMarket)
  });

  const presetsQuery = useQuery({
    queryKey: ['threshold-presets'],
    queryFn: fetchPresets
  });

  const appliedCacheAtRef = useRef<Record<Market, number>>({
    SP500: 0,
    NASDAQ100: 0,
    RUSSELL2000: 0
  });

  useEffect(() => {
    if (
      cachedQuery.data &&
      activeScanMarket !== selectedMarket &&
      appliedCacheAtRef.current[selectedMarket] !== cachedQuery.dataUpdatedAt
    ) {
      appliedCacheAtRef.current[selectedMarket] = cachedQuery.dataUpdatedAt;
      setMarketStocks(selectedMarket, cachedQuery.data);
      setErrorMessage(null);
    }
  }, [activeScanMarket, cachedQuery.data, cachedQuery.dataUpdatedAt, selectedMarket, setMarketStocks]);

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

  const marketLabel: Record<Market, string> = {
    SP500: '标普 500',
    NASDAQ100: '纳指 100',
    RUSSELL2000: '罗素 2000'
  };

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
		setScanCompletedMarket(null);
      setScanStatus(null);
      setNotice(null);
      setErrorMessage(message);
    },
    [clearScanTimeout, setActiveScanMarket, setIsScanning, setScanStatus]
  );

  const armScanTimeout = useCallback(
    (runID: number, market: Market) => {
      clearScanTimeout();
      scanTimeoutRef.current = window.setTimeout(() => {
        if (scanRunRef.current !== runID) return;
        stopScanWithError('数据源连接超时，请检查网络连接、代理或后端服务状态后重试');
        void queryClient.invalidateQueries({ queryKey: ['screener-results', market] });
      }, SCAN_STALL_TIMEOUT_MS);
    },
    [clearScanTimeout, queryClient, stopScanWithError]
  );

  const connectToScan = useCallback(
    function connectToScan(runID: number, market: Market) {
      if (scanRunRef.current !== runID) return;
      const source = new EventSource(API_BASE_URL + '/screener/scan?market=' + market);
      eventSourceRef.current = source;

      source.onopen = () => {
        if (scanRunRef.current !== runID) return;
        setScanStatus('已连接后端，正在请求数据源');
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
            setScanProgress(event.data);
            if (event.data.total > 0) {
              setScanStatus(null);
            }
          }
          if (event.type === 'scan_complete') {
            clearScanTimeout();
            setScanProgress(event.data);
            setIsScanning(false);
			setIsScanPaused(false);
			setScanCompletedMarket(market);
            setScanStatus(null);
            setActiveScanMarket(null);
            source.close();
            eventSourceRef.current = null;
            void queryClient.invalidateQueries({ queryKey: ['screener-results', market] });
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
          setScanStatus('连接中断，正在第 ' + retryRef.current + ' 次自动重连');
          armScanTimeout(runID, market);
          window.setTimeout(() => connectToScan(runID, market), 700 * retryRef.current);
          return;
        }
        stopScanWithError('数据获取失败，请检查网络连接或后端服务状态');
        void queryClient.invalidateQueries({ queryKey: ['screener-results', market] });
      };
    },
    [
      addStockResult,
      armScanTimeout,
      clearScanTimeout,
      queryClient,
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
		setScanCompletedMarket(null);
    setScanStatus(null);

    if (typeof navigator !== 'undefined' && !navigator.onLine) {
      setErrorMessage('当前浏览器处于离线状态，请检查网络连接后重试');
      return;
    }

    setActiveScanMarket(selectedMarket);
    setIsScanning(true);
    setScanProgress({ analyzed: selectedMarketStocks.length, total: Math.max(scanProgress.total, selectedMarketStocks.length) });
    setScanStatus('正在连接后端服务，准备断点接续');
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
		setScanStatus('扫描已暂停；数据源任务仍在后台保留，可继续接收进度');
		setNotice('扫描已暂停。点击“继续扫描”可接回当前进度。');
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
		setScanStatus('正在继续接收后台扫描进度');
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
    setScanStatus('正在接回后台扫描');
    armScanTimeout(runID, market);
    connectToScan(runID, market);
  }, [activeScanMarket, armScanTimeout, connectToScan, isScanning, selectedMarket, setActiveScanMarket, setScanStatus]);

  const handleMarketChange = (market: Market) => {
    if (market === selectedMarket) return;
    setErrorMessage(null);
    setNotice(null);
		setScanCompletedMarket(null);
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
      setErrorMessage('欧奈尔默认方案不可删除');
      return;
    }
    if (!window.confirm('确认删除阈值方案「' + preset.name.replace(/奥尼尔/g, '欧奈尔') + '」？')) return;
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
		setNotice('覆盖刷新完成，缓存已更新。');
		setIsRefreshing(false);
		setScanCompletedMarket(selectedMarket);
        await queryClient.invalidateQueries({ queryKey: ['screener-results', selectedMarket] });
        return;
      }
      if (job.status === 'failed') {
		setIsRefreshing(false);
        throw new Error(job.error ?? '数据刷新失败');
      }
    }
		setNotice('覆盖刷新仍在后台执行；完成后会自动更新缓存。');
		setIsRefreshing(false);
  };

  const handleRefresh = async () => {
		if (isScanning || isScanPaused || isRefreshing) return;
    setErrorMessage(null);
		setNotice(null);
		setIsRefreshing(true);
    try {
      const job = await refreshMarket(selectedMarket);
		setNotice('正在覆盖刷新 ' + marketLabel[selectedMarket] + ' 全部缓存数据…');
      void pollRefreshJob(job.job_id).catch((error) => {
		setIsRefreshing(false);
		setErrorMessage('覆盖刷新失败：' + (error instanceof Error ? error.message : '后端任务异常'));
      });
    } catch (error) {
		setIsRefreshing(false);
		setErrorMessage('覆盖刷新请求失败：' + (error instanceof Error ? error.message : '无法创建后台任务'));
    }
  };

	const scanBusy = isScanning || isScanPaused;
	const primaryActionLabel = isScanning
		? '暂停扫描'
		: isScanPaused
			? '继续扫描'
			: isRefreshing
				? '覆盖刷新中'
				: scanCompletedMarket === selectedMarket
					? '覆盖刷新'
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
		if (scanCompletedMarket === selectedMarket) {
			void handleRefresh();
			return;
		}
		startScan();
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
                数据截止 {formatDateCN(selectedMarketDataAsOf)}
              </span>
            </div>
          </header>

          <Card className="mt-6">
            <CardHeader>
              <CardTitle className="flex items-center gap-3">
                <span className="flex h-10 w-10 items-center justify-center rounded-2xl bg-finance-blue/80">
                  <ShieldCheck className="h-5 w-5 text-cyan-100" />
                </span>
                <span>
                  <span className="block text-lg">市场筛选</span>
                </span>
              </CardTitle>
            </CardHeader>
			<MarketSelector selectedMarket={selectedMarket} onChange={handleMarketChange} disabled={scanBusy || isRefreshing} />
          </Card>

          <section className="mt-6 space-y-6">
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

            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-3">
                  <span className="flex h-10 w-10 items-center justify-center rounded-2xl bg-cyan-400 text-slate-950">
                    <Activity className="h-5 w-5" />
                  </span>
                  <span>
                    <span className="block text-lg">扫描控制台</span>
                  </span>
                </CardTitle>
              </CardHeader>
              <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
                <ScanProgress progress={scanProgress} isScanning={isScanning} statusMessage={scanStatus} />
                <div className="flex flex-wrap gap-3 lg:justify-end">
				  <Button onClick={handlePrimaryAction} disabled={isRefreshing}>
					{isScanning ? <Pause className="h-4 w-4" /> : isScanPaused ? <Play className="h-4 w-4" /> : scanCompletedMarket === selectedMarket || isRefreshing ? <RefreshCw className="h-4 w-4" /> : <Activity className="h-4 w-4" />}
					{primaryActionLabel}
				  </Button>
                </div>
              </div>
              <div className="mt-5 grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
                <div className="rounded-2xl border border-slate-800 bg-slate-950/35 p-4 text-sm text-slate-400">
                  建议：扫描结果先按 C/A/S/L 规则权重排序，筛选标签按 C/A/N/S/L/I 组合过滤。
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
            <StockList stocks={filteredStocks} hasRawData={selectedMarketStocks.length > 0} filtered={activeTagFilters.length > 0} />
          </div>
    </section>
  );
}
