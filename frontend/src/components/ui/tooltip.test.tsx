// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { Tooltip } from './tooltip';

describe('Tooltip', () => {
  afterEach(() => cleanup());

  it('isolates visibility to the tooltip that is actually hovered', () => {
    render(
      <div className="group">
        <Tooltip content="C：当季 EPS 增长"><button type="button">C</button></Tooltip>
        <Tooltip content="A：年度 EPS 增长"><button type="button">A</button></Tooltip>
      </div>
    );

    const tooltips = screen.getAllByRole('tooltip');
    expect(tooltips).toHaveLength(2);
    tooltips.forEach((tooltip) => {
      expect(tooltip.parentElement).toHaveClass('group/tooltip');
      expect(tooltip.className).toContain('group-hover/tooltip:opacity-100');
      expect(tooltip.className).not.toContain('group-hover:opacity-100');
    });
  });

  it('uses a wide single-line label box', () => {
    render(<Tooltip content="N：New，新产品、新高、新闻等 AI 写回线索" singleLine><button type="button">N</button></Tooltip>);

    expect(screen.getByRole('tooltip')).toHaveClass('w-max', 'whitespace-nowrap');
  });

  it('keeps general-purpose tooltip content wrappable by default', () => {
    render(<Tooltip content="较长的通用解释内容"><button type="button">详情</button></Tooltip>);

    expect(screen.getByRole('tooltip')).toHaveClass('whitespace-normal');
    expect(screen.getByRole('tooltip')).not.toHaveClass('whitespace-nowrap');
  });
});
