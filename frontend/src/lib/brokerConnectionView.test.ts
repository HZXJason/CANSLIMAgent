import { describe, expect, it } from 'vitest';
import { brokerConnectionView } from './brokerConnectionView';
import type { BrokerConnection } from '../types/broker';

describe('brokerConnectionView', () => {
  it('shows account name and permits sync when bound', () => {
    expect(brokerConnectionView({
      broker: 'longport',
      connected: true,
      account_name: 'Longbridge · 12345678',
      connection_status: 'connected'
    } as BrokerConnection)).toMatchObject({
      statusLabel: '已绑定',
      actionLabel: '重新连接',
      canSync: true
    });
  });

  it('offers redetection when IBKR is unavailable', () => {
    expect(brokerConnectionView({
      broker: 'ibkr',
      connected: false,
      connection_status: 'gateway_unavailable'
    } as BrokerConnection).actionLabel).toBe('重新检测');
  });

  it('keeps settings usable when an older backend omits connection status', () => {
    expect(brokerConnectionView({
      broker: 'longport',
      connected: false
    } as BrokerConnection)).toEqual({
      statusLabel: '未绑定',
      actionLabel: '连接账户',
      canSync: false
    });
  });
});
