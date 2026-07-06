import { useEffect, useMemo } from 'react';
import { fetchCompanyProfile, fetchEPSQuarterly, fetchGrossMarginQuarterly, fetchKLine, fetchRevenueAnnual, fetchRS, fetchSecondLayerData, fetchStockSummary } from '../../lib/api';
import { createInitialAnalysisState, useAnalysisStore } from '../../stores/analysisStore';
import type { ChartStatus } from '../../types/analysis';
import AISection from './ai/AISection';
import CompanyProfileCard from './CompanyProfileCard';
import EPSChart from './charts/EPSChart';
import GrossMarginChart from './charts/GrossMarginChart';
import KLineChart from './charts/KLineChart';
import RSChart from './charts/RSChart';
import RevenueChart from './charts/RevenueChart';
import StockHeader from './StockHeader';
import TradePlanPanel from './TradePlanPanel';

interface AnalysisPageProps {
  symbol: string;
  tabName: string;
}

function statusForLength(length: number): ChartStatus {
  return length > 0 ? 'success' : 'insufficient';
}

export default function AnalysisPage({ symbol, tabName }: AnalysisPageProps) {
  const normalized = symbol.toUpperCase();
  const fallbackState = useMemo(() => createInitialAnalysisState(), [normalized]);
  const state = useAnalysisStore((store) => store.analysisStates[normalized]) ?? fallbackState;
  const setSummary = useAnalysisStore((store) => store.setSummary);
  const setKLinePeriod = useAnalysisStore((store) => store.setKLinePeriod);
  const updateState = useAnalysisStore((store) => store.updateState);
  const updateChartStatus = useAnalysisStore((store) => store.updateChartStatus);

  useEffect(() => {
    let cancelled = false;
    const current = useAnalysisStore.getState().getStateForSymbol(normalized);
    if (current.stockSummary && current.chartStatus.summary === 'success') return;

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

    return () => {
      cancelled = true;
    };
  }, [normalized, setSummary, updateChartStatus]);

  useEffect(() => {
    let cancelled = false;
    const current = useAnalysisStore.getState().getStateForSymbol(normalized);
    if (current.companyProfile && current.chartStatus.profile !== 'loading') return;

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

    return () => {
      cancelled = true;
    };
  }, [normalized, updateChartStatus, updateState]);

  useEffect(() => {
    let cancelled = false;
    const period = state.klinePeriod;
    const current = useAnalysisStore.getState().getStateForSymbol(normalized);
    if (current.klineData?.period === period) return;

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

    return () => {
      cancelled = true;
    };
  }, [normalized, state.klinePeriod, updateChartStatus, updateState]);

  useEffect(() => {
    let cancelled = false;
    const period = state.klinePeriod;
    const current = useAnalysisStore.getState().getStateForSymbol(normalized);
    if (current.rsData?.period === period) return;

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

    return () => {
      cancelled = true;
    };
  }, [normalized, state.klinePeriod, updateChartStatus, updateState]);

  useEffect(() => {
    let cancelled = false;
    const current = useAnalysisStore.getState().getStateForSymbol(normalized);

    if (!current.secondLayerData) {
      updateChartStatus(normalized, 'secondLayer', 'loading');
      fetchSecondLayerData(normalized)
        .then((data) => {
          if (cancelled) return;
          updateState(normalized, { secondLayerData: data });
          updateChartStatus(normalized, 'secondLayer', data ? 'success' : 'insufficient');
        })
        .catch(() => {
          if (!cancelled) updateChartStatus(normalized, 'secondLayer', 'error');
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
  }, [normalized, updateChartStatus, updateState]);

  return (
    <div className="space-y-6">
      <StockHeader symbol={normalized} tabName={tabName} summary={state.stockSummary} />
      <section className="space-y-6">
        <CompanyProfileCard data={state.companyProfile} status={state.chartStatus.profile} />
        <KLineChart data={state.klineData} status={state.chartStatus.kline} period={state.klinePeriod} onPeriodChange={(period) => setKLinePeriod(normalized, period)} />
        <RSChart data={state.rsData} status={state.chartStatus.rs} period={state.klinePeriod} />
        <div className="grid gap-6 xl:grid-cols-3">
          <EPSChart data={state.epsData} status={state.chartStatus.eps} />
          <RevenueChart data={state.revenueData} status={state.chartStatus.revenue} />
          <GrossMarginChart data={state.grossMarginData} status={state.chartStatus.grossMargin} />
        </div>
      </section>
      <TradePlanPanel symbol={normalized} aiSummary={state.aiDecision?.summary ?? undefined} />
      <AISection symbol={normalized} state={state} />
    </div>
  );
}
