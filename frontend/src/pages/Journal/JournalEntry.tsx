import { useEffect, useMemo, useState, type TextareaHTMLAttributes } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, LockKeyhole, Save, Trash2 } from 'lucide-react';
import { useNavigate, useParams } from 'react-router-dom';
import { PageHeader } from '../../components/PageHeader';
import { Button } from '../../components/ui/button';
import { Card } from '../../components/ui/card';
import { ConfirmDialog } from '../../components/ui/confirm-dialog';
import { Input } from '../../components/ui/input';
import { Select } from '../../components/ui/select';
import { createJournalEntry, deleteJournalEntry, fetchJournalEntry, updateJournalReflection } from '../../lib/api';
import { displaySymbol, normalizeSymbolForAsset } from '../../lib/markets';
import { cn } from '../../lib/utils';
import { useMarketStore } from '../../stores/marketStore';
import type { AssetMarket } from '../../types';
import type {
  JournalEntry as JournalEntryType,
  JournalReflectionInput,
  NewJournalEntryInput,
  TradeMarket,
  TradeSide
} from '../../types/journal';

interface EntryForm {
  datetime: string;
  symbol: string;
  name: string;
  side: TradeSide;
  quantity: string;
  price: string;
  fee: string;
  market: TradeMarket;
  currency: string;
  order_id: string;
  notes: string;
}

type EntryErrors = Partial<Record<keyof EntryForm | 'submit', string>>;

const marketOptions: Array<{ value: TradeMarket; label: string }> = [
  { value: 'us', label: '美股' },
  { value: 'hk', label: '港股' },
  { value: 'china_a', label: 'A 股' },
  { value: 'crypto', label: '加密资产' },
  { value: 'other', label: '其他' }
];

function currentLocalDatetime() {
  const now = new Date();
  const local = new Date(now.getTime() - now.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function initialForm(assetMarket: AssetMarket): EntryForm {
  return {
    datetime: currentLocalDatetime(),
    symbol: '',
    name: '',
    side: 'buy',
    quantity: '',
    price: '',
    fee: '0',
    market: assetMarket === 'CN' ? 'china_a' : 'us',
    currency: assetMarket === 'CN' ? 'CNY' : 'USD',
    order_id: '',
    notes: ''
  };
}

function parsePositive(value: string, label: string) {
  const number = Number(value);
  return Number.isFinite(number) && number > 0 ? { value: number, error: '' } : { value: 0, error: label + '必须为正数' };
}

function buildInput(form: EntryForm): { input: NewJournalEntryInput | null; errors: EntryErrors } {
  const errors: EntryErrors = {};
  const rawSymbol = form.symbol.trim().toUpperCase();
  const symbol = form.market === 'china_a'
    ? normalizeSymbolForAsset('CN', rawSymbol)
    : form.market === 'us'
      ? normalizeSymbolForAsset('US', rawSymbol)
      : rawSymbol;
  if (!form.datetime) errors.datetime = '成交时间为必填项';
  if (!rawSymbol) {
    errors.symbol = '标的代码为必填项';
  } else if (!symbol) {
    errors.symbol = form.market === 'china_a'
      ? '请输入有效的 6 位 A 股代码或匹配的 SH、SZ、BJ 代码'
      : '请输入有效的美股代码，例如 AAPL 或 BRK.B';
  }
  const quantity = parsePositive(form.quantity, '成交数量');
  const price = parsePositive(form.price, '成交价格');
  if (quantity.error) errors.quantity = quantity.error;
  if (price.error) errors.price = price.error;
  const fee = form.fee.trim() ? Number(form.fee) : 0;
  if (!Number.isFinite(fee) || fee < 0) errors.fee = '费用必须为零或正数';
  if (Object.keys(errors).length > 0) return { input: null, errors };
  return {
    errors,
    input: {
      datetime: form.datetime,
      symbol,
      name: form.name.trim(),
      side: form.side,
      quantity: quantity.value,
      price: price.value,
      amount: quantity.value * price.value,
      fee,
      market: form.market,
      currency: form.currency.trim().toUpperCase(),
      order_id: form.order_id.trim() || null,
      notes: form.notes.trim() || null
    }
  };
}

function Textarea({ className, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      className={cn('min-h-28 w-full resize-y rounded-xl border border-slate-700/80 bg-slate-950/55 px-3.5 py-3 text-sm leading-6 text-slate-100 outline-none transition-[border-color,background-color,box-shadow] duration-150 ease-out placeholder:text-slate-600 hover:bg-slate-950/75 focus:border-brand-300/60 focus:bg-slate-950/80 focus:ring-2 focus:ring-brand-300/15 disabled:cursor-not-allowed disabled:bg-slate-900/55 disabled:text-slate-500', className)}
      {...props}
    />
  );
}

function FieldError({ value }: { value?: string }) {
  return value ? <p className="mt-2 text-sm font-medium text-red-300">{value}</p> : null;
}

function ReadOnlyValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-slate-800 bg-slate-950/35 px-4 py-3">
      <p className="text-sm font-semibold text-slate-400">{label}</p>
      <p className="mt-1 break-words text-sm font-semibold text-slate-100">{value || '-'}</p>
    </div>
  );
}

