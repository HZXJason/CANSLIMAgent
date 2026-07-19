import { useEffect, useMemo, useState, type TextareaHTMLAttributes } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, BookOpenCheck, Clock3, FilePenLine, LockKeyhole, PlayCircle, Save, XCircle } from 'lucide-react';
import { useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { PageHeader } from '../../components/PageHeader';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { ConfirmDialog } from '../../components/ui/confirm-dialog';
import { Input } from '../../components/ui/input';
import { Select } from '../../components/ui/select';
import { createTradePlan, executeTradePlan, fetchTradePlan, updateTradePlan } from '../../lib/api';
import { assetMarketForSymbol, displaySymbol, normalizeSymbolForAsset } from '../../lib/markets';
import { cn } from '../../lib/utils';
import { useMarketStore } from '../../stores/marketStore';
import type { AssetMarket } from '../../types';
import type { ExecuteTradePlanInput, NewTradePlanInput, TradePlan, TradePlanDirection, TradePlanStatus, TradePlanUpdateInput } from '../../types/tradePlan';

interface LocationState {
  aiSummary?: string;
}

interface PlanForm {
  ticker: string;
  direction: TradePlanDirection | '';
  status: TradePlanStatus;
  plan_date: string;
  valid_until: string;
  entry_trigger: string;
  entry_price: string;
  stop_loss: string;
  take_profit: string;
  position_size: string;
  rationale: string;
  ai_summary: string;
  notes: string;
}

interface ExecuteForm {
  datetime: string;
  quantity: string;
  price: string;
  fee: string;
  market: 'us' | 'hk' | 'china_a' | 'crypto' | 'other';
  currency: string;
  order_id: string;
  notes: string;
}

type PlanFormErrors = Partial<Record<keyof PlanForm | 'submit', string>>;
type ExecuteFormErrors = Partial<Record<keyof ExecuteForm | 'submit', string>>;

const statusLabel: Record<TradePlanStatus, string> = {
  draft: '草稿',
  waiting: '等待触发',
  executed: '已执行',
  cancelled: '已取消',
  expired: '已过期'
};

const directionLabel: Record<TradePlanDirection, string> = {
  buy: '买入',
  sell: '卖出',
  watch: '观察'
};

function todayISO() {
  const date = new Date();
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return year + '-' + month + '-' + day;
}

function currentLocalDatetime() {
  const now = new Date();
  const local = new Date(now.getTime() - now.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function initialPlanForm(symbol = '', aiSummary = '', assetMarket: AssetMarket = 'US'): PlanForm {
  return {
    ticker: normalizeSymbolForAsset(assetMarket, symbol),
    direction: 'buy',
    status: 'draft',
    plan_date: todayISO(),
    valid_until: '',
    entry_trigger: '',
    entry_price: '',
    stop_loss: '',
    take_profit: '',
    position_size: '',
    rationale: '',
    ai_summary: aiSummary,
    notes: ''
  };
}

function formFromPlan(plan: TradePlan): PlanForm {
  return {
    ticker: plan.ticker,
    direction: plan.direction,
    status: plan.status,
    plan_date: plan.plan_date,
    valid_until: plan.valid_until ?? '',
    entry_trigger: plan.entry_trigger ?? '',
    entry_price: plan.entry_price === null ? '' : String(plan.entry_price),
    stop_loss: plan.stop_loss === null ? '' : String(plan.stop_loss),
    take_profit: plan.take_profit === null ? '' : String(plan.take_profit),
    position_size: plan.position_size === null ? '' : String(plan.position_size),
    rationale: plan.rationale ?? '',
    ai_summary: plan.ai_summary ?? '',
    notes: plan.notes ?? ''
  };
}

function initialExecuteForm(plan: TradePlan): ExecuteForm {
  const isChinaA = /\.(SH|SZ|BJ)$/i.test(plan.ticker);
  return {
    datetime: currentLocalDatetime(),
    quantity: '',
    price: plan.entry_price === null ? '' : String(plan.entry_price),
    fee: '0',
    market: isChinaA ? 'china_a' : 'us',
    currency: isChinaA ? 'CNY' : 'USD',
    order_id: '',
    notes: plan.notes ?? ''
  };
}

function cleanText(value: string) {
  const trimmed = value.trim();
  return trimmed ? trimmed : null;
}

function parseOptionalPositive(value: string, field: keyof PlanForm, label: string, errors: PlanFormErrors, max?: number) {
  const trimmed = value.trim();
  if (!trimmed) return null;
  const parsed = Number(trimmed);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    errors[field] = label + '必须为正数';
    return null;
  }
  if (max !== undefined && parsed > max) {
    errors[field] = label + '不能超过 ' + max;
    return null;
  }
  return parsed;
}

function parseExecuteNumber(value: string, field: keyof ExecuteForm, label: string, errors: ExecuteFormErrors) {
  const trimmed = value.trim();
  if (!trimmed) return undefined;
  const parsed = Number(trimmed);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    errors[field] = label + '必须为正数';
    return undefined;
  }
  return parsed;
}

function buildPlanInput(form: PlanForm, coreLocked: boolean, assetMarket: AssetMarket) {
  const errors: PlanFormErrors = {};
  if (coreLocked) {
    return { errors, input: { notes: cleanText(form.notes) } as TradePlanUpdateInput };
  }

  const rawTicker = form.ticker.trim();
  const ticker = normalizeSymbolForAsset(assetMarket, rawTicker);
  if (!rawTicker) {
    errors.ticker = '代码为必填项';
  } else if (!ticker) {
    errors.ticker = assetMarket === 'CN'
      ? '请输入有效的 6 位 A 股代码或匹配的 SH、SZ、BJ 代码'
      : '请输入有效的美股代码，例如 AAPL 或 BRK.B';
  }
  if (!form.direction) errors.direction = '请选择方向';
  if (!form.plan_date) errors.plan_date = '计划日期为必填项';

  const entryPrice = parseOptionalPositive(form.entry_price, 'entry_price', '计划入场价', errors);
  const stopLoss = parseOptionalPositive(form.stop_loss, 'stop_loss', '止损价', errors);
  const takeProfit = parseOptionalPositive(form.take_profit, 'take_profit', '止盈价', errors);
  const positionSize = parseOptionalPositive(form.position_size, 'position_size', '计划仓位', errors, 100);

  if (Object.keys(errors).length > 0 || !form.direction) return { errors, input: null };

  const input: NewTradePlanInput = {
    ticker,
    direction: form.direction,
    status: form.status === 'waiting' ? 'waiting' : 'draft',
    plan_date: form.plan_date,
    valid_until: form.valid_until || null,
    entry_trigger: cleanText(form.entry_trigger),
    entry_price: entryPrice,
    stop_loss: stopLoss,
    take_profit: takeProfit,
    position_size: positionSize,
    rationale: cleanText(form.rationale),
    ai_summary: cleanText(form.ai_summary),
    notes: cleanText(form.notes)
  };
  return { errors, input };
}

function buildExecuteInput(form: ExecuteForm) {
  const errors: ExecuteFormErrors = {};
  if (!form.datetime) errors.datetime = '成交时间为必填项';
  const quantity = parseExecuteNumber(form.quantity, 'quantity', '成交数量', errors);
  const price = parseExecuteNumber(form.price, 'price', '执行价格', errors);
  const fee = form.fee.trim() ? Number(form.fee) : 0;
  if (!Number.isFinite(fee) || fee < 0) errors.fee = '费用必须为零或正数';

  if (Object.keys(errors).length > 0 || quantity === undefined || price === undefined) return { errors, input: null };

  const input: ExecuteTradePlanInput = {
    datetime: form.datetime,
    quantity,
    price,
    fee,
    market: form.market,
    currency: form.currency.trim().toUpperCase(),
    order_id: cleanText(form.order_id),
    notes: cleanText(form.notes)
  };
  return { errors, input };
}

function Textarea({ className, invalid, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement> & { invalid?: boolean }) {
  return (
    <textarea
      aria-invalid={invalid || undefined}
      className={cn(
        'min-h-28 w-full resize-y rounded-xl border bg-slate-950/55 px-3.5 py-3 text-sm leading-6 text-slate-100 outline-none transition-[border-color,background-color,box-shadow] duration-150 ease-out placeholder:text-slate-600 hover:bg-slate-950/75 focus:border-brand-300/60 focus:bg-slate-950/80 focus:ring-2 focus:ring-brand-300/15 disabled:cursor-not-allowed disabled:bg-slate-900/55 disabled:text-slate-500',
        invalid ? 'border-red-400/80 focus:border-red-400 focus:ring-red-400/15' : 'border-slate-700/80',
        className
      )}
      {...props}
    />
  );
}

function FieldError({ value }: { value?: string }) {
  if (!value) return null;
  return <p className="mt-2 text-sm font-medium text-red-300">{value}</p>;
}

function StatusBadge({ status }: { status: TradePlanStatus }) {
  const styles: Record<TradePlanStatus, string> = {
    draft: 'border-slate-700 bg-slate-950/30 text-slate-300',
    waiting: 'border-brand-400/50 bg-brand-400/10 text-brand-200',
    executed: 'border-emerald-400/60 bg-emerald-400/15 text-emerald-200',
    cancelled: 'border-red-400/50 bg-red-950/50 text-red-100',
    expired: 'border-amber-400/50 bg-amber-950/40 text-amber-100'
  };
  return <span className={cn('inline-flex h-7 items-center rounded-full border px-2.5 text-xs font-bold', styles[status])}>{statusLabel[status]}</span>;
}

function PlanFormCard({ plan }: { plan?: TradePlan }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const assetMarket = useMarketStore((state) => state.assetMarket);
  const planAssetMarket = plan ? assetMarketForSymbol(plan.ticker) ?? assetMarket : assetMarket;
  const [searchParams] = useSearchParams();
  const location = useLocation();
  const locationState = location.state as LocationState | null;
  const symbol = searchParams.get('symbol') ?? '';
  const [form, setForm] = useState<PlanForm>(() => (plan ? formFromPlan(plan) : initialPlanForm(symbol, locationState?.aiSummary ?? '', planAssetMarket)));
  const [errors, setErrors] = useState<PlanFormErrors>({});
  const [pendingSaveInput, setPendingSaveInput] = useState<NewTradePlanInput | TradePlanUpdateInput | null>(null);
  const coreLocked = Boolean(plan && plan.status !== 'draft');

  useEffect(() => {
    if (plan) {
      setForm(formFromPlan(plan));
      setErrors({});
    }
  }, [plan]);

  const canSave = useMemo(() => {
    if (coreLocked) return true;
    return Boolean(form.ticker.trim() && form.direction && form.plan_date);
  }, [coreLocked, form.direction, form.plan_date, form.ticker]);

  const saveMutation = useMutation({
    mutationFn: (input: NewTradePlanInput | TradePlanUpdateInput) => (plan ? updateTradePlan(plan.id, input) : createTradePlan(input as NewTradePlanInput)),
    onSuccess: async (saved) => {
      queryClient.setQueryData(['trade-plan', saved.id], saved);
      await queryClient.invalidateQueries({ queryKey: ['trade-plans'] });
      if (!plan) navigate('/trade-plans/' + saved.id, { replace: true });
    },
    onError: (error) => {
      setErrors({ submit: error instanceof Error ? error.message : '保存失败' });
    }
  });

  const updateField = (field: keyof PlanForm, value: string) => {
    setForm((current) => ({ ...current, [field]: field === 'ticker' ? value.toUpperCase() : value }));
    setErrors((current) => ({ ...current, [field]: undefined, submit: undefined }));
  };

  const submit = () => {
    const result = buildPlanInput(form, coreLocked, planAssetMarket);
    setErrors(result.errors);
    if (!result.input) return;
    if (!coreLocked && form.status === 'waiting') {
      setPendingSaveInput(result.input);
      return;
    }
    saveMutation.mutate(result.input);
  };

  return (
    <>
      <Card className="overflow-hidden p-0">
      {plan ? (
        <div className="flex flex-col gap-3 border-b border-slate-800 px-5 py-4 sm:flex-row sm:items-center sm:justify-between sm:px-6">
          <h2 className="text-xl font-bold text-white">{displaySymbol(form.ticker)}</h2>
          <div className="flex flex-wrap items-center gap-3">
            {coreLocked ? (
              <span className="inline-flex items-center gap-2 text-sm font-semibold text-slate-400">
                <LockKeyhole className="h-4 w-4" />
                核心字段已锁定
              </span>
            ) : null}
            <StatusBadge status={plan.status} />
          </div>
        </div>
      ) : null}

      <div className="space-y-6 p-5 sm:p-6">
        <div>
          <h3 className="mb-3 font-bold text-slate-100">基本信息</h3>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <label>
              <span className="text-sm font-semibold text-slate-300">代码</span>
              <Input className="mt-2" value={form.ticker} disabled={coreLocked} invalid={Boolean(errors.ticker)} onChange={(event) => updateField('ticker', event.target.value)} />
              <FieldError value={errors.ticker} />
            </label>
            <label>
              <span className="text-sm font-semibold text-slate-300">方向</span>
              <Select className="mt-2" value={form.direction} disabled={coreLocked} onValueChange={(value) => updateField('direction', value)}>
                {Object.entries(directionLabel).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
              </Select>
              <FieldError value={errors.direction} />
            </label>
            <label>
              <span className="text-sm font-semibold text-slate-300">状态</span>
              <Select className="mt-2" value={form.status} disabled={coreLocked} onValueChange={(value) => updateField('status', value)}>
                <option value="draft">草稿</option>
                <option value="waiting">等待触发</option>
              </Select>
            </label>
            <label>
              <span className="text-sm font-semibold text-slate-300">计划日期</span>
              <Input className="mt-2" type="date" value={form.plan_date} disabled={coreLocked} invalid={Boolean(errors.plan_date)} onChange={(event) => updateField('plan_date', event.target.value)} />
              <FieldError value={errors.plan_date} />
            </label>
            <label>
              <span className="text-sm font-semibold text-slate-300">有效期</span>
              <Input className="mt-2" type="date" value={form.valid_until} disabled={coreLocked} onChange={(event) => updateField('valid_until', event.target.value)} />
            </label>
          </div>
        </div>

        <div className="border-t border-slate-800 pt-5">
          <h3 className="mb-3 font-bold text-slate-100">价格与风险</h3>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <label>
              <span className="text-sm font-semibold text-slate-300">计划仓位（%）</span>
              <Input className="mt-2" type="number" min="0" max="100" step="0.01" value={form.position_size} disabled={coreLocked} invalid={Boolean(errors.position_size)} onChange={(event) => updateField('position_size', event.target.value)} />
              <FieldError value={errors.position_size} />
            </label>
            <label>
              <span className="text-sm font-semibold text-slate-300">计划入场价</span>
              <Input className="mt-2" type="number" min="0" step="0.0001" value={form.entry_price} disabled={coreLocked} invalid={Boolean(errors.entry_price)} onChange={(event) => updateField('entry_price', event.target.value)} />
              <FieldError value={errors.entry_price} />
            </label>
            <label>
              <span className="text-sm font-semibold text-slate-300">止损价</span>
              <Input className="mt-2" type="number" min="0" step="0.0001" value={form.stop_loss} disabled={coreLocked} invalid={Boolean(errors.stop_loss)} onChange={(event) => updateField('stop_loss', event.target.value)} />
              <FieldError value={errors.stop_loss} />
            </label>
            <label>
              <span className="text-sm font-semibold text-slate-300">止盈价</span>
              <Input className="mt-2" type="number" min="0" step="0.0001" value={form.take_profit} disabled={coreLocked} invalid={Boolean(errors.take_profit)} onChange={(event) => updateField('take_profit', event.target.value)} />
              <FieldError value={errors.take_profit} />
            </label>
          </div>
        </div>

        <div className="grid gap-4 border-t border-slate-800 pt-5 lg:grid-cols-2">
          <label>
            <span className="text-sm font-semibold text-slate-300">入场触发</span>
            <Textarea className="mt-2" value={form.entry_trigger} disabled={coreLocked} onChange={(event) => updateField('entry_trigger', event.target.value)} />
          </label>
          <label>
            <span className="text-sm font-semibold text-slate-300">计划依据</span>
            <Textarea className="mt-2" value={form.rationale} disabled={coreLocked} onChange={(event) => updateField('rationale', event.target.value)} />
          </label>
          <label>
            <span className="text-sm font-semibold text-slate-300">AI 分析摘要</span>
            <Textarea className="mt-2" value={form.ai_summary} disabled={coreLocked} onChange={(event) => updateField('ai_summary', event.target.value)} />
          </label>
          <label>
            <span className="text-sm font-semibold text-slate-300">备注</span>
            <Textarea className="mt-2" value={form.notes} onChange={(event) => updateField('notes', event.target.value)} />
          </label>
        </div>

        {errors.submit ? <div className="rounded-2xl border border-red-800 bg-red-950/60 px-4 py-3 text-sm text-red-100">{errors.submit}</div> : null}
      </div>

      <div className="flex border-t border-slate-800 px-5 py-4 sm:justify-end sm:px-6">
        <Button className="w-full sm:w-auto" onClick={submit} disabled={!canSave || saveMutation.isPending}>
          <Save className="h-4 w-4" />
          {saveMutation.isPending ? '保存中' : coreLocked ? '保存备注' : '保存计划'}
        </Button>
      </div>
      </Card>
      <ConfirmDialog
        open={pendingSaveInput !== null}
        title="锁定交易计划"
        description="保存为等待触发后，计划核心字段将无法修改。"
        confirmText="保存并锁定"
        confirmVariant="primary"
        onConfirm={() => {
          if (pendingSaveInput) saveMutation.mutate(pendingSaveInput);
        }}
        onOpenChange={(open) => {
          if (!open) setPendingSaveInput(null);
        }}
      />
    </>
  );
}

function StatusActions({ plan }: { plan: TradePlan }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [pendingStatus, setPendingStatus] = useState<Exclude<TradePlanStatus, 'executed'> | null>(null);
  const statusMutation = useMutation({
    mutationFn: (status: Exclude<TradePlanStatus, 'executed'>) => updateTradePlan(plan.id, { status }),
    onSuccess: async (saved) => {
      queryClient.setQueryData(['trade-plan', saved.id], saved);
      await queryClient.invalidateQueries({ queryKey: ['trade-plans'] });
    }
  });

  const updateStatus = (status: Exclude<TradePlanStatus, 'executed'>) => {
    setPendingStatus(status);
  };

  if (plan.status === 'executed') {
    return (
      <Card className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
        <div>
          <p className="text-sm font-semibold text-emerald-200">已执行</p>
          <p className="mt-1 text-sm text-slate-400">{plan.executed_at ? new Date(plan.executed_at).toLocaleString() : '-'}</p>
        </div>
        {plan.executed_journal_id ? (
          <Button variant="outline" onClick={() => navigate('/journal/' + plan.executed_journal_id)}>
            <BookOpenCheck className="h-4 w-4" />
            查看日志
          </Button>
        ) : null}
      </Card>
    );
  }

  if (plan.status === 'cancelled' || plan.status === 'expired') return null;

  const confirmTitle = pendingStatus === 'waiting' ? '锁定交易计划' : pendingStatus === 'expired' ? '标记计划过期' : '取消交易计划';
  const confirmDescription = pendingStatus === 'waiting' ? '设为等待触发后，计划核心字段将无法修改。' : '此操作无法撤销。';
  const confirmText = pendingStatus === 'waiting' ? '确认锁定' : pendingStatus === 'expired' ? '标记过期' : '取消计划';

  return (
    <>
      <Card className="p-5 sm:p-6">
        <h2 className="text-lg font-bold text-white">计划状态</h2>
        <div className="mt-4 flex flex-col gap-2 sm:flex-row sm:flex-wrap">
          {plan.status === 'draft' ? (
            <Button onClick={() => updateStatus('waiting')} disabled={statusMutation.isPending}>
              <Clock3 className="h-4 w-4" />
              设为等待触发
            </Button>
          ) : null}
          <Button variant="outline" onClick={() => updateStatus('expired')} disabled={statusMutation.isPending}>
            <FilePenLine className="h-4 w-4" />
            标记过期
          </Button>
          <Button variant="danger" onClick={() => updateStatus('cancelled')} disabled={statusMutation.isPending}>
            <XCircle className="h-4 w-4" />
            取消计划
          </Button>
        </div>
        {statusMutation.isError ? (
          <div className="mt-5 rounded-2xl border border-red-800 bg-red-950/60 px-4 py-3 text-sm text-red-100">{statusMutation.error instanceof Error ? statusMutation.error.message : '状态更新失败'}</div>
        ) : null}
      </Card>
      <ConfirmDialog
        open={pendingStatus !== null}
        title={confirmTitle}
        description={confirmDescription}
        confirmText={confirmText}
        confirmVariant={pendingStatus === 'waiting' ? 'primary' : 'danger'}
        onConfirm={() => {
          if (pendingStatus) statusMutation.mutate(pendingStatus);
        }}
        onOpenChange={(open) => {
          if (!open) setPendingStatus(null);
        }}
      />
    </>
  );
}

function ExecutionCard({ plan }: { plan: TradePlan }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<ExecuteForm>(() => initialExecuteForm(plan));
  const [errors, setErrors] = useState<ExecuteFormErrors>({});
  const [pendingExecution, setPendingExecution] = useState<ExecuteTradePlanInput | null>(null);

  useEffect(() => {
    setForm(initialExecuteForm(plan));
    setErrors({});
  }, [plan]);

  const executeMutation = useMutation({
    mutationFn: (input: ExecuteTradePlanInput) => executeTradePlan(plan.id, input),
    onSuccess: async (result) => {
      queryClient.setQueryData(['trade-plan', result.plan.id], result.plan);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['trade-plans'] }),
        queryClient.invalidateQueries({ queryKey: ['journal-entries'] })
      ]);
      navigate('/journal/' + result.journal.id);
    },
    onError: (error) => {
      setErrors({ submit: error instanceof Error ? error.message : '执行失败' });
    }
  });

  if (plan.direction === 'watch' || plan.status === 'executed' || plan.status === 'cancelled' || plan.status === 'expired') return null;

  const updateField = (field: keyof ExecuteForm, value: string) => {
    setForm((current) => ({ ...current, [field]: value }));
    setErrors((current) => ({ ...current, [field]: undefined, submit: undefined }));
  };

  const submit = () => {
    const result = buildExecuteInput(form);
    setErrors(result.errors);
    if (!result.input) return;
    setPendingExecution(result.input);
  };

  return (
    <>
      <Card className="overflow-hidden p-0">
        <div className="border-b border-slate-800 px-5 py-4 sm:px-6">
          <h2 className="text-lg font-bold text-white">记录成交</h2>
        </div>
        <div className="grid gap-4 p-5 sm:grid-cols-2 sm:p-6 lg:grid-cols-3">
        <label>
          <span className="text-sm font-semibold text-slate-300">成交时间</span>
          <Input className="mt-2" type="datetime-local" value={form.datetime} invalid={Boolean(errors.datetime)} onChange={(event) => updateField('datetime', event.target.value)} />
          <FieldError value={errors.datetime} />
        </label>
        <label>
          <span className="text-sm font-semibold text-slate-300">成交数量</span>
          <Input className="mt-2" type="number" min="0" step="0.00000001" value={form.quantity} invalid={Boolean(errors.quantity)} onChange={(event) => updateField('quantity', event.target.value)} />
          <FieldError value={errors.quantity} />
        </label>
        <label>
          <span className="text-sm font-semibold text-slate-300">执行价格</span>
          <Input className="mt-2" type="number" min="0" step="0.0001" value={form.price} invalid={Boolean(errors.price)} onChange={(event) => updateField('price', event.target.value)} />
          <FieldError value={errors.price} />
        </label>
        <label>
          <span className="text-sm font-semibold text-slate-300">手续费</span>
          <Input className="mt-2" type="number" min="0" step="0.00000001" value={form.fee} invalid={Boolean(errors.fee)} onChange={(event) => updateField('fee', event.target.value)} />
          <FieldError value={errors.fee} />
        </label>
        <label>
          <span className="text-sm font-semibold text-slate-300">市场</span>
          <Select className="mt-2" value={form.market} onValueChange={(value) => updateField('market', value)}>
            <option value="us">美股</option><option value="hk">港股</option><option value="china_a">A 股</option><option value="crypto">加密资产</option><option value="other">其他</option>
          </Select>
        </label>
        <label>
          <span className="text-sm font-semibold text-slate-300">币种</span>
          <Input className="mt-2" value={form.currency} onChange={(event) => updateField('currency', event.target.value)} />
        </label>
        <label>
          <span className="text-sm font-semibold text-slate-300">订单号</span>
          <Input className="mt-2" value={form.order_id} onChange={(event) => updateField('order_id', event.target.value)} />
        </label>
        <label className="sm:col-span-2 lg:col-span-3">
          <span className="text-sm font-semibold text-slate-300">执行备注</span>
          <Textarea className="mt-2" value={form.notes} onChange={(event) => updateField('notes', event.target.value)} />
        </label>
        {errors.submit ? <div className="rounded-2xl border border-red-800 bg-red-950/60 px-4 py-3 text-sm text-red-100 sm:col-span-2 lg:col-span-3">{errors.submit}</div> : null}
        </div>
        <div className="flex border-t border-slate-800 px-5 py-4 sm:justify-end sm:px-6">
          <Button className="w-full sm:w-auto" onClick={submit} disabled={executeMutation.isPending}>
            <PlayCircle className="h-4 w-4" />
            {executeMutation.isPending ? '提交中' : '确认成交'}
          </Button>
        </div>
      </Card>
      <ConfirmDialog
        open={pendingExecution !== null}
        title="确认成交"
        description={`${displaySymbol(plan.ticker)} 将标记为已执行，并创建交易日志。`}
        confirmText="确认成交"
        confirmVariant="primary"
        onConfirm={() => {
          if (pendingExecution) executeMutation.mutate(pendingExecution);
        }}
        onOpenChange={(open) => {
          if (!open) setPendingExecution(null);
        }}
      />
    </>
  );
}

