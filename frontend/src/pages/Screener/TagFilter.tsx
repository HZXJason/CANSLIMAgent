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
    <div className="flex flex-wrap items-center gap-2.5">
      <span className="mr-1 text-sm font-semibold text-slate-100">标签筛选</span>
      <div className="flex flex-wrap items-center gap-2">
        {tags.map((tag) => (
          <Tooltip key={tag} content={tagTooltips[tag]}>
            <button
              type="button"
              className={cn(
                'inline-flex h-9 min-w-9 items-center justify-center rounded-xl border px-3 text-sm font-bold transition',
                activeTags.includes(tag)
                  ? 'border-emerald-400 bg-emerald-400 text-slate-950 shadow-sm shadow-emerald-500/20'
                  : 'border-slate-700 bg-slate-950/40 text-slate-500 hover:border-cyan-400/50 hover:text-slate-200'
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
        <button type="button" className="ml-1 text-sm font-medium text-cyan-300 hover:text-cyan-200" onClick={onClear}>
          清除
        </button>
      ) : null}
    </div>
  );
}
