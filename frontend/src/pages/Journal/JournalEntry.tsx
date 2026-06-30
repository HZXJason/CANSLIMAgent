import { useEffect, useMemo, useState, type TextareaHTMLAttributes } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, LockKeyhole, Save } from 'lucide-react';
import { useNavigate, useParams } from 'react-router-dom';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { Input } from '../../components/ui/input';
import { createJournalEntry, fetchJournalEntry, updateJournalReflection } from '../../lib/api';
import { cn, formatDateCN } from '../../lib/utils';
import type { JournalEntry as JournalEntryType, JournalReflectionInput, NewJournalEntryInput, TradeDirection } from '../../types/journal';

interface NewEntryForm {
  ticker: string;
  direction: TradeDirection | '';
  trade_date: string;
  price: string;
  position_size: string;
  stop_loss: string;
  notes: string;
}

interface ReflectionForm {
  exit_price: string;
  pnl_pct: string;
  reflection: string;
}

type NewEntryErrors = Partial<Record<keyof NewEntryForm | 'submit', string>>;

const emptyReflectionForm: ReflectionForm = {
  exit_price: '',
  pnl_pct: '',
  reflection: ''
};

function todayISO() {
  const date = new Date();
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return year + '-' + month + '-' + day;
}

function initialNewEntryForm(): NewEntryForm {
  return {
    ticker: '',
    direction: '',
    trade_date: todayISO(),
    price: '',
    position_size: '',
    stop_loss: '',
    notes: ''
  };
}

function formatNumber(value: number | null, suffix = '') {
  if (value === null) return '-';
  return value.toLocaleString('en-US', { maximumFractionDigits: 4 }) + suffix;
}

function parseRequiredPositive(value: string, field: keyof NewEntryForm, label: string, errors: NewEntryErrors) {
  const trimmed = value.trim();
  if (!trimmed) {
    errors[field] = label + '为必填项';
    return null;
  }
  const parsed = Number(trimmed);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    errors[field] = label + '必须为正数';
    return null;
  }
  return parsed;
}

function validateNewEntry(form: NewEntryForm) {
  const errors: NewEntryErrors = {};
  const ticker = form.ticker.trim().toUpperCase();
  const direction = form.direction;
  if (!ticker) errors.ticker = '代码为必填项';
  if (direction !== 'buy' && direction !== 'sell') errors.direction = '请选择方向';
  if (!form.trade_date) errors.trade_date = '交易日期为必填项';

  const price = parseRequiredPositive(form.price, 'price', '价格', errors);
  const positionSize = parseRequiredPositive(form.position_size, 'position_size', '仓位', errors);
  const stopLoss = parseRequiredPositive(form.stop_loss, 'stop_loss', '止损价', errors);

  if (Object.keys(errors).length > 0 || price === null || positionSize === null || stopLoss === null || (direction !== 'buy' && direction !== 'sell')) {
    return { errors, input: null };
  }

  const input: NewJournalEntryInput = {
    ticker,
    direction,
    trade_date: form.trade_date,
    price,
    position_size: positionSize,
    stop_loss: stopLoss,
    notes: form.notes.trim() || null
  };
  return { errors, input };
}

function mapBackendCreateError(message: string): NewEntryErrors {
  const errors: NewEntryErrors = { submit: message };
  if (message.includes('ticker')) errors.ticker = message;
  if (message.includes('direction')) errors.direction = message;
  if (message.includes('trade_date')) errors.trade_date = message;
  if (message.includes('price') && !message.includes('stop')) errors.price = message;
  if (message.includes('position_size')) errors.position_size = message;
  if (message.includes('stop_loss')) errors.stop_loss = message;
  return errors;
}

function parseNullableNumber(value: string, label: string) {
  const trimmed = value.trim();
  if (!trimmed) return { value: null, error: null };
  const parsed = Number(trimmed);
  if (!Number.isFinite(parsed)) return { value: null, error: label + '必须为数字' };
  return { value: parsed, error: null };
}

function DirectionToggle({
  value,
  onChange,
  invalid
}: {
  value: TradeDirection | '';
  onChange: (value: TradeDirection) => void;
  invalid?: boolean;
}) {
  return (
    <div className={cn('grid h-11 grid-cols-2 rounded-2xl border bg-slate-950/60 p-1', invalid ? 'border-red-500' : 'border-slate-700')}>
      {(['buy', 'sell'] as const).map((direction) => (
        <button
          key={direction}
          type="button"
          className={cn(
            'rounded-xl text-sm font-bold transition',
            value === direction
              ? direction === 'buy'
                ? 'bg-emerald-400 text-slate-950'
                : 'bg-red-500 text-white'
              : 'text-slate-500 hover:text-slate-200'
          )}
          onClick={() => onChange(direction)}
        >
          {direction === 'buy' ? '买入' : '卖出'}
        </button>
      ))}
    </div>
  );
}