function NewEntryView() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const assetMarket = useMarketStore((state) => state.assetMarket);
  const [form, setForm] = useState<EntryForm>(() => initialForm(assetMarket));
  const [errors, setErrors] = useState<EntryErrors>({});
  const amount = useMemo(() => Number(form.quantity || 0) * Number(form.price || 0), [form.quantity, form.price]);

  const mutation = useMutation({
    mutationFn: createJournalEntry,
    onSuccess: async (saved) => {
      await queryClient.invalidateQueries({ queryKey: ['journal-entries'] });
      navigate('/journal/' + saved.id, { replace: true });
    },
    onError: (error) => setErrors({ submit: error instanceof Error ? error.message : '保存失败' })
  });

  const update = (field: keyof EntryForm, value: string) => {
    setForm((current) => ({ ...current, [field]: field === 'symbol' ? value.toUpperCase() : value }));
    setErrors((current) => ({ ...current, [field]: undefined, submit: undefined }));
  };

  const submit = () => {
    const result = buildInput(form);
    setErrors(result.errors);
    if (result.input) mutation.mutate(result.input);
  };

  return (
    <Card className="overflow-hidden p-0">
      <div className="space-y-6 p-5 sm:p-6">
        <div>
          <h2 className="mb-3 font-bold text-slate-100">成交信息</h2>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <label><span className="text-sm font-semibold text-slate-300">成交时间</span><Input className="mt-2" type="datetime-local" value={form.datetime} invalid={Boolean(errors.datetime)} onChange={(e) => update('datetime', e.target.value)} /><FieldError value={errors.datetime} /></label>
            <label><span className="text-sm font-semibold text-slate-300">标的代码</span><Input className="mt-2" value={form.symbol} invalid={Boolean(errors.symbol)} onChange={(e) => update('symbol', e.target.value)} /><FieldError value={errors.symbol} /></label>
            <label><span className="text-sm font-semibold text-slate-300">方向</span><Select className="mt-2" value={form.side} onValueChange={(value) => update('side', value)}><option value="buy">买入</option><option value="sell">卖出</option></Select></label>
            <label><span className="text-sm font-semibold text-slate-300">成交数量</span><Input className="mt-2" type="number" min="0" step="0.00000001" value={form.quantity} invalid={Boolean(errors.quantity)} onChange={(e) => update('quantity', e.target.value)} /><FieldError value={errors.quantity} /></label>
            <label><span className="text-sm font-semibold text-slate-300">成交价格</span><Input className="mt-2" type="number" min="0" step="0.00000001" value={form.price} invalid={Boolean(errors.price)} onChange={(e) => update('price', e.target.value)} /><FieldError value={errors.price} /></label>
            <label><span className="text-sm font-semibold text-slate-300">成交额</span><Input className="mt-2" value={Number.isFinite(amount) ? amount.toFixed(4) : ''} disabled /></label>
          </div>
        </div>

        <div className="border-t border-slate-800 pt-5">
          <h2 className="mb-3 font-bold text-slate-100">附加信息</h2>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <label><span className="text-sm font-semibold text-slate-300">标的名称</span><Input className="mt-2" value={form.name} onChange={(e) => update('name', e.target.value)} /></label>
            <label><span className="text-sm font-semibold text-slate-300">市场</span><Select className="mt-2" value={form.market} onValueChange={(value) => update('market', value)}>{marketOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</Select></label>
            <label><span className="text-sm font-semibold text-slate-300">币种</span><Input className="mt-2" value={form.currency} onChange={(e) => update('currency', e.target.value)} /></label>
            <label><span className="text-sm font-semibold text-slate-300">手续费</span><Input className="mt-2" type="number" min="0" step="0.00000001" value={form.fee} invalid={Boolean(errors.fee)} onChange={(e) => update('fee', e.target.value)} /><FieldError value={errors.fee} /></label>
            <label><span className="text-sm font-semibold text-slate-300">订单号</span><Input className="mt-2" value={form.order_id} onChange={(e) => update('order_id', e.target.value)} /></label>
          </div>
          <label className="mt-4 block"><span className="text-sm font-semibold text-slate-300">备注</span><Textarea className="mt-2" value={form.notes} onChange={(e) => update('notes', e.target.value)} /></label>
        </div>

        {errors.submit ? <div className="rounded-2xl border border-red-800 bg-red-950/60 px-4 py-3 text-sm text-red-100">{errors.submit}</div> : null}
      </div>
      <div className="flex border-t border-slate-800 px-5 py-4 sm:justify-end sm:px-6">
        <Button className="w-full sm:w-auto" onClick={submit} disabled={mutation.isPending}><Save className="h-4 w-4" />{mutation.isPending ? '保存中' : '保存成交'}</Button>
      </div>
    </Card>
  );
}

