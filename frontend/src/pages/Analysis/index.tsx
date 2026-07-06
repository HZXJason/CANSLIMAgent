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

  const handleAdd = () => {
    const nextSymbol = window.prompt('请输入股票代码');
    if (!nextSymbol?.trim()) return;
    const normalized = nextSymbol.trim().toUpperCase();
    openTab(normalized, normalized);
    navigate('/analysis/' + normalized);
  };

  const handleClose = (targetSymbol: string) => {
    const normalized = targetSymbol.trim().toUpperCase();
    const remainingTabs = tabs.filter((tab) => tab.symbol !== normalized);
    const nextActiveTabId = displayedTabId === normalized ? remainingTabs[remainingTabs.length - 1]?.symbol ?? null : displayedTabId;
    closeAIStream(normalized);
    removeAnalysisState(normalized);
    closeTab(normalized);
    if (displayedTabId === normalized) {
      navigate(nextActiveTabId ? '/analysis/' + nextActiveTabId : '/analysis', { replace: true });
    }
  };

  return (
    <section className="mx-auto flex w-full max-w-[1440px] flex-col gap-6 px-4 py-6 lg:px-8">
      <header className="rounded-[1.25rem] border border-slate-800/90 bg-slate-900/70 p-5 shadow-2xl shadow-slate-950/25 backdrop-blur">
        <div className="mb-5">
          <div>
            <p className="text-sm font-semibold text-cyan-300/80">AlphaLens · 个股深度分析</p>
            <h1 className="mt-2 text-3xl font-black tracking-tight text-white">个股分析工作台</h1>
          </div>
        </div>
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
        <section className="flex min-h-[420px] items-center justify-center rounded-[1.25rem] border border-slate-800/90 bg-slate-900/70 text-center text-slate-400">
          <p>暂无打开的股票标签页，请从选股器列表进入或点击 “+” 添加。</p>
        </section>
      )}
    </section>
  );
}
