import { useState, type FormEvent } from 'react';
import { ChevronRight, Save, Trash2 } from 'lucide-react';
import type { ThresholdConfig, ThresholdPreset } from '../../types';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Input } from '../../components/ui/input';
import { cn } from '../../lib/utils';

type ThresholdKey = keyof ThresholdConfig;

const fields: Array<{ key: ThresholdKey; label: string; suffix: string; min: number; max: number; step: number }> = [
  { key: 'c_min_growth', label: 'C · 当季 EPS 增长 ≥', suffix: '%', min: 1, max: 200, step: 1 },
  { key: 'a_min_growth', label: 'A · 年度 EPS 增长 ≥', suffix: '%', min: 1, max: 200, step: 1 },
  { key: 'a_min_years', label: 'A · 连续年数 ≥', suffix: '年', min: 1, max: 5, step: 1 },
  { key: 's_volume_ratio', label: 'S · 突破成交量 ≥ 均量', suffix: '%', min: 100, max: 500, step: 5 },
  { key: 's_ma_period', label: 'S · 均量周期', suffix: '日', min: 10, max: 60, step: 1 },
  { key: 'l_min_rs', label: 'L · RS 评级 ≥', suffix: '', min: 50, max: 99, step: 1 }
];

const displayPresetName = (name: string) => name.replace(/奥尼尔/g, '欧奈尔');

interface ThresholdPanelProps {
  open: boolean;
  thresholds: ThresholdConfig;
  presets: ThresholdPreset[];
  activePresetId: number | null;
  onToggle: () => void;
  onChange: (thresholds: Partial<ThresholdConfig>) => void;
  onApplyPreset: (preset: ThresholdPreset) => void;
  onSavePreset: (name: string) => Promise<boolean>;
  onDeletePreset: (preset: ThresholdPreset) => Promise<boolean>;
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
  const [editorMode, setEditorMode] = useState<'save' | 'delete' | null>(null);
  const [presetName, setPresetName] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  const closeEditor = () => {
    if (isSubmitting) return;
    setEditorMode(null);
    setPresetName('');
  };

  const handleSave = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const name = presetName.trim();
    if (!name || isSubmitting) return;
    setIsSubmitting(true);
    const saved = await onSavePreset(name);
    setIsSubmitting(false);
    if (saved) closeEditor();
  };

  const handleDelete = async () => {
    if (!activePreset || activePreset.is_default || isSubmitting) return;
    setIsSubmitting(true);
    const deleted = await onDeletePreset(activePreset);
    setIsSubmitting(false);
    if (deleted) closeEditor();
  };

  return (
    <Card className="overflow-hidden p-0">
      <div className="flex flex-col gap-3 p-4 sm:p-5 lg:flex-row lg:items-center lg:justify-between">
        <button
          type="button"
          className="flex min-w-0 items-center gap-2.5 text-left"
          onClick={onToggle}
          aria-expanded={open}
          aria-controls="screener-threshold-fields"
        >
          <ChevronRight className={cn('h-5 w-5 shrink-0 text-cyan-300 transition-transform', open && 'rotate-90')} />
          <span className="truncate text-base font-semibold text-white">筛选阈值</span>
        </button>

        <div className="flex flex-wrap items-center gap-2">
          <select
            className="h-10 min-w-36 flex-1 rounded-xl border border-slate-700 bg-slate-950/60 px-3 text-sm text-slate-100 outline-none focus:border-cyan-400/60 lg:flex-none"
            value={activePresetId ?? ''}
            aria-label="阈值方案"
            onChange={(event) => {
              const preset = presets.find((item) => String(item.id) === event.target.value);
              if (preset) {
                setEditorMode(null);
                setPresetName('');
                onApplyPreset(preset);
              }
            }}
          >
            <option value="">自定义方案</option>
            {presets.map((preset) => (
              <option key={preset.id} value={preset.id}>
                {displayPresetName(preset.name)}
              </option>
            ))}
          </select>
          <Button
            type="button"
            variant="secondary"
            className="h-10 rounded-xl px-3 py-0"
            onClick={() => {
              setEditorMode('save');
              setPresetName('');
            }}
          >
            <Save className="h-4 w-4" />
            保存方案
          </Button>
          {activePreset && !activePreset.is_default ? (
            <Button
              type="button"
              variant="danger"
              className="h-10 rounded-xl px-3 py-0"
              onClick={() => setEditorMode('delete')}
            >
              <Trash2 className="h-4 w-4" />
              删除
            </Button>
          ) : null}
        </div>
      </div>

      {editorMode === 'save' ? (
        <form className="flex flex-col gap-3 border-t border-slate-800 px-4 py-3 sm:flex-row sm:items-center sm:px-5" onSubmit={handleSave}>
          <Input
            autoFocus
            className="h-10 max-w-sm rounded-xl"
            value={presetName}
            maxLength={50}
            placeholder="方案名称"
            aria-label="方案名称"
            onChange={(event) => setPresetName(event.target.value)}
          />
          <div className="flex items-center gap-2">
            <Button type="submit" className="h-10 rounded-xl py-0" disabled={!presetName.trim() || isSubmitting}>
              {isSubmitting ? '保存中' : '保存'}
            </Button>
            <Button type="button" variant="ghost" className="h-10 rounded-xl py-0" onClick={closeEditor} disabled={isSubmitting}>
              取消
            </Button>
          </div>
        </form>
      ) : null}

      {editorMode === 'delete' && activePreset && !activePreset.is_default ? (
        <div className="flex flex-col gap-3 border-t border-red-900/60 bg-red-950/20 px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5">
          <span className="text-sm font-medium text-red-100">删除「{displayPresetName(activePreset.name)}」？</span>
          <div className="flex items-center gap-2">
            <Button type="button" variant="danger" className="h-10 rounded-xl py-0" onClick={() => void handleDelete()} disabled={isSubmitting}>
              {isSubmitting ? '删除中' : '确认删除'}
            </Button>
            <Button type="button" variant="ghost" className="h-10 rounded-xl py-0" onClick={closeEditor} disabled={isSubmitting}>
              取消
            </Button>
          </div>
        </div>
      ) : null}

      {open ? (
        <div id="screener-threshold-fields" className="grid gap-4 border-t border-slate-800 p-4 sm:grid-cols-2 sm:p-5 lg:grid-cols-3 xl:grid-cols-6">
          {fields.map((field) => {
            const value = thresholds[field.key];
            const invalid = value < field.min || value > field.max;
            return (
              <label key={field.key} className="min-w-0 space-y-2">
                <span className="block text-sm font-medium leading-5 text-slate-300">{field.label}</span>
                <div className="flex items-center gap-2">
                  <Input
                    className="h-10 rounded-xl"
                    invalid={invalid}
                    type="number"
                    min={field.min}
                    max={field.max}
                    step={field.step}
                    value={value}
                    onChange={(event) => onChange({ [field.key]: Number(event.target.value) } as Partial<ThresholdConfig>)}
                  />
                  {field.suffix ? <span className="shrink-0 text-sm text-slate-400">{field.suffix}</span> : null}
                </div>
                {invalid ? <span className="block text-xs text-red-300">可用范围 {field.min}–{field.max}</span> : null}
              </label>
            );
          })}
        </div>
      ) : null}
    </Card>
  );
}
