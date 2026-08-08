// @vitest-environment jsdom

import '@testing-library/jest-dom/vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { createCandidateSnapshot, createResearchCase, createUserThesis } from '../../lib/decisionSpineApi';
import DecisionThesisPanel from './DecisionThesisPanel';

vi.mock('../../lib/decisionSpineApi', () => ({
  createCandidateSnapshot: vi.fn(),
  createResearchCase: vi.fn(),
  createUserThesis: vi.fn()
}));

describe('DecisionThesisPanel', () => {
  it('persists the candidate, research case, and user thesis before planning', async () => {
    vi.mocked(createCandidateSnapshot).mockResolvedValue({ id: 'candidate-1' } as never);
    vi.mocked(createResearchCase).mockResolvedValue({ id: 'research-1' } as never);
    vi.mocked(createUserThesis).mockResolvedValue({ id: 'thesis-1', version: 1 } as never);
    const onSaved = vi.fn();
    render(<DecisionThesisPanel symbol="NVDA" market="NASDAQ100" aiSummary="基本面改善但估值偏高" onSaved={onSaved} />);
    expect(screen.queryByText(/AI 补充盲区/)).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('我的当前观点'), { target: { value: '等待突破后考虑入场' } });
    fireEvent.change(screen.getByLabelText('我不同意或暂不采纳的观点'), { target: { value: '高估值一定意味着立即下跌' } });
    fireEvent.change(screen.getByLabelText('仍待确认的问题'), { target: { value: '财报前成交量是否持续' } });
    fireEvent.change(screen.getByLabelText('什么情况证明我错了'), { target: { value: '跌破 119 且无法收回' } });
    fireEvent.click(screen.getByRole('button', { name: '保存我的论点' }));

    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(expect.objectContaining({ id: 'thesis-1', version: 1 })));
    expect(createCandidateSnapshot).toHaveBeenCalledWith(expect.objectContaining({ symbol: 'NVDA', market: 'NASDAQ100' }));
    expect(createResearchCase).toHaveBeenCalledWith(expect.objectContaining({ candidate_snapshot_id: 'candidate-1' }));
    expect(createUserThesis).toHaveBeenCalledWith('research-1', expect.objectContaining({
      summary: '等待突破后考虑入场',
      rejected_arguments: ['高估值一定意味着立即下跌'],
      unresolved_questions: ['财报前成交量是否持续'],
      falsifiers: ['跌破 119 且无法收回']
    }));
  });
});
