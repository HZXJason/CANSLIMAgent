// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { DataValue, LedgerSection, SourceStamp } from './ledger';

describe('ledger primitives', () => {
  it('exposes a named section without relying on decorative numbering', () => {
    render(
      <LedgerSection title="事实证据">
        <p>内容</p>
      </LedgerSection>
    );

    expect(screen.getByRole('heading', { name: '事实证据' })).toBeVisible();
    expect(screen.getByRole('region', { name: '事实证据' })).toBeVisible();
  });

  it('uses data typography only for measurements and source metadata', () => {
    render(
      <>
        <DataValue label="相对强度" value="92" />
        <SourceStamp source="SEC 10-Q" date="2026-07-10" />
      </>
    );

    expect(screen.getByText('92')).toHaveClass('font-data');
    expect(screen.getByText(/SEC 10-Q/)).toHaveClass('font-data');
  });
});
