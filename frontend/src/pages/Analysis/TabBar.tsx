import { Check, Plus, X } from 'lucide-react';
import { FormEvent, useEffect, useRef, useState } from 'react';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { cn } from '../../lib/utils';
import type { StockTab } from '../../stores/tabStore';

interface TabBarProps {
  tabs: StockTab[];
  activeTabId: string | null;
  onActive: (symbol: string) => void;
  onClose: (symbol: string) => void;
  onAdd: (symbol: string) => void;
}

export default function TabBar({ tabs, activeTabId, onActive, onClose, onAdd }: TabBarProps) {
  const [adding, setAdding] = useState(false);
  const [symbol, setSymbol] = useState('');
  const inputRef = useRef<HTMLInputElement | null>(null);
  const tabListRef = useRef<HTMLDivElement | null>(null);
  const tabRefs = useRef(new Map<string, HTMLDivElement>());

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
      tabList.scrollTo({ left: Math.max(0, tabList.scrollLeft - (listRect.left - tabRect.left)), behavior: 'smooth' });
    } else if (tabRect.right > listRect.right) {
      tabList.scrollTo({ left: tabList.scrollLeft + (tabRect.right - listRect.right), behavior: 'smooth' });
    }
  }, [activeTabId, tabs.length]);

  const closeInput = () => {
    setAdding(false);
    setSymbol('');
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const value = symbol.trim();
    if (!value) return;
    onAdd(value);
    closeInput();
  };

  return (
    <nav className="flex min-w-0 items-center gap-2" aria-label="已打开的股票">
      <div ref={tabListRef} className="flex min-w-0 flex-1 items-center gap-2 overflow-x-auto" role="tablist">
        {tabs.map((tab) => {
          const active = activeTabId === tab.symbol;
          return (
            <div
              key={tab.symbol}
              ref={(node) => {
                if (node) tabRefs.current.set(tab.symbol, node);
                else tabRefs.current.delete(tab.symbol);
              }}
              className={cn(
                'group inline-flex shrink-0 items-center overflow-hidden rounded-xl border transition',
                active
                  ? 'border-cyan-400/50 bg-finance-blue text-white shadow-glow'
                  : 'border-slate-800 bg-slate-950/30 text-slate-400 hover:border-slate-700 hover:bg-slate-800/70 hover:text-slate-100'
              )}
            >
              <button
                type="button"
                role="tab"
                aria-selected={active}
                className="max-w-52 truncate py-2 pl-3 pr-2 text-sm font-bold"
                onClick={() => onActive(tab.symbol)}
              >
                {tab.symbol}
                {tab.name !== tab.symbol ? <span className="ml-2 font-medium opacity-70">{tab.name}</span> : null}
              </button>
              <button
                type="button"
                className="mr-1 rounded-lg p-1.5 opacity-60 transition hover:bg-black/20 hover:opacity-100"
                onClick={() => onClose(tab.symbol)}
                aria-label={'关闭 ' + tab.symbol}
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
          );
        })}
      </div>

      {adding ? (
        <form className="flex shrink-0 items-center gap-2" onSubmit={submit}>
          <Input
            ref={inputRef}
            value={symbol}
            onChange={(event) => setSymbol(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Escape') closeInput();
            }}
            className="h-9 w-36 rounded-xl uppercase sm:w-44"
            placeholder="股票代码"
            aria-label="股票代码"
          />
          <Button type="submit" className="h-9 w-9 rounded-xl px-0" disabled={!symbol.trim()} aria-label="打开股票">
            <Check className="h-4 w-4" />
          </Button>
          <Button type="button" variant="ghost" className="h-9 w-9 rounded-xl px-0" onClick={closeInput} aria-label="取消添加">
            <X className="h-4 w-4" />
          </Button>
        </form>
      ) : (
        <Button variant="outline" className="h-9 w-9 shrink-0 rounded-xl px-0" onClick={() => setAdding(true)} aria-label="添加股票">
          <Plus className="h-4 w-4" />
        </Button>
      )}
    </nav>
  );
}
