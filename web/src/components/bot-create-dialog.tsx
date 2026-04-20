// SPDX-License-Identifier: GPL-2.0-only
import { createEffect, createSignal } from 'solid-js';
import type { CreateBotBody } from '../stores/bots';

interface Props {
  open: boolean;
  onSubmit: (body: CreateBotBody) => Promise<void>;
  onClose: () => void;
}

export function BotCreateDialog(props: Props) {
  let dialogRef!: HTMLElement & { show(): void; hide(): void };
  const [byo, setByo] = createSignal(false);
  const [apiKey, setApiKey] = createSignal('');
  const [apiKeyId, setApiKeyId] = createSignal('');
  const [roles, setRoles] = createSignal('');
  const [submitting, setSubmitting] = createSignal(false);

  createEffect(() => {
    if (props.open) dialogRef?.show?.();
    else dialogRef?.hide?.();
  });

  async function handleSubmit() {
    setSubmitting(true);
    try {
      const body: CreateBotBody = {
        hive_role_assignments: roles().split(',').map(r => r.trim()).filter(Boolean),
        bring_your_own_key: byo(),
        passport_api_key: byo() ? apiKey() : undefined,
        passport_api_key_id: byo() ? apiKeyId() : undefined,
      };
      await props.onSubmit(body);
      setRoles('');
      setApiKey('');
      setApiKeyId('');
      setByo(false);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <wf-dialog ref={dialogRef} header="Create Bot" on:wf-close={props.onClose}>
      <div style="display: flex; flex-direction: column; gap: var(--wf-space-md); padding: var(--wf-space-sm);">
        <wf-input
          placeholder="Hive roles (comma-separated)"
          value={roles()}
          on:input={(e: Event) => setRoles((e.target as HTMLInputElement).value)}
        />
        <details>
          <summary style="font-size: var(--wf-text-sm); color: var(--wf-color-text-muted); cursor: pointer;">
            Advanced — bring your own key
          </summary>
          <div style="margin-top: var(--wf-space-sm); display: flex; flex-direction: column; gap: var(--wf-space-sm);">
            <label style="display: flex; align-items: center; gap: var(--wf-space-sm); font-size: var(--wf-text-sm);">
              <input
                type="checkbox"
                checked={byo()}
                on:change={(e: Event) => setByo((e.target as HTMLInputElement).checked)}
              />
              Use existing Passport API key
            </label>
            {byo() && (
              <>
                <wf-input
                  placeholder="Passport API key"
                  value={apiKey()}
                  on:input={(e: Event) => setApiKey((e.target as HTMLInputElement).value)}
                />
                <wf-input
                  placeholder="Passport API key ID"
                  value={apiKeyId()}
                  on:input={(e: Event) => setApiKeyId((e.target as HTMLInputElement).value)}
                />
              </>
            )}
          </div>
        </details>
        <div style="display: flex; justify-content: flex-end; gap: var(--wf-space-sm);">
          <wf-button variant="text" on:click={props.onClose}>Cancel</wf-button>
          <wf-button on:click={handleSubmit} disabled={submitting()}>
            {submitting() ? 'Creating…' : 'Create Bot'}
          </wf-button>
        </div>
      </div>
    </wf-dialog>
  );
}
