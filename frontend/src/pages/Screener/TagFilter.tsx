import type { ScreenerFilterKey } from '../../types';
import { cn } from '../../lib/utils';
import { SCREENER_FILTER_KEYS } from '../../lib/computeLabels';
import { Tooltip } from '../../components/ui/tooltip';

const tags: ScreenerFilterKey[] = SCREENER_FILTER_KEYS;

const tagTooltips: Record<ScreenerFilterKey, string> = {
  C: 'C：当季 EPS 增长',
  A: 'A：年度 EPS 增长',
  N: 'N：New，新产品、新高、新闻等 AI 写回线索',
  S: 'S：供需关系，突破成交量',
  L: 'L：行业或市场相对强势',
  I: 'I：机构持仓 / 机构赞助'
};

interface TagFilterProps {
  activeTags: ScreenerFilterKey[];
  onToggle: (tag: ScreenerFilterKey) => void;
  onClear: () => void;
}

export default function TagFilter({ activeTags, onToggle, onClear }: TagFilterProps) {
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
      <div className="min-w-0 sm:mr-2">
        <span className="block text-sm font-bold text-slate-100">标签筛选</span>
        <span className="mt-0.5 block text-xs text-slate-500">组合条件只保留同时达标的股票</span>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {tags.map((tag) => (
          <Tooltip key={tag} content={tagTooltips[tag]}>
            <button
              type="button"
              className={cn(
                'interactive-press inline-flex h-11 min-w-11 items-center justify-center rounded-xl border px-3 text-sm font-extrabold sm:h-10 sm:min-w-10',
                activeTags.includes(tag)
                  ? 'border-emerald-400 bg-emerald-400 text-slate-950 shadow-sm shadow-emerald-500/20'
                  : 'border-slate-700/80 bg-slate-950/40 text-slate-400 hover:border-cyan-300/35 hover:bg-slate-800/50 hover:text-slate-100'
              )}
              onClick={() => onToggle(tag)}
              aria-label={tagTooltips[tag]}
              aria-pressed={activeTags.includes(tag)}
            >
              {tag}
            </button>
          </Tooltip>
        ))}
      </div>
      {activeTags.length > 0 ? (
        <button type="button" className="interactive-press inline-flex min-h-11 items-center rounded-xl px-2 text-sm font-bold text-cyan-300 hover:bg-cyan-300/[0.07] hover:text-cyan-200 sm:min-h-10" onClick={onClear}>
          清除
        </button>
      ) : null}
    </div>
  );
}
