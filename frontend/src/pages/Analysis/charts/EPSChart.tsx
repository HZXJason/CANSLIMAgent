import { Bar, BarChart, CartesianGrid, Cell, ResponsiveContainer, Tooltip as RechartsTooltip, XAxis, YAxis } from 'recharts';
import type { ChartStatus, EPSData, EPSQuarter } from '../../../types/analysis';
import ChartFrame from './ChartFrame';
import { chartGridColor, chartTextColor, statusFromLength } from './chartUtils';

interface EPSChartProps {
  data: EPSData | null;
  status: ChartStatus;
}

function TooltipContent({ active, payload }: { active?: boolean; payload?: Array<{ payload: EPSQuarter }> }) {
  if (!active || !payload?.length) return null;
  const item = payload[0].payload;
  return (
    <div className="rounded-xl border border-slate-700 bg-slate-950/95 px-3 py-2 text-xs text-slate-200 shadow-xl">
      <p className="font-semibold text-white">{item.period}</p>
      <p className="mt-1">EPS：{item.eps.toFixed(2)}</p>
      <p>同比：{item.eps_yoy_growth.toFixed(2)}%</p>
      {!item.data_sufficient ? <p className="mt-1 text-amber-300">同比基准不足，显示参考值</p> : null}
    </div>
  );
}

export default function EPSChart({ data, status }: EPSChartProps) {
  const items = [...(data?.quarters ?? [])].reverse();
  const finalStatus = statusFromLength(status, items.length);

  return (
    <ChartFrame title="季度 EPS 趋势" subtitle="柱形显示 EPS 同比增长率" status={finalStatus} height={250} source={data?.source} dataAsOf={data?.data_as_of}>
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={items} margin={{ top: 8, right: 12, bottom: 0, left: 0 }}>
          <CartesianGrid stroke={chartGridColor} vertical={false} />
          <XAxis dataKey="period" tick={{ fill: chartTextColor, fontSize: 11 }} stroke={chartGridColor} minTickGap={18} />
          <YAxis tick={{ fill: chartTextColor, fontSize: 11 }} stroke={chartGridColor} width={48} tickFormatter={(value: number) => value + '%'} />
          <RechartsTooltip content={<TooltipContent />} />
          <Bar dataKey="eps_yoy_growth" radius={[4, 4, 0, 0]} isAnimationActive={false}>
            {items.map((item) => (
              <Cell key={item.period} fill={item.eps_yoy_growth >= 0 ? '#26A69A' : '#EF5350'} opacity={item.data_sufficient ? 1 : 0.55} />
            ))}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </ChartFrame>
  );
}
