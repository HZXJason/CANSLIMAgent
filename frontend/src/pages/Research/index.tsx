import { useEffect, useMemo, useState } from 'react';
import { ChevronDown, ChevronUp, Filter, Search } from 'lucide-react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Input } from '../../components/ui/input';
import { fetchCompanyProfile, fetchEPSQuarterly, fetchGrossMarginQuarterly, fetchKLine, fetchRevenueAnnual, fetchRS, fetchStockSummary } from '../../lib/api';
import { assetMarketForSymbol, normalizeSymbolForAsset } from '../../lib/markets';
import { createInitialAnalysisState, hasGeneratedAIAnalysis, useAnalysisStore } from '../../stores/analysisStore';
import { useMarketStore } from '../../stores/marketStore';
import { useTabStore } from '../../stores/tabStore';
import type { Market } from '../../types';
import type { ChartStatus } from '../../types/analysis';
import type { EvidenceCardsPack } from '../../types/evidenceCards';
import CompanyProfileCard from '../Analysis/CompanyProfileCard';
import StockHeader from '../Analysis/StockHeader';
import TabBar from '../Analysis/TabBar';
import EPSChart from '../Analysis/charts/EPSChart';
import GrossMarginChart from '../Analysis/charts/GrossMarginChart';
import KLineChart from '../Analysis/charts/KLineChart';
import RSChart from '../Analysis/charts/RSChart';
import RevenueChart from '../Analysis/charts/RevenueChart';
import ScreenerPage from '../Screener';
import EvidenceCardsPanel from './EvidenceCardsPanel';
import QuickPlanCockpit from './QuickPlanCockpit';

function statusForLength(length: number): ChartStatus {
  return length > 0 ? 'success' : 'insufficient';
}

