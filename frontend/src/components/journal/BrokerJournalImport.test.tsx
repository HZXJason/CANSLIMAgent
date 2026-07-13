// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import BrokerJournalImport from './BrokerJournalImport';
import type { BrokerConnection, BrokerSyncResult } from '../../types/broker';

vi.mock('../../lib/api', () => ({ importBrokerExecutions: vi.fn(), syncBrokerConnection: vi.fn() }));
import { importBrokerExecutions, syncBrokerConnection } from '../../lib/api';

const connection: BrokerConnection = { id: 'longport-1', broker: 'longport', label: '长桥 LongPort', account_name: '长桥账户（已授权）', connection_method: 'oauth', connection_status: 'connected', message: '', connected: true, secure_storage_ready: true, last_synced_at: null };
const result: BrokerSyncResult = { positions: [], executions: [{ external_id: 'fill-1', symbol: 'AAPL', name: 'Apple', side: 'buy', quantity: 1, price: 200, fee: 1, market: 'US', executed_at: '2026-07-12T10:00:00Z', currency: 'USD' }], warnings: [] };

describe('BrokerJournalImport', () => {
  afterEach(() => { cleanup(); vi.resetAllMocks(); });

  it('reads and imports a broker preview from the journal page only after confirmation', async () => {
    vi.mocked(syncBrokerConnection).mockResolvedValue(result);
    vi.mocked(importBrokerExecutions).mockResolvedValue({ created: 1, skipped: 0, warnings: [] });
    const onImported = vi.fn();
    render(<BrokerJournalImport connections={[connection]} onImported={onImported} />);

    fireEvent.change(screen.getByLabelText('开始日期'), { target: { value: '2026-01-01' } });
    fireEvent.change(screen.getByLabelText('结束日期'), { target: { value: '2026-01-31' } });
    fireEvent.click(screen.getByRole('button', { name: '获取长桥 LongPort交易日志' }));
    expect(syncBrokerConnection).toHaveBeenCalledWith('longport-1', { start_at: '2026-01-01T00:00:00.000Z', end_at: '2026-01-31T23:59:59.999Z' });
    expect(await screen.findByText('1 笔可导入成交')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '确认导入成交' }));
    await waitFor(() => expect(importBrokerExecutions).toHaveBeenCalledWith('longport-1', result.executions));
    expect(onImported).toHaveBeenCalledOnce();
  });
});
