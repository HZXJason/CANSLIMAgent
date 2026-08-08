import { request } from './api';
import type { AssetMarket } from '../types';
import type { NewScreenDefinition, ScreenDefinition } from '../types/discovery';

export function fetchScreenDefinitions(market: AssetMarket) { return request<ScreenDefinition[]>('/screen-definitions?asset_market=' + market); }
export function createScreenDefinition(input: NewScreenDefinition) { return request<ScreenDefinition>('/screen-definitions', { method: 'POST', body: JSON.stringify(input) }); }
