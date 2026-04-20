// SPDX-License-Identifier: GPL-2.0-only
import { createSignal } from 'solid-js';

interface Props {
  plaintext: string;
  onDismiss: () => void;
}

export function KeyRevealModal(props: Props) {
  const [acknowledged, setAcknowledged] = createSignal(false);

  function copyToClipboard() {
    navigator.clipboard?.writeText(props.plaintext).catch(() => {});
  }

  return (
    <div style="border: 1px solid var(--wf-color-border, #ccc); border-radius: 4px; padding: var(--wf-space-md); background: var(--wf-color-surface, #fff);">
      <p style="font-weight: bold; margin-top: 0;">Save your API key — it will not be shown again.</p>
      <code style="display: block; padding: var(--wf-space-sm); background: var(--wf-color-surface-alt, #f5f5f5); border-radius: 4px; word-break: break-all; margin-bottom: var(--wf-space-md);">
        {props.plaintext}
      </code>
      <wf-button variant="text" on:click={copyToClipboard}>
        Copy to clipboard
      </wf-button>
      <div style="margin-top: var(--wf-space-md);">
        <label style="display: flex; align-items: center; gap: var(--wf-space-sm);">
          <input
            type="checkbox"
            checked={acknowledged()}
            on:change={(e: Event) => setAcknowledged((e.target as HTMLInputElement).checked)}
          />
          I have saved this key in a secure location.
        </label>
      </div>
      <div style="margin-top: var(--wf-space-md); display: flex; justify-content: flex-end;">
        <wf-button disabled={!acknowledged()} on:click={() => { if (acknowledged()) props.onDismiss(); }}>
          I have saved it
        </wf-button>
      </div>
    </div>
  );
}
