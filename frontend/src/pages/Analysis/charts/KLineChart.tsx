import { Bar, CartesianGrid, Cell, ComposedChart, ResponsiveContainer, Tooltip as RechartsTooltip, XAxis, YAxis } from 'recharts';
import { cn } from '../../../lib/utils';
import type { AssetMarket } from '../../../types';
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
  assetMarket: AssetMarket;
  onPeriodChange: (period: KLinePeriod) => void;
}

export const KLINE_MARKET_COLORS: Record<AssetMarket, { up: string; down: string; volumeUp: string; volumeDown: string }> = {
  US: {
    up: '#26a69a',
    down: '#ef5350',
    volumeUp: 'rgba(38, 166, 154, 0.32)',
    volumeDown: 'rgba(239, 83, 80, 0.32)'
  },
  CN: {
    up: '#ef5350',
    down: '#26a69a',
    volumeUp: 'rgba(239, 83, 80, 0.32)',
    volumeDown: 'rgba(38, 166, 154, 0.32)'
  }
};

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
  assetMarket?: AssetMarket;
}

function CandleStickShape({ x = 0, y = 0, width = 0, height = 0, payload, assetMarket = 'US' }: CandleShapeProps) {
  if (!payload) return null;
  const colors = KLINE_MARKET_COLORS[assetMarket];
  const color = payload.close >= payload.open ? colors.up : colors.down;
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

export default function KLineChart({ data, status, period, assetMarket, onPeriodChange }: KLineChartProps) {
  const items: KLineDatum[] = (data?.candles ?? []).map((candle) => ({
    ...candle,
    priceRange: [candle.low, candle.high],
    up: candle.close >= candle.open
  }));
  const finalStatus = statusFromLength(status, items.length);
  const maxVolume = Math.max(...items.map((item) => item.volume), 1);
  const colors = KLINE_MARKET_COLORS[assetMarket];

  return (
    <ChartFrame
      title="K 线与成交量"
      status={finalStatus}
      height={420}
      heightClassName="h-[320px] sm:h-[360px] xl:h-[420px]"
      source={data?.source}
      dataAsOf={data?.data_as_of}
      action={
        <div
          className="grid grid-cols-3 overflow-hidden rounded-xl border border-slate-700/80 bg-slate-950/35"
          role="group"
          aria-label="K 线周期"
        >
          {(Object.keys(periodLabels) as KLinePeriod[]).map((item) => (
            <button
              key={item}
              type="button"
              aria-pressed={period === item}
              className={cn(
                'interactive-press min-h-11 min-w-11 px-3 text-sm font-bold transition-[background-color,color] duration-150',
                period === item ? 'bg-brand-300 text-slate-950' : 'text-slate-400 hover:bg-slate-800 hover:text-white'
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
            tick={{ fill: chartTextColor, fontSize: 11 }}
            tickFormatter={(value: string) => formatAxisDate(value, period)}
            minTickGap={34}
            stroke={chartGridColor}
          />
          <YAxis yAxisId="price" domain={['dataMin', 'dataMax']} tick={{ fill: chartTextColor, fontSize: 11 }} stroke={chartGridColor} width={48} />
          <YAxis yAxisId="volume" orientation="right" hide domain={[0, maxVolume * 4]} />
          <RechartsTooltip content={<TooltipContent />} />
          <Bar yAxisId="volume" dataKey="volume" barSize={6} isAnimationActive={false}>
            {items.map((item) => (
              <Cell key={item.date + '-volume'} fill={item.up ? colors.volumeUp : colors.volumeDown} />
            ))}
          </Bar>
          <Bar yAxisId="price" dataKey="priceRange" shape={<CandleStickShape assetMarket={assetMarket} />} isAnimationActive={false} />
        </ComposedChart>
      </ResponsiveContainer>
    </ChartFrame>
  );
}
