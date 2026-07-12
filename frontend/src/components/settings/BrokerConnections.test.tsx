// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import BrokerConnections from './BrokerConnections';
import type { BrokerConnection, BrokerSyncResult } from '../../types/broker';

vi.mock('../../lib/api', () => ({
  deleteBrokerConnection: vi.fn(),
  fetchBrokerConnections: vi.fn(),
  importBrokerExecutions: vi.fn(),
  startBrokerConnection: vi.fn(),
  syncBrokerConnection: vi.fn()
}));

import {
  deleteBrokerConnection,
  fetchBrokerConnections,
  importBrokerExecutions,
  startBrokerConnection,
  syncBrokerConnection
} from '../../lib/api';

const mockDeleteBrokerConnection = vi.mocked(deleteBrokerConnection);
const mockFetchConnections = vi.mocked(fetchBrokerConnections);
const mockImportBrokerExecutions = vi.mocked(importBrokerExecutions);
const mockStartBrokerConnection = vi.mocked(startBrokerConnection);
const mockSyncBrokerConnection = vi.mocked(syncBrokerConnection);

const connection = (overrides: Partial<BrokerConnection>): BrokerConnection => ({
  id: 'connection-longport',
  broker: 'longport',
  label: '',
  account_name: '',
  connection_method: '',
  connection_status: 'unconnected',
  message: '',
  connected: false,
  secure_storage_ready: true,
  last_synced_at: null,
  ...overrides
});

const connectedLongport = connection({
  id: 'longport-1',
  broker: 'longport',
  account_name: 'Longbridge · 12345678',
  connection_method: 'oauth',
  connection_status: 'connected',
  connected: true,
  last_synced_at: '2026-07-12T02:30:00Z'
});
const unconnectedTiger = connection({ id: 'tiger-1', broker: 'tiger' });
const unavailableFutu = connection({
  id: 'futu-1',
  broker: 'futu',
  connection_method: 'gateway',
  connection_status: 'gateway_unavailable',
  message: '请先启动 OpenD'
});
const unconnectedIBKR = connection({ id: 'ibkr-1', broker: 'ibkr', connection_method: 'gateway' });

const syncResult: BrokerSyncResult = {
  positions: [],
  executions: [{
    external_id: 'execution-1',
    symbol: 'AAPL',
    name: 'Apple Inc.',
    market: 'US',
    side: 'buy',
    quantity: 2,
    price: 200,
    fee: 1,
    executed_at: '2026-07-12T02:30:00Z',
    currency: 'USD'
  }],
  warnings: []
};

describe('BrokerConnections', () => {
  let locationAssign: ReturnType<typeof vi.fn>;
  let locationSearch = '';

  beforeEach(() => {
    locationAssign = vi.fn();
    const appWindow = Object.create(window) as Window;
    Object.defineProperty(appWindow, 'location', { value: { assign: locationAssign, get search() { return locationSearch; } } });
    Object.defineProperty(appWindow, 'confirm', { value: vi.fn(() => true) });
    vi.stubGlobal('window', appWindow);
    mockFetchConnections.mockResolvedValue([connectedLongport, unconnectedTiger, unavailableFutu, unconnectedIBKR]);
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.resetAllMocks();
    locationSearch = '';
  });

  it('renders four broker rows and a bound account', async () => {
    render(<BrokerConnections />);

    expect(await screen.findByText('长桥 LongPort')).toBeInTheDocument();
    expect(screen.getByText('Longbridge · 12345678')).toBeInTheDocument();
    expect(screen.getByText('老虎证券 Tiger')).toBeInTheDocument();
    expect(screen.getByText('富途证券 Futu')).toBeInTheDocument();
    expect(screen.getByText('盈透证券 IBKR')).toBeInTheDocument();
    expect(screen.queryByLabelText('OAuth Access Token')).not.toBeInTheDocument();
  });

  it('navigates only to the API OAuth URL', async () => {
    mockStartBrokerConnection.mockResolvedValue({ action: 'redirect', authorization_url: 'https://oauth.example/authorize' });
    render(<BrokerConnections />);

    fireEvent.click((await screen.findAllByRole('button', { name: '连接账户' }))[0]);

    await waitFor(() => expect(locationAssign).toHaveBeenCalledWith('https://oauth.example/authorize'));
    expect(mockStartBrokerConnection).toHaveBeenCalledWith('longport');
  });

  it('refreshes card state after a direct local gateway connection', async () => {
    mockStartBrokerConnection.mockResolvedValue(connection({
      id: 'futu-1',
      broker: 'futu',
      account_name: 'Futu · 9988',
      connection_method: 'gateway',
      connection_status: 'connected',
      connected: true
    }));
    mockFetchConnections
      .mockResolvedValueOnce([connectedLongport, unconnectedTiger, unavailableFutu, unconnectedIBKR])
      .mockResolvedValueOnce([connectedLongport, unconnectedTiger, connection({
        id: 'futu-1',
        broker: 'futu',
        account_name: 'Futu · 9988',
        connection_method: 'gateway',
        connection_status: 'connected',
        connected: true
      }), unconnectedIBKR]);
    render(<BrokerConnections />);

    fireEvent.click(await screen.findByRole('button', { name: '重新检测' }));

    await waitFor(() => expect(mockFetchConnections).toHaveBeenCalledTimes(2));
    expect(mockStartBrokerConnection).toHaveBeenCalledWith('futu');
    expect(await screen.findByText('Futu · 9988')).toBeInTheDocument();
  });

  it('shows callback error only for the broker callback parameters', async () => {
    locationSearch = '?broker=longport&connection_error=access_denied&ignored=not-shown';
    render(<BrokerConnections />);

    expect(await screen.findByText('LongPort 连接失败：access_denied')).toBeInTheDocument();
    expect(screen.queryByText(/not-shown/)).not.toBeInTheDocument();
  });

  it('keeps sync preview, import, and disconnect available for a connected account', async () => {
    mockSyncBrokerConnection.mockResolvedValue(syncResult);
    mockImportBrokerExecutions.mockResolvedValue({ created: 1, skipped: 0, warnings: [] });
    render(<BrokerConnections />);

    fireEvent.click(await screen.findByRole('button', { name: '同步预览' }));
    expect(await screen.findByText('0 项持仓，1 笔可导入成交')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '确认导入成交' }));
    await waitFor(() => expect(mockImportBrokerExecutions).toHaveBeenCalledWith('longport-1', syncResult.executions));
    fireEvent.click(screen.getByRole('button', { name: '断开' }));
    await waitFor(() => expect(mockDeleteBrokerConnection).toHaveBeenCalledWith('longport-1'));
  });
});
