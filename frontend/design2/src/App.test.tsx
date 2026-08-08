// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import App from './App';

describe('AlphaLens design2 workspace', () => {
  afterEach(() => cleanup());

  it('filters the evidence table and keeps the scan summary visible', () => {
    render(<App />);

    expect(screen.getByRole('heading', { name: '证据扫描' })).toBeInTheDocument();
    expect(screen.getByText('503')).toBeInTheDocument();
    expect(screen.getByText('DELL')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '只看需要核查' }));

    expect(screen.getByText('EME')).toBeInTheDocument();
    expect(screen.queryByText('DELL')).not.toBeInTheDocument();
  });

  it('exposes the research-terminal identity in the first viewport', () => {
    render(<App />);

    expect(screen.getAllByText('证券研究终端').length).toBeGreaterThan(0);
    expect(screen.getByText(/来源 \/ 期间 \/ 状态/)).toBeInTheDocument();
  });
});
