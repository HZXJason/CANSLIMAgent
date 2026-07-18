// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import AIActions from './AIActions';

describe('AIActions', () => {
  afterEach(() => cleanup());

  it('delegates regeneration directly to the parent confirmation flow', () => {
    const onRegenerate = vi.fn();
    const queryClient = new QueryClient();
    render(
      <QueryClientProvider client={queryClient}>
        <AIActions symbol="MU" reportContent="" nLabelWritten={false} analysisComplete={false} onRegenerate={onRegenerate} />
      </QueryClientProvider>
    );

    fireEvent.click(screen.getByRole('button', { name: '重新生成' }));

    expect(onRegenerate).toHaveBeenCalledOnce();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});
