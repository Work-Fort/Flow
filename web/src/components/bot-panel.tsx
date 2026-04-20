// SPDX-License-Identifier: GPL-2.0-only
import { createResource, createSignal, Show } from 'solid-js';
import { getBot, createBot, deleteBot, rotateBotKey, type Bot, type BotPlaintextOutput } from '../stores/bots';
import { BotCreateDialog } from './bot-create-dialog';
import { KeyRevealModal } from './key-reveal-modal';

interface Props {
  projectId: string;
}

export function BotPanel(props: Props) {
  const [bot, { refetch }] = createResource<Bot | null>(
    () => getBot(props.projectId)
  );
  const [createOpen, setCreateOpen] = createSignal(false);
  const [revealKey, setRevealKey] = createSignal<string | null>(null);
  let deleteDialogRef!: HTMLElement & { show(): void; hide(): void };

  async function handleCreateBot(body: Parameters<typeof createBot>[1]) {
    const result: BotPlaintextOutput = await createBot(props.projectId, body);
    setCreateOpen(false);
    if (result.plaintext_api_key) {
      setRevealKey(result.plaintext_api_key);
    } else {
      refetch();
    }
  }

  async function handleRotate() {
    const result: BotPlaintextOutput = await rotateBotKey(props.projectId);
    if (result.plaintext_api_key) {
      setRevealKey(result.plaintext_api_key);
    } else {
      refetch();
    }
  }

  async function handleDelete() {
    await deleteBot(props.projectId);
    deleteDialogRef?.hide?.();
    refetch();
  }

  return (
    <div style="margin-top: var(--wf-space-md);">
      <h3 style="margin: 0 0 var(--wf-space-sm);">Bot Identity</h3>

      <Show when={revealKey()}>
        <KeyRevealModal
          plaintext={revealKey()!}
          onDismiss={() => { setRevealKey(null); refetch(); }}
        />
      </Show>

      <Show when={!revealKey()}>
        <Show when={bot.loading}>
          <wf-skeleton />
        </Show>
        <Show when={bot.error}>
          <wf-banner variant="error" headline="Failed to load bot" />
        </Show>
        <Show when={!bot.loading && !bot.error}>
          <Show when={bot()} fallback={
            <div>
              <p style="color: var(--wf-color-text-muted);">No bot configured for this project.</p>
              <wf-button on:click={() => setCreateOpen(true)}>Create Bot</wf-button>
            </div>
          }>
            <div>
              <dl style="display: grid; grid-template-columns: max-content 1fr; gap: var(--wf-space-sm) var(--wf-space-md);">
                <dt style="color: var(--wf-color-text-muted);">Key ID</dt>
                <dd>{bot()!.passport_api_key_id ?? '—'}</dd>
                <dt style="color: var(--wf-color-text-muted);">Roles</dt>
                <dd>{(bot()!.hive_role_assignments ?? []).join(', ') || '—'}</dd>
              </dl>
              <div style="display: flex; gap: var(--wf-space-sm); margin-top: var(--wf-space-sm);">
                <wf-button variant="text" on:click={handleRotate}>Rotate Key</wf-button>
                <wf-button variant="text" on:click={() => deleteDialogRef?.show?.()}>Delete Bot</wf-button>
              </div>
            </div>
          </Show>
        </Show>
      </Show>

      <BotCreateDialog
        open={createOpen()}
        onSubmit={handleCreateBot}
        onClose={() => setCreateOpen(false)}
      />

      <wf-dialog ref={deleteDialogRef} header="Delete Bot">
        <div style="padding: var(--wf-space-sm);">
          <p>Remove the bot from this project? The Passport API key will not be revoked automatically.</p>
          <div style="display: flex; justify-content: flex-end; gap: var(--wf-space-sm); margin-top: var(--wf-space-md);">
            <wf-button variant="text" on:click={() => deleteDialogRef?.hide?.()}>Cancel</wf-button>
            <wf-button on:click={handleDelete}>Delete</wf-button>
          </div>
        </div>
      </wf-dialog>
    </div>
  );
}
