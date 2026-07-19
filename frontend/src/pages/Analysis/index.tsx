import { useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { closeAIStream } from '../../hooks/useAIStream';
import { assetMarketForSymbol, normalizeSymbolForAsset } from '../../lib/markets';
import { useAnalysisStore } from '../../stores/analysisStore';
import { useMarketStore } from '../../stores/marketStore';
import { useTabStore } from '../../stores/tabStore';
import AnalysisContent from './AnalysisPage';
import TabBar from './TabBar';

interface LocationState {
  name?: string;
}

function tabDomID(symbol: string) {
  return symbol.replace(/[^a-zA-Z0-9_-]/g, '-');
}

function invalidSymbolMessage(assetMarket: 'US' | 'CN', value?: string) {
  const prefix = value?.trim() ? `“${value.trim().toUpperCase()}”` : '该代码';
  return assetMarket === 'CN'
    ? `${prefix}不是有效的 A 股代码，请输入 6 位代码，或带 SH、SZ、BJ 交易所前后缀的代码。`
    : `${prefix}不是有效的美股代码，请输入常见美股代码，例如 AAPL 或 BRK.B。`;
}

export default function AnalysisPage() {
  const { symbol } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const assetMarket = useMarketStore((store) => store.assetMarket);
  const setAssetMarket = useMarketStore((store) => store.setAssetMarket);
  const state = location.state as LocationState | null;
  const rawRouteSymbol = symbol?.trim() ?? '';
  const routeSymbol = rawRouteSymbol ? normalizeSymbolForAsset(assetMarket, rawRouteSymbol) : null;
  const routeAssetMarket = rawRouteSymbol ? assetMarketForSymbol(rawRouteSymbol) : null;
  const [reconciledRouteSymbol, setReconciledRouteSymbol] = useState<string | null>(
    () => (!routeAssetMarket || routeAssetMarket === assetMarket ? rawRouteSymbol : null)
  );
  const deepLinkReconciled = reconciledRouteSymbol === rawRouteSymbol;
  const allTabs = useTabStore((store) => store.tabs);
  const activeTabIds = useTabStore((store) => store.activeTabIds);
  const openTab = useTabStore((store) => store.openTab);
  const closeTab = useTabStore((store) => store.closeTab);
  const setActiveTab = useTabStore((store) => store.setActiveTab);
  const removeAnalysisState = useAnalysisStore((store) => store.removeState);
  const tabs = useMemo(
    () => allTabs.filter((tab) => assetMarketForSymbol(tab.symbol) === assetMarket),
    [allTabs, assetMarket]
  );
  const storedActiveTabId = activeTabIds[assetMarket];
  const activeTabId = tabs.some((tab) => tab.symbol === storedActiveTabId)
    ? storedActiveTabId
    : tabs[tabs.length - 1]?.symbol ?? null;

  useEffect(() => {
    if (deepLinkReconciled) return;
    if (routeAssetMarket && routeAssetMarket !== assetMarket) {
      setAssetMarket(routeAssetMarket);
    }
    setReconciledRouteSymbol(rawRouteSymbol);
  }, [assetMarket, deepLinkReconciled, rawRouteSymbol, routeAssetMarket, setAssetMarket]);

  useEffect(() => {
    if (routeSymbol) {
      const existingName = useTabStore.getState().tabs.find((tab) => tab.symbol === routeSymbol)?.name;
      openTab(routeSymbol, state?.name ?? existingName ?? routeSymbol);
      if (rawRouteSymbol !== routeSymbol) {
        navigate('/analysis/' + encodeURIComponent(routeSymbol), { replace: true, state: location.state });
      }
    }
  }, [location.state, navigate, openTab, rawRouteSymbol, routeSymbol, state?.name]);

  useEffect(() => {
    if (!deepLinkReconciled) return;
    if (routeSymbol) return;
    if (rawRouteSymbol) {
      if (routeAssetMarket && routeAssetMarket !== assetMarket) {
        navigate(activeTabId ? '/analysis/' + encodeURIComponent(activeTabId) : '/analysis', { replace: true });
      }
      return;
    }
    if (activeTabId) {
      navigate('/analysis/' + encodeURIComponent(activeTabId), { replace: true });
    }
  }, [activeTabId, assetMarket, deepLinkReconciled, navigate, rawRouteSymbol, routeAssetMarket, routeSymbol]);

  useEffect(() => {
    if (activeTabId && activeTabId !== storedActiveTabId) setActiveTab(activeTabId);
  }, [activeTabId, setActiveTab, storedActiveTabId]);

  const displayedTabId = routeSymbol ?? (rawRouteSymbol ? null : activeTabId);
  const activeTab =
    tabs.find((tab) => tab.symbol === displayedTabId) ??
    (routeSymbol ? { symbol: routeSymbol, name: state?.name ?? routeSymbol, openedAt: 0 } : null);
  const routeError = rawRouteSymbol && !routeSymbol && !routeAssetMarket
    ? invalidSymbolMessage(assetMarket, rawRouteSymbol)
    : null;

  const handleActive = (targetSymbol: string) => {
    const normalized = normalizeSymbolForAsset(assetMarket, targetSymbol);
    if (!normalized) return;
    setActiveTab(normalized);
    navigate('/analysis/' + encodeURIComponent(normalized));
  };

  const handleAdd = (nextSymbol: string) => {
    const normalized = normalizeSymbolForAsset(assetMarket, nextSymbol);
    if (!normalized) return false;
    openTab(normalized, normalized);
    navigate('/analysis/' + encodeURIComponent(normalized));
    return true;
  };

  const handleClose = (targetSymbol: string) => {
    const normalized = targetSymbol.trim().toUpperCase();
    const closedIndex = tabs.findIndex((tab) => tab.symbol === normalized);
    const remainingTabs = tabs.filter((tab) => tab.symbol !== normalized);
    const adjacentTab = remainingTabs[Math.min(Math.max(closedIndex, 0), remainingTabs.length - 1)];
    const nextActiveTabId = displayedTabId === normalized ? adjacentTab?.symbol ?? null : displayedTabId;
    closeAIStream(normalized);
    removeAnalysisState(normalized);
    closeTab(normalized);
    if (displayedTabId === normalized) {
      navigate(nextActiveTabId ? '/analysis/' + encodeURIComponent(nextActiveTabId) : '/analysis', { replace: true });
    }
  };

  return (
    <section className="mx-auto flex w-full max-w-[1600px] flex-col gap-3 px-3 py-3 sm:gap-4 sm:px-5 sm:py-4 lg:px-6">
      <header className="min-w-0 rounded-[18px] border border-white/[0.075] bg-surface p-1.5 shadow-[0_14px_38px_rgba(0,0,0,0.14)] sm:p-2" aria-label="股票分析工作区">
        <TabBar
          tabs={tabs}
          activeTabId={displayedTabId}
          onActive={handleActive}
          onClose={handleClose}
          onAdd={handleAdd}
          invalidSymbolMessage={invalidSymbolMessage(assetMarket)}
        />
      </header>

      {activeTab ? (
        <div
          id={`analysis-panel-${tabDomID(activeTab.symbol)}`}
          role="tabpanel"
          aria-labelledby={`analysis-tab-${tabDomID(activeTab.symbol)}`}
          className="min-w-0"
        >
          <AnalysisContent symbol={activeTab.symbol} tabName={activeTab.name} />
        </div>
      ) : (
        <section className="flex min-h-72 items-center justify-center rounded-[20px] border border-dashed border-slate-700/80 bg-surface px-5 text-center text-slate-400">
          <div>
            <p className="text-base font-bold text-slate-200">{routeError ? '代码不适用于当前市场' : '添加股票开始分析'}</p>
            <p className={routeError ? 'mt-2 max-w-xl text-sm text-red-300' : 'mt-2 text-sm text-slate-500'} role={routeError ? 'alert' : undefined}>
              {routeError ?? '使用上方添加按钮输入股票代码'}
            </p>
          </div>
        </section>
      )}
    </section>
  );
}
