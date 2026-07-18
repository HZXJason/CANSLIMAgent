// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import ScanProgress from './ScanProgress';

describe('Screener ScanProgress', () => {
  afterEach(() => cleanup());

  it('shows cached rows without presenting them as a completed scan', () => {
    render(
      <ScanProgress
        progress={{ analyzed: 503, total: 503 }}
        isScanning={false}
        isPaused={false}
        isComplete={false}
        showProgress={false}
      />
    );

    expect(screen.getByText('已加载 503 条缓存')).toBeInTheDocument();
    expect(screen.queryByText('扫描完成')).not.toBeInTheDocument();
    expect(screen.queryByText('503 / 503')).not.toBeInTheDocument();
  });

  it('uses pause and completion labels only for their real states', () => {
    const { rerender } = render(
      <ScanProgress
        progress={{ analyzed: 120, total: 500 }}
        isScanning={false}
        isPaused
        isComplete={false}
        showProgress
      />
    );

    expect(screen.getByText('扫描已暂停')).toBeInTheDocument();
    expect(screen.getByText('120 / 500')).toBeInTheDocument();

    rerender(
      <ScanProgress
        progress={{ analyzed: 500, total: 500 }}
        isScanning={false}
        isPaused={false}
        isComplete
        showProgress
      />
    );

    expect(screen.getByText('扫描完成')).toBeInTheDocument();
    expect(screen.getByText('500 / 500')).toBeInTheDocument();
  });
});