function DetailView({ entry }: { entry: JournalEntryType }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [reflection, setReflection] = useState(entry.reflection ?? '');
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const mutation = useMutation({
    mutationFn: (input: JournalReflectionInput) => updateJournalReflection(entry.id, input),
    onSuccess: async (saved) => {
      queryClient.setQueryData(['journal-entry', saved.id], saved);
      await queryClient.invalidateQueries({ queryKey: ['journal-entries'] });
    }
  });
  const deleteMutation = useMutation({
    mutationFn: () => deleteJournalEntry(entry.id),
    onSuccess: async () => {
      queryClient.removeQueries({ queryKey: ['journal-entry', entry.id] });
      await queryClient.invalidateQueries({ queryKey: ['journal-entries'] });
      navigate('/journal', { replace: true });
    }
  });
  useEffect(() => setReflection(entry.reflection ?? ''), [entry.id, entry.reflection]);
  const number = (value: number) => value.toLocaleString('en-US', { maximumFractionDigits: 8 });
  const deleteEntry = () => {
    deleteMutation.mutate();
  };
  return (
    <div className="space-y-5">
      <Card className="overflow-hidden p-0">
        <div className="flex flex-col gap-3 border-b border-slate-800 px-5 py-4 sm:flex-row sm:items-center sm:justify-between sm:px-6">
          <h2 className="text-xl font-bold text-white">{displaySymbol(entry.symbol)} · {entry.side === 'buy' ? '买入' : '卖出'}</h2>
          <span className="inline-flex self-start items-center gap-2 rounded-full bg-slate-800 px-3 py-1.5 text-xs font-semibold text-slate-300 sm:self-auto">
            <LockKeyhole className="h-4 w-4" />
            成交信息不可修改
          </span>
        </div>
        <div className="p-5 sm:p-6">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <ReadOnlyValue label="成交时间" value={new Date(entry.datetime).toLocaleString()} />
            <ReadOnlyValue label="标的" value={[displaySymbol(entry.symbol), entry.name].filter(Boolean).join(' · ')} />
            <ReadOnlyValue label="市场 / 币种" value={entry.market + ' / ' + (entry.currency || '-')} />
            <ReadOnlyValue label="成交数量" value={number(entry.quantity)} />
            <ReadOnlyValue label="成交价格" value={number(entry.price)} />
            <ReadOnlyValue label="成交额 / 手续费" value={number(entry.amount) + ' / ' + number(entry.fee)} />
            <ReadOnlyValue label="订单号" value={entry.order_id ?? '-'} />
            <ReadOnlyValue label="来源" value={entry.source} />
          </div>
          {entry.notes ? <div className="mt-4"><ReadOnlyValue label="备注" value={entry.notes} /></div> : null}
          {deleteMutation.isError ? <p className="mt-4 text-sm text-red-300">{deleteMutation.error instanceof Error ? deleteMutation.error.message : '删除失败'}</p> : null}
        </div>
        <div className="flex border-t border-slate-800 px-5 py-4 sm:justify-end sm:px-6">
          <Button className="w-full sm:w-auto" variant="danger" onClick={() => setDeleteConfirmOpen(true)} disabled={deleteMutation.isPending}><Trash2 className="h-4 w-4" />{deleteMutation.isPending ? '删除中' : '删除成交记录'}</Button>
        </div>
      </Card>
      <Card className="overflow-hidden p-0">
        <div className="border-b border-slate-800 px-5 py-4 sm:px-6"><h2 className="text-lg font-bold text-white">成交复盘</h2></div>
        <div className="p-5 sm:p-6">
          <Textarea value={reflection} onChange={(e) => setReflection(e.target.value)} />
          {mutation.isError ? <p className="mt-3 text-sm text-red-300">{mutation.error instanceof Error ? mutation.error.message : '保存失败'}</p> : null}
        </div>
        <div className="flex border-t border-slate-800 px-5 py-4 sm:justify-end sm:px-6">
          <Button className="w-full sm:w-auto" onClick={() => mutation.mutate({ reflection: reflection.trim() || null })} disabled={mutation.isPending}><Save className="h-4 w-4" />{mutation.isPending ? '保存中' : '保存复盘'}</Button>
        </div>
      </Card>
      <ConfirmDialog
        open={deleteConfirmOpen}
        title="删除成交记录"
        description={`删除 ${displaySymbol(entry.symbol)} 的这条成交记录后将无法恢复。`}
        confirmText="删除记录"
        onConfirm={deleteEntry}
        onOpenChange={setDeleteConfirmOpen}
      />
    </div>
  );
}