function ResearchSymbolView({ symbol, tabName }: { symbol: string; tabName: string }) {
  const normalized = symbol.toUpperCase();
  const assetMarket = assetMarketForSymbol(normalized) ?? 'US';
  const fallbackState = useMemo(() => createInitialAnalysisState(), [normalized]);
  const state = useAnalysisStore((store) => store.analysisStates[normalized]) ?? fallbackState;
  const setSummary = useAnalysisStore((store) => store.setSummary);
  const setKLinePeriod = useAnalysisStore((store) => store.setKLinePeriod);
  const updateState = useAnalysisStore((store) => store.updateState);
  const updateChartStatus = useAnalysisStore((store) => store.updateChartStatus);
  const [evidencePack, setEvidencePack] = useState<EvidenceCardsPack | undefined>(undefined);

  useEffect(() => {
    setEvidencePack(undefined);
  }, [normalized]);

  useEffect(() => {
    let cancelled = false;
    const current = useAnalysisStore.getState().getStateForSymbol(normalized);
    if (!current.stockSummary || current.chartStatus.summary !== 'success') {
      updateChartStatus(normalized, 'summary', 'loading');
      fetchStockSummary(normalized)
        .then((summary) => {
          if (cancelled) return;
          setSummary(normalized, summary);
          updateChartStatus(normalized, 'summary', 'success');
        })
        .catch(() => {
          if (!cancelled) updateChartStatus(normalized, 'summary', 'error');
        });
    }
    if (!current.companyProfile) {
      updateChartStatus(normalized, 'profile', 'loading');
      fetchCompanyProfile(normalized)
        .then((profile) => {
          if (cancelled) return;
          updateState(normalized, { companyProfile: profile });
          updateChartStatus(normalized, 'profile', profile.description ? 'success' : 'insufficient');
        })
        .catch(() => {
          if (!cancelled) updateChartStatus(normalized, 'profile', 'error');
        });
    }
    if (!current.epsData) {
      updateChartStatus(normalized, 'eps', 'loading');
      fetchEPSQuarterly(normalized)
        .then((data) => {
          if (cancelled) return;
          updateState(normalized, { epsData: data });
          updateChartStatus(normalized, 'eps', statusForLength(data.quarters.length));
        })
        .catch(() => {
          if (!cancelled) updateChartStatus(normalized, 'eps', 'error');
        });
    }
    if (!current.revenueData) {
      updateChartStatus(normalized, 'revenue', 'loading');
      fetchRevenueAnnual(normalized)
        .then((data) => {
          if (cancelled) return;
          updateState(normalized, { revenueData: data });
          updateChartStatus(normalized, 'revenue', statusForLength(data.years.length));
        })
        .catch(() => {
          if (!cancelled) updateChartStatus(normalized, 'revenue', 'error');
        });
    }
    if (!current.grossMarginData) {
      updateChartStatus(normalized, 'grossMargin', 'loading');
      fetchGrossMarginQuarterly(normalized)
        .then((data) => {
          if (cancelled) return;
          updateState(normalized, { grossMarginData: data });
          updateChartStatus(normalized, 'grossMargin', statusForLength(data.quarters.length));
        })
        .catch(() => {
          if (!cancelled) updateChartStatus(normalized, 'grossMargin', 'error');
        });
    }
    return () => {
      cancelled = true;
    };
  }, [normalized, setSummary, updateChartStatus, updateState]);

  useEffect(() => {
    let cancelled = false;
    const period = state.klinePeriod;
    const current = useAnalysisStore.getState().getStateForSymbol(normalized);
    if (current.klineData?.period !== period) {
      updateChartStatus(normalized, 'kline', 'loading');
      fetchKLine(normalized, period)
        .then((data) => {
          if (cancelled) return;
          updateState(normalized, { klineData: data });
          updateChartStatus(normalized, 'kline', statusForLength(data.candles.length));
        })
        .catch(() => {
          if (!cancelled) updateChartStatus(normalized, 'kline', 'error');
        });
    }
    if (current.rsData?.period !== period) {
      updateChartStatus(normalized, 'rs', 'loading');
      fetchRS(normalized, period)
        .then((data) => {
          if (cancelled) return;
          updateState(normalized, { rsData: data });
          updateChartStatus(normalized, 'rs', statusForLength(data.rs_line.length));
        })
        .catch(() => {
          if (!cancelled) updateChartStatus(normalized, 'rs', 'error');
        });
    }
    return () => {
      cancelled = true;
    };
  }, [normalized, state.klinePeriod, updateChartStatus, updateState]);

  const decisionMarket = (state.stockSummary?.market ?? (assetMarket === 'CN' ? 'SSE_MAIN' : 'NASDAQ100')) as Market;

  return (
    <div className="space-y-5">
      <StockHeader
        symbol={normalized}
        tabName={tabName}
        summary={state.stockSummary}
        assetMarket={assetMarket}
        hasCurrentAIAnalysis={state.aiStatus === 'done' && hasGeneratedAIAnalysis(state)}
      />
      <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1.55fr)_minmax(340px,1fr)]">
        <div className="min-w-0 space-y-4">
          <KLineChart
            data={state.klineData}
            status={state.chartStatus.kline}
            period={state.klinePeriod}
            assetMarket={assetMarket}
            onPeriodChange={(period) => setKLinePeriod(normalized, period)}
          />
          <EvidenceCardsPanel symbol={normalized} onPackLoaded={setEvidencePack} />
          <div className="grid gap-3 sm:grid-cols-2">
            <RSChart data={state.rsData} status={state.chartStatus.rs} period={state.klinePeriod} />
            <CompanyProfileCard data={state.companyProfile} status={state.chartStatus.profile} />
          </div>
          <div className="grid gap-3 md:grid-cols-3">
            <EPSChart data={state.epsData} status={state.chartStatus.eps} />
            <RevenueChart data={state.revenueData} status={state.chartStatus.revenue} />
            <GrossMarginChart data={state.grossMarginData} status={state.chartStatus.grossMargin} />
          </div>
        </div>
        <div className="xl:sticky xl:top-6">
          <QuickPlanCockpit symbol={normalized} market={decisionMarket} evidencePack={evidencePack} />
        </div>
      </div>
    </div>
  );
}

