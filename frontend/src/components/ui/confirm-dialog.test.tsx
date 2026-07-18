// @vitest-environment jsdom
import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ConfirmDialog } from './confirm-dialog';

describe('ConfirmDialog', () => {
  afterEach(() => cleanup());

  it('labels the dialog, traps focus, and restores the trigger', () => {
    const onOpenChange = vi.fn();
    const trigger = document.createElement('button');
    document.body.appendChild(trigger);
    trigger.focus();

    const { unmount } = render(
      <ConfirmDialog
        open
        title="删除记录"
        description="此操作无法恢复。"
        confirmText="确认删除"
        onConfirm={vi.fn()}
        onOpenChange={onOpenChange}
      />
    );

    const dialog = screen.getByRole('dialog', { name: '删除记录' });
    const cancel = screen.getByRole('button', { name: '取消' });
    const confirm = screen.getByRole('button', { name: '确认删除' });
    expect(dialog).toHaveAccessibleDescription('此操作无法恢复。');
    expect(cancel).toHaveFocus();

    confirm.focus();
    fireEvent.keyDown(document, { key: 'Tab' });
    expect(cancel).toHaveFocus();

    fireEvent.keyDown(document, { key: 'Tab', shiftKey: true });
    expect(confirm).toHaveFocus();

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onOpenChange).toHaveBeenCalledWith(false);
    unmount();
    expect(trigger).toHaveFocus();
    trigger.remove();
  });
});