function FieldError({ value }: { value?: string }) {
  if (!value) return null;
  return <p className="mt-2 text-xs font-medium text-red-300">{value}</p>;
}

function Textarea({
  className,
  invalid,
  ...props
}: TextareaHTMLAttributes<HTMLTextAreaElement> & { invalid?: boolean }) {
  return (
    <textarea
      className={cn(
        'min-h-28 w-full resize-y rounded-2xl border bg-slate-950/60 px-3 py-3 text-sm text-slate-100 outline-none transition placeholder:text-slate-500 focus:border-cyan-400/60 focus:ring-2 focus:ring-cyan-400/20',
        invalid ? 'border-red-500' : 'border-slate-700',
        className
      )}
      {...props}
    />
  );
}

function ReadOnlyValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-slate-800 bg-slate-950/35 px-4 py-3">
      <p className="text-xs font-semibold text-slate-500">{label}</p>
      <p className="mt-2 break-words text-sm font-semibold text-slate-100">{value}</p>
    </div>
  );
}

function DirectionBadge({ direction }: { direction: TradeDirection }) {
  return (
    <span
      className={cn(
        'inline-flex h-8 items-center rounded-full border px-3 text-xs font-bold',
        direction === 'buy' ? 'border-emerald-400 bg-emerald-400 text-slate-950' : 'border-red-400 bg-red-500 text-white'
      )}
    >
      {direction === 'buy' ? '买入' : '卖出'}
    </span>
  );
}

function NewEntryView() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<NewEntryForm>(() => initialNewEntryForm());
  const [errors, setErrors] = useState<NewEntryErrors>({});

  const isComplete = useMemo(() => {
    return Boolean(
      form.ticker.trim() &&
        form.direction &&
        form.trade_date &&
        form.price.trim() &&
        form.position_size.trim() &&
        form.stop_loss.trim()
    );
  }, [form]);

  const createMutation = useMutation({
    mutationFn: createJournalEntry,
    onSuccess: async (saved) => {
      await queryClient.invalidateQueries({ queryKey: ['journal-entries'] });
      navigate('/journal/' + saved.id, { replace: true });
    },
    onError: (error) => {
      const message = error instanceof Error ? error.message : '保存失败';
      setErrors(mapBackendCreateError(message));
    }
  });

  const updateField = (field: keyof NewEntryForm, value: string) => {
    setForm((current) => ({ ...current, [field]: field === 'ticker' ? value.toUpperCase() : value }));
    setErrors((current) => ({ ...current, [field]: undefined, submit: undefined }));
  };

  const submit = () => {
    const result = validateNewEntry(form);
    setErrors(result.errors);
    if (!result.input) return;
    createMutation.mutate(result.input);
  };

  return (
    <Card>
      <div className="mb-5 flex items-center justify-between gap-3">
        <div>
          <p className="text-sm font-semibold text-cyan-300/80">Decision</p>
          <h2 className="mt-1 text-xl font-bold text-white">新建交易记录</h2>
        </div>
        <LockKeyhole className="h-5 w-5 text-slate-500" />
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <label>
          <span className="text-sm font-semibold text-slate-300">代码</span>
          <Input className="mt-2" value={form.ticker} invalid={Boolean(errors.ticker)} onChange={(event) => updateField('ticker', event.target.value)} />
          <FieldError value={errors.ticker} />
        </label>

        <label>
          <span className="text-sm font-semibold text-slate-300">方向</span>
          <div className="mt-2">
            <DirectionToggle
              value={form.direction}
              invalid={Boolean(errors.direction)}
              onChange={(direction) => {
                setForm((current) => ({ ...current, direction }));
                setErrors((current) => ({ ...current, direction: undefined, submit: undefined }));
              }}
            />
          </div>
          <FieldError value={errors.direction} />
        </label>

        <label>
          <span className="text-sm font-semibold text-slate-300">交易日期</span>
          <Input className="mt-2" type="date" value={form.trade_date} invalid={Boolean(errors.trade_date)} onChange={(event) => updateField('trade_date', event.target.value)} />
          <FieldError value={errors.trade_date} />
        </label>

        <label>
          <span className="text-sm font-semibold text-slate-300">价格</span>
          <Input className="mt-2" type="number" min="0" step="0.0001" value={form.price} invalid={Boolean(errors.price)} onChange={(event) => updateField('price', event.target.value)} />
          <FieldError value={errors.price} />
        </label>

        <label>
          <span className="text-sm font-semibold text-slate-300">仓位（%）</span>
          <Input className="mt-2" type="number" min="0" step="0.01" value={form.position_size} invalid={Boolean(errors.position_size)} onChange={(event) => updateField('position_size', event.target.value)} />
          <FieldError value={errors.position_size} />
        </label>

        <label>
          <span className="text-sm font-semibold text-slate-300">止损价</span>
          <Input className="mt-2" type="number" min="0" step="0.0001" value={form.stop_loss} invalid={Boolean(errors.stop_loss)} onChange={(event) => updateField('stop_loss', event.target.value)} />
          <FieldError value={errors.stop_loss} />
        </label>
      </div>

      <label className="mt-4 block">
        <span className="text-sm font-semibold text-slate-300">备注</span>
        <Textarea className="mt-2" value={form.notes} onChange={(event) => updateField('notes', event.target.value)} />
      </label>

      {errors.submit ? <div className="mt-5 rounded-2xl border border-red-800 bg-red-950/60 px-4 py-3 text-sm text-red-100">{errors.submit}</div> : null}

      <div className="mt-6 flex justify-end">
        <Button onClick={submit} disabled={!isComplete || createMutation.isPending}>
          <Save className="h-4 w-4" />
          {createMutation.isPending ? '保存中' : '保存记录'}
        </Button>
      </div>
    </Card>
  );
}

