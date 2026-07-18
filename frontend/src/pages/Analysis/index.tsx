import { useEffect } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { closeAIStream } from '../../hooks/useAIStream';
import { useAnalysisStore } from '../../stores/analysisStore';
import { useTabStore } from '../../stores/tabStore';
import AnalysisContent from './AnalysisPage';
import TabBar from './TabBar';

interface LocationState {
  name?: string;
}

export default function AnalysisPage() {
  const { symbol } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const state = location.state as LocationState | null;
  const routeSymbol = symbol?.trim().toUpperCase() ?? null;
  const tabs = useTabStore((store) => store.tabs);
  const activeTabId = useTabStore((store) => store.activeTabId);
  const openTab = useTabStore((store) => store.openTab);
  const closeTab = useTabStore((store) => store.closeTab);
  const setActiveTab = useTabStore((store) => store.setActiveTab);
  const removeAnalysisState = useAnalysisStore((store) => store.removeState);

  useEffect(() => {
    if (routeSymbol) {
      openTab(routeSymbol, state?.name ?? routeSymbol);
    }
  }, [openTab, routeSymbol, state?.name]);

  useEffect(() => {
    if (!routeSymbol && activeTabId) {
      navigate('/analysis/' + activeTabId, { replace: true });
    }
  }, [activeTabId, navigate, routeSymbol]);

  const displayedTabId = routeSymbol ?? activeTabId;
  const activeTab =
    tabs.find((tab) => tab.symbol === displayedTabId) ??
    (routeSymbol ? { symbol: routeSymbol, name: state?.name ?? routeSymbol, openedAt: 0 } : null);

  const handleActive = (targetSymbol: string) => {
    const normalized = targetSymbol.trim().toUpperCase();
    setActiveTab(normalized);
    navigate('/analysis/' + normalized);
  };

  const handleAdd = (nextSymbol: string) => {
    const normalized = nextSymbol.trim().toUpperCase();
    if (!normalized) return;
    openTab(normalized, normalized);
    navigate('/analysis/' + normalized);
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
      navigate(nextActiveTabId ? '/analysis/' + nextActiveTabId : '/analysis', { replace: true });
    }
  };

  return (
    <section className="mx-auto flex w-full max-w-[1600px] flex-col gap-4 px-3 py-4 sm:px-5 lg:px-6">
      <header className="rounded-2xl border border-slate-800/90 bg-slate-900/70 p-2.5 shadow-xl shadow-slate-950/20 backdrop-blur">
        <TabBar
          tabs={tabs}
          activeTabId={displayedTabId}
          onActive={handleActive}
          onClose={handleClose}
          onAdd={handleAdd}
        />
      </header>

      {activeTab ? (
        <AnalysisContent symbol={activeTab.symbol} tabName={activeTab.name} />
      ) : (
        <section className="flex min-h-72 items-center justify-center rounded-[1.25rem] border border-dashed border-slate-800 bg-slate-900/40 text-center text-slate-400">
          <p className="text-sm font-semibold">添加股票开始分析</p>
        </section>
      )}
    </section>
  );
}
