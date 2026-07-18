// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { CompanyProfile } from '../../types/analysis';
import CompanyProfileCard from './CompanyProfileCard';

const shortButWrappedDescription = '美光科技开发并生产内存与存储产品，服务于数据中心、个人电脑、移动设备、汽车和工业市场。公司产品覆盖动态随机存取存储器、闪存及固态硬盘。';
const profile: CompanyProfile = {
  symbol: 'MU',
  name: 'Micron',
  description: shortButWrappedDescription,
  sector: '科技',
  industry: '半导体',
  website: 'https://www.micron.com',
  source: 'yfinance',
  updated_at: '2026-07-17'
};

describe('CompanyProfileCard', () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it('offers expansion when a short description actually exceeds five rendered lines', () => {
    vi.stubGlobal('ResizeObserver', undefined);
    render(<CompanyProfileCard data={profile} status="success" />);
    const description = screen.getByText(shortButWrappedDescription);

    Object.defineProperty(description, 'scrollHeight', { configurable: true, value: 168 });
    Object.defineProperty(description, 'clientHeight', { configurable: true, value: 140 });
    fireEvent(window, new Event('resize'));

    fireEvent.click(screen.getByRole('button', { name: '展开' }));
    expect(description).not.toHaveClass('line-clamp-5');
    expect(screen.getByRole('button', { name: '收起' })).toBeInTheDocument();
  });
});