export default function ResearchPage() {
  const { symbol } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const assetMarket = useMarketStore((state) => state.assetMarket);
  const allTabs = useTabStore((state) => state.tabs);
  const activeTabIds = useTabStore((state) => state.activeTabIds);
  const openTab = useTabStore((state) => state.openTab);
  const closeTab = useTabStore((state) => state.closeTab);
  const setActiveTab = useTabStore((state) => state.setActiveTab);

  const rawSymbol = symbol?.trim() ?? '';
  const routeSymbol = rawSymbol ? normalizeSymbolForAsset(assetMarket, rawSymbol) : null;
  const tabs = useMemo(
    () => allTabs.filter((tab) => assetMarketForSymbol(tab.symbol) === assetMarket),
    [allTabs, assetMarket]
  );
  const storedActiveId = activeTabIds[assetMarket];
  const activeTabId = tabs.some((tab) => tab.symbol === storedActiveId)
    ? storedActiveId
    : tabs[tabs.length - 1]?.symbol ?? null;
  const displayedSymbol = routeSymbol ?? activeTabId;

  const [screenerOpen, setScreenerOpen] = useState<boolean>(() => !displayedSymbol);
  const [symbolInput, setSymbolInput] = useState('');

  useEffect(() => {
    if (routeSymbol) {
      const stateName = (location.state as { name?: string } | null)?.name;
      const existing = useTabStore.getState().tabs.find((t) => t.symbol === routeSymbol)?.name;
      openTab(routeSymbol, stateName ?? existing ?? routeSymbol);
      setScreenerOpen(false);
    }
  }, [location.state, openTab, routeSymbol]);

  const handleOpenSymbol = (target: string) => {
    const canonical = normalizeSymbolForAsset(assetMarket, target);
    if (!canonical) return false;
    openTab(canonical, canonical);
    navigate('/research/' + encodeURIComponent(canonical));
    setSymbolInput('');
    return true;
  };

  return (
    <div className="mx-auto w-full max-w-[1440px] px-4 py-6 sm:px-6 lg:py-8">
      {/* Unified Research Bar */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-rule pb-4">
        <div className="flex items-center gap-3">
          <h1 className="font-display text-2xl font-black text-white">研究台</h1>
          <Button
            variant={screenerOpen ? 'primary' : 'outline'}
            className="h-9 px-3 text-xs"
            onClick={() => setScreenerOpen((prev) => !prev)}
          >
            <Filter className="h-3.5 w-3.5" />
            候选池筛选器
            {screenerOpen ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
          </Button>
        </div>
        <form
          className="flex items-center gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            handleOpenSymbol(symbolInput);
          }}
        >
          <Input
            aria-label="快速输入研究代码"
            value={symbolInput}
            onChange={(e) => setSymbolInput(e.target.value)}
            placeholder={assetMarket === 'CN' ? '输入 A 股代码如 600519' : '输入美股代码如 NVDA'}
            className="h-9 w-52 text-xs"
          />
          <Button type="submit" variant="outline" className="h-9 px-3 text-xs">
            <Search className="h-3.5 w-3.5" />打开
          </Button>
        </form>
      </div>

      {screenerOpen ? (
        <div className="mt-4 rounded-sm border border-rule bg-slate-950/50 p-2">
          <ScreenerPage />
        </div>
      ) : null}

      {tabs.length > 0 ? (
        <div className="mt-4">
          <TabBar
            tabs={tabs}
            activeTabId={displayedSymbol}
            onActive={(next) => {
              setActiveTab(next);
              navigate('/research/' + encodeURIComponent(next));
            }}
            onClose={(target) => {
              closeTab(target);
              if (displayedSymbol === target) {
                const remaining = tabs.filter((t) => t.symbol !== target);
                const next = remaining[remaining.length - 1]?.symbol;
                navigate(next ? '/research/' + encodeURIComponent(next) : '/research', { replace: true });
              }
            }}
            onAdd={handleOpenSymbol}
          />
        </div>
      ) : null}

      <div className="mt-5">
        {displayedSymbol ? (
          <ResearchSymbolView
            symbol={displayedSymbol}
            tabName={tabs.find((t) => t.symbol === displayedSymbol)?.name ?? displayedSymbol}
          />
        ) : (
          <Card className="p-8 text-center text-sm text-slate-400">
            请从上方 CANSLIM 候选池中选择一只股票，或直接输入代码开始 45 秒研究与头寸规划。
          </Card>
        )}
      </div>
    </div>
  );
}
