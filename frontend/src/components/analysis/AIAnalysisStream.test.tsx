// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import AIAnalysisStream from './AIAnalysisStream';

const baseProps = {
  progress: [],
  results: [],
  error: null,
  liveNarration: '',
  liveNarrationByAgent: {},
  fallbackReport: ''
};

describe('AIAnalysisStream', () => {
  afterEach(() => cleanup());

  it('puts the localized decision ahead of report details', () => {
    render(
      <AIAnalysisStream
        {...baseProps}
        status="done"
        decision={{ action: 'BUY', confidence: 'LOW', summary: '等待价格确认' }}
        fallbackReport={'## FundamentalsAnalyst\n基本面报告'}
      />
    );

    expect(screen.getByText('买入')).toBeInTheDocument();
    expect(screen.getByText('置信度 低')).toBeInTheDocument();
    expect(screen.getByText('等待价格确认')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /基本面分析师/ })).toBeInTheDocument();
  });

  it('does not present an empty completed run as a valid result', () => {
    render(<AIAnalysisStream {...baseProps} status="done" decision={null} />);

    expect(screen.getByText('未收到分析结果')).toBeInTheDocument();
  });
});
