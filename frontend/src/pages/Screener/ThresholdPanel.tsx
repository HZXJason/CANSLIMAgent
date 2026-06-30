import type { ThresholdConfig, ThresholdPreset } from '../../types';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Input } from '../../components/ui/input';

type ThresholdKey = keyof ThresholdConfig;

const fields: Array<{ key: ThresholdKey; label: string; suffix: string; min: number; max: number; step: number }> = [
  { key: 'c_min_growth', label: 'C：当季EPS增长 ≥', suffix: '%', min: 1, max: 200, step: 1 },
  { key: 'a_min_growth', label: 'A：年度EPS增长 ≥', suffix: '%', min: 1, max: 200, step: 1 },
  { key: 'a_min_years', label: 'A：连续年数 ≥', suffix: '年', min: 1, max: 5, step: 1 },
  { key: 's_volume_ratio', label: 'S：突破成交量 ≥ 均量', suffix: '%', min: 100, max: 500, step: 5 },
  { key: 's_ma_period', label: 'S：均量周期', suffix: '日', min: 10, max: 60, step: 1 },
  { key: 'l_min_rs', label: 'L：RS评级 ≥', suffix: '', min: 50, max: 99, step: 1 }
];

interface ThresholdPanelProps {
  open: boolean;
  thresholds: ThresholdConfig;
  presets: ThresholdPreset[];
  activePresetId: number | null;
  onToggle: () => void;
  onChange: (thresholds: Partial<ThresholdConfig>) => void;
  onApplyPreset: (preset: ThresholdPreset) => void;
  onSavePreset: () => void;
  onDeletePreset: (preset: ThresholdPreset) => void;
}

export default function ThresholdPanel({
  open,
  thresholds,
  presets,
  activePresetId,
  onToggle,
  onChange,
  onApplyPreset,
  onSavePreset,
  onDeletePreset
}: ThresholdPanelProps) {
  const activePreset = presets.find((preset) => preset.id === activePresetId);

  return (
    <Card className="h-full">
      <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <button className="flex items-start gap-3 text-left" onClick={onToggle}>
          <span
            className="mt-1 inline-flex h-9 w-9 items-center justify-center rounded-2xl bg-finance-blue/70 text-sm text-white transition-transform"
            style={{ transform: open ? 'rotate(90deg)' : 'rotate(0deg)' }}
          >
            ▶
          </span>
          <span>
            <span className="text-xs font-semibold uppercase tracking-[0.22em] text-cyan-300/80">Strategy</span>
            <span className="mt-1 block text-xl font-bold text-white">CANSLIM 阈值配置</span>
            <span className="mt-1 block max-w-xl text-sm text-slate-400">
              把原来的折叠表单升级成常驻策略面板，扫描前能直接确认关键规则。
            </span>
          </span>
        </button>
        <div className="flex flex-wrap items-center gap-2">
          <select
            className="h-11 rounded-2xl border border-slate-700 bg-slate-950/60 px-3 text-sm text-slate-100 outline-none"
            value={activePresetId ?? ''}
            onChange={(event) => {
              const preset = presets.find((item) => String(item.id) === event.target.value);
              if (preset) onApplyPreset(preset);
            }}
          >
            <option value="">自定义方案</option>
            {presets.map((preset) => (
              <option key={preset.id} value={preset.id}>
                {preset.name}
              </option>
            ))}
          </select>
          <Button variant="secondary" onClick={onSavePreset}>
            保存
          </Button>
          {activePreset && !activePreset.is_default ? (
            <Button variant="danger" onClick={() => onDeletePreset(activePreset)}>
              删除
            </Button>
          ) : null}
        </div>
      </div>

      <div className="grid overflow-hidden transition-all duration-200 ease-in-out" style={{ gridTemplateRows: open ? '1fr' : '0fr' }}>
        <div className="min-h-0">
          <div className="mt-5 grid gap-3 rounded-[1.25rem] border border-slate-800 bg-slate-950/30 p-4 lg:grid-cols-2">
            {fields.map((field) => {
              const value = thresholds[field.key];
              const invalid = value < field.min || value > field.max;
              return (
                <label key={field.key} className="space-y-2 rounded-2xl border border-slate-800/80 bg-slate-900/45 p-3">
                  <span className="text-sm font-medium text-slate-300">{field.label}</span>
                  <div className="flex items-center gap-2">
                    <Input
                      invalid={invalid}
                      type="number"
                      step={field.step}
                      value={value}
                      onChange={(event) => onChange({ [field.key]: Number(event.target.value) } as Partial<ThresholdConfig>)}
                    />
                    <span className="w-10 text-sm text-slate-400">{field.suffix}</span>
                  </div>
                  {invalid ? <p className="text-xs text-red-300">超出建议范围（{field.min}~{field.max}），请确认是否继续</p> : null}
                </label>
              );
            })}
          </div>
          <div className="mt-3 rounded-2xl border border-amber-300/30 bg-amber-300/10 px-4 py-3 text-sm text-amber-100">
            I：机构持仓维度暂未接入 13F 数据，本轮筛选不参与评分和过滤。
          </div>
        </div>
      </div>
    </Card>
  );
}
