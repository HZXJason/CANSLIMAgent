import { Check, Plus, X } from 'lucide-react';
import { FormEvent, KeyboardEvent, useEffect, useRef, useState } from 'react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { displaySymbol } from '../../lib/markets';
import { cn } from '../../lib/utils';
import type { StockTab } from '../../stores/tabStore';

interface TabBarProps {
  tabs: StockTab[];
  activeTabId: string | null;
  onActive: (symbol: string) => void;
  onClose: (symbol: string) => void;
  onAdd: (symbol: string) => boolean | void;
  invalidSymbolMessage?: string;
}

function tabDomID(symbol: string) {
  return symbol.replace(/[^a-zA-Z0-9_-]/g, '-');
}

function preferredScrollBehavior(): ScrollBehavior {
  if (typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
    return 'auto';
  }
  return 'smooth';
}

export default function TabBar({ tabs, activeTabId, onActive, onClose, onAdd, invalidSymbolMessage = '请输入当前市场支持的股票代码。' }: TabBarProps) {
  const [adding, setAdding] = useState(false);
  const [symbol, setSymbol] = useState('');
  const [inputError, setInputError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const addButtonRef = useRef<HTMLButtonElement | null>(null);
  const tabListRef = useRef<HTMLDivElement | null>(null);
  const tabRefs = useRef(new Map<string, HTMLDivElement>());
  const tabButtonRefs = useRef(new Map<string, HTMLButtonElement>());

  useEffect(() => {
    if (adding) inputRef.current?.focus();
  }, [adding]);

  useEffect(() => {
    if (!activeTabId) return;
    const tabList = tabListRef.current;
    const activeTab = tabRefs.current.get(activeTabId);
    if (!tabList || !activeTab) return;

    const listRect = tabList.getBoundingClientRect();
    const tabRect = activeTab.getBoundingClientRect();
    if (tabRect.left < listRect.left) {
      tabList.scrollTo({
        left: Math.max(0, tabList.scrollLeft - (listRect.left - tabRect.left)),
        behavior: preferredScrollBehavior()
      });
    } else if (tabRect.right > listRect.right) {
      tabList.scrollTo({
        left: tabList.scrollLeft + (tabRect.right - listRect.right),
        behavior: preferredScrollBehavior()
      });
    }
  }, [activeTabId, tabs.length]);

  const closeInput = (restoreFocus = false) => {
    setAdding(false);
    setSymbol('');
    setInputError(null);
    if (restoreFocus) {
      window.requestAnimationFrame(() => addButtonRef.current?.focus());
    }
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const value = symbol.trim();
    if (!value) return;
    if (onAdd(value) === false) {
      setInputError(invalidSymbolMessage);
      return;
    }
    closeInput();
  };

  const handleTabKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key) || tabs.length === 0) return;
    event.preventDefault();

    let nextIndex = index;
    if (event.key === 'ArrowLeft') nextIndex = (index - 1 + tabs.length) % tabs.length;
    if (event.key === 'ArrowRight') nextIndex = (index + 1) % tabs.length;
    if (event.key === 'Home') nextIndex = 0;
    if (event.key === 'End') nextIndex = tabs.length - 1;

    const nextTab = tabs[nextIndex];
    tabButtonRefs.current.get(nextTab.symbol)?.focus();
    if (nextTab.symbol !== activeTabId) onActive(nextTab.symbol);
  };

  return (
    <nav className="flex min-w-0 items-center gap-1.5 sm:gap-2" aria-label="已打开的股票">
      <div
        ref={tabListRef}
        className={cn(
          'min-w-0 flex-1 items-center gap-1.5 overflow-x-auto overscroll-x-contain sm:gap-2',
          adding ? 'hidden sm:flex' : 'flex'
        )}
        role="tablist"
        aria-label="股票分析标签"
      >
        {tabs.map((tab, index) => {
          const active = activeTabId === tab.symbol;
          const domID = tabDomID(tab.symbol);
          const visibleSymbol = displaySymbol(tab.symbol);
          const hasDistinctName = tab.name !== tab.symbol && tab.name !== visibleSymbol;
          return (
            <div
              key={tab.symbol}
              ref={(node) => {
                if (node) tabRefs.current.set(tab.symbol, node);
                else tabRefs.current.delete(tab.symbol);
              }}
              role="presentation"
              className={cn(
                'group inline-flex min-h-11 shrink-0 items-stretch overflow-hidden rounded-xl border transition-[border-color,background-color,color,box-shadow] duration-150 sm:min-h-10',
                active
                  ? 'border-brand-300/35 bg-brand-300/[0.09] text-brand-50 shadow-[inset_0_-2px_0_var(--accent)]'
                  : 'border-white/[0.07] bg-slate-950/25 text-slate-400 hover:border-slate-700 hover:bg-slate-800/60 hover:text-slate-100'
              )}
            >
              <button
                ref={(node) => {
                  if (node) tabButtonRefs.current.set(tab.symbol, node);
                  else tabButtonRefs.current.delete(tab.symbol);
                }}
                id={`analysis-tab-${domID}`}
                type="button"
                role="tab"
                aria-selected={active}
                aria-controls={`analysis-panel-${domID}`}
                aria-label={hasDistinctName ? `${visibleSymbol} ${tab.name}` : visibleSymbol}
                tabIndex={active ? 0 : -1}
                className="min-h-11 max-w-52 truncate py-2 pl-3 pr-2 text-left text-sm font-bold tracking-[-0.01em] focus-visible:z-10 sm:min-h-10"
                onClick={() => onActive(tab.symbol)}
                onKeyDown={(event) => handleTabKeyDown(event, index)}
              >
                {visibleSymbol}
                {hasDistinctName ? <span className="ml-2 font-medium opacity-70">{tab.name}</span> : null}
              </button>
              <button
                type="button"
                tabIndex={active ? 0 : -1}
                className="interactive-press flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-slate-400 hover:bg-black/20 hover:text-white focus-visible:z-10 sm:h-10 sm:w-10"
                onClick={() => onClose(tab.symbol)}
                aria-label={'关闭 ' + visibleSymbol}
              >
                <X className="h-4 w-4" />
              </button>
            </div>
          );
        })}
      </div>

      {adding ? (
        <form className="flex min-w-0 flex-1 items-start gap-1.5 sm:flex-none sm:gap-2" aria-label="添加股票" onSubmit={submit}>
          <div className="min-w-0 flex-1 sm:w-48 sm:flex-none">
            <Input
              ref={inputRef}
              value={symbol}
              onChange={(event) => {
                setSymbol(event.target.value);
                setInputError(null);
              }}
              onKeyDown={(event) => {
                if (event.key === 'Escape') closeInput(true);
              }}
              className="min-w-0 uppercase"
              placeholder="股票代码"
              aria-label="股票代码"
              aria-describedby={inputError ? 'analysis-symbol-error' : undefined}
              invalid={Boolean(inputError)}
            />
            {inputError ? <p id="analysis-symbol-error" className="mt-1.5 text-xs font-medium leading-5 text-red-300" role="alert">{inputError}</p> : null}
          </div>
          <Button type="submit" className="h-11 w-11 shrink-0 px-0 sm:h-10 sm:w-10" disabled={!symbol.trim()} aria-label="打开股票">
            <Check className="h-4 w-4" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            className="h-11 w-11 shrink-0 px-0 sm:h-10 sm:w-10"
            onClick={() => closeInput(true)}
            aria-label="取消添加"
          >
            <X className="h-4 w-4" />
          </Button>
        </form>
      ) : (
        <Button
          ref={addButtonRef}
          type="button"
          variant="outline"
          className="h-11 w-11 shrink-0 px-0 sm:h-10 sm:w-10"
          onClick={() => setAdding(true)}
          aria-label="添加股票"
        >
          <Plus className="h-4 w-4" />
        </Button>
      )}
    </nav>
  );
}
