import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip as RechartsTooltip, XAxis, YAxis } from 'recharts';
import type { ChartStatus, GrossMarginData, GrossMarginQuarter } from '../../../types/analysis';
import ChartFrame from './ChartFrame';
import { chartGridColor, chartTextColor, statusFromLength } from './chartUtils';

interface GrossMarginChartProps {
  data: GrossMarginData | null;
  status: ChartStatus;
}

function TooltipContent({ active, payload }: { active?: boolean; payload?: Array<{ payload: GrossMarginQuarter }> }) {
  if (!active || !payload?.length) return null;
  const item = payload[0].payload;
  return (
    <div className="rounded-xl border border-slate-700 bg-slate-950/95 px-3 py-2 text-xs text-slate-200 shadow-xl">
      <p className="font-semibold text-white">{item.period}</p>
      <p className="mt-1">毛利率：{item.gross_margin.toFixed(2)}%</p>
    </div>
  );
}

export default function GrossMarginChart({ data, status }: GrossMarginChartProps) {
  const items = [...(data?.quarters ?? [])].reverse();
  const finalStatus = statusFromLength(status, items.length);

  return (
    <ChartFrame title="季度毛利率走势" subtitle="观察盈利质量与产品结构变化" status={finalStatus} height={250} source={data?.source} dataAsOf={data?.data_as_of}>
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={items} margin={{ top: 8, right: 12, bottom: 0, left: 0 }}>
          <CartesianGrid stroke={chartGridColor} vertical={false} />
          <XAxis dataKey="period" tick={{ fill: chartTextColor, fontSize: 11 }} stroke={chartGridColor} minTickGap={18} />
          <YAxis tick={{ fill: chartTextColor, fontSize: 11 }} stroke={chartGridColor} width={48} tickFormatter={(value: number) => value + '%'} />
          <RechartsTooltip content={<TooltipContent />} />
          <Line type="monotone" dataKey="gross_margin" stroke="#FF9800" strokeWidth={2.3} dot={{ r: 3 }} isAnimationActive={false} />
        </LineChart>
      </ResponsiveContainer>
    </ChartFrame>
  );
}
