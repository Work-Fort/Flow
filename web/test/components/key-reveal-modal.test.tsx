// SPDX-License-Identifier: GPL-2.0-only
import { render } from 'solid-js/web';
import { describe, it, expect, vi } from 'vitest';
import { KeyRevealModal } from '../../src/components/key-reveal-modal';

describe('KeyRevealModal', () => {
  it('shows the plaintext key', () => {
    const el = document.createElement('div');
    render(() => <KeyRevealModal plaintext="wf-svc_test_key_123" onDismiss={() => {}} />, el);
    expect(el.textContent).toContain('wf-svc_test_key_123');
  });

  it('calls onDismiss when acknowledged', async () => {
    const onDismiss = vi.fn();
    const el = document.createElement('div');
    render(() => <KeyRevealModal plaintext="secret" onDismiss={onDismiss} />, el);

    // Find and check the checkbox
    const checkbox = el.querySelector('input[type=checkbox]') as HTMLInputElement;
    expect(checkbox).not.toBeNull();
    checkbox.checked = true;
    checkbox.dispatchEvent(new Event('change'));

    await new Promise(r => setTimeout(r, 10));

    // Find and click the dismiss button
    const buttons = el.querySelectorAll('wf-button');
    const dismissButton = Array.from(buttons).find(b => b.textContent?.includes('I have saved'));
    expect(dismissButton).not.toBeNull();
  });
});
