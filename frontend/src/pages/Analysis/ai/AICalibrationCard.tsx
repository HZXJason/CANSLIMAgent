import { Gauge, RefreshCw, Square } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { Button } from '../../../components/ui/button';
import { Card } from '../../../components/ui/card';
import { ConfirmDialog } from '../../../components/ui/confirm-dialog';
import { fetchAICalibration, startAICalibration, stopAICalibration } from '../../../lib/api';
import type { AICalibrationRun, AnalysisConfig } from '../../../types/analysis';

interface AICalibrationCardProps {
  symbol: string;
  config: AnalysisConfig;
  disabled?: boolean;
}

const confidenceLabels: Record<string, string> = {
  HIGH: '高',
  MEDIUM: '中',
  LOW: '低',
  INSUFFICIENT: '样本不足'
};

const analystLabels: Record<string, string> = {
  technical: '技术分析师',
  market: '技术分析师',
  fundamental: '基本面分析师',
  fundamentals: '基本面分析师',
  news: '新闻分析师',
  sentiment: '情绪分析师',
  social: '情绪分析师'
};

const baselineLabels: Record<string, string> = {
  BUY: '恒定看多',
  SELL: '恒定看空',
  HOLD: '恒定中性'
};

function formatLift(value = 0) {
  const percentage = value * 100;
  return `${percentage > 0 ? '+' : ''}${percentage.toFixed(0)}%`;
}