function DetailView({ id }: { id: string }) {
  const navigate = useNavigate();
  const planQuery = useQuery({
    queryKey: ['trade-plan', id],
    queryFn: () => fetchTradePlan(id)
  });

  if (planQuery.isLoading) {
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

  if (planQuery.isError || !planQuery.data) {
    const message = planQuery.error instanceof Error ? planQuery.error.message : '交易计划读取失败';
    return (
      <Card className="flex min-h-[320px] flex-col items-center justify-center gap-4 text-center">
        <p className="text-lg font-semibold text-red-100">{message.includes('not found') ? '交易计划不存在' : '交易计划读取失败'}</p>
        <p className="text-sm text-slate-500">{message}</p>
        <Button variant="outline" onClick={() => navigate('/trade-plans')}>
          <ArrowLeft className="h-4 w-4" />
          返回列表
        </Button>
      </Card>
    );
  }

  return (
    <div className="space-y-5">
      <PlanFormCard plan={planQuery.data} />
      <ExecutionCard plan={planQuery.data} />
      <StatusActions plan={planQuery.data} />
    </div>
  );
}

export default function TradePlanEntry() {
  const { id } = useParams();
  const navigate = useNavigate();
  const isNew = !id;

  return (
    <section className="mx-auto flex w-full max-w-[1040px] flex-col gap-6 px-4 py-6 sm:px-6 sm:py-8 lg:px-8 lg:py-10">
      <PageHeader
        eyebrow="执行工作台"
        title={isNew ? '新建交易计划' : '交易计划'}
        description={isNew ? '先定义触发条件与风险边界，再决定何时进入等待执行。' : '查看计划状态、执行条件与成交结果。'}
        meta={isNew ? '草稿可随时修改' : '非草稿计划的核心字段将保持锁定'}
        actions={
          <Button className="w-full sm:w-auto" variant="outline" onClick={() => navigate('/trade-plans')}>
            <ArrowLeft className="h-4 w-4" />
            返回列表
          </Button>
        }
      />

      {id ? <DetailView id={id} /> : <PlanFormCard />}
    </section>
  );
}
