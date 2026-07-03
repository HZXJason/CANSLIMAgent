import type { ScreenerFilterKey } from '../../types';
import { cn } from '../../lib/utils';
import { SCREENER_FILTER_KEYS } from '../../lib/computeLabels';

const tags: ScreenerFilterKey[] = SCREENER_FILTER_KEYS;

interface TagFilterProps {
  activeTags: ScreenerFilterKey[];
  onToggle: (tag: ScreenerFilterKey) => void;
}

export default function TagFilter({ activeTags, onToggle }: TagFilterProps) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <span className="mr-2 text-sm font-semibold text-slate-100">筛选条件</span>
      {tags.map((tag) => (
        <button
          key={tag}
          className={cn(
            'inline-flex h-9 min-w-9 items-center justify-center rounded-full border px-3 text-sm font-bold transition',
            activeTags.includes(tag)
              ? 'border-emerald-400 bg-emerald-400 text-slate-950 shadow-sm shadow-emerald-500/20'
              : 'border-slate-700 bg-slate-900/70 text-slate-500 hover:border-cyan-400/50 hover:text-slate-200'
          )}
          onClick={() => onToggle(tag)}
          aria-pressed={activeTags.includes(tag)}
        >
          {tag}
        </button>
      ))}
      <span className="text-xs text-slate-500">多选使用 AND 逻辑</span>
    </div>
  );
}
