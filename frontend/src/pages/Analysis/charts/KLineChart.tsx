import { Bar, CartesianGrid, Cell, ComposedChart, ResponsiveContainer, Tooltip as RechartsTooltip, XAxis, YAxis } from 'recharts';
import { cn } from '../../../lib/utils';
import type { Candle, ChartStatus, KLineData, KLinePeriod } from '../../../types/analysis';
import ChartFrame from './ChartFrame';
import { chartGridColor, chartTextColor, formatAxisDate, formatCompactNumber, statusFromLength } from './chartUtils';

const periodLabels: Record<KLinePeriod, string> = {
  daily: '日',
  weekly: '周',
  monthly: '月'
};

interface KLineChartProps {
  data: KLineData | null;
  status: ChartStatus;
  period: KLinePeriod;
  onPeriodChange: (period: KLinePeriod) => void;
}

interface KLineDatum extends Candle {
  priceRange: [number, number];
  up: boolean;
}

interface CandleShapeProps {
  x?: number;
  y?: number;
  width?: number;
  height?: number;
  payload?: KLineDatum;
}

function CandleStickShape({ x = 0, y = 0, width = 0, height = 0, payload }: CandleShapeProps) {
  if (!payload) return null;
  const color = payload.close >= payload.open ? '#26A69A' : '#EF5350';
  const range = payload.high - payload.low || 1;
  const priceToY = (price: number) => y + ((payload.high - price) / range) * height;
  const openY = priceToY(payload.open);
  const closeY = priceToY(payload.close);
  const bodyY = Math.min(openY, closeY);
  const bodyHeight = Math.max(2, Math.abs(closeY - openY));
  const centerX = x + width / 2;
  const bodyWidth = Math.max(4, Math.min(width * 0.58, 12));

  return (
    <g>
      <line x1={centerX} x2={centerX} y1={y} y2={y + height} stroke={color} strokeWidth={1.4} />
      <rect x={centerX - bodyWidth / 2} y={bodyY} width={bodyWidth} height={bodyHeight} rx={1.5} fill={color} />
    </g>
  );
}

function TooltipContent({ active, payload }: { active?: boolean; payload?: Array<{ payload: KLineDatum }> }) {
  if (!active || !payload?.length) return null;
  const item = payload[0].payload;
  return (
    <div className="rounded-xl border border-slate-700 bg-slate-950/95 px-3 py-2 text-xs text-slate-200 shadow-xl">
      <p className="font-semibold text-white">{item.date}</p>
      <p className="mt-1">开盘：{item.open.toFixed(2)}</p>
      <p>最高：{item.high.toFixed(2)}</p>
      <p>最低：{item.low.toFixed(2)}</p>
      <p>收盘：{item.close.toFixed(2)}</p>
      <p>成交量：{formatCompactNumber(item.volume)}</p>
    </div>
  );
}

export default function KLineChart({ data, status, period, onPeriodChange }: KLineChartProps) {
  const items: KLineDatum[] = (data?.candles ?? []).map((candle) => ({
    ...candle,
    priceRange: [candle.low, candle.high],
    up: candle.close >= candle.open
  }));
  const finalStatus = statusFromLength(status, items.length);
  const maxVolume = Math.max(...items.map((item) => item.volume), 1);

  return (
    <ChartFrame
      title="K 线与成交量"
      status={finalStatus}
      height={420}
      source={data?.source}
      dataAsOf={data?.data_as_of}
      action={
        <div className="grid grid-cols-3 overflow-hidden rounded-xl border border-slate-700 bg-slate-950/30">
          {(Object.keys(periodLabels) as KLinePeriod[]).map((item) => (
            <button
              key={item}
              className={cn(
                'h-9 min-w-12 px-3 text-sm font-semibold transition',
                period === item ? 'bg-cyan-400 text-slate-950' : 'text-slate-400 hover:bg-slate-800 hover:text-white'
              )}
              onClick={() => onPeriodChange(item)}
            >
              {periodLabels[item]}
            </button>
          ))}
        </div>
      }
    >
      <ResponsiveContainer width="100%" height="100%">
        <ComposedChart data={items} margin={{ top: 8, right: 12, bottom: 0, left: 0 }}>
          <CartesianGrid stroke={chartGridColor} vertical={false} />
          <XAxis
            dataKey="date"
            tick={{ fill: chartTextColor, fontSize: 12 }}
            tickFormatter={(value: string) => formatAxisDate(value, period)}
            minTickGap={28}
            stroke={chartGridColor}
          />
          <YAxis yAxisId="price" domain={['dataMin', 'dataMax']} tick={{ fill: chartTextColor, fontSize: 12 }} stroke={chartGridColor} width={54} />
          <YAxis yAxisId="volume" orientation="right" hide domain={[0, maxVolume * 4]} />
          <RechartsTooltip content={<TooltipContent />} />
          <Bar yAxisId="volume" dataKey="volume" barSize={6} isAnimationActive={false}>
            {items.map((item) => (
              <Cell key={item.date + '-volume'} fill={item.up ? 'rgba(38, 166, 154, 0.32)' : 'rgba(239, 83, 80, 0.32)'} />
            ))}
          </Bar>
          <Bar yAxisId="price" dataKey="priceRange" shape={<CandleStickShape />} isAnimationActive={false} />
        </ComposedChart>
      </ResponsiveContainer>
    </ChartFrame>
  );
}
