// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';

vi.mock('../../components/settings/BrokerConnections', () => ({ default: () => null }));
vi.mock('../../lib/api', () => ({
  API_BASE_URL: '/api/v1',
  deleteLLMCredential: vi.fn(),
  discoverAIModels: vi.fn(),
  fetchAIAnalysisConfig: vi.fn(),
  fetchLLMCredentialStatus: vi.fn(),
  saveLLMCredential: vi.fn()
}));

import SettingsPage from './index';
import { useMarketStore } from '../../stores/marketStore';
import {
  discoverAIModels,
  fetchAIAnalysisConfig,
  fetchLLMCredentialStatus,
  saveLLMCredential
} from '../../lib/api';

const mockFetchConfig = vi.mocked(fetchAIAnalysisConfig);
const mockDiscoverModels = vi.mocked(discoverAIModels);
const mockFetchCredential = vi.mocked(fetchLLMCredentialStatus);
const mockSaveCredential = vi.mocked(saveLLMCredential);

const options = {
  providers: [{
    id: 'anthropic',
    name: 'Anthropic',
    models: { deep: ['claude-opus-4-6'], quick: ['claude-haiku-4-5-20251001'] }
  }],
  analysts: [{ id: 'fundamental', name: '基本面分析师', description: '基本面' }],
  debate_rounds: [1],
  risk_rounds: [1]
};

describe('SettingsPage LLM credentials', () => {
  beforeEach(() => {
    window.localStorage.clear();
    useMarketStore.setState({ assetMarket: 'US', hasSelectedMarket: true });
    mockFetchConfig.mockResolvedValue(options);
    mockDiscoverModels.mockResolvedValue({ models: ['claude-opus-4-6'], source: 'api.anthropic.com' });
    mockFetchCredential.mockResolvedValue({
      provider: 'anthropic',
      backend_host: '',
      configured: false,
      storage_ready: true
    });
    mockSaveCredential.mockResolvedValue({
      provider: 'anthropic',
      backend_host: '',
      configured: true,
      storage_ready: true,
      updated_at: '2026-07-13T15:00:00Z'
    });
  });

  afterEach(() => {
    cleanup();
    vi.resetAllMocks();
  });

  it('accepts a key, stores it in the backend, and clears the browser input', async () => {
    render(<SettingsPage />);

    const input = await screen.findByLabelText('Anthropic API Key');
    fireEvent.change(input, { target: { value: 'sk-user-entered-key' } });
    const save = screen.getByRole('button', { name: '保存 Key' });
    await waitFor(() => expect(save).toBeEnabled());
    fireEvent.click(save);

    await waitFor(() => expect(mockSaveCredential).toHaveBeenCalledWith('anthropic', 'sk-user-entered-key', ''));
    await waitFor(() => expect(input).toHaveValue(''));
    expect(await screen.findByText('已加密保存在后端')).toBeInTheDocument();
  });

  it('changes the shared investor market preference', () => {
    render(<SettingsPage />);

    fireEvent.change(screen.getByLabelText('主要投资市场'), { target: { value: 'CN' } });

    expect(useMarketStore.getState()).toMatchObject({ assetMarket: 'CN', hasSelectedMarket: true });
  });
});
