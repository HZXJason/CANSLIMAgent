// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import DiscoveryTemplates from './DiscoveryTemplates';

vi.mock('../../lib/discoveryApi', () => ({
  fetchScreenDefinitions: vi.fn().mockResolvedValue([]),
  createScreenDefinition: vi.fn()
}));

const thresholds = {
  c_min_growth: 25,
  a_min_growth: 25,
  a_min_years: 3,
  s_volume_ratio: 150,
  s_ma_period: 20,
  l_min_rs: 80,
  i_min_delta: 0
};

describe('DiscoveryTemplates', () => {
  it('keeps template management collapsed until requested', () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <DiscoveryTemplates
          assetMarket="US"
          stockPool="SP500"
          thresholds={thresholds}
          onApply={vi.fn()}
          onUnknownRules={vi.fn()}
        />
      </QueryClientProvider>
    );

    expect(screen.getByRole('heading', { name: '条件模板' })).toBeInTheDocument();
    expect(screen.getByLabelText('发现模板')).toBeInTheDocument();
    expect(screen.queryByLabelText('新模板名称')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '管理模板' }));

    expect(screen.getByLabelText('新模板名称')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '保存' })).toBeInTheDocument();
  });
});
