import { request } from './api';
import type { PositionSizeInput, PositionSizeResult } from '../types/trades';

export function calculatePositionSize(input: PositionSizeInput) {
  return request<PositionSizeResult>('/position-sizer', {
    method: 'POST',
    body: JSON.stringify(input)
  });
}
