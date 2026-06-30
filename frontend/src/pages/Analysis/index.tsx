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
  const tabs = useTabStore((store) => store.tabs);
  const activeTabId = useTabStore((store) => store.activeTabId);
  const openTab = useTabStore((store) => store.openTab);
  const closeTab = useTabStore((store) => store.closeTab);
  const setActiveTab = useTabStore((store) => store.setActiveTab);
  const removeAnalysisState = useAnalysisStore((store) => store.removeState);

  useEffect(() => {
    if (symbol) {
      const normalized = symbol.toUpperCase();
      openTab(normalized, state?.name ?? normalized);
    }
  }, [openTab, state?.name, symbol]);

  useEffect(() => {
    if (activeTabId && activeTabId !== symbol?.toUpperCase()) {
      navigate('/analysis/' + activeTabId, { replace: true });
    }
  }, [activeTabId, navigate, symbol]);

  const activeTab = tabs.find((tab) => tab.symbol === activeTabId);

  const handleAdd = () => {
    const nextSymbol = window.prompt('请输入股票代码');
    if (!nextSymbol?.trim()) return;
    const normalized = nextSymbol.trim().toUpperCase();
    openTab(normalized, normalized);
    navigate('/analysis/' + normalized);
  };

  const handleClose = (targetSymbol: string) => {
    closeAIStream(targetSymbol);
    removeAnalysisState(targetSymbol);
    closeTab(targetSymbol);
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
          activeTabId={activeTabId}
          onActive={setActiveTab}
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
