import { ChevronDown, ChevronRight } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { useAnalysisStore } from '../../../stores/analysisStore';
import type { AIStatus, ThoughtLine } from '../../../types/analysis';

interface ThoughtStreamProps {
  symbol: string;
  lines: ThoughtLine[];
  expanded: boolean;
  status: AIStatus;
}

export default function ThoughtStream({ symbol, lines, expanded, status }: ThoughtStreamProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const updateState = useAnalysisStore((store) => store.updateState);

  useEffect(() => {
    if (expanded && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [expanded, lines.length]);

  const done = status === 'done';
  const title = done ? '运行日志（已完成）' : '运行日志';

  return (
    <div className="rounded-2xl border border-slate-800 bg-slate-950/40">
      <button
        className="flex w-full items-center justify-between gap-3 px-4 py-3 text-left text-sm font-semibold text-slate-200"
        onClick={() => updateState(symbol, { thoughtExpanded: !expanded })}
      >
        <span>{title}</span>
        {expanded ? <ChevronDown className="h-4 w-4 text-slate-500" /> : <ChevronRight className="h-4 w-4 text-slate-500" />}
      </button>
      {expanded ? (
        <div ref={containerRef} className="max-h-[300px] overflow-y-auto border-t border-slate-800 px-4 py-3 font-mono text-xs leading-6 text-slate-400">
          {lines.length === 0 ? (
            <p className="text-slate-600">等待分析流返回...</p>
          ) : (
            lines.map((line, index) => (
              <p key={line.timestamp + '-' + index}>
                <span className="text-cyan-300">[{line.agent}]</span> {line.content}
              </p>
            ))
          )}
        </div>
      ) : null}
    </div>
  );
}