function DecisionSection({ entry }: { entry: JournalEntryType }) {
  return (
    <Card>
      <div className="mb-5 flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <p className="text-sm font-semibold text-cyan-300/80">Decision</p>
          <h2 className="mt-1 text-xl font-bold text-white">决策 · {formatDateCN(entry.trade_date)}</h2>
        </div>
        <div className="flex items-center gap-2 text-sm font-semibold text-slate-500">
          <LockKeyhole className="h-4 w-4" />
          已锁定
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        <ReadOnlyValue label="代码" value={entry.ticker} />
        <div className="rounded-2xl border border-slate-800 bg-slate-950/35 px-4 py-3">
          <p className="text-xs font-semibold text-slate-500">方向</p>
          <div className="mt-2">
            <DirectionBadge direction={entry.direction} />
          </div>
        </div>
        <ReadOnlyValue label="价格" value={formatNumber(entry.price)} />
        <ReadOnlyValue label="仓位" value={entry.position_size.toFixed(2) + '%'} />
        <ReadOnlyValue label="止损价" value={formatNumber(entry.stop_loss)} />
        <ReadOnlyValue label="记录时间" value={new Date(entry.created_at).toLocaleString()} />
      </div>

      <div className="mt-4 rounded-2xl border border-slate-800 bg-slate-950/35 px-4 py-3">
        <p className="text-xs font-semibold text-slate-500">备注</p>
        <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-slate-100">{entry.notes || '-'}</p>
      </div>
    </Card>
  );
}

