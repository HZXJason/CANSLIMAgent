import type { BrokerConnection } from '../types/broker';

export type BrokerConnectionView = {
  statusLabel: string;
  actionLabel: string;
  canSync: boolean;
};

export function brokerConnectionView(connection: BrokerConnection): BrokerConnectionView {
  switch (connection.connection_status) {
    case 'unconnected':
      return { statusLabel: '未绑定', actionLabel: '连接账户', canSync: false };
    case 'pending_authorization':
      return { statusLabel: '等待授权', actionLabel: '等待授权', canSync: false };
    case 'connected':
      return { statusLabel: '已绑定', actionLabel: '重新连接', canSync: true };
    case 'reconnect_required':
      return { statusLabel: '需要重新连接', actionLabel: '重新连接', canSync: false };
    case 'gateway_unavailable':
      return { statusLabel: '未检测到本地网关', actionLabel: '重新检测', canSync: false };
    default:
      return { statusLabel: '未绑定', actionLabel: '连接账户', canSync: false };
  }
}
