// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ComponentProps } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ThresholdConfig, ThresholdPreset } from '../../types';
import ThresholdPanel from './ThresholdPanel';

const thresholds: ThresholdConfig = {
  c_min_growth: 25,
  a_min_growth: 25,
  a_min_years: 3,
  s_volume_ratio: 150,
  s_ma_period: 50,
  l_min_rs: 80,
  i_min_delta: 1
};

const customPreset: ThresholdPreset = {
  id: 2,
  name: '成长方案',
  is_default: false,
  created_at: '2026-07-17',
  ...thresholds
};

function renderPanel(overrides: Partial<ComponentProps<typeof ThresholdPanel>> = {}) {
  const props: ComponentProps<typeof ThresholdPanel> = {
    open: false,
    thresholds,
    presets: [customPreset],
    activePresetId: customPreset.id,
    onToggle: vi.fn(),
    onChange: vi.fn(),
    onApplyPreset: vi.fn(),
    onSavePreset: vi.fn().mockResolvedValue(true),
    onDeletePreset: vi.fn().mockResolvedValue(true),
    ...overrides
  };
  render(<ThresholdPanel {...props} />);
  return props;
}

describe('Screener ThresholdPanel', () => {
  afterEach(() => cleanup());

  it('saves a named preset through the inline editor', async () => {
    const props = renderPanel();

    fireEvent.click(screen.getByRole('button', { name: '保存方案' }));
    fireEvent.change(screen.getByRole('textbox', { name: '方案名称' }), { target: { value: '动量方案' } });
    fireEvent.click(screen.getByRole('button', { name: '保存' }));

    await waitFor(() => expect(props.onSavePreset).toHaveBeenCalledWith('动量方案'));
    await waitFor(() => expect(screen.queryByRole('textbox', { name: '方案名称' })).not.toBeInTheDocument());
  });

  it('requires an inline confirmation before deleting a preset', async () => {
    const props = renderPanel();

    fireEvent.click(screen.getByRole('button', { name: '删除' }));
    expect(screen.getByText('删除「成长方案」？')).toBeInTheDocument();
    expect(props.onDeletePreset).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: '确认删除' }));
    await waitFor(() => expect(props.onDeletePreset).toHaveBeenCalledWith(customPreset));
  });
});
