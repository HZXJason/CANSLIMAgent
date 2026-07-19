import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip as RechartsTooltip, XAxis, YAxis } from 'recharts';
import type { ChartStatus, KLinePeriod, RSData, RSPoint } from '../../../types/analysis';
import ChartFrame from './ChartFrame';
import { chartGridColor, chartTextColor, formatAxisDate, statusFromLength } from './chartUtils';

interface RSChartProps {
  data: RSData | null;
  status: ChartStatus;
  period: KLinePeriod;
}

function TooltipContent({ active, payload }: { active?: boolean; payload?: Array<{ payload: RSPoint }> }) {
  if (!active || !payload?.length) return null;
  const item = payload[0].payload;
  return (
    <div className="rounded-xl border border-slate-700 bg-slate-950/95 px-3 py-2 text-xs text-slate-200 shadow-xl">
      <p className="font-semibold text-white">{item.date}</p>
      <p className="mt-1">RS 强度：{item.value.toFixed(1)}</p>
    </div>
  );
}

export default function RSChart({ data, status, period }: RSChartProps) {
  const items = data?.rs_line ?? [];
  const finalStatus = statusFromLength(status, items.length);

  return (
    <ChartFrame title="RS 相对强度" status={finalStatus} height={180} source={data?.source} dataAsOf={data?.data_as_of}>
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={items} margin={{ top: 8, right: 12, bottom: 0, left: 0 }}>
          <CartesianGrid stroke={chartGridColor} vertical={false} />
          <XAxis
            dataKey="date"
            tick={{ fill: chartTextColor, fontSize: 11 }}
            tickFormatter={(value: string) => formatAxisDate(value, period)}
            minTickGap={28}
            stroke={chartGridColor}
          />
          <YAxis domain={[0, 99]} tick={{ fill: chartTextColor, fontSize: 11 }} stroke={chartGridColor} width={38} />
          <RechartsTooltip content={<TooltipContent />} />
          <Line type="monotone" dataKey="value" stroke="var(--accent-strong)" strokeWidth={2} dot={false} isAnimationActive={false} />
        </LineChart>
      </ResponsiveContainer>
    </ChartFrame>
  );
}