function EntryDetail({ id }: { id: string }) {
  const navigate = useNavigate();
  const query = useQuery({ queryKey: ['journal-entry', id], queryFn: () => fetchJournalEntry(id) });
  if (query.isLoading) return <Card className="h-64 animate-pulse bg-slate-900/60" />;
  if (query.isError || !query.data) return <Card><p className="text-red-200">交易记录读取失败</p><Button className="mt-4" variant="outline" onClick={() => navigate('/journal')}>返回列表</Button></Card>;
  return <DetailView entry={query.data} />;
}

export default function JournalEntry() {
  const { id } = useParams();
  const navigate = useNavigate();
  return (
    <section className="mx-auto flex w-full max-w-[1040px] flex-col gap-6 px-4 py-6 sm:px-6 sm:py-8 lg:px-8 lg:py-10">
      <PageHeader
        eyebrow="决策复盘"
        title={id ? '成交记录' : '新建成交记录'}
        description={id ? '核对成交事实，并把复盘结论留在原始记录旁。' : '记录真实成交信息，为后续表现分析保留可靠输入。'}
        meta={id ? '成交信息只读 · 复盘内容可更新' : '成交数量与价格为必填项'}
        actions={
          <Button className="w-full sm:w-auto" variant="outline" onClick={() => navigate('/journal')}>
            <ArrowLeft className="h-4 w-4" />
            返回列表
          </Button>
        }
      />
      {id ? <EntryDetail id={id} /> : <NewEntryView />}
    </section>
  );
}
