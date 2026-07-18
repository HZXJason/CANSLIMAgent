import { Bar, CartesianGrid, ComposedChart, Line, ResponsiveContainer, Tooltip as RechartsTooltip, XAxis, YAxis } from 'recharts';
import type { ChartStatus, RevenueData, RevenueYear } from '../../../types/analysis';
import ChartFrame from './ChartFrame';
import { chartGridColor, chartTextColor, formatCompactNumber, statusFromLength } from './chartUtils';

interface RevenueChartProps {
  data: RevenueData | null;
  status: ChartStatus;
}

function TooltipContent({ active, payload }: { active?: boolean; payload?: Array<{ payload: RevenueYear }> }) {
  if (!active || !payload?.length) return null;
  const item = payload[0].payload;
  return (
    <div className="rounded-xl border border-slate-700 bg-slate-950/95 px-3 py-2 text-xs text-slate-200 shadow-xl">
      <p className="font-semibold text-white">{item.year} 年</p>
      <p className="mt-1">营收：{formatCompactNumber(item.revenue)}</p>
      <p>同比：{item.data_sufficient ? item.revenue_yoy_growth.toFixed(2) + '%' : '数据不足'}</p>
    </div>
  );
}

export default function RevenueChart({ data, status }: RevenueChartProps) {
  const items = [...(data?.years ?? [])].reverse();
  const finalStatus = statusFromLength(status, items.length);

  return (
    <ChartFrame title="年度营收" status={finalStatus} height={240} source={data?.source} dataAsOf={data?.data_as_of}>
      <ResponsiveContainer width="100%" height="100%">
        <ComposedChart data={items} margin={{ top: 8, right: 12, bottom: 0, left: 0 }}>
          <CartesianGrid stroke={chartGridColor} vertical={false} />
          <XAxis dataKey="year" tick={{ fill: chartTextColor, fontSize: 11 }} stroke={chartGridColor} />
          <YAxis yAxisId="left" tick={{ fill: chartTextColor, fontSize: 11 }} stroke={chartGridColor} width={60} tickFormatter={formatCompactNumber} />
          <YAxis
            yAxisId="right"
            orientation="right"
            tick={{ fill: chartTextColor, fontSize: 11 }}
            stroke={chartGridColor}
            width={44}
            tickFormatter={(value: number) => value + '%'}
          />
          <RechartsTooltip content={<TooltipContent />} />
          <Bar yAxisId="left" dataKey="revenue" fill="rgba(34, 211, 238, 0.58)" radius={[4, 4, 0, 0]} isAnimationActive={false} />
          <Line yAxisId="right" type="monotone" dataKey="revenue_yoy_growth" stroke="#26A69A" strokeWidth={2} dot={{ r: 3 }} isAnimationActive={false} />
        </ComposedChart>
      </ResponsiveContainer>
    </ChartFrame>
  );
}
