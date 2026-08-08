import { useState } from 'react';
import { Save } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Select } from '../../components/ui/select';
import { createCandidateSnapshot, createResearchCase, createUserThesis } from '../../lib/decisionSpineApi';
import type { Market } from '../../types';
import type { UserThesis, UserThesisStance } from '../../types/decisionSpine';

interface DecisionThesisPanelProps {
  symbol: string;
  market: Market;
  aiSummary?: string;
  onSaved: (thesis: UserThesis) => void;
}

function lines(value: string) {
  return value.split(/\n+/).map((line) => line.trim()).filter(Boolean);
}

function ThesisTextarea({ label, value, onChange, required = false }: { label: string; value: string; onChange: (value: string) => void; required?: boolean }) {
  return (
    <label className="block">
      <span className="text-sm font-semibold text-slate-300">{label}</span>
      <textarea
        aria-label={label}
        className="mt-2 min-h-24 w-full resize-y rounded-xl border border-slate-700/80 bg-slate-950/55 px-3.5 py-3 text-sm leading-6 text-slate-100 outline-none focus:border-brand-300/60 focus:ring-2 focus:ring-brand-300/15"
        required={required}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  );
}

export default function DecisionThesisPanel({ symbol, market, aiSummary, onSaved }: DecisionThesisPanelProps) {
  const [stance, setStance] = useState<UserThesisStance>('watch');
  const [summary, setSummary] = useState('');
  const [accepted, setAccepted] = useState('');
  const [rejected, setRejected] = useState('');
  const [unresolved, setUnresolved] = useState('');
  const [falsifiers, setFalsifiers] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [savedVersion, setSavedVersion] = useState<number | null>(null);

  const save = async () => {
    if (!summary.trim()) {
      setError('请先写下你自己的当前观点');
      return;
    }
    setSaving(true);
    setError('');
    try {
      const capturedAt = new Date().toISOString();
      const candidate = await createCandidateSnapshot({
        symbol,
        market,
        screen_name: 'analysis_workspace',
        screen_definition_id: null,
        inclusion_reasons: [],
        exclusion_risks: [],
        context_snapshot: { origin: 'analysis_workspace', ai_summary: aiSummary ?? null, captured_at: capturedAt },
        data_as_of: capturedAt
      });
      const research = await createResearchCase({
        candidate_snapshot_id: candidate.id,
        symbol,
        market,
        analysis_run_id: null,
        evidence_snapshot: { ai_summary: aiSummary ?? null, status: aiSummary ? 'available' : 'not_run', captured_at: capturedAt }
      });
      const thesis = await createUserThesis(research.id, {
        stance,
        summary: summary.trim(),
        accepted_arguments: lines(accepted),
        rejected_arguments: lines(rejected),
        unresolved_questions: lines(unresolved),
        falsifiers: lines(falsifiers)
      });
      setSavedVersion(thesis.version);
      onSaved(thesis);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '论点保存失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card className="p-4 sm:p-5">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 className="text-lg font-black text-white">形成我的论点</h2>
        </div>
        <label className="min-w-36">
          <span className="text-sm font-semibold text-slate-300">当前立场</span>
          <Select className="mt-2" value={stance} onValueChange={(value) => setStance(value as UserThesisStance)}>
            <option value="long">偏多</option>
            <option value="short">偏空</option>
            <option value="watch">观察</option>
            <option value="pass">放弃</option>
            <option value="insufficient">信息不足</option>
          </Select>
        </label>
      </div>
      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <ThesisTextarea label="我的当前观点" required value={summary} onChange={setSummary} />
        <ThesisTextarea label="我认可的观点" value={accepted} onChange={setAccepted} />
        <ThesisTextarea label="我不同意或暂不采纳的观点" value={rejected} onChange={setRejected} />
        <ThesisTextarea label="仍待确认的问题" value={unresolved} onChange={setUnresolved} />
        <div className="lg:col-span-2">
          <ThesisTextarea label="什么情况证明我错了" value={falsifiers} onChange={setFalsifiers} />
        </div>
      </div>
      {error ? <p className="mt-4 text-sm font-semibold text-red-300">{error}</p> : null}
      {savedVersion !== null ? <p className="mt-4 text-sm font-semibold text-emerald-200">已保存为论点版本 {savedVersion}，新建计划将引用此版本。</p> : null}
      <div className="mt-4 flex justify-end">
        <Button onClick={save} disabled={saving}><Save className="h-4 w-4" />{saving ? '保存中' : '保存我的论点'}</Button>
      </div>
    </Card>
  );
}