export default function AICalibrationCard({ symbol, config, disabled = false }: AICalibrationCardProps) {
  const [run, setRun] = useState<AICalibrationRun | null>(null);
  const [mediumRun, setMediumRun] = useState<AICalibrationRun | null>(null);
  const [runs, setRuns] = useState<AICalibrationRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [pendingHorizon, setPendingHorizon] = useState<20 | 40>(20);

  const load = useCallback(async () => {
    try {
      const independentAnalysts = config.analysts.filter((analyst) => analyst in analystLabels && !['sentiment', 'social'].includes(analyst));
      const [result, mediumResult, ...independentResults] = await Promise.all([
        fetchAICalibration(symbol, config, 20),
        fetchAICalibration(symbol, config, 40),
        ...[20, 40].flatMap((horizon) => independentAnalysts.map((analyst) =>
          fetchAICalibration(symbol, { ...config, analysts: [analyst] }, horizon)
        ))
      ]);
      setRun(result);
      setMediumRun(mediumResult);
      setRuns(independentResults.flatMap((item, index) => {
        if (!item) return [];
        const analyst = independentAnalysts[index % independentAnalysts.length];
        return [{ ...item, payload: { ...item.payload, requested_analysts: [analyst] } }];
      }));
      setError(null);
      return result;
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : '读取历史校准失败');
      return null;
    } finally {
      setLoading(false);
    }
  }, [config, symbol]);

  useEffect(() => {
    setLoading(true);
    void load();
  }, [load]);

  useEffect(() => {
    if (run?.status !== 'running' && mediumRun?.status !== 'running' && !runs.some((item) => item.status === 'running')) return;
    const timer = window.setInterval(() => void load(), 4000);
    return () => window.clearInterval(timer);
  }, [load, mediumRun?.status, run?.status, runs]);

  const start = async (horizonDays: 20 | 40) => {
    setError(null);
    try {
      const existing = horizonDays === 20 ? run : mediumRun;
      const result = await startAICalibration(symbol, config, Boolean(existing), horizonDays);
      if (horizonDays === 20) setRun(result);
      else setMediumRun(result);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : '启动历史校准失败');
    }
  };

  const stop = async (horizonDays: 20 | 40) => {
    setError(null);
    try {
      const result = await stopAICalibration(symbol, config, horizonDays);
      if (horizonDays === 20) setRun(result);
      else setMediumRun(result);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : '停止历史校准失败');
    }
  };

  const running = run?.status === 'running';
  const mediumRunning = mediumRun?.status === 'running';
  const complete = run?.status === 'complete';
  const excluded = run?.payload?.excluded_analysts ?? [];
  const totalNodes = run?.total_nodes || 10;
  const completedNodes = Math.min(run?.completed_nodes ?? 0, totalNodes);
  const progressPercent = totalNodes > 0 ? Math.round((completedNodes / totalNodes) * 100) : 0;
  const independentRuns = runs.filter((item) => item.payload?.requested_analysts?.length === 1);
  const scoredNodes = run?.sample_count ?? 0;
  const replayedNodes = run?.payload?.total_node_count ?? run?.total_nodes ?? 0;
  const baselines = Object.entries(run?.payload?.baselines ?? {});

  return (
    <Card className="space-y-4 border-violet-400/20 bg-slate-950/70">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <span className="flex h-10 w-10 items-center justify-center rounded-xl border border-violet-400/25 bg-violet-400/10 text-violet-200">
            <Gauge className="h-5 w-5" />
          </span>
          <div>
            <h2 className="text-lg font-black text-white">模型历史校准</h2>
            <p className="mt-1 text-sm text-slate-500">独立重放 10 个历史节点；普通 AI 分析只读取结果</p>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          {([20, 40] as const).map((horizon) => {
            const horizonRun = horizon === 20 ? run : mediumRun;
            const horizonRunning = horizonRun?.status === 'running';
            return (
              <Button
                key={horizon}
                variant={horizonRunning ? 'danger' : 'outline'}
                disabled={disabled || loading || (horizon === 20 ? mediumRunning : running)}
                onClick={() => {
                  if (horizonRunning) void stop(horizon);
                  else {
                    setPendingHorizon(horizon);
                    setConfirmOpen(true);
                  }
                }}
              >
                {horizonRunning ? <Square className="h-4 w-4" /> : horizonRun?.status === 'complete' ? <RefreshCw className="h-4 w-4" /> : <Gauge className="h-4 w-4" />}
                {horizonRunning ? `停止 ${horizon} 日` : `${horizon} 日校准`}
              </Button>
            );
          })}
        </div>
      </div>

      {loading ? (
        <p className="text-sm text-slate-500">正在读取当前模型组合的 Run Card…</p>
      ) : complete && run ? (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <div className="rounded-xl border border-slate-800 bg-slate-900/50 p-3">
            <p className="text-xs text-slate-500">方向机会命中</p>
            <p className="mt-1 text-xl font-black text-white">{run.correct_count}/{run.sample_count}</p>
          </div>
          <div className="rounded-xl border border-slate-800 bg-slate-900/50 p-3">
            <p className="text-xs text-slate-500">可执行命中</p>
            <p className="mt-1 text-xl font-black text-white">
              {run.payload?.execution_correct_count ?? 0}/{run.sample_count}
              <span className="ml-2 text-sm text-slate-400">
                {((run.payload?.execution_accuracy ?? 0) * 100).toFixed(0)}%
              </span>
            </p>
          </div>
          <div className="rounded-xl border border-slate-800 bg-slate-900/50 p-3">
            <p className="text-xs text-slate-500">有效证据覆盖</p>
            <p className="mt-1 text-xl font-black text-white">{scoredNodes}/{replayedNodes}</p>
            <p className="mt-1 text-xs text-slate-500">证据不足不进入分母</p>
          </div>
          <div className="rounded-xl border border-slate-800 bg-slate-900/50 p-3">
            <p className="text-xs text-slate-500">校准置信度</p>
            <p className="mt-1 text-xl font-black text-violet-200">{confidenceLabels[run.confidence] ?? run.confidence}</p>
          </div>
          <div className="rounded-xl border border-slate-800 bg-slate-900/50 p-3">
            <p className="text-xs text-slate-500">验证周期</p>
            <p className="mt-1 text-xl font-black text-white">{run.horizon_days} 个交易日</p>
          </div>
        </div>
      ) : running ? (
        <div className="space-y-3 rounded-xl border border-violet-400/20 bg-violet-400/5 px-3 py-3 text-sm text-violet-100">
          <div className="flex items-center justify-between gap-3">
            <span>历史节点进度：{completedNodes}/{totalNodes}</span>
            <span className="font-bold">{progressPercent}%</span>
          </div>
          <div className="h-2 overflow-hidden rounded-full bg-slate-800" role="progressbar" aria-valuemin={0} aria-valuemax={totalNodes} aria-valuenow={completedNodes}>
            <div className="h-full rounded-full bg-violet-400 transition-[width] duration-500" style={{ width: `${progressPercent}%` }} />
          </div>
          <p className="text-xs text-slate-400">
            默认 7 路并行{run.cached_nodes > 0 ? ` · 已复用 ${run.cached_nodes} 个缓存节点` : ''}
            {run.failed_nodes > 0 ? ` · ${run.failed_nodes} 个节点等待重试` : ''}。停止后不再派发新节点，已发出的请求会短暂收尾。
          </p>
        </div>
      ) : run?.status === 'failed' ? (
        <p className="rounded-xl border border-red-800 bg-red-950/60 px-3 py-3 text-sm text-red-100">校准失败：{run.error ?? '未知错误'}</p>
      ) : (
        <p className="text-sm text-slate-500">当前标的与模型组合尚无历史校准结果。</p>
      )}

      {complete && run ? (
        <div className="space-y-2 rounded-xl border border-slate-800 bg-slate-900/40 p-3">
          <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-sm font-bold text-slate-200">恒定方向基准</p>
            <p className="text-xs text-slate-500">
              模型相对最佳基准：机会 {formatLift(run.payload?.opportunity_lift_vs_best_baseline)} · 执行 {formatLift(run.payload?.execution_lift_vs_best_baseline)}
            </p>
          </div>
          {baselines.length > 0 ? (
            <div className="grid gap-2 sm:grid-cols-3">
              {baselines.map(([action, baseline]) => (
                <div key={action} className="rounded-lg border border-slate-800 bg-slate-950/60 px-3 py-2 text-xs text-slate-400">
                  <p className="font-semibold text-slate-200">{baselineLabels[action] ?? action}</p>
                  <p className="mt-1">机会 {(baseline.opportunity_accuracy * 100).toFixed(0)}% · 执行 {(baseline.execution_accuracy * 100).toFixed(0)}%</p>
                </div>
              ))}
            </div>
          ) : <p className="text-xs text-slate-500">本次结果尚未包含基准数据。</p>}
        </div>
      ) : null}

      {(run?.status === 'complete' || mediumRun?.status === 'complete') ? (
        <div className="grid gap-2 sm:grid-cols-2">
          {[run, mediumRun].map((item, index) => {
            const horizon = index === 0 ? 20 : 40;
            return (
              <div key={horizon} className="rounded-xl border border-slate-800 bg-slate-900/40 p-3 text-sm">
                <p className="font-bold text-slate-200">{horizon} 日方向</p>
                {item?.status === 'complete' ? (
                  <p className="mt-1 text-slate-400">
                    机会 {(item.accuracy * 100).toFixed(0)}% · 执行 {((item.payload?.execution_accuracy ?? 0) * 100).toFixed(0)}% · 相对基准 {formatLift(item.payload?.opportunity_lift_vs_best_baseline)}
                  </p>
                ) : <p className="mt-1 text-slate-500">尚未校准</p>}
              </div>
            );
          })}
        </div>
      ) : null}

      {complete && run ? (
        <div className="text-xs leading-5 text-slate-500">
          模型组合：{run.deep_think_llm} / {run.quick_think_llm} · 市场基准：{run.payload?.benchmark ?? 'SPY'} · 方向预测周期：未来 {run.horizon_days} 个交易日
          {(run.payload?.unscorable_count ?? 0) > 0 ? ` · ${run.payload?.unscorable_count} 个节点因历史证据不足未评分` : ''}
          {excluded.length > 0 ? ` · 为避免未来数据，未重放：${excluded.join('、')}` : ''}
        </div>
      ) : null}

      {independentRuns.length > 0 ? (
        <div className="space-y-2 rounded-xl border border-slate-800 bg-slate-900/40 p-3">
          <div className="flex items-center justify-between gap-3">
            <p className="text-sm font-bold text-slate-200">单分析师校准</p>
            <p className="text-xs text-slate-500">每 4 秒自动更新</p>
          </div>
          {independentRuns.map((item) => {
            const analyst = item.payload?.requested_analysts?.[0] ?? 'unknown';
            const itemTotal = item.total_nodes || 10;
            const itemCompleted = Math.min(item.completed_nodes, itemTotal);
            const itemPercent = itemTotal > 0 ? Math.round((itemCompleted / itemTotal) * 100) : 0;
            return (
              <div key={item.id} className="space-y-2 rounded-lg border border-slate-800 bg-slate-950/60 px-3 py-2">
                <div className="flex items-center justify-between gap-3 text-sm">
                  <span className="font-semibold text-slate-200">{analystLabels[analyst] ?? analyst} · {item.horizon_days} 日</span>
                  <span className={item.status === 'failed' ? 'text-red-300' : item.status === 'complete' ? 'text-emerald-300' : 'text-violet-200'}>
                    {item.status === 'complete'
                      ? `${item.correct_count}/${item.sample_count} · ${(item.accuracy * 100).toFixed(0)}% · 可评分 ${item.sample_count}/${item.payload?.total_node_count ?? item.total_nodes}`
                      : item.status === 'failed'
                        ? '失败'
                        : `${itemCompleted}/${itemTotal} · ${itemPercent}%${item.failed_nodes > 0 ? ` · ${item.failed_nodes} 失败` : ''}`}
                  </span>
                </div>
                {item.status === 'running' ? (
                  <div className="h-1.5 overflow-hidden rounded-full bg-slate-800">
                    <div className="h-full rounded-full bg-violet-400 transition-[width] duration-500" style={{ width: `${itemPercent}%` }} />
                  </div>
                ) : null}
                {item.status === 'failed' && item.error ? <p className="text-xs text-red-300">{item.error}</p> : null}
              </div>
            );
          })}
        </div>
      ) : null}
      {error ? <div className="rounded-xl border border-red-800 bg-red-950/60 px-3 py-2 text-sm text-red-100">{error}</div> : null}

      <ConfirmDialog
        open={confirmOpen}
        title={`${pendingHorizon} 日历史校准`}
        description={`该任务会调用当前模型重放 10 个历史节点，预测未来 ${pendingHorizon} 个交易日方向；证据不足节点不会进入准确率。40 日目标会按 √2 放大。耗时和模型费用约为一次分析的 10 倍，之后会复用持久化结果。`}
        cancelText="取消"
        confirmText="开始校准"
        onConfirm={() => void start(pendingHorizon)}
        onOpenChange={setConfirmOpen}
      />
    </Card>
  );
}
