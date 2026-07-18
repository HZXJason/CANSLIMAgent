// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import TabBar from './TabBar';

const tabs = [
  { symbol: 'AAPL', name: 'Apple', openedAt: 1 },
  { symbol: 'MSFT', name: 'Microsoft', openedAt: 2 },
  { symbol: 'PLTR', name: 'Palantir', openedAt: 3 },
  { symbol: 'MU', name: 'Micron', openedAt: 4 }
];

function rect(left: number, right: number) {
  return {
    left,
    right,
    top: 0,
    bottom: 40,
    width: right - left,
    height: 40,
    x: left,
    y: 0,
    toJSON: () => ({})
  } as DOMRect;
}

describe('Analysis TabBar', () => {
  afterEach(() => cleanup());

  it('adds a stock through the inline input', () => {
    const onAdd = vi.fn();
    render(<TabBar tabs={tabs} activeTabId="AAPL" onActive={vi.fn()} onClose={vi.fn()} onAdd={onAdd} />);

    fireEvent.click(screen.getByRole('button', { name: '添加股票' }));
    fireEvent.change(screen.getByRole('textbox', { name: '股票代码' }), { target: { value: 'nvda' } });
    fireEvent.click(screen.getByRole('button', { name: '打开股票' }));

    expect(onAdd).toHaveBeenCalledWith('nvda');
    expect(screen.queryByRole('textbox', { name: '股票代码' })).not.toBeInTheDocument();
  });

  it('keeps tab selection and close actions separate', () => {
    const onActive = vi.fn();
    const onClose = vi.fn();
    render(<TabBar tabs={tabs} activeTabId="AAPL" onActive={onActive} onClose={onClose} onAdd={vi.fn()} />);

    fireEvent.click(screen.getByRole('tab', { name: /MSFT/ }));
    fireEvent.click(screen.getByRole('button', { name: '关闭 AAPL' }));

    expect(onActive).toHaveBeenCalledWith('MSFT');
    expect(onClose).toHaveBeenCalledWith('AAPL');
  });

  it('scrolls a clipped active tab into view without moving visible tabs', () => {
    const props = { tabs, onActive: vi.fn(), onClose: vi.fn(), onAdd: vi.fn() };
    const { rerender } = render(<TabBar {...props} activeTabId="AAPL" />);
    const tabList = screen.getByRole('tablist');
    const leftClippedTab = screen.getByRole('tab', { name: /AAPL/ }).parentElement as HTMLDivElement;
    const visibleTab = screen.getByRole('tab', { name: /MSFT/ }).parentElement as HTMLDivElement;
    const clippedTab = screen.getByRole('tab', { name: /MU/ }).parentElement as HTMLDivElement;
    const scrollTo = vi.fn();

    vi.spyOn(tabList, 'getBoundingClientRect').mockReturnValue(rect(0, 390));
    vi.spyOn(leftClippedTab, 'getBoundingClientRect').mockReturnValue(rect(-60, 40));
    vi.spyOn(visibleTab, 'getBoundingClientRect').mockReturnValue(rect(120, 240));
    vi.spyOn(clippedTab, 'getBoundingClientRect').mockReturnValue(rect(470, 570));
    tabList.scrollTo = scrollTo;

    rerender(<TabBar {...props} activeTabId="MSFT" />);
    expect(scrollTo).not.toHaveBeenCalled();

    rerender(<TabBar {...props} activeTabId="MU" />);
    expect(scrollTo).toHaveBeenCalledWith({ left: 180, behavior: 'smooth' });

    tabList.scrollLeft = 180;
    rerender(<TabBar {...props} activeTabId="AAPL" />);
    expect(scrollTo).toHaveBeenLastCalledWith({ left: 120, behavior: 'smooth' });
  });
});
