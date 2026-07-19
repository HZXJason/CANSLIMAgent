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

function tabDomID(symbol: string) {
  return symbol.replace(/[^a-zA-Z0-9_-]/g, '-');
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
    <section className="mx-auto flex w-full max-w-[1600px] flex-col gap-3 px-3 py-3 sm:gap-4 sm:px-5 sm:py-4 lg:px-6">
      <header className="min-w-0 rounded-[18px] border border-white/[0.075] bg-[#0d1421] p-1.5 shadow-[0_14px_38px_rgba(0,0,0,0.14)] sm:p-2" aria-label="股票分析工作区">
        <TabBar
          tabs={tabs}
          activeTabId={displayedTabId}
          onActive={handleActive}
          onClose={handleClose}
          onAdd={handleAdd}
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
        <section className="flex min-h-72 items-center justify-center rounded-[20px] border border-dashed border-slate-700/80 bg-[#0d1421] px-5 text-center text-slate-400">
          <div>
            <p className="text-base font-bold text-slate-200">添加股票开始分析</p>
            <p className="mt-2 text-sm text-slate-500">使用上方添加按钮输入股票代码</p>
          </div>
        </section>
      )}
    </section>
  );
}