function ReflectionSection({ entry }: { entry: JournalEntryType }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<ReflectionForm>(emptyReflectionForm);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  useEffect(() => {
    setForm({
      exit_price: entry.exit_price === null ? '' : String(entry.exit_price),
      pnl_pct: entry.pnl_pct === null ? '' : String(entry.pnl_pct),
      reflection: entry.reflection ?? ''
    });
    setError(null);
    setNotice(null);
  }, [entry.id, entry.exit_price, entry.pnl_pct, entry.reflection]);

  const updateMutation = useMutation({
    mutationFn: (input: JournalReflectionInput) => updateJournalReflection(entry.id, input),
    onSuccess: async (saved) => {
      queryClient.setQueryData(['journal-entry', saved.id], saved);
      await queryClient.invalidateQueries({ queryKey: ['journal-entries'] });
      setNotice('反思已保存');
    },
    onError: (mutationError) => {
      setNotice(null);
      setError(mutationError instanceof Error ? mutationError.message : '保存失败');
    }
  });

  const save = () => {
    const exitPrice = parseNullableNumber(form.exit_price, '离场价格');
    if (exitPrice.error) {
      setError(exitPrice.error);
      return;
    }
    const pnlPct = parseNullableNumber(form.pnl_pct, 'P&L');
    if (pnlPct.error) {
      setError(pnlPct.error);
      return;
    }

    setError(null);
    setNotice(null);
    updateMutation.mutate({
      exit_price: exitPrice.value,
      pnl_pct: pnlPct.value,
      reflection: form.reflection.trim() || null
    });
  };

  return (
    <Card>
      <div className="mb-5">
        <p className="text-sm font-semibold text-cyan-300/80">Reflection</p>
        <h2 className="mt-1 text-xl font-bold text-white">反思</h2>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <label>
          <span className="text-sm font-semibold text-slate-300">离场价格</span>
          <Input
            className="mt-2"
            type="number"
            step="0.0001"
            value={form.exit_price}
            onChange={(event) => {
              setForm((current) => ({ ...current, exit_price: event.target.value }));
              setError(null);
              setNotice(null);
            }}
          />
        </label>

        <label>
          <span className="text-sm font-semibold text-slate-300">P&L %</span>
          <Input
            className="mt-2"
            type="number"
            step="0.0001"
            value={form.pnl_pct}
            onChange={(event) => {
              setForm((current) => ({ ...current, pnl_pct: event.target.value }));
              setError(null);
              setNotice(null);
            }}
          />
        </label>
      </div>

      <label className="mt-4 block">
        <span className="text-sm font-semibold text-slate-300">复盘记录</span>
        <Textarea
          className="mt-2"
          value={form.reflection}
          onChange={(event) => {
            setForm((current) => ({ ...current, reflection: event.target.value }));
            setError(null);
            setNotice(null);
          }}
        />
      </label>

      {error ? <div className="mt-5 rounded-2xl border border-red-800 bg-red-950/60 px-4 py-3 text-sm text-red-100">{error}</div> : null}
      {notice ? <div className="mt-5 rounded-2xl border border-cyan-400/40 bg-finance-navy/60 px-4 py-3 text-sm text-blue-100">{notice}</div> : null}

      <div className="mt-6 flex justify-end">
        <Button onClick={save} disabled={updateMutation.isPending}>
          <Save className="h-4 w-4" />
          {updateMutation.isPending ? '保存中' : '保存反思'}
        </Button>
      </div>
    </Card>
  );
}

function DetailView({ id }: { id: string }) {
  const navigate = useNavigate();
  const entryQuery = useQuery({
    queryKey: ['journal-entry', id],
    queryFn: () => fetchJournalEntry(id)
  });

  if (entryQuery.isLoading) {
    return (
      <Card className="space-y-4">
        <div className="h-8 w-48 animate-pulse rounded-xl bg-slate-800/60" />
        <div className="grid gap-4 md:grid-cols-3">
          {Array.from({ length: 6 }).map((_, index) => (
            <div key={index} className="h-20 animate-pulse rounded-2xl bg-slate-800/60" />
          ))}
        </div>
      </Card>
    );
  }

  if (entryQuery.isError || !entryQuery.data) {
    const message = entryQuery.error instanceof Error ? entryQuery.error.message : '交易记录读取失败';
    return (
      <Card className="flex min-h-[320px] flex-col items-center justify-center gap-4 text-center">
        <p className="text-lg font-semibold text-red-100">{message.includes('entry not found') ? 'Entry not found' : '交易记录读取失败'}</p>
        <p className="text-sm text-slate-500">{message}</p>
        <Button variant="outline" onClick={() => navigate('/journal')}>
          <ArrowLeft className="h-4 w-4" />
          返回列表
        </Button>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <DecisionSection entry={entryQuery.data} />
      <ReflectionSection entry={entryQuery.data} />
    </div>
  );
}

export default function JournalEntry() {
  const { id } = useParams();
  const navigate = useNavigate();
  const isNew = !id;

  return (
    <section className="mx-auto flex w-full max-w-[980px] flex-col gap-6 px-4 py-6 lg:px-8">
      <header className="flex flex-col gap-4 rounded-[1.25rem] border border-slate-800/90 bg-slate-900/70 p-5 shadow-2xl shadow-slate-950/25 backdrop-blur md:flex-row md:items-center md:justify-between">
        <div>
          <p className="text-sm font-semibold text-cyan-300/80">AlphaLens · Trade Journal</p>
          <h1 className="mt-2 text-3xl font-black tracking-tight text-white">{isNew ? '新建交易记录' : '交易记录'}</h1>
        </div>
        <Button variant="outline" onClick={() => navigate('/journal')}>
          <ArrowLeft className="h-4 w-4" />
          返回列表
        </Button>
      </header>

      {id ? <DetailView id={id} /> : <NewEntryView />}
    </section>
  );
}
