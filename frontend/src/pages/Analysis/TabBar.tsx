import { Plus, X } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { cn } from '../../lib/utils';
import type { StockTab } from '../../stores/tabStore';

interface TabBarProps {
  tabs: StockTab[];
  activeTabId: string | null;
  onActive: (symbol: string) => void;
  onClose: (symbol: string) => void;
  onAdd: () => void;
}

export default function TabBar({ tabs, activeTabId, onActive, onClose, onAdd }: TabBarProps) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      {tabs.map((tab) => (
        <button
          key={tab.symbol}
          className={cn(
            'inline-flex items-center gap-2 rounded-2xl border px-4 py-2.5 text-sm font-semibold transition',
            activeTabId === tab.symbol
              ? 'border-cyan-400/50 bg-finance-blue text-white shadow-glow'
              : 'border-slate-800 bg-slate-950/30 text-slate-400 hover:border-cyan-400/40 hover:bg-slate-800/70 hover:text-slate-100'
          )}
          onClick={() => onActive(tab.symbol)}
        >
          <span>{tab.symbol} · {tab.name}</span>
          <span
            className="rounded-full p-0.5 hover:bg-black/20"
            onClick={(event) => {
              event.stopPropagation();
              onClose(tab.symbol);
            }}
            aria-label="关闭标签页"
            role="button"
          >
            <X className="h-3 w-3" />
          </span>
        </button>
      ))}
      <Button variant="outline" className="rounded-2xl px-3" onClick={onAdd}>
        <Plus className="h-4 w-4" />
      </Button>
    </div>
  );
}
